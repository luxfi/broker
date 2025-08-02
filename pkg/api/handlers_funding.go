package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/luxfi/broker/pkg/funding"
)

// handleDeposit processes a deposit into a trading account via a payment processor.
func (s *Server) handleDeposit(w http.ResponseWriter, r *http.Request) {
	var req funding.DepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.Currency == "" {
		writeError(w, http.StatusBadRequest, "currency required")
		return
	}

	result, err := s.funding.Deposit(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// handleWithdraw processes a withdrawal from a trading account via a payment processor.
func (s *Server) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	var req funding.WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	if req.Currency == "" {
		writeError(w, http.StatusBadRequest, "currency required")
		return
	}

	result, err := s.funding.Withdraw(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// handleFundingWebhook validates and processes incoming payment processor webhooks.
func (s *Server) handleFundingWebhook(w http.ResponseWriter, r *http.Request) {
	processorName := chi.URLParam(r, "processor")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	signature := r.Header.Get("X-Webhook-Signature")
	if signature == "" {
		// Braintree uses bt_signature form field
		signature = r.FormValue("bt_signature")
	}

	event, err := s.funding.ValidateWebhook(r.Context(), processorName, body, signature)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, event)
}

// handleListProcessors returns available payment processors.
func (s *Server) handleListProcessors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"processors": s.funding.ListProcessors(r.Context()),
	})
}
