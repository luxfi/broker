package fireblocks

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
	ProdURL    = "https://api.fireblocks.io"
	SandboxURL = "https://sandbox-api.fireblocks.io"
)

type Config struct {
	BaseURL        string
	APIKey         string
	PrivateKeyPEM  string // RSA private key for JWT signing
	VaultAccountID string
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

func (p *Provider) Name() string { return "fireblocks" }

var errNotSupported = fmt.Errorf("not supported by fireblocks")

func (p *Provider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+"/v1"+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	// In production, add JWT Authorization header signed with RSA private key

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fireblocks error %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	body := map[string]interface{}{"name": req.Identity.GivenName + " " + req.Identity.FamilyName}
	data, err := p.doRequest(ctx, "POST", "/vault/accounts", body)
	if err != nil {
		return nil, err
	}
	var resp struct{ ID string `json:"id"`; Name string `json:"name"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: resp.ID, Provider: "fireblocks", ProviderID: resp.ID, Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (p *Provider) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/vault/accounts/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct{ ID string `json:"id"`; Name string `json:"name"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: resp.ID, Provider: "fireblocks", ProviderID: resp.ID, Status: "active"}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/vault/accounts", nil)
	if err != nil {
		return nil, err
	}
	var items []struct{ ID string `json:"id"`; Name string `json:"name"` }
	json.Unmarshal(data, &items)
	accounts := make([]*types.Account, 0, len(items))
	for _, item := range items {
		accounts = append(accounts, &types.Account{ID: item.ID, Provider: "fireblocks", ProviderID: item.ID, Status: "active"})
	}
	return accounts, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, id string) (*types.Portfolio, error) {
	data, err := p.doRequest(ctx, "GET", "/vault/accounts/"+id, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Assets []struct{ ID string `json:"id"`; Balance string `json:"balance"`; Available string `json:"available"` } `json:"assets"`
	}
	json.Unmarshal(data, &resp)
	positions := make([]types.Position, 0, len(resp.Assets))
	for _, a := range resp.Assets {
		positions = append(positions, types.Position{Symbol: a.ID, Qty: a.Balance, AssetClass: "crypto"})
	}
	return &types.Portfolio{AccountID: id, Positions: positions}, nil
}

func (p *Provider) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	body := map[string]interface{}{
		"assetId": req.Symbol, "amount": req.Qty,
		"source": map[string]string{"type": "VAULT_ACCOUNT", "id": accountID},
		"operation": "TRANSFER",
	}
	data, err := p.doRequest(ctx, "POST", "/transactions", body)
	if err != nil {
		return nil, err
	}
	var resp struct{ ID string `json:"id"`; Status string `json:"status"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.ID, Provider: "fireblocks", ProviderID: resp.ID, AccountID: accountID, Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Type: req.Type, Status: resp.Status, CreatedAt: time.Now()}, nil
}

func (p *Provider) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/transactions", nil)
	if err != nil {
		return nil, err
	}
	var items []struct{ ID string `json:"id"`; Status string `json:"status"`; AssetID string `json:"assetId"` }
	json.Unmarshal(data, &items)
	orders := make([]*types.Order, 0, len(items))
	for _, item := range items {
		orders = append(orders, &types.Order{ID: item.ID, Provider: "fireblocks", ProviderID: item.ID, Symbol: item.AssetID, Status: item.Status})
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/transactions/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	var resp struct{ ID string `json:"id"`; Status string `json:"status"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.ID, Provider: "fireblocks", ProviderID: resp.ID, Status: resp.Status}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, accountID, orderID string) error {
	_, err := p.doRequest(ctx, "POST", "/transactions/"+orderID+"/cancel", nil)
	return err
}

func (p *Provider) CreateTransfer(ctx context.Context, accountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, errNotSupported
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return nil, errNotSupported
}
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, errNotSupported
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return nil, errNotSupported
}

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	data, err := p.doRequest(ctx, "GET", "/supported_assets", nil)
	if err != nil {
		return nil, err
	}
	var items []struct{ ID string `json:"id"`; Name string `json:"name"`; Type string `json:"type"` }
	json.Unmarshal(data, &items)
	assets := make([]*types.Asset, 0, len(items))
	for _, item := range items {
		assets = append(assets, &types.Asset{ID: item.ID, Provider: "fireblocks", Symbol: item.ID, Name: item.Name, Class: "crypto", Status: "active", Tradable: true})
	}
	return assets, nil
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	return &types.Asset{ID: symbolOrID, Provider: "fireblocks", Symbol: symbolOrID, Class: "crypto", Status: "active", Tradable: true}, nil
}

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) { return nil, errNotSupported }
func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) { return nil, errNotSupported }
func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) { return nil, errNotSupported }
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) { return nil, errNotSupported }
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) { return nil, errNotSupported }
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) { return nil, errNotSupported }
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) { return nil, errNotSupported }
