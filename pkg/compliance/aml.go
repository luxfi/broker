package compliance

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luxfi/broker/pkg/webhook"
	"github.com/rs/zerolog/log"
)

// amlHandler holds AML screening HTTP handler state.
type amlHandler struct {
	store        ComplianceStore
	scamDB       *ScamDB
	webhookStore webhook.Store
}

// handleScreen creates an AML sanctions screening record for an account.
// The record is queued as pending for manual review; wallet screening below
// covers automated OFAC/ScamSniffer checks.
func (h *amlHandler) handleScreen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID string `json:"account_id"`
		UserID    string `json:"user_id"`
		Name      string `json:"name"`
		Country   string `json:"country,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	screening := &AMLScreening{
		AccountID: req.AccountID,
		UserID:    req.UserID,
		Type:      "sanctions",
		Status:    AMLPending,
		RiskLevel: RiskLow,
		Provider:  "manual",
	}

	if err := h.store.SaveAMLScreening(screening); err != nil {
		log.Error().Err(err).Str("account", req.AccountID).Msg("aml: failed to save screening")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	orgID := r.Header.Get("X-Org-Id")
	switch screening.Status {
	case AMLFlagged:
		webhook.Deliver(h.webhookStore, orgID, "aml.flagged", map[string]interface{}{
			"screening_id": screening.ID,
			"account_id":   req.AccountID,
			"risk_level":   string(screening.RiskLevel),
			"risk_score":   screening.RiskScore,
		})
	case AMLCleared:
		webhook.Deliver(h.webhookStore, orgID, "aml.cleared", map[string]interface{}{
			"screening_id": screening.ID,
			"account_id":   req.AccountID,
		})
	}

	writeJSON(w, http.StatusCreated, screening)
}

// handleGet returns a single AML screening by ID.
func (h *amlHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	screening, err := h.store.GetAMLScreening(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, screening)
}

// handleListByAccount returns all AML screenings for a given account.
func (h *amlHandler) handleListByAccount(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "account_id query parameter is required")
		return
	}
	writeJSON(w, http.StatusOK, h.store.ListAMLScreeningsByAccount(accountID))
}

// handleListFlagged returns all AML screenings that are flagged and need review.
func (h *amlHandler) handleListFlagged(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListAMLScreeningsByStatus(AMLFlagged))
}

// handleReview marks a flagged AML screening as cleared or blocked after manual review.
func (h *amlHandler) handleReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.GetAMLScreening(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Extract reviewer identity from JWT context — never trust the request body.
	// This prevents audit trail forgery (CRITICAL-2).
	reviewer := r.Header.Get("X-User-Id")
	if reviewer == "" {
		writeError(w, http.StatusUnauthorized, "reviewer identity not found in token")
		return
	}

	var req struct {
		Decision string `json:"decision"` // cleared, blocked
		Details  string `json:"details,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "cleared" && req.Decision != "blocked" {
		writeError(w, http.StatusBadRequest, "decision must be 'cleared' or 'blocked'")
		return
	}

	switch req.Decision {
	case "cleared":
		existing.Status = AMLCleared
	case "blocked":
		existing.Status = AMLBlocked
	}
	existing.ReviewedBy = reviewer
	existing.ReviewedAt = time.Now().UTC()
	if req.Details != "" {
		existing.Details = req.Details
	}

	if err := h.store.SaveAMLScreening(existing); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	orgID := r.Header.Get("X-Org-Id")
	switch existing.Status {
	case AMLCleared:
		webhook.Deliver(h.webhookStore, orgID, "aml.cleared", map[string]interface{}{
			"screening_id": existing.ID,
			"account_id":   existing.AccountID,
			"reviewed_by":  reviewer,
		})
	case AMLBlocked:
		webhook.Deliver(h.webhookStore, orgID, "aml.flagged", map[string]interface{}{
			"screening_id": existing.ID,
			"account_id":   existing.AccountID,
			"reviewed_by":  reviewer,
			"decision":     "blocked",
		})
	}

	writeJSON(w, http.StatusOK, existing)
}

