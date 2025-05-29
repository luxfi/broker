package falcon

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
	SandboxURL = "https://api.sandbox.falconx.io"
	ProdURL    = "https://api.falconx.io"
)

// Config for FalconX provider.
// FalconX is an institutional crypto trading platform with RFQ (Request for Quote) model.
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
	Passphrase string `json:"passphrase"`
}

// Provider implements the broker Provider interface for FalconX.
type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Provider) Name() string { return "falcon" }

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("FX-ACCESS-KEY", p.cfg.APIKey)
	req.Header.Set("FX-ACCESS-PASSPHRASE", p.cfg.Passphrase)
	// FalconX uses HMAC-SHA256 signature — simplified here
	req.Header.Set("FX-ACCESS-SIGN", "")
	req.Header.Set("FX-ACCESS-TIMESTAMP", fmt.Sprintf("%d", time.Now().Unix()))

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("falcon %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("falcon: account creation handled via FalconX onboarding")
}

func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/balances", nil)
	if err != nil {
		return nil, err
	}
	_ = data
	return &types.Account{
		Provider:   "falcon",
		ProviderID: providerAccountID,
		Status:     "active",
		Currency:   "USD",
	}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	return []*types.Account{{
		Provider:   "falcon",
		ProviderID: "default",
		Status:     "active",
		Currency:   "USD",
	}}, nil
}

// --- Portfolio ---

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/balances", nil)
	if err != nil {
		return nil, err
	}
	var resp []struct {
		Platform string  `json:"platform"`
		Token    string  `json:"token"`
		Balance  float64 `json:"balance"`
	}
	json.Unmarshal(data, &resp)

	positions := make([]types.Position, len(resp))
	for i, b := range resp {
		positions[i] = types.Position{
			Symbol:     b.Token,
			Qty:        fmt.Sprintf("%.8f", b.Balance),
			AssetClass: "crypto",
		}
	}
	return &types.Portfolio{
		AccountID: "default",
		Positions: positions,
	}, nil
}

// --- Orders (RFQ model) ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	// FalconX uses RFQ: get quote first, then execute
	quoteBody := map[string]interface{}{
		"token_pair": map[string]string{
			"base_token":  req.Symbol,
			"quote_token": "USD",
		},
		"quantity": map[string]string{
			"token": req.Symbol,
			"value": req.Qty,
		},
		"side": req.Side,
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/quotes", quoteBody)
	if err != nil {
		return nil, err
	}
	var quote struct {
		FxQuoteID string  `json:"fx_quote_id"`
		BuyPrice  float64 `json:"buy_price"`
		SellPrice float64 `json:"sell_price"`
		Status    string  `json:"status"`
	}
	json.Unmarshal(data, &quote)

	// Execute the quote
	execData, _, err := p.do(ctx, http.MethodPost, "/v1/quotes/execute", map[string]string{
		"fx_quote_id": quote.FxQuoteID,
	})
	if err != nil {
		return nil, err
	}
	var exec struct {
		FxQuoteID string `json:"fx_quote_id"`
		Status    string `json:"status"`
	}
	json.Unmarshal(execData, &exec)

	return &types.Order{
		Provider:   "falcon",
		ProviderID: exec.FxQuoteID,
		Symbol:     req.Symbol,
		Qty:        req.Qty,
		Side:       req.Side,
		Type:       "rfq",
		Status:     exec.Status,
		CreatedAt:  time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trades", nil)
	if err != nil {
		return nil, err
	}
	var resp []struct {
		FxQuoteID string  `json:"fx_quote_id"`
		BaseToken string  `json:"base_token"`
		Side      string  `json:"side"`
		Quantity  float64 `json:"quantity"`
		Price     float64 `json:"price"`
		Status    string  `json:"status"`
		Timestamp string  `json:"t_execute"`
	}
	json.Unmarshal(data, &resp)

	orders := make([]*types.Order, len(resp))
	for i, t := range resp {
		orders[i] = &types.Order{
			Provider:       "falcon",
			ProviderID:     t.FxQuoteID,
			Symbol:         t.BaseToken,
			Side:           t.Side,
			Qty:            fmt.Sprintf("%.8f", t.Quantity),
			FilledAvgPrice: fmt.Sprintf("%.2f", t.Price),
			Type:           "rfq",
			Status:         t.Status,
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	orders, err := p.ListOrders(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		if o.ProviderID == orderID {
			return o, nil
		}
	}
	return nil, fmt.Errorf("falcon: order %s not found", orderID)
}

func (p *Provider) CancelOrder(_ context.Context, _, _ string) error {
	return fmt.Errorf("falcon: RFQ orders cannot be cancelled after execution")
}

// --- Transfers / Bank / Assets ---

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("falcon: transfers handled via settlement")
}

func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return []*types.Transfer{}, nil
}

func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("falcon: not applicable")
}

func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	pairs := []string{"BTC", "ETH", "SOL", "AVAX", "DOT", "LINK", "UNI", "AAVE", "MATIC", "ARB", "OP"}
	assets := make([]*types.Asset, len(pairs))
	for i, s := range pairs {
		assets[i] = &types.Asset{
			ID: s, Provider: "falcon", Symbol: s, Name: s + "/USD",
			Class: "crypto", Status: "active", Tradable: true,
		}
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, symbolOrID string) (*types.Asset, error) {
	return &types.Asset{
		ID: symbolOrID, Provider: "falcon", Symbol: symbolOrID,
		Class: "crypto", Status: "active", Tradable: true,
	}, nil
}

// --- Market Data ---

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("falcon: market data not supported")
}

func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("falcon: market data not supported")
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("falcon: market data not supported")
}

func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("falcon: market data not supported")
}

func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("falcon: market data not supported")
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, fmt.Errorf("falcon: clock not supported")
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("falcon: calendar not supported")
}
