package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-jwt-secret-32bytes-minimum!"

func TestNewStore(t *testing.T) {
	s := NewStore(testSecret)
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
	if s.admins == nil {
		t.Fatal("admins map not initialized")
	}
	if string(s.secret) != testSecret {
		t.Fatalf("secret mismatch: got %q, want %q", string(s.secret), testSecret)
	}
}

func TestAddAdmin_PasswordHashedNotPlaintext(t *testing.T) {
	s := NewStore(testSecret)
	password := "supersecret123"

	if err := s.AddAdmin("alice", password, "admin"); err != nil {
		t.Fatalf("AddAdmin: %v", err)
	}

	admin := s.admins["alice"]
	if admin == nil {
		t.Fatal("admin not found in store after AddAdmin")
	}

	// Password must NEVER be stored as plaintext
	if admin.PasswordHash == password {
		t.Fatal("CRITICAL: password stored as plaintext")
	}
	if admin.PasswordHash == "" {
		t.Fatal("password hash is empty")
	}
	if admin.Salt == "" {
		t.Fatal("salt is empty")
	}
	if admin.Username != "alice" {
		t.Fatalf("username: got %q, want %q", admin.Username, "alice")
	}
	if admin.Role != "admin" {
		t.Fatalf("role: got %q, want %q", admin.Role, "admin")
	}
	if admin.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}

	// Verify the hash is deterministic with the correct salt
	expected := hashPassword(password, admin.Salt)
	if admin.PasswordHash != expected {
		t.Fatalf("hash mismatch: got %q, want %q", admin.PasswordHash, expected)
	}
}

func TestAddAdmin_UniqueSalts(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("user1", "samepassword", "admin")
	s.AddAdmin("user2", "samepassword", "admin")

	a1 := s.admins["user1"]
	a2 := s.admins["user2"]

	if a1.Salt == a2.Salt {
		t.Fatal("two users have identical salts — salt generation is broken")
	}
	// Same password with different salts must produce different hashes
	if a1.PasswordHash == a2.PasswordHash {
		t.Fatal("same password produced same hash with different salts")
	}
}

func TestAuthenticate_CorrectCredentials(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass123", "super_admin")

	token, err := s.Authenticate("alice", "pass123")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	// Token must be a valid 3-part JWT
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
}

func TestAuthenticate_WrongPassword(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass123", "admin")

	_, err := s.Authenticate("alice", "wrongpassword")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthenticate_NonExistentUser(t *testing.T) {
	s := NewStore(testSecret)

	_, err := s.Authenticate("nobody", "pass123")
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateToken_Valid(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass123", "reviewer")

	token, _ := s.Authenticate("alice", "pass123")
	claims, err := s.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Sub != "alice" {
		t.Fatalf("sub: got %q, want %q", claims.Sub, "alice")
	}
	if claims.Role != "reviewer" {
		t.Fatalf("role: got %q, want %q", claims.Role, "reviewer")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	s := NewStore(testSecret)

	// Manually create an expired token
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := Claims{
		Sub:  "alice",
		Role: "admin",
		Iat:  time.Now().Add(-48 * time.Hour).Unix(),
		Exp:  time.Now().Add(-24 * time.Hour).Unix(), // expired 24h ago
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := header + "." + payload
	sig := signForTest([]byte(signingInput), s.secret)
	token := signingInput + "." + sig

	_, err := s.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected 'expired' in error, got: %v", err)
	}
}

func TestValidateToken_TamperedSignature(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass123", "admin")

	token, _ := s.Authenticate("alice", "pass123")

	// Replace last character of signature to tamper with it
	tampered := token[:len(token)-1] + "X"

	_, err := s.ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected 'signature' in error, got: %v", err)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	s1 := NewStore("secret-one")
	s2 := NewStore("secret-two")

	s1.AddAdmin("alice", "pass123", "admin")
	token, _ := s1.Authenticate("alice", "pass123")

	// Validate with a different secret — must fail
	_, err := s2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong secret, got nil")
	}
}

func TestValidateToken_MalformedTokens(t *testing.T) {
	s := NewStore(testSecret)

	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no dots", "notajwt"},
		{"one dot", "part1.part2"},
		{"four dots", "a.b.c.d"},
		{"just dots", ".."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.ValidateToken(tc.token)
			if err == nil {
				t.Fatalf("expected error for malformed token %q, got nil", tc.token)
			}
		})
	}
}

func TestJWTClaims_Content(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("bob", "mypass", "super_admin")

	before := time.Now().Unix()
	token, _ := s.Authenticate("bob", "mypass")
	after := time.Now().Unix()

	claims, err := s.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	// sub
	if claims.Sub != "bob" {
		t.Fatalf("sub: got %q, want %q", claims.Sub, "bob")
	}
	// role
	if claims.Role != "super_admin" {
		t.Fatalf("role: got %q, want %q", claims.Role, "super_admin")
	}
	// iat should be between before and after
	if claims.Iat < before || claims.Iat > after {
		t.Fatalf("iat %d not in range [%d, %d]", claims.Iat, before, after)
	}
	// exp should be ~24h after iat
	expectedExp := claims.Iat + 24*3600
	if claims.Exp != expectedExp {
		t.Fatalf("exp: got %d, want %d (iat+24h)", claims.Exp, expectedExp)
	}
}

