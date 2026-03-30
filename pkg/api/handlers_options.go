package api

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/luxfi/broker/pkg/audit"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// --- Input validation ---

// occSymbolRe validates OCC option symbol format: ROOT(1-6) + DATE(6) + TYPE(1) + STRIKE(8).
// Examples: AAPL260418C00150000, SPY260620P00500000, X261231C00001000
var occSymbolRe = regexp.MustCompile(`^[A-Z]{1,6}\d{6}[CP]\d{8}$`)

// underlyingSymbolRe validates underlying equity symbols: 1-6 uppercase letters.
var underlyingSymbolRe = regexp.MustCompile(`^[A-Z]{1,6}$`)

// expirationRe validates YYYY-MM-DD date format.
var expirationRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// maxMultiLegLegs caps multi-leg orders. Alpaca supports up to 4 legs (Level 3).
// Iron condor is 4 legs; anything beyond that is invalid.
const maxMultiLegLegs = 4

// maxOptionOrderQty prevents absurd quantity values that indicate API abuse.
const maxOptionOrderQty = 10000

// maxRequestBodyBytes limits request body size for options endpoints (64 KB).
const maxRequestBodyBytes = 64 * 1024

// validOptionActions is the allowlist for option order actions.
var validOptionActions = map[string]bool{
	"buy_to_open":  true,
	"buy_to_close": true,
	"sell_to_open":  true,
	"sell_to_close": true,
}

// validOrderTypes is the allowlist for order types.
var validOrderTypes = map[string]bool{
	"market":     true,
	"limit":      true,
	"stop":       true,
	"stop_limit": true,
}

// validTimeInForce is the allowlist for time-in-force values.
var validTimeInForce = map[string]bool{
	"day": true,
	"gtc": true,
	"ioc": true,
	"fok": true,
	"opg": true,
	"cls": true,
}

// validContractTypes is the allowlist for option contract types.
var validContractTypes = map[string]bool{
	"call": true,
	"put":  true,
}

// validSides is the allowlist for routing side parameter.
var validSides = map[string]bool{
	"buy":  true,
	"sell": true,
}

func validateUnderlyingSymbol(symbol string) bool {
	return underlyingSymbolRe.MatchString(strings.ToUpper(symbol))
}

func validateOCCSymbol(symbol string) bool {
	return occSymbolRe.MatchString(symbol)
}

func validateExpiration(exp string) (time.Time, bool) {
	if !expirationRe.MatchString(exp) {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", exp)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// validateExpirationNotPast checks that the expiration is today or in the future.
func validateExpirationNotPast(exp string) bool {
	t, ok := validateExpiration(exp)
	if !ok {
		return false
	}
	// Compare date only (expirations are end-of-day, so today is valid)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	return !t.Before(today)
}

func validateStrike(strike string) bool {
	if strike == "" {
		return false
	}
	f, err := strconv.ParseFloat(strike, 64)
	if err != nil {
		return false
	}
	return f > 0
}

func validateQty(qty string) bool {
	if qty == "" {
		return false
	}
	q, err := strconv.Atoi(qty)
	if err != nil {
		return false
	}
	return q > 0 && q <= maxOptionOrderQty
}

func validatePrice(price string) bool {
	if price == "" {
		return true // optional
	}
	f, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return false
	}
	return f >= 0
}

// limitedBody wraps the request body with a byte limit to prevent abuse.
func limitedBody(r *http.Request) io.Reader {
	return io.LimitReader(r.Body, maxRequestBodyBytes)
}

// --- Provider resolution ---

// getOptionsProvider type-asserts the provider to OptionsProvider.
func (s *Server) getOptionsProvider(r *http.Request) (provider.OptionsProvider, error) {
	p, err := s.getProvider(r)
	if err != nil {
		return nil, err
	}
	op, ok := p.(provider.OptionsProvider)
	if !ok {
		return nil, errProviderNoOptions(chi.URLParam(r, "provider"))
	}
	return op, nil
}

func errProviderNoOptions(name string) error {
	return &providerCapErr{provider: name, capability: "options"}
}

type providerCapErr struct {
	provider   string
	capability string
}

func (e *providerCapErr) Error() string {
	return "provider " + e.provider + " does not support " + e.capability
}

// --- Option Chain & Expirations ---

func (s *Server) handleGetOptionExpirations(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if !validateUnderlyingSymbol(symbol) {
		writeError(w, http.StatusBadRequest, "invalid symbol: must be 1-6 uppercase letters")
		return
	}

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

func (s *Server) handleGetOptionChain(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if !validateUnderlyingSymbol(symbol) {
		writeError(w, http.StatusBadRequest, "invalid symbol: must be 1-6 uppercase letters")
		return
	}

	expiration := chi.URLParam(r, "expiration")
	if expiration == "" {
		expiration = r.URL.Query().Get("expiration")
	}
	if expiration != "" {
		if _, ok := validateExpiration(expiration); !ok {
			writeError(w, http.StatusBadRequest, "invalid expiration: must be YYYY-MM-DD format")
			return
		}
	}

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	chain, err := op.GetOptionChain(r.Context(), strings.ToUpper(symbol), expiration)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:   audit.ActionOptionChainQuery,
		Provider: chi.URLParam(r, "provider"),
		Symbol:   strings.ToUpper(symbol),
		Metadata: map[string]interface{}{"expiration": expiration},
	})

	writeJSON(w, http.StatusOK, chain)
}

func (s *Server) handleGetOptionQuote(w http.ResponseWriter, r *http.Request) {
	contractSymbol := chi.URLParam(r, "contractSymbol")
	if !validateOCCSymbol(contractSymbol) {
		writeError(w, http.StatusBadRequest, "invalid contract symbol: must match OCC format (e.g. AAPL260418C00150000)")
		return
	}

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	quote, err := op.GetOptionQuote(r.Context(), contractSymbol)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, quote)
}

