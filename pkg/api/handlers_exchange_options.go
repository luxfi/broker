package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/luxfi/broker/pkg/audit"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// resolveOptionsAccount finds the first user account whose provider supports options.
// Returns the resolved provider, account ID, and provider name.
func (s *Server) resolveOptionsAccount(r *http.Request) (provider.OptionsProvider, string, string, error) {
	accounts, err := s.resolveUserAccounts(r)
	if err != nil {
		return nil, "", "", err
	}

	for _, acct := range accounts {
		p, err := s.registry.Get(acct.provider)
		if err != nil {
			continue
		}
		op, ok := p.(provider.OptionsProvider)
		if ok {
			return op, acct.accountID, acct.provider, nil
		}
	}

	return nil, "", "", &apiError{
		Status:  http.StatusBadRequest,
		Message: "no provider with options support found for user",
	}
}

// handleExchangeOptionChain returns the option chain for a symbol.
// GET /v1/exchange/options/chain/{symbol}
func (s *Server) handleExchangeOptionChain(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if !validateUnderlyingSymbol(symbol) {
		writeError(w, http.StatusBadRequest, "invalid symbol: must be 1-6 uppercase letters")
		return
	}

	op, _, providerName, err := s.resolveOptionsAccount(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	expiration := r.URL.Query().Get("expiration")
	if expiration != "" {
		if _, ok := validateExpiration(expiration); !ok {
			writeError(w, http.StatusBadRequest, "invalid expiration: must be YYYY-MM-DD format")
			return
		}
	}

	chain, err := op.GetOptionChain(r.Context(), strings.ToUpper(symbol), expiration)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:   audit.ActionOptionChainQuery,
		Provider: providerName,
		Symbol:   strings.ToUpper(symbol),
		Metadata: map[string]interface{}{"expiration": expiration},
	})

	writeJSON(w, http.StatusOK, chain)
}

// handleExchangeOptionExpirations returns available expiration dates.
// GET /v1/exchange/options/expirations/{symbol}
func (s *Server) handleExchangeOptionExpirations(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if !validateUnderlyingSymbol(symbol) {
		writeError(w, http.StatusBadRequest, "invalid symbol: must be 1-6 uppercase letters")
		return
	}

	op, _, _, err := s.resolveOptionsAccount(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	expirations, err := op.GetOptionExpirations(r.Context(), strings.ToUpper(symbol))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"symbol":      strings.ToUpper(symbol),
		"expirations": expirations,
	})
}

