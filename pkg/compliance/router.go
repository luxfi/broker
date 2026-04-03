package compliance

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/luxfi/compliance/pkg/jube"
	"github.com/rs/zerolog/log"
)

// RouterOption configures optional dependencies for the compliance router.
type RouterOption func(*routerConfig)

type routerConfig struct {
	jubeClient *jube.Client
}

// WithJubeClient adds the Jube AML sidecar client for live screening.
func WithJubeClient(c *jube.Client) RouterOption {
	return func(cfg *routerConfig) { cfg.jubeClient = c }
}

// NewRouter creates a chi sub-router with all compliance endpoints.
// Mount this under /compliance on the main router.
// The adminStore parameter provides authentication and RBAC. When non-nil,
// all routes (except /healthz) require a valid admin JWT and write operations
// are gated by role-based permissions.
// Optional RouterOption values add the auth store and Jube client.
func NewRouter(store ComplianceStore, opts ...RouterOption) chi.Router {
	if store == nil {
		store = NewMemoryStore()
	}

	var cfg routerConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	kyc := &kycHandler{store: store}
	onboard := &onboardingHandler{store: store}
	funds := &fundsHandler{store: store}
	esign := &esignHandler{store: store}
	roles := &rolesHandler{store: store}
	dash := &dashboardHandler{store: store}
	users := &usersHandler{store: store}
	txns := &transactionsHandler{store: store}
	reports := &reportsHandler{}
	settings := &settingsHandler{store: store}
	creds := &credentialsHandler{store: store}
	billing := &billingHandler{}
	aml := &amlHandler{store: store, jubeClient: cfg.jubeClient}
	apps := &applicationHandler{store: store}

	// guard wraps a handler with RBAC using the stored role/permission system.
	guard := func(module, action string, h http.HandlerFunc) http.HandlerFunc {
		return requireRole(store, module, action, h)
	}

	r := chi.NewRouter()

	// CORS — explicit production origins only. No wildcards to prevent
	// subdomain takeover attacks (MEDIUM-2). Localhost origins gated behind
	// environment check (LOW-2).
	// CORS origins from env (comma-separated) or defaults
	corsOrigins := []string{
		"https://lux.exchange",
		"https://admin.lux.exchange",
	}
	if extra := os.Getenv("CORS_ALLOWED_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				corsOrigins = append(corsOrigins, trimmed)
			}
		}
	}
	if os.Getenv("BROKER_ENV") != "production" {
		corsOrigins = append(corsOrigins, "http://localhost:3000", "http://localhost:3100")
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Request body size limit — 1MB max to prevent abuse.
	r.Use(maxBodySize(1 << 20))

	// Security headers for all compliance responses.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	})

	// Health check — no auth required
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": "0.1.0",
		})
	})

	// All routes require IAM authentication via gateway headers.
	// No internal admin JWT — the gateway is the only auth layer.
	r.Group(func(r chi.Router) {

		// KYC
		r.Route("/kyc", func(r chi.Router) {
			r.Post("/verify", rateLimitSensitive(guard("kyc", "write", kyc.handleVerify)))
			r.Get("/", guard("kyc", "read", kyc.handleListByUser))
			r.Get("/{id}", guard("kyc", "read", kyc.handleGet))
			r.Patch("/{id}", guard("kyc", "write", kyc.handleUpdateStatus))
			r.Post("/{id}/documents", guard("kyc", "write", kyc.handleUploadDocument))
		})

		// AML
		r.Route("/aml", func(r chi.Router) {
			r.Post("/screen", rateLimitSensitive(guard("aml", "write", aml.handleScreen)))
			r.Post("/risk-assessment", rateLimitSensitive(guard("aml", "write", aml.handleRiskAssessment)))
			r.Get("/screenings", guard("aml", "read", aml.handleListByAccount))
			r.Get("/flagged", guard("aml", "read", aml.handleListFlagged))
			r.Get("/screenings/{id}", guard("aml", "read", aml.handleGet))
			r.Post("/screenings/{id}/review", guard("aml", "write", aml.handleReview))
		})

		// Onboarding Applications
		r.Route("/applications", func(r chi.Router) {
			r.Get("/", guard("applications", "read", apps.handleList))
			r.Post("/", guard("applications", "write", apps.handleCreate))
			r.Get("/lookup", guard("applications", "read", apps.handleGetByUser))
			r.Get("/{id}", guard("applications", "read", apps.handleGet))
			r.Post("/{id}/step/1", guard("applications", "write", apps.handleStep1))
			r.Post("/{id}/step/2", guard("applications", "write", apps.handleStep2))
			r.Post("/{id}/step/3", guard("applications", "write", apps.handleStep3))
			r.Post("/{id}/step/4", guard("applications", "write", apps.handleStep4))
			r.Post("/{id}/step/5", guard("applications", "write", apps.handleStep5))
			r.Post("/{id}/review", guard("applications", "write", apps.handleReview))
			r.Get("/{id}/documents", guard("applications", "read", apps.handleGetDocuments))
		})

		// Pipelines
		r.Route("/pipelines", func(r chi.Router) {
			r.Get("/", guard("pipelines", "read", onboard.handleListPipelines))
			r.Post("/", guard("pipelines", "write", onboard.handleCreatePipeline))
			r.Get("/{id}", guard("pipelines", "read", onboard.handleGetPipeline))
			r.Patch("/{id}", guard("pipelines", "write", onboard.handleUpdatePipeline))
			r.Delete("/{id}", guard("pipelines", "delete", onboard.handleDeletePipeline))
		})

		// Sessions
		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", guard("sessions", "read", onboard.handleListSessions))
			r.Post("/", guard("sessions", "write", onboard.handleCreateSession))
			r.Get("/{id}", guard("sessions", "read", onboard.handleGetSession))
			r.Patch("/{id}", guard("sessions", "write", onboard.handleUpdateSession))
			r.Get("/{id}/steps", guard("sessions", "read", onboard.handleGetSessionSteps))
		})

		// Funds
		r.Route("/funds", func(r chi.Router) {
			r.Get("/", guard("funds", "read", funds.handleListFunds))
			r.Post("/", guard("funds", "write", funds.handleCreateFund))
			r.Get("/{id}", guard("funds", "read", funds.handleGetFund))
			r.Patch("/{id}", guard("funds", "write", funds.handleUpdateFund))
			r.Delete("/{id}", guard("funds", "delete", funds.handleDeleteFund))
			r.Get("/{id}/investors", guard("funds", "read", funds.handleListInvestors))
		})

		// eSign
		r.Route("/esign", func(r chi.Router) {
			r.Get("/envelopes", guard("esign", "read", esign.handleListEnvelopes))
			r.Post("/envelopes", guard("esign", "write", esign.handleCreateEnvelope))
			r.Get("/envelopes/{id}", guard("esign", "read", esign.handleGetEnvelope))
			r.Post("/envelopes/{id}/sign", guard("esign", "write", esign.handleSign))
			r.Get("/templates", guard("esign", "read", esign.handleListTemplates))
			r.Post("/templates", guard("esign", "write", esign.handleCreateTemplate))
		})

		// Roles — GET requires "roles:read" RBAC (HIGH-1).
		// Mutation (POST/PATCH/DELETE) restricted to superadmin/owner to
		// prevent role self-escalation (HIGH-3).
		r.Route("/roles", func(r chi.Router) {
			r.Get("/", guard("roles", "read", roles.handleListRoles))
			r.Post("/", requireSuperAdmin(roles.handleCreateRole))
			r.Get("/{id}", guard("roles", "read", roles.handleGetRole))
			r.Patch("/{id}", requireSuperAdmin(roles.handleUpdateRole))
			r.Delete("/{id}", requireSuperAdmin(roles.handleDeleteRole))
		})

		// Modules (for permission matrix)
		r.Get("/modules", guard("roles", "read", roles.handleListModules))

		// Dashboard
		r.Get("/dashboard", guard("dashboard", "read", dash.handleDashboard))

		// Users
		r.Get("/users", guard("roles", "read", users.handleListUsers))
		r.Post("/users", guard("roles", "write", users.handleCreateUser))

		// Transactions
		r.Get("/transactions", guard("transactions", "read", txns.handleListTransactions))

		// Reports
		r.Get("/reports", guard("transactions", "read", reports.handleListReports))

		// Settings
		r.Get("/settings", guard("dashboard", "read", settings.handleGetSettings))
		r.Put("/settings", guard("dashboard", "admin", settings.handleUpdateSettings))

		// Credentials (API key management)
		r.Get("/credentials", guard("roles", "admin", creds.handleListCredentials))
		r.Post("/credentials", guard("roles", "admin", creds.handleCreateCredential))
		r.Delete("/credentials/{id}", guard("roles", "admin", creds.handleDeleteCredential))

		// Billing
		r.Get("/billing", guard("dashboard", "read", billing.handleGetBilling))

		// eSign dashboard (aggregate stats)
		r.Get("/esign-dashboard", guard("esign", "read", dash.handleESignDashboard))

		// Envelope views by direction
		r.Get("/envelopes/inbox", guard("esign", "read", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, store.ListEnvelopesByDirection("inbox"))
		}))
		r.Get("/envelopes/sent", guard("esign", "read", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, store.ListEnvelopesByDirection("sent"))
		}))
	})

	return r
}

