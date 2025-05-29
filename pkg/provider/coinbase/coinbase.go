package coinbase

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	ProdURL    = "https://api.coinbase.com/api/v3/brokerage"
	SandboxURL = "https://api-public.sandbox.exchange.coinbase.com"
)

// Config for Coinbase Advanced Trade API.
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

func (p *Provider) Name() string { return "coinbase" }

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(bodyBytes)
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	msg := ts + method + path + string(bodyBytes)
	mac := hmac.New(sha256.New, []byte(p.cfg.APISecret))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CB-ACCESS-KEY", p.cfg.APIKey)
	req.Header.Set("CB-ACCESS-SIGN", sig)
	req.Header.Set("CB-ACCESS-TIMESTAMP", ts)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("coinbase %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("coinbase: account creation via coinbase.com")
}

func (p *Provider) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/accounts/"+id, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Account struct {
			UUID     string `json:"uuid"`
			Name     string `json:"name"`
			Currency string `json:"currency"`
			Active   bool   `json:"active"`
		} `json:"account"`
	}
	json.Unmarshal(data, &raw)
	status := "inactive"
	if raw.Account.Active {
		status = "active"
	}
	return &types.Account{
		Provider: "coinbase", ProviderID: raw.Account.UUID,
		AccountNumber: raw.Account.Name, Currency: raw.Account.Currency,
		Status: status,
	}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/accounts?limit=50", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Accounts []struct {
			UUID     string `json:"uuid"`
			Name     string `json:"name"`
			Currency string `json:"currency"`
			Active   bool   `json:"active"`
			Balance  struct {
				Value string `json:"value"`
			} `json:"available_balance"`
		} `json:"accounts"`
	}
	json.Unmarshal(data, &raw)
	accts := make([]*types.Account, len(raw.Accounts))
	for i, a := range raw.Accounts {
		status := "inactive"
		if a.Active {
			status = "active"
		}
		accts[i] = &types.Account{
			Provider: "coinbase", ProviderID: a.UUID,
			AccountNumber: a.Name, Currency: a.Currency, Status: status,
		}
	}
	return accts, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	accts, err := p.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	return &types.Portfolio{Positions: make([]types.Position, 0, len(accts))}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	productID := normalizeProductID(req.Symbol)
	body := map[string]interface{}{
		"client_order_id": fmt.Sprintf("lux-%d", time.Now().UnixNano()),
		"product_id":      productID,
		"side":            strings.ToUpper(req.Side),
	}
	switch req.Type {
	case "market":
		if req.Side == "buy" {
			body["order_configuration"] = map[string]interface{}{
				"market_market_ioc": map[string]string{"quote_size": req.Qty},
			}
		} else {
			body["order_configuration"] = map[string]interface{}{
				"market_market_ioc": map[string]string{"base_size": req.Qty},
			}
		}
	case "limit":
		body["order_configuration"] = map[string]interface{}{
			"limit_limit_gtc": map[string]string{
				"base_size":   req.Qty,
				"limit_price": req.LimitPrice,
			},
		}
	}

	data, _, err := p.do(ctx, http.MethodPost, "/orders", body)
	if err != nil {
		return nil, err
	}
	var raw struct {
		OrderID string `json:"order_id"`
		Success bool   `json:"success"`
	}
	json.Unmarshal(data, &raw)
	status := "new"
	if !raw.Success {
		status = "rejected"
	}
	return &types.Order{
		Provider: "coinbase", ProviderID: raw.OrderID,
		Symbol: req.Symbol, Qty: req.Qty, Side: req.Side,
		Type: req.Type, Status: status, CreatedAt: time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/orders/historical/batch?limit=50", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Orders []struct {
			OrderID   string `json:"order_id"`
			ProductID string `json:"product_id"`
			Side      string `json:"side"`
			Status    string `json:"status"`
		} `json:"orders"`
	}
	json.Unmarshal(data, &raw)
	orders := make([]*types.Order, len(raw.Orders))
	for i, o := range raw.Orders {
		orders[i] = &types.Order{
			Provider: "coinbase", ProviderID: o.OrderID,
			Symbol: o.ProductID, Side: strings.ToLower(o.Side), Status: strings.ToLower(o.Status),
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/orders/historical/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Order struct {
			OrderID       string `json:"order_id"`
			ProductID     string `json:"product_id"`
			Side          string `json:"side"`
			Status        string `json:"status"`
			FilledSize    string `json:"filled_size"`
			AveragePrice  string `json:"average_filled_price"`
		} `json:"order"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider: "coinbase", ProviderID: raw.Order.OrderID,
		Symbol: raw.Order.ProductID, Side: strings.ToLower(raw.Order.Side),
		Status: strings.ToLower(raw.Order.Status),
		FilledQty: raw.Order.FilledSize, FilledAvgPrice: raw.Order.AveragePrice,
	}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, _ string, orderID string) error {
	_, _, err := p.do(ctx, http.MethodPost, "/orders/batch_cancel", map[string]interface{}{
		"order_ids": []string{orderID},
	})
	return err
}

// --- Stubs ---

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("coinbase: transfers via coinbase.com")
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return []*types.Transfer{}, nil
}
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("coinbase: bank linking via coinbase.com")
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/products?limit=250", nil)
	if err != nil {
		return defaultAssets(), nil
	}
	var raw struct {
		Products []struct {
			ProductID     string `json:"product_id"`
			BaseCurrency  string `json:"base_currency_id"`
			QuoteCurrency string `json:"quote_currency_id"`
			Status        string `json:"status"`
		} `json:"products"`
	}
	json.Unmarshal(data, &raw)
	assets := make([]*types.Asset, 0, len(raw.Products))
	for _, prod := range raw.Products {
		if prod.QuoteCurrency != "USD" && prod.QuoteCurrency != "USDC" {
			continue
		}
		assets = append(assets, &types.Asset{
			ID: prod.ProductID, Provider: "coinbase",
			Symbol: prod.BaseCurrency + "/" + prod.QuoteCurrency,
			Name: prod.ProductID, Class: "crypto",
			Status: "active", Tradable: true, Fractionable: true,
		})
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{
		ID: sym, Provider: "coinbase", Symbol: sym,
		Class: "crypto", Status: "active", Tradable: true,
	}, nil
}

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("coinbase: use websocket for real-time data")
}
func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("coinbase: use websocket for real-time data")
}
func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("coinbase: bars not yet implemented")
}
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("coinbase: not yet implemented")
}
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("coinbase: not yet implemented")
}
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return &types.MarketClock{Timestamp: time.Now().UTC().Format(time.RFC3339), IsOpen: true}, nil
}
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("coinbase: crypto is 24/7")
}

func normalizeProductID(sym string) string {
	s := strings.ToUpper(sym)
	s = strings.ReplaceAll(s, "/", "-")
	if !strings.Contains(s, "-") {
		if len(s) > 3 {
			return s[:len(s)-3] + "-" + s[len(s)-3:]
		}
	}
	return s
}

func defaultAssets() []*types.Asset {
	syms := []string{"BTC", "ETH", "SOL", "AVAX", "DOT", "LINK", "UNI", "AAVE", "MATIC", "ARB", "OP", "LTC", "XRP", "ADA", "DOGE", "SHIB", "ATOM", "NEAR"}
	out := make([]*types.Asset, len(syms))
	for i, s := range syms {
		out[i] = &types.Asset{ID: s + "-USD", Provider: "coinbase", Symbol: s + "/USD", Class: "crypto", Status: "active", Tradable: true, Fractionable: true}
	}
	return out
}
