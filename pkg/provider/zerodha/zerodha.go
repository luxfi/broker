// Package zerodha implements the broker Provider interface for Zerodha (India, SEBI).
package zerodha

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

const ProdURL = "https://api.kite.trade"

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
		cfg.BaseURL = ProdURL
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string                                                            { return "zerodha" }
func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) { return nil, fmt.Errorf("zerodha: CreateAccount not implemented") }
func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error)          { return nil, fmt.Errorf("zerodha: GetAccount not implemented") }
func (p *Provider) ListAccounts(_ context.Context) ([]*types.Account, error)                { return nil, fmt.Errorf("zerodha: ListAccounts not implemented") }
func (p *Provider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error)      { return nil, fmt.Errorf("zerodha: GetPortfolio not implemented") }
func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) { return nil, fmt.Errorf("zerodha: CreateOrder not implemented") }
func (p *Provider) ListOrders(_ context.Context, _ string) ([]*types.Order, error)          { return nil, fmt.Errorf("zerodha: ListOrders not implemented") }
func (p *Provider) GetOrder(_ context.Context, _, _ string) (*types.Order, error)           { return nil, fmt.Errorf("zerodha: GetOrder not implemented") }
func (p *Provider) CancelOrder(_ context.Context, _, _ string) error                        { return fmt.Errorf("zerodha: CancelOrder not implemented") }
func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, fmt.Errorf("zerodha: CreateTransfer not implemented") }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error)    { return nil, fmt.Errorf("zerodha: ListTransfers not implemented") }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, fmt.Errorf("zerodha: CreateBankRelationship not implemented") }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, fmt.Errorf("zerodha: ListBankRelationships not implemented") }
func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error)          { return nil, fmt.Errorf("zerodha: ListAssets not implemented") }
func (p *Provider) GetAsset(_ context.Context, _ string) (*types.Asset, error)              { return nil, fmt.Errorf("zerodha: GetAsset not implemented") }
func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error)  { return nil, fmt.Errorf("zerodha: GetSnapshot not implemented") }
func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) { return nil, fmt.Errorf("zerodha: GetSnapshots not implemented") }
func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) { return nil, fmt.Errorf("zerodha: GetBars not implemented") }
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) { return nil, fmt.Errorf("zerodha: GetLatestTrades not implemented") }
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) { return nil, fmt.Errorf("zerodha: GetLatestQuotes not implemented") }
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error)                  { return nil, fmt.Errorf("zerodha: GetClock not implemented") }
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) { return nil, fmt.Errorf("zerodha: GetCalendar not implemented") }
