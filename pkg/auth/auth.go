package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APIKey represents an authenticated API client.
type APIKey struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`         // human-readable name
	OrgID       string   `json:"org_id"`       // organization
	Permissions []string `json:"permissions"`   // read, trade, admin
	RateLimit   int      `json:"rate_limit"`    // requests per minute
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Store holds API keys. In production, back this with a database.
type Store struct {
	mu   sync.RWMutex
	keys map[string]*APIKey // key -> APIKey
}

func NewStore() *Store {
	return &Store{keys: make(map[string]*APIKey)}
}

// Add registers an API key.
func (s *Store) Add(k *APIKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[k.Key] = k
}

// Validate checks an API key and returns the associated client.
func (s *Store) Validate(key string) (*APIKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for storedKey, ak := range s.keys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) == 1 {
			if ak.ExpiresAt != nil && time.Now().After(*ak.ExpiresAt) {
				return nil, false
			}
			return ak, true
		}
	}
	return nil, false
}

// HasPermission checks if an API key has a specific permission.
func (ak *APIKey) HasPermission(perm string) bool {
	for _, p := range ak.Permissions {
		if p == perm || p == "admin" {
			return true
		}
	}
	return false
}

// Middleware returns an HTTP middleware that validates auth.
// Accepts:
//   - IAM headers: X-User-Id + X-Org-Id (Gateway-validated JWT)
//   - API key: Authorization: Bearer <key> or X-API-Key: <key>
func Middleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health check
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			// IAM auth: Gateway injects identity headers after JWT validation.
			// Accept X-User-Id (new) and X-IAM-User-Id (legacy, until gateway updates).
			userId := r.Header.Get("X-User-Id")
			if userId == "" {
				userId = r.Header.Get("X-IAM-User-Id")
			}
			if userId != "" {
				orgId := r.Header.Get("X-Org-Id")
				if orgId == "" {
					orgId = r.Header.Get("X-IAM-Org-Id")
				}
				r.Header.Set("X-Org-ID", orgId)
				r.Header.Set("X-API-Key-Name", "iam-user:"+userId)
				next.ServeHTTP(w, r)
				return
			}
			// API key auth: for service-to-service and admin access
			key := extractAPIKey(r)
			if key == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"authentication required"}`))
				return
			}

			ak, valid := store.Validate(key)
			if !valid {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid or expired API key"}`))
				return
			}

			// Store API key info in request context via header (simple approach)
			r.Header.Set("X-Org-ID", ak.OrgID)
			r.Header.Set("X-API-Key-Name", ak.Name)
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission returns middleware that checks for a specific permission.
// IAM users are authorized based on roles from the X-User-Roles header (set by Gateway
// from JWT claims). Admin role grants all permissions. Regular users get "read" and "trade".
func RequirePermission(store *Store, perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// IAM auth — check roles propagated by Gateway from JWT claims
			userId := r.Header.Get("X-User-Id")
			if userId == "" {
				userId = r.Header.Get("X-IAM-User-Id")
			}
			if userId != "" {
				roles := r.Header.Get("X-User-Roles")
				if hasRole(roles, perm) || hasRole(roles, "admin") {
					next.ServeHTTP(w, r)
					return
				}
				// Default permissions for authenticated IAM users without explicit roles:
				// read (browsing) is allowed. Trade requires explicit role assignment
				// to prevent pre-KYC users from placing orders (SEC Rule 301(b)(5)).
				if perm == "read" {
					next.ServeHTTP(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"insufficient permissions"}`))
				return
			}
			key := extractAPIKey(r)
			if key == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"authentication required"}`))
				return
			}
			ak, valid := store.Validate(key)
			if !valid || !ak.HasPermission(perm) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"insufficient permissions"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hasRole checks if a comma-separated roles string contains the given role.
func hasRole(roles, role string) bool {
	for _, r := range strings.Split(roles, ",") {
		if strings.TrimSpace(r) == role {
			return true
		}
	}
	return false
}

func extractAPIKey(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// Check X-API-Key header
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}
	return ""
}
