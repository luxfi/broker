package compliance

import "net/http"

// transactionsHandler holds transaction HTTP handler state.
type transactionsHandler struct {
	store ComplianceStore
}

func (h *transactionsHandler) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListTransactions())
}
