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
	"os"
	"strings"
	"sync"
	"time"
)

// jwksCache caches RSA public keys from IAM JWKS, keyed by kid.
var (
	jwksMu      sync.RWMutex
	jwksKeys    map[string]*rsa.PublicKey
	jwksExpiry  time.Time
	jwksRefresh sync.Mutex // serializes refresh calls to prevent stampede
)

// untrustedHeaders are identity headers that must be stripped at the
// start of every request to prevent injection via direct pod access.
// The auth middleware re-sets them from validated JWT claims.
var untrustedHeaders = []string{
	"X-User-Id",
	"X-Org-Id",
	"X-User-Email",
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

	// Expected audience for the broker service. Defaults to the IAM endpoint
	// (matching Casdoor's default aud behavior) unless overridden by env.
	expectedAud := os.Getenv("BROKER_JWT_AUDIENCE")
	if expectedAud == "" {
		expectedAud = iamEndpoint
	}

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

			// Validate issuer matches the IAM endpoint.
			if iss := ClaimStr(claims, "iss"); iss != "" && iss != iamEndpoint {
				writeErr(w, http.StatusUnauthorized, "invalid token issuer")
				return
			}

			// Validate audience contains the expected service identifier.
			if !checkAudience(claims, expectedAud) {
				writeErr(w, http.StatusUnauthorized, "invalid token audience")
				return
			}

			r.Header.Set("X-User-Id", ClaimStr(claims, "sub"))
			r.Header.Set("X-Org-Id", ClaimStr(claims, "owner"))
			r.Header.Set("X-User-Email", ClaimStr(claims, "email"))
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOrg returns middleware that rejects requests from users outside
// the specified org. Used to restrict compliance endpoints to built-in
// org (superadmin operators) and keep customer orgs separate.
func RequireOrg(allowedOrg string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			org := r.Header.Get("X-Org-Id")
			if org != allowedOrg {
				writeErr(w, http.StatusForbidden, "access restricted to "+allowedOrg+" org")
				return
			}
			next.ServeHTTP(w, r)
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
	// Fast path: cache hit for this kid.
	jwksMu.RLock()
	if jwksKeys != nil && time.Now().Before(jwksExpiry) {
		if key, ok := jwksKeys[kid]; ok {
			jwksMu.RUnlock()
			return key, nil
		}
	}
	jwksMu.RUnlock()

	// Serialize refresh calls to prevent stampede.
	jwksRefresh.Lock()
	// Double-check after acquiring lock.
	jwksMu.RLock()
	if jwksKeys != nil && time.Now().Before(jwksExpiry) {
		if key, ok := jwksKeys[kid]; ok {
			jwksMu.RUnlock()
			jwksRefresh.Unlock()
			return key, nil
		}
	}
	jwksMu.RUnlock()

	resp, err := http.Get(jwksURL)
	if err != nil {
		jwksRefresh.Unlock()
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
		jwksRefresh.Unlock()
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
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
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(new(big.Int).SetBytes(eBytes).Int64())}
	}

	jwksMu.Lock()
	jwksKeys = keys
	jwksExpiry = time.Now().Add(time.Hour)
	jwksMu.Unlock()
	jwksRefresh.Unlock()

	if key, ok := keys[kid]; ok {
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

// checkAudience verifies the JWT aud claim contains the expected audience.
// Handles both string and []string aud formats per RFC 7519.
func checkAudience(claims map[string]interface{}, expected string) bool {
	aud, ok := claims["aud"]
	if !ok {
		// No aud claim — accept for backward compatibility with tokens
		// issued before aud enforcement was added.
		return true
	}
	switch v := aud.(type) {
	case string:
		return v == expected
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
