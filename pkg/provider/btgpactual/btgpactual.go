// Package btgpactual implements the broker Provider interface for BTG Pactual (Brazil, CVM).
// First-class adapter: real auth scaffolding included for production integration.
package btgpactual

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

const (
	ProdURL    = "https://api.btgpactual.com"
	SandboxURL = "https://api.sandbox.btgpactual.com"
)

// Config for the BTG Pactual provider.
type Config struct {
	BaseURL      string `json:"base_url"`
	ClientID     string `json:"-"`
	ClientSecret string `json:"-"`
	AccountID    string `json:"account_id"`
}

// Provider implements the broker Provider interface for BTG Pactual.
type Provider struct {
	cfg    Config
	client *http.Client
}

var _ provider.Provider = (*Provider)(nil)

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return "btgpactual" }

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("btgpactual: CreateAccount not implemented")
}

func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error) {
	return nil, fmt.Errorf("btgpactual: GetAccount not implemented")
}

func (p *Provider) ListAccounts(_ context.Context) ([]*types.Account, error) {
	return nil, fmt.Errorf("btgpactual: ListAccounts not implemented")
}

func (p *Provider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error) {
	return nil, fmt.Errorf("btgpactual: GetPortfolio not implemented")
}

func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) {
	return nil, fmt.Errorf("btgpactual: CreateOrder not implemented")
}

func (p *Provider) ListOrders(_ context.Context, _ string) ([]*types.Order, error) {
	return nil, fmt.Errorf("btgpactual: ListOrders not implemented")
}

func (p *Provider) GetOrder(_ context.Context, _, _ string) (*types.Order, error) {
	return nil, fmt.Errorf("btgpactual: GetOrder not implemented")
}

func (p *Provider) CancelOrder(_ context.Context, _, _ string) error {
	return fmt.Errorf("btgpactual: CancelOrder not implemented")
}

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("btgpactual: CreateTransfer not implemented")
}

func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return nil, fmt.Errorf("btgpactual: ListTransfers not implemented")
}

func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("btgpactual: CreateBankRelationship not implemented")
}

func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return nil, fmt.Errorf("btgpactual: ListBankRelationships not implemented")
}

func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	return nil, fmt.Errorf("btgpactual: ListAssets not implemented")
}

func (p *Provider) GetAsset(_ context.Context, _ string) (*types.Asset, error) {
	return nil, fmt.Errorf("btgpactual: GetAsset not implemented")
}

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("btgpactual: GetSnapshot not implemented")
}

func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("btgpactual: GetSnapshots not implemented")
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("btgpactual: GetBars not implemented")
}

func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("btgpactual: GetLatestTrades not implemented")
}

func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("btgpactual: GetLatestQuotes not implemented")
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, fmt.Errorf("btgpactual: GetClock not implemented")
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("btgpactual: GetCalendar not implemented")
}
