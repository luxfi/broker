package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/marketdata"
	"github.com/luxfi/broker/pkg/router"
)

// --- TWAP Handlers ---

func (s *Server) handleStartTWAP(w http.ResponseWriter, r *http.Request) {
	if s.twap == nil {
		writeError(w, http.StatusNotImplemented, "TWAP scheduler not configured")
		return
	}
	var req struct {
		Symbol          string  `json:"symbol"`
		Side            string  `json:"side"`
		TotalQty        float64 `json:"total_qty"`
		DurationSeconds int64   `json:"duration_seconds"`
		Slices          int     `json:"slices"`
		MaxSlippageBps  float64 `json:"max_slippage_bps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build accounts map server-side from authenticated user's resolver mappings.
	userID := r.Header.Get("X-User-Id")
	accounts := s.resolver.UserAccounts(userID)
	if len(accounts) == 0 {
		writeError(w, http.StatusBadRequest, "no trading accounts configured")
		return
	}

	exec, err := s.twap.Start(r.Context(), router.TWAPConfig{
		Symbol:      req.Symbol,
		Side:        req.Side,
		TotalQty:    req.TotalQty,
		Duration:    time.Duration(req.DurationSeconds) * time.Second,
		Slices:      req.Slices,
		MaxSlippage: req.MaxSlippageBps,
	}, accounts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, exec)
}

func (s *Server) handleGetTWAP(w http.ResponseWriter, r *http.Request) {
	if s.twap == nil {
		writeError(w, http.StatusNotImplemented, "TWAP scheduler not configured")
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		// List all
		writeJSON(w, http.StatusOK, s.twap.List())
		return
	}
	exec, err := s.twap.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, exec)
}

func (s *Server) handleCancelTWAP(w http.ResponseWriter, r *http.Request) {
	if s.twap == nil {
		writeError(w, http.StatusNotImplemented, "TWAP scheduler not configured")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.twap.Cancel(req.ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// --- Arbitrage Handlers ---

func (s *Server) handleScanArbitrage(w http.ResponseWriter, r *http.Request) {
	if s.arbDetector == nil {
		writeError(w, http.StatusNotImplemented, "arbitrage detector not configured")
		return
	}
	symbol := r.URL.Query().Get("symbol")
	var opps []*marketdata.ArbitrageOpportunity
	if symbol != "" {
		opps = s.arbDetector.CheckSymbol(symbol)
	} else {
		opps = s.arbDetector.Scan()
	}
	if opps == nil {
		opps = []*marketdata.ArbitrageOpportunity{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"opportunities": opps,
		"count":         len(opps),
		"threshold_bps": s.arbDetectorThreshold(),
	})
}

func (s *Server) arbDetectorThreshold() float64 {
	if s.arbDetector == nil {
		return 0
	}
	if v := s.arbDetectorThresholdBps; v > 0 {
		return v
	}
	return 5 // default
}

