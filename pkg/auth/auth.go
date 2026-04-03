package auth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// jwksCache caches the RSA public key from IAM JWKS.
var (
	jwksMu     sync.RWMutex
	jwksKey    *rsa.PublicKey
	jwksExpiry time.Time
)

// untrustedHeaders are identity headers that must be stripped at the
// start of every request to prevent injection via direct pod access.
// The auth middleware re-sets them from validated JWT claims.
var untrustedHeaders = []string{
	"X-User-Id",
	"X-Org-Id",
	"X-User-Email",
	"X-User-Roles",
	"X-User-IsAdmin",
	"X-Account-Id",
	"X-Account-Provider",
	"X-Gateway-User-Id",
	"X-Hanzo-User-Id",
}

// Middleware validates IAM JWTs via JWKS.
// All identity headers are stripped first, then re-set from validated claims.
// /healthz is always public.
func Middleware(iamEndpoint string) func(http.Handler) http.Handler {
	jwksURL := iamEndpoint + "/.well-known/jwks"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Strip all identity headers unconditionally to prevent injection.
			for _, h := range untrustedHeaders {
				r.Header.Del(h)
			}

			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			// Validate Bearer token via IAM JWKS
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeErr(w, http.StatusUnauthorized, "authentication required")
				return
			}

			claims, err := ValidateJWT(strings.TrimPrefix(auth, "Bearer "), jwksURL)
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "invalid token")
				return
			}

			r.Header.Set("X-User-Id", ClaimStr(claims, "sub"))
			r.Header.Set("X-Org-Id", ClaimStr(claims, "owner"))
			r.Header.Set("X-User-Email", ClaimStr(claims, "email"))
			if roles := ClaimStr(claims, "roles"); roles != "" {
				r.Header.Set("X-User-Roles", roles)
			}
			if ClaimBool(claims, "isAdmin") {
				r.Header.Set("X-User-IsAdmin", "true")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission checks X-User-Roles or X-User-IsAdmin.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roles := r.Header.Get("X-User-Roles")
			isAdmin := strings.EqualFold(r.Header.Get("X-User-IsAdmin"), "true")
			if isAdmin || HasRole(roles, perm) || HasRole(roles, "admin") {
				next.ServeHTTP(w, r)
				return
			}
			if perm == "read" {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusForbidden, "insufficient permissions")
		})
	}
}

// ValidateJWT validates RS256 JWT signature against JWKS and returns claims.
func ValidateJWT(tokenStr, jwksURL string) (map[string]interface{}, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("bad header")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("bad header json")
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported alg: %s", header.Alg)
	}

	key, err := getJWKSKey(jwksURL, header.Kid)
	if err != nil {
		return nil, err
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("bad signature")
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return nil, fmt.Errorf("signature invalid")
	}

	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("bad claims")
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return nil, fmt.Errorf("bad claims json")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("expired")
		}
	}

	return claims, nil
}

func getJWKSKey(jwksURL, kid string) (*rsa.PublicKey, error) {
	jwksMu.RLock()
	if jwksKey != nil && time.Now().Before(jwksExpiry) {
		jwksMu.RUnlock()
		return jwksKey, nil
	}
	jwksMu.RUnlock()

	jwksMu.Lock()
	defer jwksMu.Unlock()
	if jwksKey != nil && time.Now().Before(jwksExpiry) {
		return jwksKey, nil
	}

	resp, err := http.Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || (kid != "" && k.Kid != kid) {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(new(big.Int).SetBytes(eBytes).Int64())}
		jwksKey = key
		jwksExpiry = time.Now().Add(time.Hour)
		return key, nil
	}
	return nil, fmt.Errorf("no key for kid=%s", kid)
}

// HasRole checks whether a comma-separated role list contains the given role.
func HasRole(roles, role string) bool {
	for _, r := range strings.Split(roles, ",") {
		if strings.TrimSpace(r) == role {
			return true
		}
	}
	return false
}

// ClaimStr extracts a string claim from JWT claims.
func ClaimStr(claims map[string]interface{}, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}

// ClaimBool extracts a boolean claim from JWT claims.
func ClaimBool(claims map[string]interface{}, key string) bool {
	if v, ok := claims[key].(bool); ok {
		return v
	}
	return false
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
