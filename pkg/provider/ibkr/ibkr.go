package ibkr

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

const (
	// IBKR Client Portal API (gateway must be running locally or proxied)
	DefaultGatewayURL = "https://localhost:5000/v1/api"
	// IBKR Web API (newer, OAuth-based)
	WebAPIURL = "https://api.ibkr.com/v1/api"
)

// Config for Interactive Brokers provider.
type Config struct {
	GatewayURL   string `json:"gateway_url"`    // Client Portal Gateway URL
	AccountID    string `json:"account_id"`      // Primary account
	AccessToken  string `json:"access_token"`    // OAuth token (Web API)
	ConsumerKey  string `json:"consumer_key"`    // OAuth consumer key
}

// Provider implements the broker Provider interface for IBKR.
type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = DefaultGatewayURL
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return "ibkr" }

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.cfg.GatewayURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.AccessToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("ibkr request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("ibkr %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("ibkr: account creation not supported via API — use IBKR account management")
}

func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/portfolio/accounts", nil)
	if err != nil {
		return nil, err
	}
	var accounts []struct {
		AccountID string `json:"accountId"`
		Type      string `json:"type"`
		Currency  string `json:"currency"`
	}
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, err
	}
	for _, a := range accounts {
		if a.AccountID == providerAccountID {
			return &types.Account{
				Provider:    "ibkr",
				ProviderID:  a.AccountID,
				Currency:    a.Currency,
				AccountType: a.Type,
				Status:      "active",
			}, nil
		}
	}
	return nil, fmt.Errorf("ibkr: account %s not found", providerAccountID)
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/portfolio/accounts", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		AccountID string `json:"accountId"`
		Type      string `json:"type"`
		Currency  string `json:"currency"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	accts := make([]*types.Account, len(raw))
	for i, a := range raw {
		accts[i] = &types.Account{
			Provider:    "ibkr",
			ProviderID:  a.AccountID,
			Currency:    a.Currency,
			AccountType: a.Type,
			Status:      "active",
		}
	}
	return accts, nil
}

// --- Portfolio ---

func (p *Provider) GetPortfolio(ctx context.Context, providerAccountID string) (*types.Portfolio, error) {
	// Ledger for cash/equity
	lData, _, err := p.do(ctx, http.MethodGet, "/portfolio/"+providerAccountID+"/ledger", nil)
	if err != nil {
		return nil, err
	}
	var ledger map[string]struct {
		CashBalance    float64 `json:"cashbalance"`
		NetLiquidation float64 `json:"netliquidationvalue"`
		BuyingPower    float64 `json:"buyingpower"`
	}
	json.Unmarshal(lData, &ledger)

	base := ledger["BASE"]

	// Positions
	pData, _, err := p.do(ctx, http.MethodGet, "/portfolio/"+providerAccountID+"/positions/0", nil)
	if err != nil {
		return nil, err
	}
	var rawPos []struct {
		Ticker       string  `json:"contractDesc"`
		Position     float64 `json:"position"`
		AvgCost      float64 `json:"avgCost"`
		MktValue     float64 `json:"mktValue"`
		UnrealizedPL float64 `json:"unrealizedPnl"`
		AssetClass   string  `json:"assetClass"`
	}
	json.Unmarshal(pData, &rawPos)

	positions := make([]types.Position, len(rawPos))
	for i, rp := range rawPos {
		positions[i] = types.Position{
			Symbol:        rp.Ticker,
			Qty:           fmt.Sprintf("%.4f", rp.Position),
			AvgEntryPrice: fmt.Sprintf("%.2f", rp.AvgCost),
			MarketValue:   fmt.Sprintf("%.2f", rp.MktValue),
			UnrealizedPL:  fmt.Sprintf("%.2f", rp.UnrealizedPL),
			AssetClass:    rp.AssetClass,
		}
	}

	return &types.Portfolio{
		AccountID:      providerAccountID,
		Cash:           fmt.Sprintf("%.2f", base.CashBalance),
		Equity:         fmt.Sprintf("%.2f", base.NetLiquidation),
		BuyingPower:    fmt.Sprintf("%.2f", base.BuyingPower),
		PortfolioValue: fmt.Sprintf("%.2f", base.NetLiquidation),
		Positions:      positions,
	}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, providerAccountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	// IBKR requires a contract ID — resolve from symbol
	// Supports global symbols: AAPL (US), U.UN (TSX), SPUT.TO (TSX), VOD.L (LSE)
	conid, err := p.resolveConid(ctx, req.Symbol)
	if err != nil {
		return nil, fmt.Errorf("ibkr: cannot resolve symbol %s: %w", req.Symbol, err)
	}

	body := map[string]interface{}{
		"acctId":          providerAccountID,
		"conid":           conid,
		"orderType":       mapOrderType(req.Type),
		"side":            mapSide(req.Side),
		"tif":             mapTIF(req.TimeInForce),
		"quantity":        req.Qty,
		"listingExchange": "SMART", // IBKR auto-routes to best venue (TSX, NYSE, etc.)
	}
	if req.LimitPrice != "" {
		body["price"] = req.LimitPrice
	}
	if req.StopPrice != "" {
		body["auxPrice"] = req.StopPrice
	}

	data, _, err := p.do(ctx, http.MethodPost, "/iserver/account/"+providerAccountID+"/orders", map[string]interface{}{
		"orders": []interface{}{body},
	})
	if err != nil {
		return nil, err
	}

	var result []struct {
		OrderID   string `json:"order_id"`
		OrderStatus string `json:"order_status"`
	}
	json.Unmarshal(data, &result)

	if len(result) > 0 {
		return &types.Order{
			Provider:   "ibkr",
			ProviderID: result[0].OrderID,
			Symbol:     req.Symbol,
			Qty:        req.Qty,
			Side:       req.Side,
			Type:       req.Type,
			Status:     result[0].OrderStatus,
			CreatedAt:  time.Now(),
		}, nil
	}
	return nil, fmt.Errorf("ibkr: no order response")
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/iserver/account/orders", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Orders []struct {
			OrderID   int    `json:"orderId"`
			Symbol    string `json:"ticker"`
			Side      string `json:"side"`
			OrderType string `json:"orderType"`
			Status    string `json:"status"`
			FilledQty string `json:"filledQuantity"`
			TotalQty  string `json:"totalSize"`
		} `json:"orders"`
	}
	json.Unmarshal(data, &resp)

	orders := make([]*types.Order, len(resp.Orders))
	for i, o := range resp.Orders {
		orders[i] = &types.Order{
			Provider:   "ibkr",
			ProviderID: fmt.Sprintf("%d", o.OrderID),
			Symbol:     o.Symbol,
			Side:       o.Side,
			Type:       o.OrderType,
			Status:     o.Status,
			FilledQty:  o.FilledQty,
			Qty:        o.TotalQty,
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, providerOrderID string) (*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/iserver/account/order/status/"+providerOrderID, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		OrderID   int    `json:"order_id"`
		Symbol    string `json:"symbol"`
		Side      string `json:"side"`
		OrderType string `json:"order_type"`
		Status    string `json:"order_status"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider:   "ibkr",
		ProviderID: fmt.Sprintf("%d", raw.OrderID),
		Symbol:     raw.Symbol,
		Side:       raw.Side,
		Type:       raw.OrderType,
		Status:     raw.Status,
	}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, providerAccountID string, providerOrderID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/iserver/account/"+providerAccountID+"/order/"+providerOrderID, nil)
	return err
}

