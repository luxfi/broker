package compliance

import "net/http"

// dashboardHandler holds dashboard HTTP handler state.
type dashboardHandler struct {
	store ComplianceStore
}

func (h *dashboardHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ComputeDashboard())
}

func (h *dashboardHandler) handleESignDashboard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ComputeESignStats())
}
