package sfox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

const (
	ProdURL    = "https://api.sfox.com/v1"
	SandboxURL = "https://api.sandbox.sfox.com/v1"
)

// Config for SFOX crypto prime dealer.
// SFOX aggregates liquidity from 30+ exchanges and OTC desks.
// Supports: smart order routing, algorithmic execution (TWAP/VWAP),
// OTC block trading, and settlement.
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
}

// Provider implements the broker Provider interface for SFOX.
type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = ProdURL
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return "sfox" }

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("sfox %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("sfox: account creation handled via SFOX onboarding portal")
}

func (p *Provider) GetAccount(ctx context.Context, _ string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/account", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID     int    `json:"id"`
		Email  string `json:"email"`
		Status string `json:"status"`
	}
	json.Unmarshal(data, &raw)
	return &types.Account{
		Provider:   "sfox",
		ProviderID: strconv.Itoa(raw.ID),
		Status:     raw.Status,
		Currency:   "USD",
		Contact:    &types.Contact{Email: raw.Email},
	}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	acct, err := p.GetAccount(ctx, "")
	if err != nil {
		return nil, err
	}
	return []*types.Account{acct}, nil
}

// --- Portfolio ---

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/user/balance", nil)
	if err != nil {
		return nil, err
	}
	// SFOX returns { "btc": {"balance": ..., "available": ...}, "usd": {...}, ... }
	var balances map[string]struct {
		Balance   float64 `json:"balance"`
		Available float64 `json:"available"`
	}
	if err := json.Unmarshal(data, &balances); err != nil {
		return nil, err
	}

	var positions []types.Position
	var cash string
	for coin, bal := range balances {
		if strings.EqualFold(coin, "usd") {
			cash = fmt.Sprintf("%.2f", bal.Balance)
			continue
		}
		if bal.Balance > 0 {
			positions = append(positions, types.Position{
				Symbol:     strings.ToUpper(coin),
				Qty:        fmt.Sprintf("%.8f", bal.Balance),
				AssetClass: "crypto",
			})
		}
	}

	return &types.Portfolio{
		Cash:      cash,
		Positions: positions,
	}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	// SFOX supports: market, limit, and algo orders (TWAP, VWAP, Polar, Iceberg)
	body := map[string]interface{}{
		"pair":     normalizePair(req.Symbol),
		"side":     req.Side,
		"type":     mapOrderType(req.Type),
		"quantity": req.Qty,
	}
	if req.LimitPrice != "" {
		body["price"] = req.LimitPrice
	}

	data, _, err := p.do(ctx, http.MethodPost, "/orders", body)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Price  string `json:"price"`
		Filled string `json:"filled"`
	}
	json.Unmarshal(data, &raw)

	return &types.Order{
		Provider:       "sfox",
		ProviderID:     strconv.Itoa(raw.ID),
		Symbol:         req.Symbol,
		Qty:            req.Qty,
		Side:           req.Side,
		Type:           req.Type,
		Status:         mapStatus(raw.Status),
		FilledAvgPrice: raw.Price,
		FilledQty:      raw.Filled,
		CreatedAt:      time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/orders", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID        int    `json:"id"`
		Pair      string `json:"pair"`
		Side      string `json:"side"`
		Type      string `json:"type"`
		Status    string `json:"status"`
		Quantity  string `json:"quantity"`
		Price     string `json:"price"`
		Filled    string `json:"filled"`
		CreatedAt string `json:"date_added"`
	}
	json.Unmarshal(data, &raw)

	orders := make([]*types.Order, len(raw))
	for i, o := range raw {
		orders[i] = &types.Order{
			Provider:       "sfox",
			ProviderID:     strconv.Itoa(o.ID),
			Symbol:         o.Pair,
			Side:           o.Side,
			Type:           o.Type,
			Status:         mapStatus(o.Status),
			Qty:            o.Quantity,
			FilledQty:      o.Filled,
			FilledAvgPrice: o.Price,
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID       int    `json:"id"`
		Pair     string `json:"pair"`
		Side     string `json:"side"`
		Type     string `json:"type"`
		Status   string `json:"status"`
		Quantity string `json:"quantity"`
		Price    string `json:"price"`
		Filled   string `json:"filled"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider:       "sfox",
		ProviderID:     strconv.Itoa(raw.ID),
		Symbol:         raw.Pair,
		Side:           raw.Side,
		Type:           raw.Type,
		Status:         mapStatus(raw.Status),
		Qty:            raw.Quantity,
		FilledQty:      raw.Filled,
		FilledAvgPrice: raw.Price,
	}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, _ string, orderID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/orders/"+orderID, nil)
	return err
}

// --- Transfers ---

func (p *Provider) CreateTransfer(ctx context.Context, _ string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	if req.Direction == "outgoing" {
		// Crypto withdrawal
		body := map[string]interface{}{
			"currency": req.Type,
			"amount":   req.Amount,
			"address":  req.RelationshipID,
		}
		data, _, err := p.do(ctx, http.MethodPost, "/user/withdraw", body)
		if err != nil {
			return nil, err
		}
		var raw struct {
			ID string `json:"id"`
		}
		json.Unmarshal(data, &raw)
		return &types.Transfer{
			Provider:   "sfox",
			ProviderID: raw.ID,
			Type:       "crypto",
			Direction:  "outgoing",
			Amount:     req.Amount,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}, nil
	}
	return nil, fmt.Errorf("sfox: incoming transfers via SFOX dashboard")
}

func (p *Provider) ListTransfers(ctx context.Context, _ string) ([]*types.Transfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/user/transactions", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID       string  `json:"id"`
		Type     string  `json:"action"` // deposit, withdrawal
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
		Status   string  `json:"status"`
		Date     string  `json:"date_added"`
	}
	json.Unmarshal(data, &raw)

	transfers := make([]*types.Transfer, len(raw))
	for i, t := range raw {
		dir := "incoming"
		if t.Type == "withdrawal" {
			dir = "outgoing"
		}
		transfers[i] = &types.Transfer{
			Provider:   "sfox",
			ProviderID: t.ID,
			Type:       "crypto",
			Direction:  dir,
			Amount:     fmt.Sprintf("%.8f", t.Amount),
			Currency:   t.Currency,
			Status:     t.Status,
		}
	}
	return transfers, nil
}

// --- Bank Relationships ---

func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("sfox: bank linking via SFOX dashboard")
}

func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/markets/currency-pairs", nil)
	if err != nil {
		// Fallback to known SFOX pairs
		return p.defaultAssets(), nil
	}
	var pairs []struct {
		Pair          string `json:"formatted_symbol"`
		BaseCurrency  string `json:"base_currency"`
		QuoteCurrency string `json:"quote_currency"`
	}
	if err := json.Unmarshal(data, &pairs); err != nil {
		return p.defaultAssets(), nil
	}
	assets := make([]*types.Asset, len(pairs))
	for i, pair := range pairs {
		assets[i] = &types.Asset{
			ID:       pair.Pair,
			Provider: "sfox",
			Symbol:   pair.BaseCurrency + "/" + pair.QuoteCurrency,
			Name:     pair.BaseCurrency + "/" + pair.QuoteCurrency,
			Class:    "crypto",
			Status:   "active",
			Tradable: true,
		}
	}
	return assets, nil
}

func (p *Provider) defaultAssets() []*types.Asset {
	pairs := []string{
		"BTC", "ETH", "SOL", "AVAX", "DOT", "LINK", "UNI", "AAVE",
		"MATIC", "ARB", "OP", "LTC", "XRP", "ADA", "DOGE", "SHIB",
		"ATOM", "NEAR", "APT", "SUI", "TIA", "SEI", "INJ", "FTM",
	}
	assets := make([]*types.Asset, len(pairs))
	for i, s := range pairs {
		assets[i] = &types.Asset{
			ID: s + "USD", Provider: "sfox", Symbol: s + "/USD",
			Name: s + "/USD", Class: "crypto", Status: "active", Tradable: true,
		}
	}
	return assets
}

func (p *Provider) GetAsset(_ context.Context, symbolOrID string) (*types.Asset, error) {
	sym := strings.ToUpper(symbolOrID)
	return &types.Asset{
		ID: sym, Provider: "sfox", Symbol: sym,
		Class: "crypto", Status: "active", Tradable: true,
	}, nil
}

// --- Market Data ---

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	pair := normalizePair(symbol)
	data, _, err := p.do(ctx, http.MethodGet, "/markets/best-price/"+pair, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		BuyPrice  float64 `json:"buyPrice"`
		SellPrice float64 `json:"sellPrice"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Open      float64 `json:"open"`
		Last      float64 `json:"last"`
		Volume    float64 `json:"volume"`
		VWAP      float64 `json:"vwap"`
	}
	json.Unmarshal(data, &raw)

	return &types.MarketSnapshot{
		Symbol: symbol,
		LatestQuote: &types.Quote{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			BidPrice:  raw.SellPrice, // SFOX sell = maker's bid
			AskPrice:  raw.BuyPrice,  // SFOX buy = maker's ask
			BidSize:   0,
			AskSize:   0,
		},
		LatestTrade: &types.Trade{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Price:     raw.Last,
			Size:      0,
		},
		DailyBar: &types.Bar{
			Open:   raw.Open,
			High:   raw.High,
			Low:    raw.Low,
			Close:  raw.Last,
			Volume: raw.Volume,
			VWAP:   raw.VWAP,
		},
	}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot, len(symbols))
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err != nil {
			continue
		}
		result[sym] = snap
	}
	return result, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("sfox: historical bars not yet implemented")
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	result := make(map[string]*types.Trade, len(symbols))
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err != nil || snap.LatestTrade == nil {
			continue
		}
		result[sym] = snap.LatestTrade
	}
	return result, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	result := make(map[string]*types.Quote, len(symbols))
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err != nil || snap.LatestQuote == nil {
			continue
		}
		result[sym] = snap.LatestQuote
	}
	return result, nil
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	// Crypto markets are 24/7
	return &types.MarketClock{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		IsOpen:    true,
		NextOpen:  time.Now().UTC().Format(time.RFC3339),
		NextClose: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}, nil
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("sfox: crypto markets are 24/7")
}

// --- Helpers ---

// normalizePair converts "BTC/USD" or "BTCUSD" to "btcusd" for SFOX API.
func normalizePair(symbol string) string {
	s := strings.ToLower(symbol)
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

func mapOrderType(t string) string {
	switch t {
	case "market":
		return "market"
	case "limit":
		return "limit"
	default:
		return "market"
	}
}

func mapStatus(s string) string {
	switch strings.ToLower(s) {
	case "started", "pending":
		return "new"
	case "cancel pending":
		return "pending_cancel"
	case "done", "filled":
		return "filled"
	case "canceled", "cancelled":
		return "cancelled"
	default:
		return s
	}
}
