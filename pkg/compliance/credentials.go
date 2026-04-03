package compliance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// credentialsHandler manages API credential records for audit visibility.
type credentialsHandler struct {
	store ComplianceStore
}

func (h *credentialsHandler) handleListCredentials(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListCredentials())
}

func (h *credentialsHandler) handleCreateCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Permissions) == 0 {
		req.Permissions = []string{"read"}
	}

	// Generate a random API key.
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	fullKey := hex.EncodeToString(keyBytes)

	// Save a credential record (with key prefix only) in the compliance store.
	cred := &Credential{
		Name:        req.Name,
		KeyPrefix:   fullKey[:8],
		Permissions: req.Permissions,
	}
	if err := h.store.SaveCredential(cred); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return the full key only on creation. After this, only the prefix is visible.
	resp := struct {
		Credential
		Key string `json:"key"`
	}{
		Credential: *cred,
		Key:        fullKey,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *credentialsHandler) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteCredential(id); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
