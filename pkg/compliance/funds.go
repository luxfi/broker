package compliance

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// fundsHandler holds fund management HTTP handler state.
type fundsHandler struct {
	store ComplianceStore
}

func (h *fundsHandler) handleListFunds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListFunds())
}

func (h *fundsHandler) handleCreateFund(w http.ResponseWriter, r *http.Request) {
	var f Fund
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if f.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if f.Status == "" {
		f.Status = "raising"
	}
	if err := h.store.SaveFund(&f); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &f)
}

func (h *fundsHandler) handleGetFund(w http.ResponseWriter, r *http.Request) {
	f, err := h.store.GetFund(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *fundsHandler) handleUpdateFund(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.GetFund(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var patch struct {
		Name          *string  `json:"name,omitempty"`
		Type          *string  `json:"type,omitempty"`
		MinInvestment *float64 `json:"min_investment,omitempty"`
		Status        *string  `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Type != nil {
		existing.Type = *patch.Type
	}
	if patch.MinInvestment != nil {
		existing.MinInvestment = *patch.MinInvestment
	}
	if patch.Status != nil {
		existing.Status = *patch.Status
	}
	if err := h.store.SaveFund(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *fundsHandler) handleDeleteFund(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteFund(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *fundsHandler) handleListInvestors(w http.ResponseWriter, r *http.Request) {
	fundID := chi.URLParam(r, "id")
	// Verify fund exists.
	if _, err := h.store.GetFund(fundID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	investors := h.store.ListFundInvestors(fundID)
	if investors == nil {
		investors = make([]*FundInvestor, 0)
	}
	writeJSON(w, http.StatusOK, investors)
}
