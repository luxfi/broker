package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/provider"
)

const testUserID = "test-user-001"

// testJWKS holds a test RSA key pair for mocking IAM JWKS in tests.
var testJWKS struct {
	key    *rsa.PrivateKey
	server *httptest.Server
}

func init() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("failed to generate test RSA key: " + err.Error())
	}
	testJWKS.key = key

	// Serve JWKS endpoint.
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]interface{}{
				{
					"kty": "RSA",
					"kid": "test-kid",
					"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
				},
			},
		})
	})
	testJWKS.server = httptest.NewServer(mux)
}

// signTestJWT creates a test RS256 JWT signed with the test key.
func signTestJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT","kid":"test-kid"}`))
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sigInput := header + "." + payload
	digest := sha256.Sum256([]byte(sigInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, testJWKS.key, 0x05, digest[:]) // crypto.SHA256 = 0x05
	if err != nil {
		panic("sign failed: " + err.Error())
	}
	return sigInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// testToken returns a valid test JWT with admin claims.
func testToken() string {
	return signTestJWT(map[string]interface{}{
		"sub":     testUserID,
		"owner":   "test-org",
		"email":   "test@example.com",
		"roles":   "admin",
		"isAdmin": true,
		"exp":     float64(time.Now().Add(time.Hour).Unix()),
	})
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("IAM_ENDPOINT", testJWKS.server.URL)
	registry := provider.NewRegistry()
	srv := NewServer(registry, ":0")
	return httptest.NewServer(srv.Handler())
}

// setupTestServerWithRef returns both the httptest.Server and the broker Server
// so tests can register account mappings for ownership verification.
func setupTestServerWithRef(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	t.Setenv("IAM_ENDPOINT", testJWKS.server.URL)
	registry := provider.NewRegistry()
	srv := NewServer(registry, ":0")
	return httptest.NewServer(srv.Handler()), srv
}

// authedGet makes a GET request with a valid test JWT.
func authedGet(url string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+testToken())
	return http.DefaultClient.Do(req)
}

// authedPost makes a POST request with a valid test JWT.
func authedPost(url, contentType string, body *strings.Reader) (*http.Response, error) {
	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("Authorization", "Bearer "+testToken())
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

// authedRequest makes a request with a valid test JWT and custom user ID.
func authedRequest(method, url string, body *strings.Reader, userID string) (*http.Response, error) {
	var req *http.Request
	if body != nil {
		req, _ = http.NewRequest(method, url, body)
	} else {
		req, _ = http.NewRequest(method, url, nil)
	}
	token := signTestJWT(map[string]interface{}{
		"sub":     userID,
		"owner":   "test-org",
		"email":   fmt.Sprintf("%s@test.io", userID),
		"roles":   "admin",
		"isAdmin": true,
		"exp":     float64(time.Now().Add(time.Hour).Unix()),
	})
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func TestHealthz(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}
}

func TestListProviders(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/providers")
	if err != nil {
		t.Fatalf("GET /v1/providers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	providers, ok := body["providers"].([]interface{})
	if !ok {
		t.Fatal("expected providers array")
	}
	// Empty registry = empty list
	if len(providers) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(providers))
	}
}
