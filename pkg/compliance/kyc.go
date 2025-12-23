package compliance

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// KYCProvider is the interface for pluggable identity verification backends
// (Onfido, Berbix, IDMerit, etc.).
type KYCProvider interface {
	VerifyIdentity(ctx context.Context, userID string, docs []Document) (*Identity, error)
	GetVerificationStatus(ctx context.Context, verificationID string) (*Identity, error)
}

// kycHandler holds KYC HTTP handler state.
type kycHandler struct {
	store    ComplianceStore
	provider KYCProvider // nil if no external provider configured
}

func (h *kycHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    string     `json:"user_id"`
		Provider  string     `json:"provider"`
		Documents []Document `json:"documents,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	// If an external KYC provider is configured, delegate to it.
	if h.provider != nil {
		ident, err := h.provider.VerifyIdentity(r.Context(), req.UserID, req.Documents)
		if err != nil {
			log.Error().Err(err).Str("user_id", req.UserID).Msg("KYC provider verification failed")
			writeError(w, http.StatusBadGateway, "identity verification failed")
			return
		}
		if err := h.store.SaveIdentity(ident); err != nil {
			log.Error().Err(err).Str("user_id", req.UserID).Msg("failed to save identity")
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusCreated, ident)
		return
	}

	// No external provider: create a pending identity record.
	ident := &Identity{
		UserID:   req.UserID,
		Provider: req.Provider,
		Status:   KYCPending,
		Data:     make(map[string]interface{}),
	}
	if err := h.store.SaveIdentity(ident); err != nil {
		log.Error().Err(err).Str("user_id", req.UserID).Msg("failed to save identity")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, ident)
}

func (h *kycHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// If an external provider is configured, check it first.
	if h.provider != nil {
		ident, err := h.provider.GetVerificationStatus(r.Context(), id)
		if err == nil {
			// Update local store with latest status.
			_ = h.store.SaveIdentity(ident)
			writeJSON(w, http.StatusOK, ident)
			return
		}
		// Fall through to local store lookup.
	}

	ident, err := h.store.GetIdentity(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, ident)
}

// handleListByUser returns all KYC identity records for a given user.
func (h *kycHandler) handleListByUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id query parameter is required")
		return
	}
	writeJSON(w, http.StatusOK, h.store.ListIdentitiesByUser(userID))
}

// handleUpdateStatus allows updating a KYC identity's status (for admin review).
func (h *kycHandler) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ident, err := h.store.GetIdentity(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var req struct {
		Status KYCStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}
	if req.Status != KYCPending && req.Status != KYCVerified && req.Status != KYCFailed && req.Status != KYCExpired {
		writeError(w, http.StatusBadRequest, "invalid status value")
		return
	}

	ident.Status = req.Status
	if err := h.store.SaveIdentity(ident); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, ident)
}

// handleUploadDocument saves a document metadata record linked to a KYC identity.
func (h *kycHandler) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ident, err := h.store.GetIdentity(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "identity not found")
		return
	}

	var req struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		MimeType string `json:"mime_type"`
		Content  string `json:"content,omitempty"` // base64
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	doc := Document{
		ID:       generateID(),
		Type:     req.Type,
		Name:     req.Name,
		MimeType: req.MimeType,
		Content:  req.Content,
		Status:   "pending",
	}

	// Store document reference in identity data.
	if ident.Data == nil {
		ident.Data = make(map[string]interface{})
	}
	var docs []interface{}
	if existing, ok := ident.Data["documents"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			docs = arr
		}
	}
	docs = append(docs, map[string]interface{}{
		"id":        doc.ID,
		"type":      doc.Type,
		"name":      doc.Name,
		"mime_type": doc.MimeType,
		"status":    doc.Status,
	})
	ident.Data["documents"] = docs

	if err := h.store.SaveIdentity(ident); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, doc)
}
