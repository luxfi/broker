package circle

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
	ProdURL    = "https://api.circle.com"
	SandboxURL = "https://api-sandbox.circle.com"
)

type Config struct{ BaseURL, APIKey string }

type Provider struct{ cfg Config; client *http.Client }

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" { cfg.BaseURL = ProdURL }
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "circle" }

var errNotSupported = fmt.Errorf("not supported by circle")

func (p *Provider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil { b, _ := json.Marshal(body); reqBody = bytes.NewReader(b) }
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil { return nil, err }
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 { return nil, fmt.Errorf("circle error %d: %s", resp.StatusCode, string(data)) }
	return data, nil
}

func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	body := map[string]interface{}{"idempotencyKey": fmt.Sprintf("lux_%d", time.Now().UnixNano()), "description": req.Identity.GivenName}
	data, err := p.doRequest(ctx, "POST", "/v1/businessAccount/wallets", body)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ WalletID string `json:"walletId"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: resp.Data.WalletID, Provider: "circle", ProviderID: resp.Data.WalletID, Status: "active", Currency: "USD", CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (p *Provider) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/businessAccount/wallets/"+id, nil)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ WalletID string `json:"walletId"`; EntityID string `json:"entityId"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: resp.Data.WalletID, Provider: "circle", ProviderID: resp.Data.WalletID, Status: "active"}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/businessAccount/wallets", nil)
	if err != nil { return nil, err }
	var resp struct{ Data []struct{ WalletID string `json:"walletId"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	accts := make([]*types.Account, 0, len(resp.Data))
	for _, w := range resp.Data { accts = append(accts, &types.Account{ID: w.WalletID, Provider: "circle", ProviderID: w.WalletID, Status: "active"}) }
	return accts, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, id string) (*types.Portfolio, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/businessAccount/balances", nil)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ Available []struct{ Amount string `json:"amount"`; Currency string `json:"currency"` } `json:"available"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	positions := make([]types.Position, 0)
	for _, b := range resp.Data.Available { positions = append(positions, types.Position{Symbol: b.Currency, Qty: b.Amount, AssetClass: "crypto"}) }
	return &types.Portfolio{AccountID: id, Positions: positions}, nil
}

func (p *Provider) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	body := map[string]interface{}{"idempotencyKey": fmt.Sprintf("lux_%d", time.Now().UnixNano()), "source": map[string]string{"type": "wallet", "id": accountID}, "amount": map[string]string{"amount": req.Qty, "currency": "USD"}, "destination": map[string]string{"type": "blockchain", "chain": "ETH"}}
	data, err := p.doRequest(ctx, "POST", "/v1/businessAccount/transfers", body)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ ID string `json:"id"`; Status string `json:"status"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.Data.ID, Provider: "circle", ProviderID: resp.Data.ID, AccountID: accountID, Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Status: resp.Data.Status, CreatedAt: time.Now()}, nil
}

func (p *Provider) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/businessAccount/transfers", nil)
	if err != nil { return nil, err }
	var resp struct{ Data []struct{ ID string `json:"id"`; Status string `json:"status"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	orders := make([]*types.Order, 0, len(resp.Data))
	for _, t := range resp.Data { orders = append(orders, &types.Order{ID: t.ID, Provider: "circle", ProviderID: t.ID, Status: t.Status}) }
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/businessAccount/transfers/"+orderID, nil)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ ID string `json:"id"`; Status string `json:"status"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.Data.ID, Provider: "circle", ProviderID: resp.Data.ID, Status: resp.Data.Status}, nil
}

func (p *Provider) CancelOrder(_ context.Context, _, _ string) error { return errNotSupported }
func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, errNotSupported }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, errNotSupported }
func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	return []*types.Asset{
		{ID: "USDC", Provider: "circle", Symbol: "USDC", Name: "USD Coin", Class: "crypto", Status: "active", Tradable: true},
		{ID: "EURC", Provider: "circle", Symbol: "EURC", Name: "Euro Coin", Class: "crypto", Status: "active", Tradable: true},
	}, nil
}
func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{ID: sym, Provider: "circle", Symbol: sym, Class: "crypto", Status: "active", Tradable: true}, nil
}
func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) { return nil, errNotSupported }
func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) { return nil, errNotSupported }
func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) { return nil, errNotSupported }
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) { return nil, errNotSupported }
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) { return nil, errNotSupported }
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) { return nil, errNotSupported }
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) { return nil, errNotSupported }
