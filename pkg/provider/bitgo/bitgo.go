package bitgo

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
	TestURL = "https://app.bitgo-test.com/api/v2"
	ProdURL = "https://app.bitgo.com/api/v2"
)

// Config for the BitGo provider.
type Config struct {
	BaseURL     string `json:"base_url"`
	AccessToken string `json:"access_token"`
	Enterprise  string `json:"enterprise"` // enterprise ID
}

// Provider implements the broker Provider interface for BitGo.
// BitGo is primarily a custody/wallet provider, not a traditional broker.
// It supports crypto wallets, transfers, and trading via its Prime product.
type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = TestURL
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return "bitgo" }

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.AccessToken)
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
		return nil, resp.StatusCode, fmt.Errorf("bitgo %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Accounts (BitGo enterprises/wallets) ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("bitgo: use wallet creation APIs instead")
}

func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	// In BitGo, "account" maps to enterprise
	data, _, err := p.do(ctx, http.MethodGet, "/enterprise/"+providerAccountID, nil)
	if err != nil {
		return nil, err
	}
	var ent struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(data, &ent)
	return &types.Account{
		Provider:      "bitgo",
		ProviderID:    ent.ID,
		AccountNumber: ent.Name,
		Status:        "active",
		Currency:      "USD",
	}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	// List wallets across all coins
	data, _, err := p.do(ctx, http.MethodGet, "/wallets", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Wallets []struct {
			ID    string `json:"id"`
			Coin  string `json:"coin"`
			Label string `json:"label"`
		} `json:"wallets"`
	}
	json.Unmarshal(data, &resp)

	accts := make([]*types.Account, len(resp.Wallets))
	for i, w := range resp.Wallets {
		accts[i] = &types.Account{
			Provider:      "bitgo",
			ProviderID:    w.ID,
			AccountNumber: w.Label,
			Currency:      w.Coin,
			Status:        "active",
			AccountType:   "wallet",
		}
	}
	return accts, nil
}

// --- Portfolio ---

func (p *Provider) GetPortfolio(ctx context.Context, providerAccountID string) (*types.Portfolio, error) {
	// Get wallet balance
	data, _, err := p.do(ctx, http.MethodGet, "/wallet/"+providerAccountID, nil)
	if err != nil {
		return nil, err
	}
	var wallet struct {
		Coin    string `json:"coin"`
		Balance int64  `json:"balance"`
		// Balance is in base units (satoshi for BTC, wei for ETH, etc.)
	}
	json.Unmarshal(data, &wallet)

	return &types.Portfolio{
		AccountID:      providerAccountID,
		Cash:           fmt.Sprintf("%d", wallet.Balance),
		PortfolioValue: fmt.Sprintf("%d", wallet.Balance),
		Positions: []types.Position{
			{
				Symbol:      wallet.Coin,
				Qty:         fmt.Sprintf("%d", wallet.Balance),
				AssetClass:  "crypto",
			},
		},
	}, nil
}

// --- Orders (BitGo Prime Trading) ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	body := map[string]interface{}{
		"product":  req.Symbol,
		"side":     req.Side,
		"type":     req.Type,
		"quantity": req.Qty,
	}
	if req.LimitPrice != "" {
		body["limitPrice"] = req.LimitPrice
	}

	data, _, err := p.do(ctx, http.MethodPost, "/prime/trading/orders", body)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider:   "bitgo",
		ProviderID: raw.ID,
		Symbol:     req.Symbol,
		Qty:        req.Qty,
		Side:       req.Side,
		Type:       req.Type,
		Status:     raw.Status,
		CreatedAt:  time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/prime/trading/orders", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Orders []struct {
			ID      string `json:"id"`
			Product string `json:"product"`
			Side    string `json:"side"`
			Status  string `json:"status"`
			Qty     string `json:"quantity"`
		} `json:"orders"`
	}
	json.Unmarshal(data, &resp)
	orders := make([]*types.Order, len(resp.Orders))
	for i, o := range resp.Orders {
		orders[i] = &types.Order{
			Provider:   "bitgo",
			ProviderID: o.ID,
			Symbol:     o.Product,
			Side:       o.Side,
			Status:     o.Status,
			Qty:        o.Qty,
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/prime/trading/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID      string `json:"id"`
		Product string `json:"product"`
		Side    string `json:"side"`
		Status  string `json:"status"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider:   "bitgo",
		ProviderID: raw.ID,
		Symbol:     raw.Product,
		Side:       raw.Side,
		Status:     raw.Status,
	}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, _ string, orderID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/prime/trading/orders/"+orderID, nil)
	return err
}

// --- Transfers (crypto sends) ---

func (p *Provider) CreateTransfer(ctx context.Context, providerAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	// BitGo: providerAccountID is wallet ID, req has amount/direction
	// This is a crypto send, not ACH
	return nil, fmt.Errorf("bitgo: use wallet send APIs directly for crypto transfers")
}

func (p *Provider) ListTransfers(ctx context.Context, providerAccountID string) ([]*types.Transfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/wallet/"+providerAccountID+"/transfers", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Transfers []struct {
			ID    string `json:"id"`
			Coin  string `json:"coin"`
			Value int64  `json:"value"`
			State string `json:"state"`
			TxID  string `json:"txid"`
			Date  string `json:"date"`
		} `json:"transfers"`
	}
	json.Unmarshal(data, &resp)

	transfers := make([]*types.Transfer, len(resp.Transfers))
	for i, t := range resp.Transfers {
		dir := "incoming"
		if t.Value < 0 {
			dir = "outgoing"
		}
		transfers[i] = &types.Transfer{
			Provider:   "bitgo",
			ProviderID: t.ID,
			AccountID:  providerAccountID,
			Type:       "crypto",
			Direction:  dir,
			Amount:     fmt.Sprintf("%d", t.Value),
			Currency:   t.Coin,
			Status:     t.State,
		}
		if ts, err := time.Parse(time.RFC3339, t.Date); err == nil {
			transfers[i].CreatedAt = ts
		}
	}
	return transfers, nil
}

// --- Bank Relationships (not applicable) ---

func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("bitgo: bank relationships not applicable for crypto custody")
}

func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	// BitGo supported coins
	coins := []struct {
		symbol string
		name   string
	}{
		{"btc", "Bitcoin"}, {"eth", "Ethereum"}, {"ltc", "Litecoin"},
		{"xrp", "XRP"}, {"xlm", "Stellar"}, {"eos", "EOS"},
		{"trx", "Tron"}, {"sol", "Solana"}, {"avax", "Avalanche"},
		{"dot", "Polkadot"}, {"algo", "Algorand"}, {"near", "NEAR"},
		{"usdt", "Tether"}, {"usdc", "USD Coin"}, {"dai", "Dai"},
	}
	assets := make([]*types.Asset, len(coins))
	for i, c := range coins {
		assets[i] = &types.Asset{
			ID:       c.symbol,
			Provider: "bitgo",
			Symbol:   c.symbol,
			Name:     c.name,
			Class:    "crypto",
			Status:   "active",
			Tradable: true,
		}
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, symbolOrID string) (*types.Asset, error) {
	return &types.Asset{
		ID:       symbolOrID,
		Provider: "bitgo",
		Symbol:   symbolOrID,
		Class:    "crypto",
		Status:   "active",
		Tradable: true,
	}, nil
}

// --- Market Data ---

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("bitgo: market data not supported")
}

func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("bitgo: market data not supported")
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("bitgo: market data not supported")
}

func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("bitgo: market data not supported")
}

func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("bitgo: market data not supported")
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, fmt.Errorf("bitgo: clock not supported")
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("bitgo: calendar not supported")
}