// handleExchangeOptionPositions returns option positions across the user's accounts.
// GET /v1/exchange/options/positions
func (s *Server) handleExchangeOptionPositions(w http.ResponseWriter, r *http.Request) {
	op, accountID, _, err := s.resolveOptionsAccount(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	positions, err := op.GetOptionPositions(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if positions == nil {
		positions = []*types.OptionPosition{}
	}

	writeJSON(w, http.StatusOK, positions)
}

// handleExchangeCreateOptionOrder places a single-leg option order.
// POST /v1/exchange/options/orders
func (s *Server) handleExchangeCreateOptionOrder(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	op, accountID, providerName, err := s.resolveOptionsAccount(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req types.CreateOptionOrderRequest
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate action
	action := strings.ToLower(req.Action)
	if !validOptionActions[action] {
		writeError(w, http.StatusBadRequest, "action must be one of: buy_to_open, buy_to_close, sell_to_open, sell_to_close")
		return
	}
	req.Action = action

	// Validate quantity
	if !validateQty(req.Qty) {
		writeError(w, http.StatusBadRequest, "qty must be a positive integer no greater than "+strconv.Itoa(maxOptionOrderQty))
		return
	}

	// Validate order type
	if req.OrderType == "" {
		req.OrderType = "limit"
	}
	if !validOrderTypes[strings.ToLower(req.OrderType)] {
		writeError(w, http.StatusBadRequest, "order_type must be one of: market, limit, stop, stop_limit")
		return
	}
	req.OrderType = strings.ToLower(req.OrderType)

	// Validate time in force
	if req.TimeInForce == "" {
		req.TimeInForce = "day"
	}
	if !validTimeInForce[strings.ToLower(req.TimeInForce)] {
		writeError(w, http.StatusBadRequest, "time_in_force must be one of: day, gtc, ioc, fok, opg, cls")
		return
	}
	req.TimeInForce = strings.ToLower(req.TimeInForce)

	// Validate contract identification
	if req.ContractSymbol != "" {
		if !validateOCCSymbol(req.ContractSymbol) {
			writeError(w, http.StatusBadRequest, "invalid contract_symbol: must match OCC format (e.g. AAPL260418C00150000)")
			return
		}
	} else {
		if req.Symbol == "" {
			writeError(w, http.StatusBadRequest, "symbol or contract_symbol is required")
			return
		}
		if !validateUnderlyingSymbol(req.Symbol) {
			writeError(w, http.StatusBadRequest, "invalid symbol: must be 1-6 uppercase letters")
			return
		}
		if !validContractTypes[strings.ToLower(req.ContractType)] {
			writeError(w, http.StatusBadRequest, "contract_type must be 'call' or 'put'")
			return
		}
		if !validateStrike(req.Strike) {
			writeError(w, http.StatusBadRequest, "strike must be a positive number")
			return
		}
		if !validateExpirationNotPast(req.Expiration) {
			writeError(w, http.StatusBadRequest, "expiration must be a valid future date in YYYY-MM-DD format")
			return
		}
	}

	// Validate prices
	if req.OrderType == "limit" || req.OrderType == "stop_limit" {
		if !validatePrice(req.LimitPrice) || req.LimitPrice == "" {
			writeError(w, http.StatusBadRequest, "limit_price is required and must be non-negative for limit/stop_limit orders")
			return
		}
	}
	if req.OrderType == "stop" || req.OrderType == "stop_limit" {
		if !validatePrice(req.StopPrice) || req.StopPrice == "" {
			writeError(w, http.StatusBadRequest, "stop_price is required and must be non-negative for stop/stop_limit orders")
			return
		}
	}

	order, err := op.CreateOptionOrder(r.Context(), accountID, &req)

	status := "success"
	var errMsg error
	if err != nil {
		status = "failed"
		errMsg = err
	}
	symbol := req.ContractSymbol
	if symbol == "" {
		symbol = req.Symbol
	}
	s.auditLog.RecordOrder(r.Context(), audit.ActionOptionOrderCreate, providerName, accountID, symbol, req.Action, req.Qty, "", "", status, time.Since(start), errMsg)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, order)
}

// handleExchangeCreateMultiLegOrder places a multi-leg options strategy order.
// POST /v1/exchange/options/multi-leg
func (s *Server) handleExchangeCreateMultiLegOrder(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	op, accountID, providerName, err := s.resolveOptionsAccount(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req types.CreateMultiLegOrderRequest
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate leg count
	if len(req.Legs) < 2 {
		writeError(w, http.StatusBadRequest, "multi-leg order requires at least 2 legs")
		return
	}
	if len(req.Legs) > maxMultiLegLegs {
		writeError(w, http.StatusBadRequest, "multi-leg order supports at most "+strconv.Itoa(maxMultiLegLegs)+" legs")
		return
	}

	// Validate underlying symbol
	if req.Symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}
	if !validateUnderlyingSymbol(req.Symbol) {
		writeError(w, http.StatusBadRequest, "invalid symbol: must be 1-6 uppercase letters")
		return
	}

	// Validate order type
	if req.OrderType == "" {
		req.OrderType = "limit"
	}
	if !validOrderTypes[strings.ToLower(req.OrderType)] {
		writeError(w, http.StatusBadRequest, "order_type must be one of: market, limit, stop, stop_limit")
		return
	}
	req.OrderType = strings.ToLower(req.OrderType)

	// Validate time in force
	if req.TimeInForce == "" {
		req.TimeInForce = "day"
	}
	if !validTimeInForce[strings.ToLower(req.TimeInForce)] {
		writeError(w, http.StatusBadRequest, "time_in_force must be one of: day, gtc, ioc, fok, opg, cls")
		return
	}
	req.TimeInForce = strings.ToLower(req.TimeInForce)

	// Validate each leg
	for i, leg := range req.Legs {
		prefix := "legs[" + strconv.Itoa(i) + "]"

		if leg.ContractSymbol != "" {
			if !validateOCCSymbol(leg.ContractSymbol) {
				writeError(w, http.StatusBadRequest, prefix+".contract_symbol: invalid OCC format")
				return
			}
		} else {
			if !validContractTypes[strings.ToLower(leg.ContractType)] {
				writeError(w, http.StatusBadRequest, prefix+".contract_type must be 'call' or 'put'")
				return
			}
			if !validateStrike(leg.Strike) {
				writeError(w, http.StatusBadRequest, prefix+".strike must be a positive number")
				return
			}
			if !validateExpirationNotPast(leg.Expiration) {
				writeError(w, http.StatusBadRequest, prefix+".expiration must be a valid future date in YYYY-MM-DD format")
				return
			}
		}

		action := strings.ToLower(leg.Action)
		if !validOptionActions[action] {
			writeError(w, http.StatusBadRequest, prefix+".action must be one of: buy_to_open, buy_to_close, sell_to_open, sell_to_close")
			return
		}
		req.Legs[i].Action = action

		if !validateQty(leg.Quantity) {
			writeError(w, http.StatusBadRequest, prefix+".qty must be a positive integer no greater than "+strconv.Itoa(maxOptionOrderQty))
			return
		}
	}

	// Validate limit price for limit orders
	if req.OrderType == "limit" || req.OrderType == "stop_limit" {
		if !validatePrice(req.LimitPrice) || req.LimitPrice == "" {
			writeError(w, http.StatusBadRequest, "limit_price is required for limit/stop_limit orders")
			return
		}
	}

	result, err := op.CreateMultiLegOrder(r.Context(), accountID, &req)

	status := "success"
	var errMsg error
	orderID := ""
	if err != nil {
		status = "failed"
		errMsg = err
	} else if result != nil {
		orderID = result.StrategyOrderID
	}
	s.auditLog.RecordOrder(r.Context(), audit.ActionOptionMultiLeg, providerName, accountID, req.Symbol, req.StrategyType, strconv.Itoa(len(req.Legs))+" legs", orderID, "", status, time.Since(start), errMsg)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// handleExchangeCancelOptionOrder cancels an option order by ID.
// POST /v1/exchange/options/orders/{id}/cancel
func (s *Server) handleExchangeCancelOptionOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeError(w, http.StatusBadRequest, "order id is required")
		return
	}

	accounts, err := s.resolveUserAccounts(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Cancel uses the base Provider interface (options orders are orders).
	for _, acct := range accounts {
		p, err := s.registry.Get(acct.provider)
		if err != nil {
			continue
		}
		// Check if this provider supports options
		if _, ok := p.(provider.OptionsProvider); !ok {
			continue
		}
		if err := p.CancelOrder(r.Context(), acct.accountID, orderID); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}

		s.auditLog.Record(audit.Entry{
			Action:    audit.ActionOrderCancel,
			Provider:  acct.provider,
			AccountID: acct.accountID,
			OrderID:   orderID,
			Status:    "success",
			Metadata:  map[string]interface{}{"source": "exchange_options"},
		})

		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "order_id": orderID})
		return
	}

	writeError(w, http.StatusBadRequest, "no provider with options support found for user")
}

