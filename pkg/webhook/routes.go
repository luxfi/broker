package webhook

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter creates a chi sub-router for webhook management.
// Mount at /v1/bd/webhooks on the main router.
//
//	POST   /              — register webhook
//	GET    /              — list webhooks
//	DELETE /{id}          — delete webhook
//	GET    /{id}/deliveries — list delivery history
func NewRouter(store Store) chi.Router {
	h := &handler{store: store}
	r := chi.NewRouter()

	r.Post("/", h.handleCreate)
	r.Get("/", h.handleList)
	r.Delete("/{id}", h.handleDelete)
	r.Get("/{id}/deliveries", h.handleListDeliveries)

	return r
}

type handler struct {
	store Store
}

func (h *handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-Id")
	if orgID == "" {
		writeErr(w, http.StatusUnauthorized, "org identity required")
		return
	}

	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		writeErr(w, http.StatusBadRequest, "url is required")
		return
	}
	if len(req.Events) == 0 {
		writeErr(w, http.StatusBadRequest, "events is required")
		return
	}

	secret := generateSecret()
	wh := &Webhook{
		OrgID:  orgID,
		URL:    req.URL,
		Secret: secret,
		Events: req.Events,
		Active: true,
	}
	if err := h.store.Save(wh); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return secret only on creation so the caller can store it.
	resp := map[string]interface{}{
		"id":         wh.ID,
		"url":        wh.URL,
		"events":     wh.Events,
		"active":     wh.Active,
		"secret":     secret,
		"created_at": wh.CreatedAt,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *handler) handleList(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-Id")
	if orgID == "" {
		writeErr(w, http.StatusUnauthorized, "org identity required")
		return
	}

	hooks, err := h.store.List(orgID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if hooks == nil {
		hooks = []Webhook{}
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (h *handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-Id")
	if orgID == "" {
		writeErr(w, http.StatusUnauthorized, "org identity required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.store.Delete(orgID, id); err != nil {
		writeErr(w, http.StatusNotFound, "webhook not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *handler) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-Id")
	if orgID == "" {
		writeErr(w, http.StatusUnauthorized, "org identity required")
		return
	}

	id := chi.URLParam(r, "id")
	// Verify the webhook belongs to this org.
	if _, err := h.store.GetByID(orgID, id); err != nil {
		writeErr(w, http.StatusNotFound, "webhook not found")
		return
	}

	deliveries, err := h.store.ListDeliveries(id, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if deliveries == nil {
		deliveries = []Delivery{}
	}
	writeJSON(w, http.StatusOK, deliveries)
}

func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
