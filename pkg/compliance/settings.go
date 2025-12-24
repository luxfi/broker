package compliance

import (
	"encoding/json"
	"net/http"
)

// settingsHandler holds settings HTTP handler state.
type settingsHandler struct {
	store ComplianceStore
}

func (h *settingsHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.GetSettings())
}

func (h *settingsHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	existing := h.store.GetSettings()

	var patch struct {
		BusinessName      *string `json:"business_name,omitempty"`
		Timezone          *string `json:"timezone,omitempty"`
		Currency          *string `json:"currency,omitempty"`
		NotificationEmail *string `json:"notification_email,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if patch.BusinessName != nil {
		existing.BusinessName = *patch.BusinessName
	}
	if patch.Timezone != nil {
		existing.Timezone = *patch.Timezone
	}
	if patch.Currency != nil {
		existing.Currency = *patch.Currency
	}
	if patch.NotificationEmail != nil {
		existing.NotificationEmail = *patch.NotificationEmail
	}
	h.store.SaveSettings(existing)
	writeJSON(w, http.StatusOK, existing)
}
