package compliance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
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

	// Hash the full key with bcrypt for later validation.
	hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash key")
		return
	}

	// Save a credential record with prefix + bcrypt hash.
	cred := &Credential{
		Name:        req.Name,
		KeyPrefix:   fullKey[:8],
		KeyHash:     string(hash),
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

// ValidateCredential checks a raw API key against all stored credentials.
// Returns the matching credential if found, nil otherwise.
func ValidateCredential(store ComplianceStore, rawKey string) *Credential {
	if len(rawKey) < 8 {
		return nil
	}
	prefix := rawKey[:8]
	for _, cred := range store.ListCredentials() {
		if cred.KeyPrefix != prefix {
			continue
		}
		if cred.KeyHash == "" {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(cred.KeyHash), []byte(rawKey)) == nil {
			return cred
		}
	}
	return nil
}

func (h *credentialsHandler) handleDeleteCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteCredential(id); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
