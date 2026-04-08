package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// Alpaca options contract response from /v1/options/contracts
type alpacaOptionContract struct {
	ID               string  `json:"id"`
	Symbol           string  `json:"symbol"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	Tradable         bool    `json:"tradable"`
	ExpirationDate   string  `json:"expiration_date"`
	RootSymbol       string  `json:"root_symbol"`
	UnderlyingSymbol string  `json:"underlying_symbol"`
	Type             string  `json:"type"` // call, put
	Style            string  `json:"style"`
	StrikePrice      string  `json:"strike_price"`
	Size             string  `json:"size"`
	OpenInterest     string  `json:"open_interest"`
	ClosePrice       string  `json:"close_price"`
	OpenPrice        string  `json:"open_price"`
}

// Alpaca option snapshot from data API
type alpacaOptionSnapshot struct {
	LatestQuote *struct {
		Timestamp string  `json:"t"`
		BidPrice  float64 `json:"bp"`
		BidSize   int     `json:"bs"`
		AskPrice  float64 `json:"ap"`
		AskSize   int     `json:"as"`
	} `json:"latestQuote"`
	LatestTrade *struct {
		Timestamp string  `json:"t"`
		Price     float64 `json:"p"`
		Size      int     `json:"s"`
	} `json:"latestTrade"`
	Greeks *struct {
		Delta float64 `json:"delta"`
		Gamma float64 `json:"gamma"`
		Theta float64 `json:"theta"`
		Vega  float64 `json:"vega"`
		Rho   float64 `json:"rho"`
		IV    float64 `json:"implied_volatility"`
	} `json:"greeks"`
}

func (c *alpacaOptionContract) toUnified() types.OptionContract {
	strike, _ := strconv.ParseFloat(c.StrikePrice, 64)
	oi, _ := strconv.Atoi(c.OpenInterest)
	close, _ := strconv.ParseFloat(c.ClosePrice, 64)

	return types.OptionContract{
		Symbol:       c.Symbol,
		Underlying:   c.UnderlyingSymbol,
		ContractType: c.Type,
		Strike:       strike,
		Expiration:   c.ExpirationDate,
		Style:        c.Style,
		Status:       c.Status,
		Tradable:     c.Tradable,
		Last:         close,
		OpenInterest: oi,
	}
}

// GetOptionChain returns all contracts for a symbol and expiration.
// Alpaca API: GET /v1/options/contracts?underlying_symbols={symbol}&expiration_date={date}
func (p *Provider) GetOptionChain(ctx context.Context, symbol string, expiration string) (*types.OptionChain, error) {
	params := url.Values{}
	params.Set("underlying_symbols", strings.ToUpper(symbol))
	if expiration != "" {
		params.Set("expiration_date", expiration)
	}
	params.Set("status", "active")

	data, _, err := p.do(ctx, http.MethodGet, "/v1/options/contracts?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("get option chain: %w", err)
	}

	// Alpaca returns paginated response
	var resp struct {
		Contracts     []alpacaOptionContract `json:"option_contracts"`
		NextPageToken string                 `json:"next_page_token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode option chain: %w", err)
	}

	chain := &types.OptionChain{
		Symbol:     strings.ToUpper(symbol),
		Expiration: expiration,
	}

	// Fetch snapshots for pricing data if we got contracts
	snapshots := make(map[string]*alpacaOptionSnapshot)
	if len(resp.Contracts) > 0 {
		symbols := make([]string, 0, len(resp.Contracts))
		for _, c := range resp.Contracts {
			symbols = append(symbols, c.Symbol)
		}
		snapshots = p.fetchOptionSnapshots(ctx, symbols)
	}

	for _, c := range resp.Contracts {
		oc := c.toUnified()

		// Merge snapshot data (quotes + greeks)
		if snap, ok := snapshots[c.Symbol]; ok {
			if snap.LatestQuote != nil {
				oc.Bid = snap.LatestQuote.BidPrice
				oc.Ask = snap.LatestQuote.AskPrice
			}
			if snap.LatestTrade != nil {
				oc.Last = snap.LatestTrade.Price
			}
			if snap.Greeks != nil {
				oc.Greeks = types.Greeks{
					Delta: snap.Greeks.Delta,
					Gamma: snap.Greeks.Gamma,
					Theta: snap.Greeks.Theta,
					Vega:  snap.Greeks.Vega,
					Rho:   snap.Greeks.Rho,
					IV:    snap.Greeks.IV,
				}
			}
		}

		switch c.Type {
		case "call":
			chain.Calls = append(chain.Calls, oc)
		case "put":
			chain.Puts = append(chain.Puts, oc)
		}
	}

	return chain, nil
}

