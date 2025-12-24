package compliance

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// esignHandler holds eSign HTTP handler state.
type esignHandler struct {
	store ComplianceStore
}

func (h *esignHandler) handleListEnvelopes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListEnvelopes())
}

func (h *esignHandler) handleCreateEnvelope(w http.ResponseWriter, r *http.Request) {
	var env Envelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if env.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	if len(env.Signers) == 0 {
		writeError(w, http.StatusBadRequest, "at least one signer is required")
		return
	}
	env.Status = EnvelopePending
	for i := range env.Signers {
		if env.Signers[i].ID == "" {
			env.Signers[i].ID = generateID()
		}
		env.Signers[i].Status = "pending"
	}
	if err := h.store.SaveEnvelope(&env); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &env)
}

func (h *esignHandler) handleGetEnvelope(w http.ResponseWriter, r *http.Request) {
	env, err := h.store.GetEnvelope(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (h *esignHandler) handleSign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	env, err := h.store.GetEnvelope(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		SignerID string `json:"signer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SignerID == "" {
		writeError(w, http.StatusBadRequest, "signer_id is required")
		return
	}

	found := false
	allSigned := true
	for i := range env.Signers {
		if env.Signers[i].ID == req.SignerID {
			env.Signers[i].Status = "signed"
			env.Signers[i].SignedAt = time.Now().UTC().Format(time.RFC3339)
			found = true
		}
		if env.Signers[i].Status != "signed" {
			allSigned = false
		}
	}
	if !found {
		writeError(w, http.StatusBadRequest, "signer not found in envelope")
		return
	}

	if allSigned {
		env.Status = EnvelopeCompleted
	} else {
		env.Status = EnvelopeSigned
	}

	if err := h.store.SaveEnvelope(env); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, env)
}

// --- Templates ---

func (h *esignHandler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListTemplates())
}

func (h *esignHandler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var t Template
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if t.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.store.SaveTemplate(&t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &t)
}
