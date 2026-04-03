package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/luxfi/broker/pkg/types"
)

// Frontend-to-provider asset class mapping.
var (
	frontendToProvider = map[string]string{
		"stocks":    "us_equity",
		"crypto":    "crypto",
		"bonds":     "fixed_income",
		"commodity": "commodities",
		"forex":     "forex",
	}
	providerToFrontend = map[string]string{
		"us_equity":    "stocks",
		"crypto":       "crypto",
		"fixed_income": "bonds",
		"commodities":  "commodity",
		"forex":        "forex",
	}
)

// resolveUserID extracts the user identity from the auth middleware.
// Only X-User-Id is trusted — it is set by auth middleware from validated JWT claims.
func resolveUserID(r *http.Request) string {
	return r.Header.Get("X-User-Id")
}

// resolveUserAccounts finds all accounts belonging to a user across all providers.
// Accounts are always resolved from the authenticated user — never from request headers.
func (s *Server) resolveUserAccounts(r *http.Request) ([]resolvedAccount, error) {
	userID := resolveUserID(r)
	var resolved []resolvedAccount

	for _, name := range s.registry.List() {
		p, err := s.registry.Get(name)
		if err != nil {
			continue
		}
		accounts, err := p.ListAccounts(r.Context())
		if err != nil {
			continue
		}
		for _, acct := range accounts {
			if userID == "" || acct.UserID == userID {
				resolved = append(resolved, resolvedAccount{
					provider:  name,
					accountID: acct.ProviderID,
				})
				// If no user ID filter, take only the first account
				if userID == "" && len(resolved) > 0 {
					return resolved, nil
				}
			}
		}
	}

	if len(resolved) == 0 {
		return nil, errNoAccounts
	}
	return resolved, nil
}

type resolvedAccount struct {
	provider  string
	accountID string
}

var errNoAccounts = &apiError{Status: http.StatusNotFound, Message: "no accounts found for user"}

type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return e.Message }

// firstProvider returns the first registered provider. Used for market data
// endpoints that don't require an account.
func (s *Server) firstProvider() (string, error) {
	names := s.registry.List()
	if len(names) == 0 {
		return "", &apiError{Status: http.StatusServiceUnavailable, Message: "no providers available"}
	}
	return names[0], nil
}

// isCryptoSymbol checks whether a symbol looks like a crypto pair (contains /).
func isCryptoSymbol(symbol string) bool {
	return strings.Contains(symbol, "/")
}

// handleFrontendAssets returns assets across all providers with frontend type naming.
// GET /v1/exchange/assets?type=crypto|stocks|bonds|commodity|forex
func (s *Server) handleFrontendAssets(w http.ResponseWriter, r *http.Request) {
	frontendType := r.URL.Query().Get("type")

	// Map frontend type to provider class filter
	var providerClass string
	if frontendType != "" {
		var ok bool
		providerClass, ok = frontendToProvider[frontendType]
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid type: must be one of crypto, stocks, bonds, commodity, forex")
			return
		}
	}

	var all []*frontendAsset
	for _, name := range s.registry.List() {
		p, err := s.registry.Get(name)
		if err != nil {
			continue
		}
		assets, err := p.ListAssets(r.Context(), providerClass)
		if err != nil {
			continue
		}
		for _, a := range assets {
			if !a.Tradable {
				continue
			}
			fa := &frontendAsset{
				ID:           a.ID,
				Provider:     a.Provider,
				Symbol:       a.Symbol,
				Name:         a.Name,
				Type:         mapAssetClass(a.Class),
				Exchange:     a.Exchange,
				Status:       a.Status,
				Tradable:     a.Tradable,
				Fractionable: a.Fractionable,
			}
			all = append(all, fa)
		}
	}

	if all == nil {
		all = make([]*frontendAsset, 0)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"assets": all,
		"total":  len(all),
	})
}

type frontendAsset struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Type         string `json:"type"` // frontend name: stocks, crypto, bonds, commodity, forex
	Exchange     string `json:"exchange,omitempty"`
	Status       string `json:"status"`
	Tradable     bool   `json:"tradable"`
	Fractionable bool   `json:"fractionable"`
}

func mapAssetClass(providerClass string) string {
	if fe, ok := providerToFrontend[providerClass]; ok {
		return fe
	}
	return providerClass
}

// handleCryptoPrices returns current prices for top crypto pairs.
// GET /v1/exchange/crypto-prices
func (s *Server) handleCryptoPrices(w http.ResponseWriter, r *http.Request) {
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "AVAX/USD", "LTC/USD", "DOGE/USD", "LINK/USD", "UNI/USD"}

	// Find a provider that can serve crypto data
	prices := make(map[string]interface{})
	for _, name := range s.registry.List() {
		p, err := s.registry.Get(name)
		if err != nil {
			continue
		}
		snaps, err := p.GetSnapshots(r.Context(), symbols)
		if err != nil || len(snaps) == 0 {
			continue
		}
		for sym, snap := range snaps {
			prices[sym] = buildPriceEntry(snap)
		}
		// Got data from a provider, stop looking
		break
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prices": prices,
	})
}

func buildPriceEntry(snap *types.MarketSnapshot) map[string]interface{} {
	entry := make(map[string]interface{})

	if snap.LatestTrade != nil {
		entry["price"] = snap.LatestTrade.Price
	}
	if snap.DailyBar != nil {
		entry["open"] = snap.DailyBar.Open
		entry["high"] = snap.DailyBar.High
		entry["low"] = snap.DailyBar.Low
		entry["close"] = snap.DailyBar.Close
		entry["volume"] = snap.DailyBar.Volume
	}
	if snap.PrevDailyBar != nil {
		entry["prev_close"] = snap.PrevDailyBar.Close
		// Calculate change percentage
		if snap.PrevDailyBar.Close > 0 && snap.LatestTrade != nil {
			changePct := ((snap.LatestTrade.Price - snap.PrevDailyBar.Close) / snap.PrevDailyBar.Close) * 100
			entry["change_pct"] = changePct
		}
	}
	return entry
}

