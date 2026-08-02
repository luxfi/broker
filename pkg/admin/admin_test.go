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

	"golang.org/x/crypto/bcrypt"

	"github.com/luxfi/broker/pkg/token"
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
	if admin.Username != "alice" {
		t.Fatalf("username: got %q, want %q", admin.Username, "alice")
	}
	if admin.Role != "admin" {
		t.Fatalf("role: got %q, want %q", admin.Role, "admin")
	}
	if admin.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}

	// Verify the hash is valid bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		t.Fatalf("bcrypt verification failed: %v", err)
	}
}

func TestAddAdmin_UniqueHashes(t *testing.T) {
	s := NewStore(testSecret)
	s.AddAdmin("user1", "samepassword", "admin")
	s.AddAdmin("user2", "samepassword", "admin")

	a1 := s.admins["user1"]
	a2 := s.admins["user2"]

	// bcrypt includes random salt — same password must produce different hashes
	if a1.PasswordHash == a2.PasswordHash {
		t.Fatal("same password produced same hash — bcrypt salt generation is broken")
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

	// Replace the last character of the signature with a different one.
	// Picking a fixed letter would silently be a no-op whenever the token
	// already ends in it, so derive the substitute from what is there.
	sub := byte('A')
	if token[len(token)-1] == sub {
		sub = 'B'
	}
	tampered := token[:len(token)-1] + string(sub)

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
	}

	for i, pw := range passwords {
		user := strings.Replace(strings.Replace(
			"user"+string(rune('A'+i)), " ", "", -1), "-", "", -1)
		s.AddAdmin(user, pw, "admin")
		admin := s.admins[user]

		if admin.PasswordHash == pw {
			t.Fatalf("CRITICAL: password %q stored as plaintext for user %s", pw, user)
		}

		// Verify bcrypt can validate it
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(pw)); err != nil {
			t.Fatalf("bcrypt verification failed for user %s: %v", user, err)
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

// --- Token canonicality ---
//
// An HMAC-SHA256 signature is 32 bytes carried in 43 base64url characters:
// 258 bits of room for 256 bits of MAC. The 2 spare bits in the final
// character mean four distinct strings decode to the identical MAC. A lenient
// decoder accepts all four, so one credential arrives wearing four names and
// every string-keyed denylist, audit record and rate-limit bucket splits.

// respellSig returns the four token strings whose signature segments decode
// to the same MAC bytes, canonical first.
func respellSig(t *testing.T, tok string) []string {
	t.Helper()
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	dot := strings.LastIndex(tok, ".")
	sig := tok[dot+1:]
	if len(sig) != 43 {
		t.Fatalf("signature segment is %d chars, want 43 for a 32-byte MAC", len(sig))
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	last := strings.IndexByte(alphabet, sig[len(sig)-1])
	out := []string{tok}
	for b := 0; b < 4; b++ {
		v := tok[:dot+1] + sig[:len(sig)-1] + string(alphabet[(last&^3)|b])
		if v == tok {
			continue
		}
		got, err := base64.RawURLEncoding.DecodeString(v[dot+1:])
		if err != nil || !hmac.Equal(got, want) {
			t.Fatalf("test premise: respelling %q does not decode to the same MAC", v[len(v)-4:])
		}
		out = append(out, v)
	}
	if len(out) != 4 {
		t.Fatalf("built %d spellings, want 4", len(out))
	}
	return out
}

func TestValidateToken_AcceptsExactlyOneSpelling(t *testing.T) {
	s := NewStore(testSecret)
	if err := s.AddAdmin("root", "hunter2hunter2", "super_admin"); err != nil {
		t.Fatal(err)
	}
	tok, err := s.Authenticate("root", "hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}

	accepted := 0
	for i, v := range respellSig(t, tok) {
		_, err := s.ValidateToken(v)
		if err == nil {
			accepted++
			if i != 0 {
				t.Errorf("accepted non-canonical spelling ...%q", v[len(v)-4:])
			}
		}
	}
	if accepted != 1 {
		t.Fatalf("%d of 4 spellings authenticate as the same credential, want exactly 1", accepted)
	}
}

// TestValidateToken_RevocationHoldsAcrossSpellings is the operational failure
// the malleability caused: an operator revokes the token they were handed and
// the holder keeps using the other three. Revocation is keyed on token.ID —
// the hash of the bytes — never on the submitted string.
func TestValidateToken_RevocationHoldsAcrossSpellings(t *testing.T) {
	s := NewStore(testSecret)
	if err := s.AddAdmin("root", "hunter2hunter2", "super_admin"); err != nil {
		t.Fatal(err)
	}
	tok, err := s.Authenticate("root", "hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}

	revoked := map[string]bool{token.ID(tok): true}
	authenticates := func(submitted string) bool {
		if _, err := s.ValidateToken(submitted); err != nil {
			return false
		}
		return !revoked[token.ID(submitted)]
	}

	if authenticates(tok) {
		t.Fatal("revoked token still authenticates in its canonical spelling")
	}
	for _, v := range respellSig(t, tok)[1:] {
		if authenticates(v) {
			t.Errorf("revoked token still authenticates as ...%q", v[len(v)-4:])
		}
	}
}

func TestValidateToken_RejectsWhitespaceInSegments(t *testing.T) {
	s := NewStore(testSecret)
	if err := s.AddAdmin("root", "hunter2hunter2", "super_admin"); err != nil {
		t.Fatal(err)
	}
	tok, _ := s.Authenticate("root", "hunter2hunter2")
	dot := strings.LastIndex(tok, ".")

	// Go's base64 decoder skips \r and \n wherever they appear, so these
	// carry the same MAC bytes. Reachable over any carrier that permits them
	// (query string, JSON body, cookie) even though HTTP headers do not.
	for _, v := range []string{
		tok[:dot+1] + "\n" + tok[dot+1:],
		tok + "\n",
		tok[:dot+10] + "\r\n" + tok[dot+10:],
	} {
		if _, err := s.ValidateToken(v); err == nil {
			t.Errorf("accepted a signature segment containing CR/LF")
		}
	}
}

// The verifier is HMAC-SHA256 and never dispatches on the token's own header,
// so a foreign alg is rejected outright rather than honoured.
func TestValidateToken_RejectsForeignAlg(t *testing.T) {
	s := NewStore(testSecret)
	if err := s.AddAdmin("root", "hunter2hunter2", "super_admin"); err != nil {
		t.Fatal(err)
	}

	header := token.Encode([]byte(`{"alg":"none","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(Claims{Sub: "root", Role: "super_admin", Exp: time.Now().Add(time.Hour).Unix()})
	signingInput := header + "." + token.Encode(claimsJSON)
	forged := signingInput + "." + token.Encode(sign([]byte(signingInput), s.secret))

	if _, err := s.ValidateToken(forged); err == nil {
		t.Fatal("accepted a correctly-signed token declaring alg=none")
	}
}

func TestValidateToken_RejectsEmptySegments(t *testing.T) {
	s := NewStore(testSecret)
	if err := s.AddAdmin("root", "hunter2hunter2", "super_admin"); err != nil {
		t.Fatal(err)
	}
	tok, _ := s.Authenticate("root", "hunter2hunter2")
	parts := strings.Split(tok, ".")

	for i := range parts {
		blanked := append([]string(nil), parts...)
		blanked[i] = ""
		if _, err := s.ValidateToken(strings.Join(blanked, ".")); err == nil {
			t.Errorf("accepted a token with segment %d empty", i)
		}
	}
}
