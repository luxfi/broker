package compliance

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luxfi/broker/pkg/auth"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/webhook"
	"github.com/luxfi/compliance/pkg/jube"
)

// RouterOption configures optional dependencies for the compliance router.
type RouterOption func(*routerConfig)

type routerConfig struct {
	jubeClient   *jube.Client
	scamDB       *ScamDB
	registry     *provider.Registry
	webhookStore webhook.Store
}

// WithJubeClient adds the Jube AML sidecar client for live screening.
func WithJubeClient(c *jube.Client) RouterOption {
	return func(cfg *routerConfig) { cfg.jubeClient = c }
}

// WithScamDB adds the ScamSniffer scam address database for wallet screening.
func WithScamDB(db *ScamDB) RouterOption {
	return func(cfg *routerConfig) { cfg.scamDB = db }
}

// WithRegistry adds the provider registry for post-approval account provisioning.
func WithRegistry(r *provider.Registry) RouterOption {
	return func(cfg *routerConfig) { cfg.registry = r }
}

// WithWebhookStore adds webhook delivery for compliance events.
func WithWebhookStore(s webhook.Store) RouterOption {
	return func(cfg *routerConfig) { cfg.webhookStore = s }
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

	kyc := &kycHandler{store: store, webhookStore: cfg.webhookStore}
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
	aml := &amlHandler{store: store, jubeClient: cfg.jubeClient, scamDB: cfg.scamDB, webhookStore: cfg.webhookStore}
	apps := &applicationHandler{store: store, registry: cfg.registry, webhookStore: cfg.webhookStore}

	// Compliance endpoints are restricted to the admin org (built-in).
	// Customer orgs (liquidity, etc.) access the platform via the exchange app,
	// not the compliance admin API. Configurable via COMPLIANCE_ADMIN_ORG env var.
	adminOrg := os.Getenv("COMPLIANCE_ADMIN_ORG")
	if adminOrg == "" {
		adminOrg = "built-in"
	}

	// guard wraps a handler with org-based access control.
	// built-in org = full access, any other org = self-service modules only.
	guard := func(module, action string, h http.HandlerFunc) http.HandlerFunc {
		return requireOrgAccess(adminOrg, module, h)
	}

	r := chi.NewRouter()

	// CORS is handled by the root router — no duplicate handler here.

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

	// Customer-facing self-service endpoints — any authenticated user, any org.
	// liquidity org customers create their own onboarding sessions/applications.
	r.Route("/sessions", func(r chi.Router) {
		r.Get("/", guard("sessions", "read", onboard.handleListSessions))
		r.Post("/", guard("sessions", "write", onboard.handleCreateSession))
		r.Get("/{id}", guard("sessions", "read", onboard.handleGetSession))
		r.Patch("/{id}", guard("sessions", "write", onboard.handleUpdateSession))
		r.Get("/{id}/steps", guard("sessions", "read", onboard.handleGetSessionSteps))
	})

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
		r.Get("/{id}/documents", guard("applications", "read", apps.handleGetDocuments))
	})

	// Wallet address screening — any authenticated user, any org.
	r.Post("/wallet-screen", rateLimitSensitive(guard("kyc", "read", aml.handleWalletScreen)))

	// Admin compliance routes — restricted to built-in org (superadmin).
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireOrg(adminOrg))

		// KYC (admin review)
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

		// Application review (admin-only — self-service CRUD is above)
		r.Post("/applications/{id}/review", guard("applications", "write", apps.handleReview))

		// Pipelines
		r.Route("/pipelines", func(r chi.Router) {
			r.Get("/", guard("pipelines", "read", onboard.handleListPipelines))
			r.Post("/", guard("pipelines", "write", onboard.handleCreatePipeline))
			r.Get("/{id}", guard("pipelines", "read", onboard.handleGetPipeline))
			r.Patch("/{id}", guard("pipelines", "write", onboard.handleUpdatePipeline))
			r.Delete("/{id}", guard("pipelines", "delete", onboard.handleDeletePipeline))
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

		// Roles — all role management restricted to built-in org via RequireOrg above.
		r.Route("/roles", func(r chi.Router) {
			r.Get("/", guard("roles", "read", roles.handleListRoles))
			r.Post("/", guard("roles", "write", roles.handleCreateRole))
			r.Get("/{id}", guard("roles", "read", roles.handleGetRole))
			r.Patch("/{id}", guard("roles", "write", roles.handleUpdateRole))
			r.Delete("/{id}", guard("roles", "delete", roles.handleDeleteRole))
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

// selfServiceModules are modules accessible to any authenticated user regardless
// of org. Users in non-admin orgs can only access these modules.
var selfServiceModules = map[string]bool{
	"sessions": true, "applications": true, "kyc": true,
}

// requireOrgAccess checks X-Org-Id to determine access level.
// If the org matches adminOrg (typically "built-in"), full access is granted.
// Any other org only gets access to self-service modules.
func requireOrgAccess(adminOrg, module string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org := r.Header.Get("X-Org-Id")

		// Admin org has full access to all modules.
		if org == adminOrg {
			next(w, r)
			return
		}

		// Non-admin orgs can only access self-service modules.
		if selfServiceModules[module] {
			next(w, r)
			return
		}

		writeError(w, http.StatusForbidden, "access restricted to "+adminOrg+" org")
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