// handleExchangeExerciseOption exercises an option contract.
// POST /v1/exchange/options/exercise/{id}
func (s *Server) handleExchangeExerciseOption(w http.ResponseWriter, r *http.Request) {
	contractSymbol := chi.URLParam(r, "id")
	if contractSymbol == "" {
		writeError(w, http.StatusBadRequest, "contract symbol is required")
		return
	}
	if !validateOCCSymbol(contractSymbol) {
		writeError(w, http.StatusBadRequest, "invalid contract symbol: must match OCC format (e.g. AAPL260418C00150000)")
		return
	}

	start := time.Now()

	op, accountID, providerName, err := s.resolveOptionsAccount(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req struct {
		Qty int `json:"qty"`
	}
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Qty <= 0 || req.Qty > maxOptionOrderQty {
		writeError(w, http.StatusBadRequest, "qty must be a positive integer no greater than "+strconv.Itoa(maxOptionOrderQty))
		return
	}

	err = op.ExerciseOption(r.Context(), accountID, contractSymbol, req.Qty)

	status := "success"
	var errMsg error
	if err != nil {
		status = "failed"
		errMsg = err
	}
	s.auditLog.RecordOrder(r.Context(), audit.ActionOptionExercise, providerName, accountID, contractSymbol, "exercise", strconv.Itoa(req.Qty), "", "", status, time.Since(start), errMsg)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "exercised"})
}