// GetOptionExpirations returns available expiration dates for a symbol.
// Uses the contracts endpoint filtered by underlying to extract unique dates.
func (p *Provider) GetOptionExpirations(ctx context.Context, symbol string) ([]string, error) {
	params := url.Values{}
	params.Set("underlying_symbols", strings.ToUpper(symbol))
	params.Set("status", "active")

	data, _, err := p.do(ctx, http.MethodGet, "/v1/options/contracts?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("get option expirations: %w", err)
	}

	var resp struct {
		Contracts     []alpacaOptionContract `json:"option_contracts"`
		NextPageToken string                 `json:"next_page_token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode option expirations: %w", err)
	}

	seen := make(map[string]bool)
	var expirations []string
	for _, c := range resp.Contracts {
		if !seen[c.ExpirationDate] {
			seen[c.ExpirationDate] = true
			expirations = append(expirations, c.ExpirationDate)
		}
	}

	return expirations, nil
}

// GetOptionQuote returns a real-time quote for a specific contract symbol.
// Alpaca API: GET /v1/options/contracts/{symbol_or_id}
func (p *Provider) GetOptionQuote(ctx context.Context, contractSymbol string) (*types.OptionQuote, error) {
	// Fetch contract metadata
	data, _, err := p.do(ctx, http.MethodGet, "/v1/options/contracts/"+url.PathEscape(contractSymbol), nil)
	if err != nil {
		return nil, fmt.Errorf("get option contract: %w", err)
	}

	var contract alpacaOptionContract
	if err := json.Unmarshal(data, &contract); err != nil {
		return nil, fmt.Errorf("decode option contract: %w", err)
	}

	strike, _ := strconv.ParseFloat(contract.StrikePrice, 64)
	oi, _ := strconv.Atoi(contract.OpenInterest)
	close, _ := strconv.ParseFloat(contract.ClosePrice, 64)

	quote := &types.OptionQuote{
		Symbol:       contract.Symbol,
		Underlying:   contract.UnderlyingSymbol,
		ContractType: contract.Type,
		Strike:       strike,
		Expiration:   contract.ExpirationDate,
		Last:         close,
		OpenInterest: oi,
	}

	// Fetch snapshot for live pricing + greeks
	snapshots := p.fetchOptionSnapshots(ctx, []string{contract.Symbol})
	if snap, ok := snapshots[contract.Symbol]; ok {
		if snap.LatestQuote != nil {
			quote.Bid = snap.LatestQuote.BidPrice
			quote.Ask = snap.LatestQuote.AskPrice
		}
		if snap.LatestTrade != nil {
			quote.Last = snap.LatestTrade.Price
		}
		if snap.Greeks != nil {
			quote.Greeks = types.Greeks{
				Delta: snap.Greeks.Delta,
				Gamma: snap.Greeks.Gamma,
				Theta: snap.Greeks.Theta,
				Vega:  snap.Greeks.Vega,
				Rho:   snap.Greeks.Rho,
				IV:    snap.Greeks.IV,
			}
		}
	}

	return quote, nil
}

// CreateOptionOrder places a single-leg option order via Alpaca.
// Alpaca uses the same orders endpoint with asset_class: "us_option".
func (p *Provider) CreateOptionOrder(ctx context.Context, accountID string, req *types.CreateOptionOrderRequest) (*types.Order, error) {
	// Build the OCC symbol if not provided
	contractSymbol := req.ContractSymbol
	if contractSymbol == "" {
		sym, err := buildOCCSymbol(req.Symbol, req.Expiration, req.ContractType, req.Strike)
		if err != nil {
			return nil, err
		}
		contractSymbol = sym
	}

	// Map action to Alpaca side
	side, err := actionToSide(req.Action)
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"symbol":        contractSymbol,
		"qty":           req.Qty,
		"side":          side,
		"type":          req.OrderType,
		"time_in_force": req.TimeInForce,
		"order_class":   "simple",
	}
	if req.LimitPrice != "" {
		body["limit_price"] = req.LimitPrice
	}
	if req.StopPrice != "" {
		body["stop_price"] = req.StopPrice
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/orders", body)
	if err != nil {
		return nil, fmt.Errorf("create option order: %w", err)
	}

	order, err := p.parseOrder(data)
	if err != nil {
		return nil, err
	}
	order.AssetClass = "us_option"
	return order, nil
}

// CreateMultiLegOrder places a multi-leg strategy order via Alpaca.
func (p *Provider) CreateMultiLegOrder(ctx context.Context, accountID string, req *types.CreateMultiLegOrderRequest) (*types.MultiLegOrderResult, error) {
	if len(req.Legs) < 2 {
		return nil, fmt.Errorf("multi-leg order requires at least 2 legs")
	}

	legs := make([]map[string]interface{}, 0, len(req.Legs))
	for _, leg := range req.Legs {
		contractSymbol := leg.ContractSymbol
		if contractSymbol == "" {
			sym, err := buildOCCSymbol(req.Symbol, leg.Expiration, leg.ContractType, leg.Strike)
			if err != nil {
				return nil, fmt.Errorf("leg %s %s: %w", leg.ContractType, leg.Strike, err)
			}
			contractSymbol = sym
		}

		side, err := actionToSide(leg.Action)
		if err != nil {
			return nil, fmt.Errorf("leg %s: %w", contractSymbol, err)
		}

		legs = append(legs, map[string]interface{}{
			"symbol":          contractSymbol,
			"side":            side,
			"ratio_qty":       leg.Quantity,
			"position_intent": leg.Action,
		})
	}

	body := map[string]interface{}{
		"symbol":        strings.ToUpper(req.Symbol),
		"order_class":   "mleg",
		"legs":          legs,
		"type":          req.OrderType,
		"time_in_force": req.TimeInForce,
	}
	if req.LimitPrice != "" {
		body["limit_price"] = req.LimitPrice
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/orders", body)
	if err != nil {
		return nil, fmt.Errorf("create multi-leg order: %w", err)
	}

	order, err := p.parseOrder(data)
	if err != nil {
		return nil, err
	}

	return &types.MultiLegOrderResult{
		StrategyOrderID: order.ProviderID,
		LegOrders:       []*types.Order{order},
		NetPremium:      req.LimitPrice,
		Status:          order.Status,
	}, nil
}

// ExerciseOption exercises an option contract.
// Alpaca uses POST /v1/trading/accounts/{account_id}/positions/{symbol}/exercise
func (p *Provider) ExerciseOption(ctx context.Context, accountID string, contractSymbol string, qty int) error {
	body := map[string]interface{}{}
	if qty > 0 {
		body["qty"] = strconv.Itoa(qty)
	}

	_, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/positions/"+url.PathEscape(contractSymbol)+"/exercise", body)
	if err != nil {
		return fmt.Errorf("exercise option: %w", err)
	}
	return nil
}

// GetOptionPositions returns all option positions for an account.
// Filters positions with asset_class = "us_option" from the standard positions endpoint.
func (p *Provider) GetOptionPositions(ctx context.Context, accountID string) ([]*types.OptionPosition, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+accountID+"/positions", nil)
	if err != nil {
		return nil, fmt.Errorf("get option positions: %w", err)
	}

	var rawPositions []struct {
		Symbol        string `json:"symbol"`
		Qty           string `json:"qty"`
		AvgEntryPrice string `json:"avg_entry_price"`
		MarketValue   string `json:"market_value"`
		CurrentPrice  string `json:"current_price"`
		UnrealizedPL  string `json:"unrealized_pl"`
		Side          string `json:"side"`
		AssetClass    string `json:"asset_class"`
	}
	if err := json.Unmarshal(data, &rawPositions); err != nil {
		return nil, fmt.Errorf("decode positions: %w", err)
	}

	var positions []*types.OptionPosition
	for _, rp := range rawPositions {
		if rp.AssetClass != "us_option" {
			continue
		}

		contractType, strike, expiration := parseOCCSymbol(rp.Symbol)
		underlying := extractUnderlying(rp.Symbol)

		positions = append(positions, &types.OptionPosition{
			Symbol:        rp.Symbol,
			Underlying:    underlying,
			ContractType:  contractType,
			Strike:        strike,
			Expiration:    expiration,
			Qty:           rp.Qty,
			AvgCost:       rp.AvgEntryPrice,
			MarketValue:   rp.MarketValue,
			CurrentPrice:  rp.CurrentPrice,
			UnrealizedPnL: rp.UnrealizedPL,
			Side:          rp.Side,
		})
	}

	// Fetch greeks for option positions
	if len(positions) > 0 {
		symbols := make([]string, len(positions))
		for i, pos := range positions {
			symbols[i] = pos.Symbol
		}
		snapshots := p.fetchOptionSnapshots(ctx, symbols)
		for _, pos := range positions {
			if snap, ok := snapshots[pos.Symbol]; ok && snap.Greeks != nil {
				pos.Greeks = types.Greeks{
					Delta: snap.Greeks.Delta,
					Gamma: snap.Greeks.Gamma,
					Theta: snap.Greeks.Theta,
					Vega:  snap.Greeks.Vega,
					Rho:   snap.Greeks.Rho,
					IV:    snap.Greeks.IV,
				}
			}
		}
	}

	return positions, nil
}

// DoNotExercise marks an option contract as do-not-exercise.
// Alpaca API: POST /v1/trading/accounts/{account_id}/positions/{symbol}/do-not-exercise
func (p *Provider) DoNotExercise(ctx context.Context, accountID, contractSymbol string) error {
	_, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/positions/"+url.PathEscape(contractSymbol)+"/do-not-exercise", nil)
	if err != nil {
		return fmt.Errorf("do not exercise: %w", err)
	}
	return nil
}

// SetOptionsApprovalLevel sets the options approval level for an account.
// Alpaca API: POST /v1/accounts/{account_id}/options/approval
func (p *Provider) SetOptionsApprovalLevel(ctx context.Context, accountID string, level int) error {
	body := map[string]interface{}{
		"level": level,
	}
	_, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+accountID+"/options/approval", body)
	if err != nil {
		return fmt.Errorf("set options approval level: %w", err)
	}
	return nil
}

// fetchOptionSnapshots retrieves snapshots (quotes + greeks) from the data API.
// GET /v1beta1/options/snapshots?symbols={syms}
func (p *Provider) fetchOptionSnapshots(ctx context.Context, symbols []string) map[string]*alpacaOptionSnapshot {
	result := make(map[string]*alpacaOptionSnapshot)
	if len(symbols) == 0 {
		return result
	}

	// Batch in groups of 100
	for i := 0; i < len(symbols); i += 100 {
		end := i + 100
		if end > len(symbols) {
			end = len(symbols)
		}
		batch := symbols[i:end]

		params := url.Values{}
		params.Set("symbols", strings.Join(batch, ","))

		data, _, err := p.doData(ctx, http.MethodGet, "/v1beta1/options/snapshots?"+params.Encode())
		if err != nil {
			continue
		}

		var resp struct {
			Snapshots map[string]*alpacaOptionSnapshot `json:"snapshots"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		for k, v := range resp.Snapshots {
			result[k] = v
		}
	}
	return result
}