// requireRole returns an http.HandlerFunc that checks if the authenticated admin
// has the required module+action permission by looking up their role in the compliance
// store's roles table. The special "superadmin" role retains implicit full access
// for bootstrap/recovery scenarios. All other roles are checked against stored
// permissions (HIGH-1).
func requireRole(store ComplianceStore, module, action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roleName := r.Header.Get("X-User-Roles")
		if roleName == "" {
			writeError(w, http.StatusForbidden, "no role in token")
			return
		}

		// superadmin is an escape hatch for bootstrap/recovery.
		// Always log when this bypass is used for audit trail.
		if roleName == "superadmin" {
			user := r.Header.Get("X-User-Id")
			log.Warn().
				Str("user", user).
				Str("module", module).
				Str("action", action).
				Str("path", r.URL.Path).
				Str("method", r.Method).
				Msg("superadmin bypass used")
			next(w, r)
			return
		}

		// Look up the role's permissions from the store.
		role, err := store.GetRoleByName(roleName)
		if err != nil {
			writeError(w, http.StatusForbidden, "unknown role: "+roleName)
			return
		}

		for _, perm := range role.Permissions {
			if perm.Module == module && perm.Action == action {
				next(w, r)
				return
			}
			// "admin" action on a module implies all other actions.
			if perm.Module == module && perm.Action == "admin" {
				next(w, r)
				return
			}
		}

		writeError(w, http.StatusForbidden, "insufficient permissions for "+module+":"+action)
	}
}

