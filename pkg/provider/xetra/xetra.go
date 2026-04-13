// Package xetra implements the broker Provider interface for Deutsche Boerse / Xetra (Germany, BaFin).
package xetra

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

const (
	ProdURL    = "https://api.xetra.com"
	SandboxURL = "https://api.sandbox.xetra.com"
)

type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"-"`
	APISecret string `json:"-"`
}

type Provider struct {
	cfg    Config
	client *http.Client
}

var _ provider.Provider = (*Provider)(nil)

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "xetra" }
func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) { return nil, fmt.Errorf("xetra: CreateAccount not implemented") }
func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error)          { return nil, fmt.Errorf("xetra: GetAccount not implemented") }
func (p *Provider) ListAccounts(_ context.Context) ([]*types.Account, error)                { return nil, fmt.Errorf("xetra: ListAccounts not implemented") }
func (p *Provider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error)      { return nil, fmt.Errorf("xetra: GetPortfolio not implemented") }
func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) { return nil, fmt.Errorf("xetra: CreateOrder not implemented") }
func (p *Provider) ListOrders(_ context.Context, _ string) ([]*types.Order, error)          { return nil, fmt.Errorf("xetra: ListOrders not implemented") }
func (p *Provider) GetOrder(_ context.Context, _, _ string) (*types.Order, error)           { return nil, fmt.Errorf("xetra: GetOrder not implemented") }
func (p *Provider) CancelOrder(_ context.Context, _, _ string) error                        { return fmt.Errorf("xetra: CancelOrder not implemented") }
func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, fmt.Errorf("xetra: CreateTransfer not implemented") }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error)    { return nil, fmt.Errorf("xetra: ListTransfers not implemented") }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, fmt.Errorf("xetra: CreateBankRelationship not implemented") }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, fmt.Errorf("xetra: ListBankRelationships not implemented") }
func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error)          { return nil, fmt.Errorf("xetra: ListAssets not implemented") }
func (p *Provider) GetAsset(_ context.Context, _ string) (*types.Asset, error)              { return nil, fmt.Errorf("xetra: GetAsset not implemented") }
func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error)  { return nil, fmt.Errorf("xetra: GetSnapshot not implemented") }
func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) { return nil, fmt.Errorf("xetra: GetSnapshots not implemented") }
func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) { return nil, fmt.Errorf("xetra: GetBars not implemented") }
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) { return nil, fmt.Errorf("xetra: GetLatestTrades not implemented") }
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) { return nil, fmt.Errorf("xetra: GetLatestQuotes not implemented") }
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error)                  { return nil, fmt.Errorf("xetra: GetClock not implemented") }
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) { return nil, fmt.Errorf("xetra: GetCalendar not implemented") }