// buildOCCSymbol constructs an OCC-format symbol.
// Format: SYMBOLYYMMDDTSSSSSSSS where T=C/P, S=strike*1000 zero-padded to 8 digits.
// Example: AAPL260418C00150000 = AAPL $150 Call expiring 2026-04-18
func buildOCCSymbol(underlying, expiration, contractType, strike string) (string, error) {
	if underlying == "" || expiration == "" || contractType == "" || strike == "" {
		return "", fmt.Errorf("symbol, expiration, contract_type, and strike are required")
	}

	t, err := time.Parse("2006-01-02", expiration)
	if err != nil {
		return "", fmt.Errorf("invalid expiration date %q: %w", expiration, err)
	}

	strikeF, err := strconv.ParseFloat(strike, 64)
	if err != nil {
		return "", fmt.Errorf("invalid strike %q: %w", strike, err)
	}
	if strikeF <= 0 || strikeF >= 100000 {
		return "", fmt.Errorf("strike %q out of OCC range (0, 100000)", strike)
	}

	typeChar := "C"
	if strings.EqualFold(contractType, "put") {
		typeChar = "P"
	}

	// OCC strike is price * 1000, zero-padded to 8 digits. Round to avoid truncation.
	strikeInt := int(math.Round(strikeF * 1000))
	return fmt.Sprintf("%s%s%s%08d", strings.ToUpper(underlying), t.Format("060102"), typeChar, strikeInt), nil
}