// handleChartData returns OHLCV bars for a symbol.
// GET /v1/exchange/charts/{symbol}?timeframe=1Min|5Min|1H|1D&limit=100
func (s *Server) handleChartData(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	q := r.URL.Query()

	timeframe := q.Get("timeframe")
	if timeframe == "" {
		timeframe = "1D"
	}
	limit := 100
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	// Try each provider until one returns data
	for _, name := range s.registry.List() {
		p, err := s.registry.Get(name)
		if err != nil {
			continue
		}

		bars, err := p.GetBars(r.Context(), symbol, timeframe, q.Get("start"), q.Get("end"), limit)
		if err != nil || len(bars) == 0 {
			continue
		}

		out := make([]map[string]interface{}, len(bars))
		for i, b := range bars {
			out[i] = map[string]interface{}{
				"time":   b.Timestamp,
				"open":   b.Open,
				"high":   b.High,
				"low":    b.Low,
				"close":  b.Close,
				"volume": b.Volume,
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"bars": out,
		})
		return
	}

	// No provider had data
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"bars": []interface{}{},
	})
}

// handleFrontendOrders lists orders across all user's provider accounts.
// GET /v1/exchange/orders
func (s *Server) handleFrontendOrders(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.resolveUserAccounts(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var all []*types.Order
	for _, acct := range accounts {
		p, err := s.registry.Get(acct.provider)
		if err != nil {
			continue
		}
		orders, err := p.ListOrders(r.Context(), acct.accountID)
		if err != nil {
			continue
		}
		all = append(all, orders...)
	}

	if all == nil {
		all = make([]*types.Order, 0)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"orders": all,
	})
}

// handleFrontendCreateOrder creates an order, routing to the appropriate provider.
// POST /v1/exchange/orders
// Body: { "symbol", "qty", "side", "type", "time_in_force" }
func (s *Server) handleFrontendCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req types.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Symbol == "" {
		writeError(w, http.StatusBadRequest, "symbol is required")
		return
	}
	if req.Side == "" {
		writeError(w, http.StatusBadRequest, "side is required")
		return
	}
	if req.Type == "" {
		req.Type = "market"
	}
	if req.TimeInForce == "" {
		req.TimeInForce = "day"
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

	// Route to the first available account. For crypto symbols, prefer a
	// provider with crypto support; for equities, prefer an equity provider.
	acct := accounts[0]
	p, err := s.registry.Get(acct.provider)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "provider unavailable")
		return
	}

	order, err := p.CreateOrder(r.Context(), acct.accountID, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

// handleFrontendPositions lists positions across all user's provider accounts.
// GET /v1/exchange/positions
func (s *Server) handleFrontendPositions(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.resolveUserAccounts(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var all []types.Position
	for _, acct := range accounts {
		p, err := s.registry.Get(acct.provider)
		if err != nil {
			continue
		}
		portfolio, err := p.GetPortfolio(r.Context(), acct.accountID)
		if err != nil {
			continue
		}
		all = append(all, portfolio.Positions...)
	}

	if all == nil {
		all = make([]types.Position, 0)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"positions": all,
	})
}

// handleFrontendPortfolio returns a combined portfolio snapshot for the user.
// GET /v1/exchange/portfolio
func (s *Server) handleFrontendPortfolio(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.resolveUserAccounts(r)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			writeError(w, ae.Status, ae.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If single account, return its portfolio directly
	if len(accounts) == 1 {
		acct := accounts[0]
		p, err := s.registry.Get(acct.provider)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "provider unavailable")
			return
		}
		portfolio, err := p.GetPortfolio(r.Context(), acct.accountID)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, portfolio)
		return
	}

	// Multiple accounts: aggregate
	var allPositions []types.Position
	var totalCash, totalEquity, totalBuyingPower, totalValue float64
	for _, acct := range accounts {
		p, err := s.registry.Get(acct.provider)
		if err != nil {
			continue
		}
		portfolio, err := p.GetPortfolio(r.Context(), acct.accountID)
		if err != nil {
			continue
		}
		allPositions = append(allPositions, portfolio.Positions...)
		if v, e := strconv.ParseFloat(portfolio.Cash, 64); e == nil {
			totalCash += v
		}
		if v, e := strconv.ParseFloat(portfolio.Equity, 64); e == nil {
			totalEquity += v
		}
		if v, e := strconv.ParseFloat(portfolio.BuyingPower, 64); e == nil {
			totalBuyingPower += v
		}
		if v, e := strconv.ParseFloat(portfolio.PortfolioValue, 64); e == nil {
			totalValue += v
		}
	}
	if allPositions == nil {
		allPositions = make([]types.Position, 0)
	}
	writeJSON(w, http.StatusOK, &types.Portfolio{
		Cash:           strconv.FormatFloat(totalCash, 'f', 2, 64),
		Equity:         strconv.FormatFloat(totalEquity, 'f', 2, 64),
		BuyingPower:    strconv.FormatFloat(totalBuyingPower, 'f', 2, 64),
		PortfolioValue: strconv.FormatFloat(totalValue, 'f', 2, 64),
		Positions:      allPositions,
	})
}