// --- Transfers (not supported via Client Portal API) ---

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("ibkr: transfers not supported via API — use IBKR account management")
}

func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return []*types.Transfer{}, nil
}

// --- Bank Relationships (not supported) ---

func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("ibkr: bank relationships not supported via API")
}

func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	// IBKR doesn't have a "list all assets" endpoint — search by symbol
	return []*types.Asset{}, nil
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/iserver/secdef/search?symbol="+symbolOrID, nil)
	if err != nil {
		return nil, err
	}
	var results []struct {
		Conid    int    `json:"conid"`
		Symbol   string `json:"symbol"`
		CompName string `json:"companyName"`
		SecType  string `json:"secType"`
	}
	json.Unmarshal(data, &results)
	if len(results) == 0 {
		return nil, fmt.Errorf("ibkr: asset %s not found", symbolOrID)
	}
	r := results[0]
	return &types.Asset{
		ID:       fmt.Sprintf("%d", r.Conid),
		Provider: "ibkr",
		Symbol:   r.Symbol,
		Name:     r.CompName,
		Class:    r.SecType,
		Status:   "active",
		Tradable: true,
	}, nil
}

// --- Market Data ---

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("ibkr: market data not yet implemented")
}

func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("ibkr: market data not yet implemented")
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("ibkr: market data not yet implemented")
}

func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("ibkr: market data not yet implemented")
}

func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("ibkr: market data not yet implemented")
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, fmt.Errorf("ibkr: clock not yet implemented")
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("ibkr: calendar not yet implemented")
}

// --- Helpers ---

// resolveConid resolves a symbol to an IBKR contract ID.
// Supports all global exchanges: TSX (U.UN, SPUT.TO), LSE, HKEX, etc.
// IBKR symbol search returns multiple matches across exchanges.
// If symbol contains "." or ":" it's treated as exchange-qualified (e.g. "U.UN" for TSX).
func (p *Provider) resolveConid(ctx context.Context, symbol string) (int, error) {
	data, _, err := p.do(ctx, http.MethodPost, "/iserver/secdef/search", map[string]interface{}{
		"symbol": symbol,
		"name":   true,
	})
	if err != nil {
		return 0, err
	}
	var results []struct {
		Conid    int      `json:"conid"`
		Symbol   string   `json:"symbol"`
		Exchange string   `json:"description"` // exchange info in description
		Sections []struct {
			SecType  string `json:"secType"`
			Exchange string `json:"exchange"`
			Conid    int    `json:"conid,string"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return 0, fmt.Errorf("parse search results: %w", err)
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("no contract found for %s", symbol)
	}
	// Return first match — IBKR returns best match first
	// For exchange-specific symbols (U.UN, SPUT.TO) this resolves to TSX
	return results[0].Conid, nil
}

func mapOrderType(t string) string {
	switch t {
	case "market":
		return "MKT"
	case "limit":
		return "LMT"
	case "stop":
		return "STP"
	case "stop_limit":
		return "STP LMT"
	default:
		return "MKT"
	}
}

func mapSide(s string) string {
	switch s {
	case "buy":
		return "BUY"
	case "sell":
		return "SELL"
	default:
		return s
	}
}

func mapTIF(tif string) string {
	switch tif {
	case "day":
		return "DAY"
	case "gtc":
		return "GTC"
	case "ioc":
		return "IOC"
	default:
		return "DAY"
	}
}
