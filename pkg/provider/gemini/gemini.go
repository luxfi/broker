package gemini

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
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
	ProdURL    = "https://api.gemini.com"
	SandboxURL = "https://api.sandbox.gemini.com"
)

// Config for Gemini exchange.
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = ProdURL
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "gemini" }

func (p *Provider) doPublic(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (p *Provider) doPrivate(ctx context.Context, path string, payload map[string]interface{}) ([]byte, error) {
	if payload == nil {
		payload = make(map[string]interface{})
	}
	payload["request"] = "/v1" + path
	payload["nonce"] = strconv.FormatInt(time.Now().UnixNano(), 10)

	payloadJSON, _ := json.Marshal(payload)
	b64 := base64.StdEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha512.New384, []byte(p.cfg.APISecret))
	mac.Write([]byte(b64))
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/v1"+path, bytes.NewReader(payloadJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-GEMINI-APIKEY", p.cfg.APIKey)
	req.Header.Set("X-GEMINI-PAYLOAD", b64)
	req.Header.Set("X-GEMINI-SIGNATURE", sig)
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("gemini: account creation via gemini.com")
}

func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error) {
	return &types.Account{Provider: "gemini", ProviderID: "default", Status: "active", Currency: "USD"}, nil
}

func (p *Provider) ListAccounts(_ context.Context) ([]*types.Account, error) {
	return []*types.Account{{Provider: "gemini", ProviderID: "default", Status: "active", Currency: "USD"}}, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	data, err := p.doPrivate(ctx, "/balances", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Currency  string `json:"currency"`
		Amount    string `json:"amount"`
		Available string `json:"available"`
	}
	json.Unmarshal(data, &raw)

	var positions []types.Position
	var cash string
	for _, b := range raw {
		amt, _ := strconv.ParseFloat(b.Amount, 64)
		if amt <= 0 {
			continue
		}
		if strings.EqualFold(b.Currency, "USD") {
			cash = fmt.Sprintf("%.2f", amt)
			continue
		}
		positions = append(positions, types.Position{
			Symbol: strings.ToUpper(b.Currency), Qty: b.Amount, AssetClass: "crypto",
		})
	}
	return &types.Portfolio{Cash: cash, Positions: positions}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	sym := geminiSymbol(req.Symbol)
	payload := map[string]interface{}{
		"symbol":   sym,
		"amount":   req.Qty,
		"side":     req.Side,
		"type":     "exchange " + req.Type, // Gemini: "exchange limit", "exchange market"
	}
	if req.LimitPrice != "" {
		payload["price"] = req.LimitPrice
	} else {
		// Gemini requires price even for market — use a placeholder
		payload["price"] = "1"
		payload["options"] = []string{"immediate-or-cancel"}
	}

	data, err := p.doPrivate(ctx, "/order/new", payload)
	if err != nil {
		return nil, err
	}
	var raw struct {
		OrderID       string `json:"order_id"`
		Symbol        string `json:"symbol"`
		Side          string `json:"side"`
		Type          string `json:"type"`
		Price         string `json:"price"`
		AvgPrice      string `json:"avg_execution_price"`
		ExecutedAmount string `json:"executed_amount"`
		OriginalAmount string `json:"original_amount"`
		IsLive        bool   `json:"is_live"`
		IsCancelled   bool   `json:"is_cancelled"`
	}
	json.Unmarshal(data, &raw)

	status := "new"
	if raw.IsCancelled {
		status = "cancelled"
	} else if raw.ExecutedAmount == raw.OriginalAmount && raw.ExecutedAmount != "0" {
		status = "filled"
	}

	return &types.Order{
		Provider: "gemini", ProviderID: raw.OrderID,
		Symbol: req.Symbol, Qty: raw.OriginalAmount, Side: raw.Side,
		Type: req.Type, Status: status,
		FilledQty: raw.ExecutedAmount, FilledAvgPrice: raw.AvgPrice,
		CreatedAt: time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, err := p.doPrivate(ctx, "/orders", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		OrderID        string `json:"order_id"`
		Symbol         string `json:"symbol"`
		Side           string `json:"side"`
		Type           string `json:"type"`
		OriginalAmount string `json:"original_amount"`
		ExecutedAmount string `json:"executed_amount"`
		Price          string `json:"price"`
	}
	json.Unmarshal(data, &raw)
	orders := make([]*types.Order, len(raw))
	for i, o := range raw {
		orders[i] = &types.Order{
			Provider: "gemini", ProviderID: o.OrderID,
			Symbol: o.Symbol, Side: o.Side,
			Qty: o.OriginalAmount, FilledQty: o.ExecutedAmount,
			Status: "open",
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	data, err := p.doPrivate(ctx, "/order/status", map[string]interface{}{"order_id": orderID})
	if err != nil {
		return nil, err
	}
	var raw struct {
		OrderID        string `json:"order_id"`
		Symbol         string `json:"symbol"`
		Side           string `json:"side"`
		OriginalAmount string `json:"original_amount"`
		ExecutedAmount string `json:"executed_amount"`
		AvgPrice       string `json:"avg_execution_price"`
		IsLive         bool   `json:"is_live"`
		IsCancelled    bool   `json:"is_cancelled"`
	}
	json.Unmarshal(data, &raw)
	status := "filled"
	if raw.IsLive {
		status = "open"
	}
	if raw.IsCancelled {
		status = "cancelled"
	}
	return &types.Order{
		Provider: "gemini", ProviderID: raw.OrderID,
		Symbol: raw.Symbol, Side: raw.Side,
		Qty: raw.OriginalAmount, FilledQty: raw.ExecutedAmount,
		FilledAvgPrice: raw.AvgPrice, Status: status,
	}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, _ string, orderID string) error {
	_, err := p.doPrivate(ctx, "/order/cancel", map[string]interface{}{"order_id": orderID})
	return err
}

// --- Stubs ---

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("gemini: transfers via gemini.com")
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return []*types.Transfer{}, nil
}
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("gemini: bank linking via gemini.com")
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	data, err := p.doPublic(ctx, "/v1/symbols")
	if err != nil {
		return defaultAssets(), nil
	}
	var symbols []string
	json.Unmarshal(data, &symbols)
	assets := make([]*types.Asset, 0)
	for _, sym := range symbols {
		if !strings.HasSuffix(strings.ToLower(sym), "usd") {
			continue
		}
		base := strings.ToUpper(sym[:len(sym)-3])
		assets = append(assets, &types.Asset{
			ID: sym, Provider: "gemini", Symbol: base + "/USD",
			Class: "crypto", Status: "active", Tradable: true,
		})
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{ID: sym, Provider: "gemini", Symbol: sym, Class: "crypto", Status: "active", Tradable: true}, nil
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	sym := geminiSymbol(symbol)
	data, err := p.doPublic(ctx, "/v1/pubticker/"+sym)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Bid    string `json:"bid"`
		Ask    string `json:"ask"`
		Last   string `json:"last"`
		Volume struct {
			USD string `json:"USD"`
		} `json:"volume"`
	}
	json.Unmarshal(data, &raw)
	bid, _ := strconv.ParseFloat(raw.Bid, 64)
	ask, _ := strconv.ParseFloat(raw.Ask, 64)
	last, _ := strconv.ParseFloat(raw.Last, 64)
	vol, _ := strconv.ParseFloat(raw.Volume.USD, 64)

	return &types.MarketSnapshot{
		Symbol: symbol,
		LatestQuote: &types.Quote{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			BidPrice: bid, AskPrice: ask,
		},
		LatestTrade: &types.Trade{Timestamp: time.Now().UTC().Format(time.RFC3339), Price: last},
		DailyBar:    &types.Bar{Close: last, Volume: vol},
	}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, syms []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, s := range syms {
		if snap, err := p.GetSnapshot(ctx, s); err == nil {
			result[s] = snap
		}
	}
	return result, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("gemini: bars not yet implemented")
}
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("gemini: not yet implemented")
}
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("gemini: not yet implemented")
}
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return &types.MarketClock{Timestamp: time.Now().UTC().Format(time.RFC3339), IsOpen: true}, nil
}
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("gemini: crypto 24/7")
}

func geminiSymbol(sym string) string {
	s := strings.ToLower(sym)
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

func defaultAssets() []*types.Asset {
	syms := []string{"BTC", "ETH", "SOL", "AVAX", "DOT", "LINK", "UNI", "LTC", "DOGE", "SHIB", "MATIC"}
	out := make([]*types.Asset, len(syms))
	for i, s := range syms {
		out[i] = &types.Asset{ID: s + "USD", Provider: "gemini", Symbol: s + "/USD", Class: "crypto", Status: "active", Tradable: true}
	}
	return out
}