// walletScreenRequest is the input for wallet address screening.
type walletScreenRequest struct {
	Address   string `json:"address"`
	Chain     string `json:"chain"`     // ethereum, liquidity, bitcoin
	Direction string `json:"direction"` // send, receive
}

// walletScreenResponse is the result of a wallet address screen.
type walletScreenResponse struct {
	Address    string `json:"address"`
	Risk       string `json:"risk"` // low, medium, high, blocked
	Sanctioned bool   `json:"sanctioned"`
	Scam       bool   `json:"scam"`
	Source     string `json:"source"` // ofac, scamsniffer
}

// handleWalletScreen checks a crypto wallet address against the OFAC SDN list
// and the ScamSniffer scam-address database.
func (h *amlHandler) handleWalletScreen(w http.ResponseWriter, r *http.Request) {
	var req walletScreenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Address == "" {
		writeError(w, http.StatusBadRequest, "address is required")
		return
	}
	if req.Chain == "" {
		writeError(w, http.StatusBadRequest, "chain is required")
		return
	}
	switch req.Chain {
	case "ethereum", "liquidity", "bitcoin":
	default:
		writeError(w, http.StatusBadRequest, "chain must be ethereum, liquidity, or bitcoin")
		return
	}
	if req.Direction == "" {
		req.Direction = "receive"
	}
	if req.Direction != "send" && req.Direction != "receive" {
		writeError(w, http.StatusBadRequest, "direction must be send or receive")
		return
	}

	resp := walletScreenResponse{
		Address: req.Address,
		Risk:    "low",
	}

	// 1. Check OFAC SDN list (instant, local).
	if source, hit := isOFACSanctioned(req.Address); hit {
		resp.Risk = "blocked"
		resp.Sanctioned = true
		resp.Source = "ofac"

		log.Warn().
			Str("address", req.Address).
			Str("chain", req.Chain).
			Str("source", source).
			Msg("aml: OFAC sanctioned wallet detected")

		writeJSON(w, http.StatusOK, resp)
		return
	}

	// 2. Check ScamSniffer database (instant, local).
	if h.scamDB != nil {
		if isScam, source := h.scamDB.Check(req.Address); isScam {
			resp.Risk = "high"
			resp.Scam = true
			resp.Source = source

			log.Warn().
				Str("address", req.Address).
				Str("chain", req.Chain).
				Msg("aml: scam wallet detected via ScamSniffer")

			writeJSON(w, http.StatusOK, resp)
			return
		}
	}

	// Default source when neither OFAC nor ScamSniffer flagged the address.
	if resp.Source == "" {
		resp.Source = "ofac"
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleRiskAssessment records a transaction risk assessment for manual review.
func (h *amlHandler) handleRiskAssessment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccountID string  `json:"account_id"`
		UserID    string  `json:"user_id"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		Type      string  `json:"type"` // deposit, withdrawal, trade
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "account_id is required")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}

	screening := &AMLScreening{
		AccountID: req.AccountID,
		UserID:    req.UserID,
		Type:      "transaction",
		Status:    AMLPending,
		RiskLevel: RiskLow,
		Provider:  "manual",
	}

	if err := h.store.SaveAMLScreening(screening); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	orgID := r.Header.Get("X-Org-Id")
	if screening.Status == AMLFlagged || screening.Status == AMLBlocked {
		webhook.Deliver(h.webhookStore, orgID, "compliance.alert", map[string]interface{}{
			"screening_id": screening.ID,
			"account_id":   req.AccountID,
			"type":         req.Type,
			"risk_level":   string(screening.RiskLevel),
			"risk_score":   screening.RiskScore,
			"amount":       req.Amount,
		})
	}

	writeJSON(w, http.StatusCreated, screening)
}