// requireSuperAdmin rejects requests unless the JWT role is "superadmin" or
// "owner". Used for role mutation endpoints to prevent self-escalation (HIGH-3).
func requireSuperAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roleName := r.Header.Get("X-User-Roles")
		if roleName != "superadmin" && roleName != "owner" {
			writeError(w, http.StatusForbidden, "role mutation requires superadmin or owner role")
			return
		}
		next(w, r)
	}
}

// writeJSON encodes v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// extractIP returns the client IP from r.RemoteAddr with the port stripped.
// We use RemoteAddr instead of X-Forwarded-For because X-Forwarded-For is
// trivially spoofable. In production behind a trusted proxy (e.g., hanzoai/ingress),
// the proxy sets RemoteAddr to the real client IP.
func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMaxEntries is the maximum number of distinct IPs tracked in a
// single rate limiter map. If exceeded, the oldest half is evicted to prevent
// memory exhaustion under attack.
const rateLimitMaxEntries = 10000

// evictOldest removes the oldest half of entries from the map when it exceeds
// rateLimitMaxEntries. This prevents memory exhaustion from distributed attacks.
func evictOldest(attempts map[string][]time.Time) {
	if len(attempts) <= rateLimitMaxEntries {
		return
	}
	type entry struct {
		ip     string
		oldest time.Time
	}
	entries := make([]entry, 0, len(attempts))
	for ip, ts := range attempts {
		if len(ts) > 0 {
			entries = append(entries, entry{ip, ts[0]})
		} else {
			entries = append(entries, entry{ip, time.Time{}})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].oldest.Before(entries[j].oldest)
	})
	// Remove oldest half.
	removeCount := len(entries) / 2
	for i := 0; i < removeCount; i++ {
		delete(attempts, entries[i].ip)
	}
}

// rateLimitLogin wraps a login handler with per-IP rate limiting.
// Allows 5 attempts per minute per IP, then returns 429.
func rateLimitLogin(next http.HandlerFunc) http.HandlerFunc {
	var mu sync.Mutex
	attempts := make(map[string][]time.Time)

	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		mu.Lock()
		now := time.Now()
		window := now.Add(-1 * time.Minute)

		// Prune old entries for this IP.
		valid := attempts[ip][:0]
		for _, t := range attempts[ip] {
			if t.After(window) {
				valid = append(valid, t)
			}
		}
		// Evict empty entries to prevent memory leak.
		if len(valid) == 0 {
			delete(attempts, ip)
		} else {
			attempts[ip] = valid
		}

		if len(attempts[ip]) >= 5 {
			mu.Unlock()
			writeError(w, http.StatusTooManyRequests, "too many login attempts, try again in 1 minute")
			return
		}
		attempts[ip] = append(attempts[ip], now)
		evictOldest(attempts)
		mu.Unlock()

		next(w, r)
	}
}

// rateLimitSensitive wraps a handler with per-IP rate limiting for sensitive
// compliance operations (KYC verify, AML screen). Allows 10 requests per
// minute per IP, then returns 429.
func rateLimitSensitive(next http.HandlerFunc) http.HandlerFunc {
	var mu sync.Mutex
	attempts := make(map[string][]time.Time)

	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		mu.Lock()
		now := time.Now()
		window := now.Add(-1 * time.Minute)

		valid := attempts[ip][:0]
		for _, t := range attempts[ip] {
			if t.After(window) {
				valid = append(valid, t)
			}
		}
		// Evict empty entries to prevent memory leak.
		if len(valid) == 0 {
			delete(attempts, ip)
		} else {
			attempts[ip] = valid
		}

		if len(attempts[ip]) >= 10 {
			mu.Unlock()
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded, try again in 1 minute")
			return
		}
		attempts[ip] = append(attempts[ip], now)
		evictOldest(attempts)
		mu.Unlock()

		next(w, r)
	}
}

// maxBodySize returns middleware that limits request body size to prevent
// denial-of-service via large payloads.
func maxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
