package lmax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// LMAX Digital — Institutional-grade FX and crypto exchange.
// Central Limit Order Book (CLOB), FCA regulated, sub-millisecond matching.
// Supports spot FX, crypto/USD pairs, and precious metals.

const (
	ProdURL    = "https://trade.lmax.com"
	SandboxURL = "https://web-order.london-demo.lmax.com"
)

type Config struct {
	BaseURL  string
	Username string
	Password string
	APIKey   string
}

type Provider struct {
	cfg       Config
	client    *http.Client
	sessionID string
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" { cfg.BaseURL = ProdURL }
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "lmax" }

var errNotSupported = fmt.Errorf("not supported by lmax")

func (p *Provider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil { b, _ := json.Marshal(body); reqBody = bytes.NewReader(b) }
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil { return nil, err }
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	if p.sessionID != "" {
		req.Header.Set("Cookie", "JSESSIONID="+p.sessionID)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 { return nil, fmt.Errorf("lmax error %d: %s", resp.StatusCode, string(data)) }
	return data, nil
}

// LMAX accounts are managed externally
func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) { return nil, errNotSupported }

func (p *Provider) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/api/account", nil)
	if err != nil { return nil, err }
	var resp struct{ AccountID string `json:"accountId"`; Status string `json:"status"`; Currency string `json:"currency"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: resp.AccountID, Provider: "lmax", ProviderID: resp.AccountID, Status: resp.Status, Currency: resp.Currency}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	acct, err := p.GetAccount(ctx, "")
	if err != nil { return nil, err }
	return []*types.Account{acct}, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, id string) (*types.Portfolio, error) {
	data, err := p.doRequest(ctx, "GET", "/api/account/balance", nil)
	if err != nil { return nil, err }
	var resp struct{ Cash float64 `json:"cash"`; Equity float64 `json:"equity"`; UnrealizedPL float64 `json:"unrealisedProfitAndLoss"`; Margin float64 `json:"margin"` }
	json.Unmarshal(data, &resp)
	return &types.Portfolio{
		AccountID: id,
		Cash:      fmt.Sprintf("%.2f", resp.Cash),
		Equity:    fmt.Sprintf("%.2f", resp.Equity),
	}, nil
}

func (p *Provider) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	body := map[string]interface{}{
		"instrumentId": req.Symbol,
		"quantity":     req.Qty,
		"timeInForce":  req.TimeInForce,
	}
	if req.Type == "limit" {
		body["price"] = req.LimitPrice
		body["type"] = "LIMIT"
	} else {
		body["type"] = "MARKET"
	}

	data, err := p.doRequest(ctx, "POST", "/api/order", body)
	if err != nil { return nil, err }
	var resp struct{ OrderID string `json:"orderId"`; Status string `json:"status"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.OrderID, Provider: "lmax", ProviderID: resp.OrderID, AccountID: accountID, Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Type: req.Type, Status: resp.Status, CreatedAt: time.Now()}, nil
}

func (p *Provider) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/api/order/active", nil)
	if err != nil { return nil, err }
	var items []struct{ OrderID string `json:"orderId"`; InstrumentID string `json:"instrumentId"`; Status string `json:"status"` }
	json.Unmarshal(data, &items)
	orders := make([]*types.Order, 0, len(items))
	for _, item := range items {
		orders = append(orders, &types.Order{ID: item.OrderID, Provider: "lmax", ProviderID: item.OrderID, Symbol: item.InstrumentID, Status: item.Status})
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/api/order/"+orderID, nil)
	if err != nil { return nil, err }
	var resp struct{ OrderID string `json:"orderId"`; Status string `json:"status"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.OrderID, Provider: "lmax", ProviderID: resp.OrderID, Status: resp.Status}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, accountID, orderID string) error {
	_, err := p.doRequest(ctx, "DELETE", "/api/order/"+orderID, nil)
	return err
}

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, errNotSupported }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, errNotSupported }

// LMAX instruments
func (p *Provider) ListAssets(_ context.Context, class string) ([]*types.Asset, error) {
	instruments := []struct{ sym, name, cls string }{
		// Spot FX
		{"EURUSD", "EUR/USD", "forex"}, {"GBPUSD", "GBP/USD", "forex"}, {"USDJPY", "USD/JPY", "forex"},
		{"USDCHF", "USD/CHF", "forex"}, {"AUDUSD", "AUD/USD", "forex"}, {"USDCAD", "USD/CAD", "forex"},
		{"NZDUSD", "NZD/USD", "forex"}, {"EURGBP", "EUR/GBP", "forex"}, {"EURJPY", "EUR/JPY", "forex"},
		// Crypto
		{"BTCUSD", "BTC/USD", "crypto"}, {"ETHUSD", "ETH/USD", "crypto"}, {"LTCUSD", "LTC/USD", "crypto"},
		{"XRPUSD", "XRP/USD", "crypto"}, {"BCHUSD", "BCH/USD", "crypto"}, {"SOLUSD", "SOL/USD", "crypto"},
		// Precious metals
		{"XAUUSD", "Gold/USD", "commodity"}, {"XAGUSD", "Silver/USD", "commodity"},
	}
	assets := make([]*types.Asset, 0)
	for _, i := range instruments {
		if class != "" && i.cls != class { continue }
		assets = append(assets, &types.Asset{ID: i.sym, Provider: "lmax", Symbol: i.sym, Name: i.name, Class: i.cls, Status: "active", Tradable: true})
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{ID: sym, Provider: "lmax", Symbol: sym, Class: "forex", Status: "active", Tradable: true}, nil
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	data, err := p.doRequest(ctx, "GET", "/api/marketdata/orderbook/"+symbol, nil)
	if err != nil { return nil, err }
	var resp struct {
		Bids []struct{ Price float64 `json:"price"`; Qty float64 `json:"quantity"` } `json:"bids"`
		Asks []struct{ Price float64 `json:"price"`; Qty float64 `json:"quantity"` } `json:"asks"`
	}
	json.Unmarshal(data, &resp)
	snap := &types.MarketSnapshot{Symbol: symbol}
	if len(resp.Bids) > 0 && len(resp.Asks) > 0 {
		snap.LatestQuote = &types.Quote{BidPrice: resp.Bids[0].Price, BidSize: resp.Bids[0].Qty, AskPrice: resp.Asks[0].Price, AskSize: resp.Asks[0].Qty}
		mid := (resp.Bids[0].Price + resp.Asks[0].Price) / 2
		snap.LatestTrade = &types.Trade{Price: mid}
	}
	return snap, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err == nil { result[sym] = snap }
	}
	return result, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) { return nil, errNotSupported }

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	snaps, err := p.GetSnapshots(ctx, symbols)
	if err != nil { return nil, err }
	result := make(map[string]*types.Trade)
	for sym, snap := range snaps { if snap.LatestTrade != nil { result[sym] = snap.LatestTrade } }
	return result, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	snaps, err := p.GetSnapshots(ctx, symbols)
	if err != nil { return nil, err }
	result := make(map[string]*types.Quote)
	for sym, snap := range snaps { if snap.LatestQuote != nil { result[sym] = snap.LatestQuote } }
	return result, nil
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	// FX/Crypto: 24/5 for FX, 24/7 for crypto
	now := time.Now().UTC()
	return &types.MarketClock{IsOpen: true, Timestamp: now.Format(time.RFC3339)}, nil
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) { return nil, errNotSupported }
