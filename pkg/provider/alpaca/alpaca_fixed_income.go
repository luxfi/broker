package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/luxfi/broker/pkg/types"
)

// --- Fixed Income: Asset Retrieval (Checklist #1) ---

// ListCorporateBonds returns US corporate bond assets.
// Alpaca API: GET /v1/assets/fixed_income/us_corporates
func (p *Provider) ListCorporateBonds(ctx context.Context) ([]*types.Asset, error) {
	return p.listFixedIncomeAssets(ctx, "/v1/assets/fixed_income/us_corporates")
}

// ListTreasuryBonds returns US treasury bond assets.
// Alpaca API: GET /v1/assets/fixed_income/us_treasuries
func (p *Provider) ListTreasuryBonds(ctx context.Context) ([]*types.Asset, error) {
	return p.listFixedIncomeAssets(ctx, "/v1/assets/fixed_income/us_treasuries")
}

func (p *Provider) listFixedIncomeAssets(ctx context.Context, path string) ([]*types.Asset, error) {
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("list fixed income assets: %w", err)
	}

	// Alpaca FI endpoint returns different fields than equities.
	// Parse what the API provides and map to the unified Asset type.
	var raw []struct {
		ID           string `json:"id"`
		CUSIP        string `json:"cusip"`
		Symbol       string `json:"symbol"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		Tradable     bool   `json:"tradable"`
		Fractionable bool   `json:"fractionable"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode fixed income assets: %w", err)
	}

	assets := make([]*types.Asset, len(raw))
	for i, r := range raw {
		symbol := r.Symbol
		if symbol == "" {
			symbol = r.CUSIP
		}
		assets[i] = &types.Asset{
			ID:           r.ID,
			Provider:     "alpaca",
			Symbol:       symbol,
			Name:         r.Name,
			Class:        "fixed_income",
			Status:       r.Status,
			Tradable:     r.Tradable,
			Fractionable: r.Fractionable,
		}
	}
	return assets, nil
}

// --- Fixed Income: Order Execution (Checklist #2, #3, #4, #5) ---

// CreateFixedIncomeOrder places a fixed income order.
//
// Constraints enforced:
//   - TIF forced to "day" (#4)
//   - Only market orders for PILOT (internally limit, but client sees market)
//   - Notional validated to 2 decimal places, qty to 9 decimal places (#2, #3)
//   - Qty represents par value ($1000 per bond), recommend 9dp display (#11)
//   - Extended hours supported for sell orders (#5: order submitted out of hours pends)
func (p *Provider) CreateFixedIncomeOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	if req.Symbol == "" {
		return nil, fmt.Errorf("fixed income order: symbol (CUSIP) is required")
	}

	side := strings.ToLower(req.Side)
	if side != "buy" && side != "sell" {
		return nil, fmt.Errorf("fixed income order: side must be buy or sell, got %q", req.Side)
	}

	// Only market orders for fixed income PILOT.
	if req.Type != "" && strings.ToLower(req.Type) != "market" {
		return nil, fmt.Errorf("fixed income order: only market orders supported, got %q", req.Type)
	}

	// TIF forced to day for fixed income.
	if req.TimeInForce != "" && strings.ToLower(req.TimeInForce) != "day" {
		return nil, fmt.Errorf("fixed income order: only day TIF supported, got %q", req.TimeInForce)
	}

	// Decimal precision validation using the existing helper.
	if err := validateDecimalPlaces(req.Notional, 2, "notional"); err != nil {
		return nil, err
	}
	if err := validateDecimalPlaces(req.Qty, 9, "quantity"); err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"symbol":        req.Symbol,
		"side":          req.Side,
		"type":          "market",
		"time_in_force": "day",
	}
	if req.Qty != "" {
		body["qty"] = req.Qty
	}
	if req.Notional != "" {
		body["notional"] = req.Notional
	}
	if req.LimitPrice != "" {
		body["limit_price"] = req.LimitPrice
	}
	if req.ClientOrderID != "" {
		body["client_order_id"] = req.ClientOrderID
	}
	// Extended hours for sell orders (#3, #5).
	// Out-of-hours orders will be accepted and pend until market open.
	if req.ExtendedHours && side == "sell" {
		body["extended_hours"] = true
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/orders", body)
	if err != nil {
		return nil, fmt.Errorf("create fixed income order: %w", err)
	}

	order, err := p.parseOrder(data)
	if err != nil {
		return nil, err
	}
	order.AssetClass = "fixed_income"
	return order, nil
}

// --- Fixed Income: Cancel Order (Checklist #7) ---

// CancelFixedIncomeOrder cancels a pending fixed income order.
// Required for limit orders or orders submitted outside market hours.
func (p *Provider) CancelFixedIncomeOrder(ctx context.Context, accountID, orderID string) error {
	return p.CancelOrder(ctx, accountID, orderID)
}

// --- Fixed Income: Order Status (Checklist #9) ---

// GetFixedIncomeOrder returns the status of a fixed income order.
// FI orders may have statuses: new, pending_new, accepted, filled, canceled, expired.
func (p *Provider) GetFixedIncomeOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	order, err := p.GetOrder(ctx, accountID, orderID)
	if err != nil {
		return nil, err
	}
	order.AssetClass = "fixed_income"
	return order, nil
}

// --- Fixed Income: Account Activities (Checklist #10) ---

// GetFixedIncomeActivities returns account activities filtered to maturity corporate
// action events only (Pilot constraint). Other FI activity types are excluded.
func (p *Provider) GetFixedIncomeActivities(ctx context.Context, accountID string) ([]*types.Activity, error) {
	return p.GetAccountActivities(ctx, accountID, &types.ActivityParams{
		ActivityTypes: []string{"maturity"},
	})
}

// --- Fixed Income: Positions (Checklist #11, #12) ---

// GetFixedIncomePositions returns fixed income positions for an account.
// Note: qty represents par value ($1000 per bond), not the number of bonds.
// Display recommendation: 9 decimal places for qty.
func (p *Provider) GetFixedIncomePositions(ctx context.Context, accountID string) ([]*types.Position, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+accountID+"/positions", nil)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode positions: %w", err)
	}

	var positions []*types.Position
	for _, r := range raw {
		pos, err := p.parsePosition(r)
		if err != nil {
			continue
		}
		if pos.AssetClass == "fixed_income" {
			positions = append(positions, pos)
		}
	}
	return positions, nil
}

// CloseFixedIncomePosition closes an open fixed income position.
func (p *Provider) CloseFixedIncomePosition(ctx context.Context, accountID, symbol string, qty *float64) (*types.Order, error) {
	order, err := p.ClosePosition(ctx, accountID, symbol, qty)
	if err != nil {
		return nil, fmt.Errorf("close fixed income position: %w", err)
	}
	order.AssetClass = "fixed_income"
	return order, nil
}

// --- Fixed Income: Market Data (Checklist #13) ---
// Market data for fixed income requires the Moment Terms of Service (Moment
// is Alpaca's fixed income data vendor) to be signed before enabling.
// See Alpaca docs for Moment ToS agreement flow.

// GetFixedIncomeQuote returns market data for a fixed income instrument.
// NOT IMPLEMENTED: Requires Moment ToS agreement to be signed with Alpaca.
// Once Moment is enabled, replace this with calls to the standard data API
// endpoints using the fixed income CUSIP as the symbol.
func (p *Provider) GetFixedIncomeQuote(ctx context.Context, cusip string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("fixed income market data not available: Moment ToS agreement required (CUSIP: %s)", cusip)
}
