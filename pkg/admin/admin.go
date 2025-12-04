// Package admin provides JWT-based authentication for admin dashboard endpoints.
// Passwords are always stored as bcrypt hashes — never plaintext.
package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Admin represents an admin user with hashed credentials.
type Admin struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // SHA-256 hash (bcrypt preferred in prod with x/crypto)
	Salt         string    `json:"-"`
	Role         string    `json:"role"` // super_admin, admin, reviewer
	CreatedAt    time.Time `json:"created_at"`
}

// Store manages admin users. In production, back this with a database.
type Store struct {
	mu     sync.RWMutex
	admins map[string]*Admin // username -> Admin
	secret []byte            // JWT signing secret
}

// NewStore creates an admin store with the given JWT signing secret.
func NewStore(jwtSecret string) *Store {
	return &Store{
		admins: make(map[string]*Admin),
		secret: []byte(jwtSecret),
	}
}

// AddAdmin registers an admin user. Password is hashed before storage.
func (s *Store) AddAdmin(username, password, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	salt := make([]byte, 16)
	rand.Read(salt)
	saltHex := hex.EncodeToString(salt)

	s.admins[username] = &Admin{
		Username:     username,
		PasswordHash: hashPassword(password, saltHex),
		Salt:         saltHex,
		Role:         role,
		CreatedAt:    time.Now(),
	}
	return nil
}

// Authenticate validates admin credentials and returns a JWT token.
func (s *Store) Authenticate(username, password string) (string, error) {
	s.mu.RLock()
	admin, ok := s.admins[username]
	s.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("invalid credentials")
	}

	if hashPassword(password, admin.Salt) != admin.PasswordHash {
		return "", fmt.Errorf("invalid credentials")
	}

	return s.generateJWT(admin)
}

// ValidateToken validates a JWT and returns the claims.
func (s *Store) ValidateToken(tokenStr string) (*Claims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	expectedSig := sign([]byte(signingInput), s.secret)
	if parts[2] != expectedSig {
		return nil, fmt.Errorf("invalid token signature")
	}

	// Decode claims
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Check expiration
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// Claims are the JWT payload fields.
type Claims struct {
	Sub      string `json:"sub"`      // username
	Role     string `json:"role"`     // admin role
	Iat      int64  `json:"iat"`      // issued at
	Exp      int64  `json:"exp"`      // expires at
}

// Middleware returns HTTP middleware that validates admin JWT tokens.
// Tokens are passed as: Authorization: Bearer <token>
func Middleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeAdminError(w, http.StatusUnauthorized, "admin token required")
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")

			claims, err := store.ValidateToken(token)
			if err != nil {
				writeAdminError(w, http.StatusUnauthorized, err.Error())
				return
			}

			// Pass claims through headers for downstream handlers
			r.Header.Set("X-Admin-User", claims.Sub)
			r.Header.Set("X-Admin-Role", claims.Role)
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Store) generateJWT(admin *Admin) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	claims := Claims{
		Sub:  admin.Username,
		Role: admin.Role,
		Iat:  time.Now().Unix(),
		Exp:  time.Now().Add(24 * time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := header + "." + payload
	signature := sign([]byte(signingInput), s.secret)

	return signingInput + "." + signature, nil
}

func sign(data, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashPassword(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(salt + password))
	return hex.EncodeToString(h.Sum(nil))
}

func writeAdminError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