func (s *Server) handleGetOptionGreeks(w http.ResponseWriter, r *http.Request) {
	contractSymbol := chi.URLParam(r, "contractSymbol")
	if !validateOCCSymbol(contractSymbol) {
		writeError(w, http.StatusBadRequest, "invalid contract symbol: must match OCC format (e.g. AAPL260418C00150000)")
		return
	}

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	quote, err := op.GetOptionQuote(r.Context(), contractSymbol)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"symbol": contractSymbol,
		"greeks": quote.Greeks,
	})
}

// --- Option Orders ---

func (s *Server) handleCreateOptionOrder(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	providerName := chi.URLParam(r, "provider")
	start := time.Now()

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req types.CreateOptionOrderRequest
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate action (allowlist)
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

	// Validate order type (allowlist)
	if req.OrderType == "" {
		req.OrderType = "limit"
	}
	if !validOrderTypes[strings.ToLower(req.OrderType)] {
		writeError(w, http.StatusBadRequest, "order_type must be one of: market, limit, stop, stop_limit")
		return
	}
	req.OrderType = strings.ToLower(req.OrderType)

	// Validate time in force (allowlist)
	if req.TimeInForce == "" {
		req.TimeInForce = "day"
	}
	if !validTimeInForce[strings.ToLower(req.TimeInForce)] {
		writeError(w, http.StatusBadRequest, "time_in_force must be one of: day, gtc, ioc, fok, opg, cls")
		return
	}
	req.TimeInForce = strings.ToLower(req.TimeInForce)

	// Validate contract identification: either OCC symbol or component parts
	if req.ContractSymbol != "" {
		if !validateOCCSymbol(req.ContractSymbol) {
			writeError(w, http.StatusBadRequest, "invalid contract_symbol: must match OCC format (e.g. AAPL260418C00150000)")
			return
		}
	} else {
		// Need symbol + contract_type + strike + expiration
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

	// Audit log regardless of success/failure
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

func (s *Server) handleCreateMultiLegOrder(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	providerName := chi.URLParam(r, "provider")
	start := time.Now()

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req types.CreateMultiLegOrderRequest
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate leg count (min 2, max maxMultiLegLegs)
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

	// Audit
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

func (s *Server) handleExerciseOption(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	providerName := chi.URLParam(r, "provider")
	start := time.Now()

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req types.ExerciseOptionRequest
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ContractSymbol == "" {
		writeError(w, http.StatusBadRequest, "contract_symbol is required")
		return
	}
	if !validateOCCSymbol(req.ContractSymbol) {
		writeError(w, http.StatusBadRequest, "invalid contract_symbol: must match OCC format (e.g. AAPL260418C00150000)")
		return
	}
	if req.Qty <= 0 || req.Qty > maxOptionOrderQty {
		writeError(w, http.StatusBadRequest, "qty must be a positive integer no greater than "+strconv.Itoa(maxOptionOrderQty))
		return
	}

	err = op.ExerciseOption(r.Context(), accountID, req.ContractSymbol, req.Qty)

	// Audit
	status := "success"
	var errMsg error
	if err != nil {
		status = "failed"
		errMsg = err
	}
	s.auditLog.RecordOrder(r.Context(), audit.ActionOptionExercise, providerName, accountID, req.ContractSymbol, "exercise", strconv.Itoa(req.Qty), "", "", status, time.Since(start), errMsg)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "exercised"})
}

func (s *Server) handleDoNotExercise(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	providerName := chi.URLParam(r, "provider")
	start := time.Now()

	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		ContractSymbol string `json:"contract_symbol"`
	}
	if err := json.NewDecoder(limitedBody(r)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ContractSymbol == "" {
		writeError(w, http.StatusBadRequest, "contract_symbol is required")
		return
	}
	if !validateOCCSymbol(req.ContractSymbol) {
		writeError(w, http.StatusBadRequest, "invalid contract_symbol: must match OCC format (e.g. AAPL260418C00150000)")
		return
	}

	err = op.DoNotExercise(r.Context(), accountID, req.ContractSymbol)

	// Audit
	status := "success"
	var errMsg error
	if err != nil {
		status = "failed"
		errMsg = err
	}
	s.auditLog.RecordOrder(r.Context(), audit.ActionOptionDoNotExercise, providerName, accountID, req.ContractSymbol, "do_not_exercise", "1", "", "", status, time.Since(start), errMsg)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "do_not_exercise"})
}

func (s *Server) handleGetOptionPositions(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	op, err := s.getOptionsProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

// --- Smart Options Routing ---

func (s *Server) handleRouteOptionOrder(w http.ResponseWriter, r *http.Request) {
	contractSymbol := r.URL.Query().Get("contract")
	if contractSymbol == "" {
		writeError(w, http.StatusBadRequest, "contract query parameter is required")
		return
	}
	if !validateOCCSymbol(contractSymbol) {
		writeError(w, http.StatusBadRequest, "invalid contract: must match OCC format (e.g. AAPL260418C00150000)")
		return
	}

	side := r.URL.Query().Get("side")
	if side == "" {
		side = "buy"
	}
	if !validSides[strings.ToLower(side)] {
		writeError(w, http.StatusBadRequest, "side must be 'buy' or 'sell'")
		return
	}
	side = strings.ToLower(side)

	routes, err := s.sor.RouteOptionOrder(r.Context(), contractSymbol, side)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contract": contractSymbol,
		"side":     side,
		"routes":   routes,
	})
}