func TestJWT_HeaderAlgorithm(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass", "admin")

	token, _ := s.Authenticate("alice", "pass")
	parts := strings.Split(token, ".")

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}

	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "HS256" {
		t.Fatalf("alg: got %q, want HS256", header["alg"])
	}
	if header["typ"] != "JWT" {
		t.Fatalf("typ: got %q, want JWT", header["typ"])
	}
}

// --- Middleware Tests ---

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"user": UserFromContext(r.Context()),
			"role": RoleFromContext(r.Context()),
		})
	})
}

func TestMiddleware_NoAuthHeader(t *testing.T) {
	s := NewStore(testSecret)
	mw := Middleware(s)
	handler := mw(okHandler())

	req := httptest.NewRequest("GET", "/admin/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "admin token required") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	s := NewStore(testSecret)
	mw := Middleware(s)
	handler := mw(okHandler())

	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.here")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddleware_BearerPrefixMissing(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass", "admin")
	token, _ := s.Authenticate("alice", "pass")

	mw := Middleware(s)
	handler := mw(okHandler())

	// Send token without Bearer prefix
	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("alice", "pass123", "super_admin")
	token, _ := s.Authenticate("alice", "pass123")

	mw := Middleware(s)
	handler := mw(okHandler())

	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["user"] != "alice" {
		t.Fatalf("X-Admin-User: got %q, want %q", resp["user"], "alice")
	}
	if resp["role"] != "super_admin" {
		t.Fatalf("X-Admin-Role: got %q, want %q", resp["role"], "super_admin")
	}
}

func TestMiddleware_ExpiredToken(t *testing.T) {
	s := NewStore(testSecret)

	// Craft an expired token
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := Claims{
		Sub:  "alice",
		Role: "admin",
		Iat:  time.Now().Add(-48 * time.Hour).Unix(),
		Exp:  time.Now().Add(-1 * time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := header + "." + payload
	sig := signForTest([]byte(signingInput), s.secret)
	token := signingInput + "." + sig

	mw := Middleware(s)
	handler := mw(okHandler())

	req := httptest.NewRequest("GET", "/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMultipleAdminUsers_DifferentRoles(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("superadmin", "pass1", "super_admin")
	s.AddAdmin("reviewer", "pass2", "reviewer")
	s.AddAdmin("regular", "pass3", "admin")

	// Each user authenticates and gets correct claims
	tests := []struct {
		user string
		pass string
		role string
	}{
		{"superadmin", "pass1", "super_admin"},
		{"reviewer", "pass2", "reviewer"},
		{"regular", "pass3", "admin"},
	}

	for _, tc := range tests {
		t.Run(tc.user, func(t *testing.T) {
			token, err := s.Authenticate(tc.user, tc.pass)
			if err != nil {
				t.Fatalf("Authenticate(%q): %v", tc.user, err)
			}

			claims, err := s.ValidateToken(token)
			if err != nil {
				t.Fatalf("ValidateToken: %v", err)
			}
			if claims.Sub != tc.user {
				t.Fatalf("sub: got %q, want %q", claims.Sub, tc.user)
			}
			if claims.Role != tc.role {
				t.Fatalf("role: got %q, want %q", claims.Role, tc.role)
			}
		})
	}

	// Cross-auth: user1's password must not work for user2
	_, err := s.Authenticate("superadmin", "pass2")
	if err == nil {
		t.Fatal("expected error when using another user's password")
	}
}

func TestPasswordHash_NeverStoredPlaintext(t *testing.T) {
	s := NewStore(testSecret)

	passwords := []string{
		"simple",
		"P@ssw0rd!",
		"with spaces in it",
		"unicode-密码-пароль",
		"",
	}

	for i, pw := range passwords {
		user := strings.Replace(strings.Replace(
			"user"+string(rune('A'+i)), " ", "", -1), "-", "", -1)
		s.AddAdmin(user, pw, "admin")
		admin := s.admins[user]

		if admin.PasswordHash == pw {
			t.Fatalf("CRITICAL: password %q stored as plaintext for user %s", pw, user)
		}

		// Verify the hash is a valid hex string (64 chars for SHA-256)
		if pw != "" && len(admin.PasswordHash) != 64 {
			t.Fatalf("hash length: got %d, want 64 hex chars", len(admin.PasswordHash))
		}
	}
}

func TestWriteAdminError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAdminError(rec, http.StatusForbidden, "access denied")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type: got %q, want application/json", ct)
	}

	body, _ := io.ReadAll(rec.Body)
	var resp map[string]string
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp["error"] != "access denied" {
		t.Fatalf("error message: got %q, want %q", resp["error"], "access denied")
	}
}

// signForTest replicates the internal sign function for crafting test tokens.
func signForTest(data, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
