package auth

import (
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware that reads identity from
// gateway-propagated headers. The Hanzo Gateway validates JWTs via
// IAM JWKS and sets:
//
//   - X-User-Id    (from JWT sub claim)
//   - X-User-Email (from JWT preferred_username claim)
//   - X-Org-Id     (from JWT owner claim)
//
// Requests without X-User-Id are rejected as unauthenticated.
// /healthz is always public.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			userID := r.Header.Get("X-User-Id")
			if userID == "" {
				writeErr(w, http.StatusUnauthorized, "authentication required")
				return
			}

			orgID := r.Header.Get("X-Org-Id")
			email := r.Header.Get("X-User-Email")

			// Normalize for downstream handlers.
			r.Header.Set("X-User-Id", userID)
			r.Header.Set("X-Org-Id", orgID)
			r.Header.Set("X-User-Email", email)
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission checks roles from X-User-Roles (set by Gateway
// from JWT claims). Admin role grants all permissions. Authenticated
// users without explicit roles get "read" access.
// X-User-IsAdmin: true (from IAM JWT isAdmin claim) also grants admin.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roles := r.Header.Get("X-User-Roles")
			isAdmin := strings.EqualFold(r.Header.Get("X-User-IsAdmin"), "true")
			if isAdmin || hasRole(roles, perm) || hasRole(roles, "admin") {
				next.ServeHTTP(w, r)
				return
			}
			// Default: read is allowed for any authenticated user.
			if perm == "read" {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusForbidden, "insufficient permissions")
		})
	}
}

func hasRole(roles, role string) bool {
	for _, r := range strings.Split(roles, ",") {
		if strings.TrimSpace(r) == role {
			return true
		}
	}
	return false
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}
