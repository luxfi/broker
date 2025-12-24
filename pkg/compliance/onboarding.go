package compliance

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// onboardingHandler holds pipeline/session HTTP handler state.
type onboardingHandler struct {
	store ComplianceStore
}

// --- Pipeline Handlers ---

func (h *onboardingHandler) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListPipelines())
}

func (h *onboardingHandler) handleCreatePipeline(w http.ResponseWriter, r *http.Request) {
	var p Pipeline
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if p.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	if err := h.store.SavePipeline(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &p)
}

func (h *onboardingHandler) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetPipeline(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *onboardingHandler) handleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.GetPipeline(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var patch struct {
		Name       *string        `json:"name,omitempty"`
		Steps      []PipelineStep `json:"steps,omitempty"`
		Status     *string        `json:"status,omitempty"`
		BusinessID *string        `json:"business_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Steps != nil {
		existing.Steps = patch.Steps
	}
	if patch.Status != nil {
		existing.Status = *patch.Status
	}
	if patch.BusinessID != nil {
		existing.BusinessID = *patch.BusinessID
	}
	if err := h.store.SavePipeline(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *onboardingHandler) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeletePipeline(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Session Handlers ---

func (h *onboardingHandler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListSessions())
}

func (h *onboardingHandler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var sess Session
	if err := json.NewDecoder(r.Body).Decode(&sess); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if sess.PipelineID == "" {
		writeError(w, http.StatusBadRequest, "pipeline_id is required")
		return
	}
	if sess.InvestorEmail == "" {
		writeError(w, http.StatusBadRequest, "investor_email is required")
		return
	}

	// Validate pipeline exists and populate session steps from it.
	pipeline, err := h.store.GetPipeline(sess.PipelineID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sess.Status = SessionPending
	sess.KYCStatus = KYCPending
	sess.Steps = make([]SessionStep, len(pipeline.Steps))
	for i, ps := range pipeline.Steps {
		sess.Steps[i] = SessionStep{
			StepID: ps.ID,
			Name:   ps.Name,
			Type:   ps.Type,
			Status: "pending",
		}
	}

	if err := h.store.SaveSession(&sess); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &sess)
}

func (h *onboardingHandler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, err := h.store.GetSession(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (h *onboardingHandler) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var patch struct {
		Status    *SessionStatus `json:"status,omitempty"`
		KYCStatus *KYCStatus     `json:"kyc_status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if patch.Status != nil {
		existing.Status = *patch.Status
	}
	if patch.KYCStatus != nil {
		existing.KYCStatus = *patch.KYCStatus
	}
	if err := h.store.SaveSession(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *onboardingHandler) handleGetSessionSteps(w http.ResponseWriter, r *http.Request) {
	sess, err := h.store.GetSession(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess.Steps)
}