// actionToSide maps option action (buy_to_open, etc) to Alpaca's side (buy/sell).
func actionToSide(action string) (string, error) {
	switch strings.ToLower(action) {
	case "buy_to_open", "buy_to_close":
		return "buy", nil
	case "sell_to_open", "sell_to_close":
		return "sell", nil
	default:
		return "", fmt.Errorf("invalid option action %q: must be buy_to_open, buy_to_close, sell_to_open, or sell_to_close", action)
	}
}

// parseOCCSymbol extracts contract type, strike, and expiration from an OCC symbol.
// Returns ("call"/"put", strike float64, "YYYY-MM-DD").
func parseOCCSymbol(occ string) (string, float64, string) {
	if len(occ) < 15 {
		return "", 0, ""
	}

	// Find the C or P that separates date from strike
	// OCC format: ROOT(1-6)DATE(6)TYPE(1)STRIKE(8)
	// Scan from end: last 8 = strike, then 1 = type, then 6 = date, rest = root
	strikeStr := occ[len(occ)-8:]
	typeChar := occ[len(occ)-9 : len(occ)-8]
	dateStr := occ[len(occ)-15 : len(occ)-9]

	contractType := "call"
	if typeChar == "P" {
		contractType = "put"
	}

	strikeInt, _ := strconv.Atoi(strikeStr)
	strike := float64(strikeInt) / 1000.0

	var expiration string
	if t, err := time.Parse("060102", dateStr); err == nil {
		expiration = t.Format("2006-01-02")
	}

	return contractType, strike, expiration
}

// extractUnderlying extracts the underlying symbol from an OCC option symbol.
func extractUnderlying(occ string) string {
	if len(occ) < 15 {
		return occ
	}
	return occ[:len(occ)-15]
}
