package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/token"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func TestMiddlewareHealthzSkipsAuth(t *testing.T) {
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", rr.Code)
	}
}

func TestMiddlewareStripsInjectedHeaders(t *testing.T) {
	// Verify that externally-set identity headers are stripped and the
	// request is rejected (no valid Bearer token).
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-User-Id", "injected-user")
	req.Header.Set("X-Org-Id", "injected-org")
	req.Header.Set("X-User-Email", "injected@example.com")
	req.Header.Set("X-Account-Id", "injected-acct")
	req.Header.Set("X-Gateway-User-Id", "injected-gw")
	req.Header.Set("X-Hanzo-User-Id", "injected-hanzo")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Should be rejected because the injected headers are stripped and
	// no Bearer token is present.
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when identity headers are injected without Bearer token, got %d", rr.Code)
	}
}

func TestMiddlewareHealthzStripsHeaders(t *testing.T) {
	// Even /healthz should strip identity headers to prevent leaking
	// injected state into downstream handlers.
	var gotUserID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-Id")
		w.WriteHeader(http.StatusOK)
	})
	handler := Middleware("http://localhost:9999")(inner)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-User-Id", "injected-user")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUserID != "" {
		t.Fatalf("expected X-User-Id to be stripped on /healthz, got %q", gotUserID)
	}
}

func TestMiddlewareRejectsNoHeaders(t *testing.T) {
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareRejectsInvalidBearerToken(t *testing.T) {
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer some-jwt")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with invalid Bearer token, got %d", rr.Code)
	}
}

func TestRequireOrgAllowed(t *testing.T) {
	handler := RequireOrg("built-in")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-Org-Id", "built-in")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireOrgDenied(t *testing.T) {
	handler := RequireOrg("built-in")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-Org-Id", "liquidity")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestHasRoleCommaSeparated(t *testing.T) {
	tests := []struct {
		roles  string
		role   string
		expect bool
	}{
		{"admin,trade,read", "trade", true},
		{"admin, trade, read", "trade", true},
		{"read", "trade", false},
		{"", "trade", false},
		{"superadmin", "superadmin", true},
		{"viewer,editor", "admin", false},
	}
	for _, tt := range tests {
		got := HasRole(tt.roles, tt.role)
		if got != tt.expect {
			t.Errorf("HasRole(%q, %q) = %v, want %v", tt.roles, tt.role, got, tt.expect)
		}
	}
}

func TestWriteErrJSONSafe(t *testing.T) {
	rr := httptest.NewRecorder()
	writeErr(rr, http.StatusBadRequest, `injection"attempt`)
	body := rr.Body.String()
	// json.Marshal should properly escape the quote
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	// Verify it's valid JSON by checking it doesn't contain unescaped injection
	if !contains(body, `injection\"attempt`) && !contains(body, `injection\u0022attempt`) {
		// Either escaped form is acceptable from json.Marshal
		// Just verify the raw injection doesn't appear unescaped
		if contains(body, `injection"attempt"`) {
			t.Fatalf("writeErr did not escape JSON: %s", body)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- IAM JWT canonicality (the mounted path: pkg/api and pkg/compliance) ---

// newJWKS starts a JWKS endpoint for a fresh RSA key and returns its URL
// alongside a signer that mints canonical RS256 tokens against it.
func newJWKS(t *testing.T) (jwksURL string, mint func(claims string) string, key *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// The JWKS cache is package-level and keyed on kid alone, so tests that
	// reuse a kid would serve each other's keys. (Operationally this also
	// means a key rotated in place under an unchanged kid is not picked up
	// until the one-hour cache expiry.)
	kid := t.Name()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": kid,
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)

	mint = func(claims string) string {
		hdr := token.Encode([]byte(`{"alg":"RS256","typ":"JWT","kid":"` + kid + `"}`))
		body := hdr + "." + token.Encode([]byte(claims))
		digest := sha256.Sum256([]byte(body))
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return body + "." + token.Encode(sig)
	}
	return srv.URL, mint, key
}

// An RSA-2048 signature is 256 bytes carried in 342 base64url characters:
// 2052 bits of room for 2048 bits of signature. The 4 spare bits in the final
// character mean sixteen distinct strings decode to the identical signature.
func TestValidateJWT_AcceptsExactlyOneSpelling(t *testing.T) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

	jwksURL, mint, _ := newJWKS(t)
	tok := mint(`{"sub":"u1","iss":"i","exp":` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `}`)

	dot := strings.LastIndex(tok, ".")
	sig := tok[dot+1:]
	if len(sig) != 342 {
		t.Fatalf("signature segment is %d chars, want 342 for a 256-byte signature", len(sig))
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		t.Fatal(err)
	}
	last := strings.IndexByte(alphabet, sig[len(sig)-1])

	accepted, ids := 0, map[string]bool{}
	for b := 0; b < 16; b++ {
		v := tok[:dot+1] + sig[:len(sig)-1] + string(alphabet[(last&^15)|b])
		got, err := base64.RawURLEncoding.DecodeString(v[dot+1:])
		if err != nil || string(got) != string(want) {
			t.Fatalf("test premise: respelling %d does not decode to the same signature", b)
		}
		ids[v] = true
		if _, err := ValidateJWT(v, jwksURL); err == nil {
			accepted++
			if v != tok {
				t.Errorf("accepted non-canonical spelling ...%q", v[len(v)-4:])
			}
		}
	}
	if len(ids) != 16 {
		t.Fatalf("built %d distinct strings, want 16", len(ids))
	}
	if accepted != 1 {
		t.Fatalf("%d of 16 spellings authenticate as the same credential, want exactly 1", accepted)
	}
}

func TestValidateJWT_RejectsWhitespaceInSegments(t *testing.T) {
	jwksURL, mint, _ := newJWKS(t)
	tok := mint(`{"sub":"u1","exp":` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `}`)
	dot := strings.LastIndex(tok, ".")

	for _, v := range []string{tok + "\n", tok[:dot+1] + "\n" + tok[dot+1:], "\n" + tok} {
		if _, err := ValidateJWT(v, jwksURL); err == nil {
			t.Errorf("accepted a token segment containing CR/LF")
		}
	}
}

func TestValidateJWT_AcceptsCanonicalToken(t *testing.T) {
	jwksURL, mint, _ := newJWKS(t)
	tok := mint(`{"sub":"u1","exp":` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `}`)

	claims, err := ValidateJWT(tok, jwksURL)
	if err != nil {
		t.Fatalf("canonical token rejected: %v", err)
	}
	if ClaimStr(claims, "sub") != "u1" {
		t.Fatalf("sub = %q, want u1", ClaimStr(claims, "sub"))
	}
}
