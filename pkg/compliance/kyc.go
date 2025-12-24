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
