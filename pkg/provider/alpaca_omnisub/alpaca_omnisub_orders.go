package alpaca_omnisub

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// CreateOrder places an order scoped to a sub-account.
// Alpaca OmniSub routes: POST /v1/trading/accounts/{subID}/orders
func (p *Provider) CreateOrder(ctx context.Context, subAccountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	body := map[string]interface{}{
		"symbol":        req.Symbol,
		"side":          req.Side,
		"type":          req.Type,
		"time_in_force": req.TimeInForce,
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
	if req.StopPrice != "" {
		body["stop_price"] = req.StopPrice
	}
	if req.ClientOrderID != "" {
		body["client_order_id"] = req.ClientOrderID
	}
	if req.TrailPrice != "" {
		body["trail_price"] = req.TrailPrice
	}
	if req.TrailPercent != "" {
		body["trail_percent"] = req.TrailPercent
	}
	if req.ExtendedHours {
		body["extended_hours"] = true
	}
	if req.OrderClass != "" {
		body["order_class"] = req.OrderClass
	}
	if req.TakeProfit != nil {
		body["take_profit"] = map[string]interface{}{
			"limit_price": req.TakeProfit.LimitPrice,
		}
	}
	if req.StopLoss != nil {
		sl := map[string]interface{}{
			"stop_price": req.StopLoss.StopPrice,
		}
		if req.StopLoss.LimitPrice != "" {
			sl["limit_price"] = req.StopLoss.LimitPrice
		}
		body["stop_loss"] = sl
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+subAccountID+"/orders", body)
	if err != nil {
		return nil, err
	}
	return p.parseOrder(data)
}

// ListOrders lists all orders for a sub-account.
func (p *Provider) ListOrders(ctx context.Context, subAccountID string) ([]*types.Order, error) {
	path := "/v1/trading/accounts/" + subAccountID + "/orders?status=all"
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	orders := make([]*types.Order, 0, len(raw))
	for _, r := range raw {
		o, err := p.parseOrder(r)
		if err != nil {
			continue
		}
		orders = append(orders, o)
	}
	return orders, nil
}

// GetOrder retrieves a single order for a sub-account.
func (p *Provider) GetOrder(ctx context.Context, subAccountID, orderID string) (*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+subAccountID+"/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseOrder(data)
}

// CancelOrder cancels an order for a sub-account.
func (p *Provider) CancelOrder(ctx context.Context, subAccountID, orderID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/trading/accounts/"+subAccountID+"/orders/"+orderID, nil)
	return err
}

func (p *Provider) parseOrder(data []byte) (*types.Order, error) {
	var raw struct {
		ID             string  `json:"id"`
		Symbol         string  `json:"symbol"`
		Qty            string  `json:"qty"`
		Notional       string  `json:"notional"`
		Side           string  `json:"side"`
		Type           string  `json:"type"`
		TimeInForce    string  `json:"time_in_force"`
		LimitPrice     string  `json:"limit_price"`
		StopPrice      string  `json:"stop_price"`
		Status         string  `json:"status"`
		FilledQty      string  `json:"filled_qty"`
		FilledAvgPrice string  `json:"filled_avg_price"`
		AssetClass     string  `json:"asset_class"`
		CreatedAt      string  `json:"created_at"`
		FilledAt       *string `json:"filled_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	o := &types.Order{
		Provider:       "alpaca_omnisub",
		ProviderID:     raw.ID,
		Symbol:         raw.Symbol,
		Qty:            raw.Qty,
		Notional:       raw.Notional,
		Side:           raw.Side,
		Type:           raw.Type,
		TimeInForce:    raw.TimeInForce,
		LimitPrice:     raw.LimitPrice,
		StopPrice:      raw.StopPrice,
		Status:         raw.Status,
		FilledQty:      raw.FilledQty,
		FilledAvgPrice: raw.FilledAvgPrice,
		AssetClass:     raw.AssetClass,
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.CreatedAt); err == nil {
		o.CreatedAt = t
	}
	if raw.FilledAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *raw.FilledAt); err == nil {
			o.FilledAt = &t
		}
	}
	return o, nil
}
