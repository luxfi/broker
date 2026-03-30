package apex

import (
	"context"
	"fmt"
	"net/http"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

const (
	ProdBaseURL    = "https://api.apexclearing.com"
	SandboxBaseURL = "https://api-sandbox.apexclearing.com"
)

// Verify interface compliance at compile time.
var (
	_ provider.Provider        = (*Provider)(nil)
	_ provider.OptionsProvider = (*Provider)(nil)
	_ provider.FuturesProvider = (*Provider)(nil)
	_ provider.MarginProvider  = (*Provider)(nil)
	_ provider.TradingExtended = (*Provider)(nil)
	_ provider.AccountManager  = (*Provider)(nil)
)

// Provider implements the Apex Clearing broker provider.
// Apex is a self-clearing firm supporting equities, options (1-4 leg),
// futures, and fixed income.
type Provider struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

// New creates a new Apex provider.
func New(apiKey, apiSecret string, sandbox bool) *Provider {
	base := ProdBaseURL
	if sandbox {
		base = SandboxBaseURL
	}
	return &Provider{
		baseURL:    base,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		httpClient: &http.Client{},
	}
}

func (p *Provider) Name() string { return "apex" }

// ---------------------------------------------------------------------------
// Provider (base interface)
// ---------------------------------------------------------------------------

func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("apex: CreateAccount not yet implemented")
}

func (p *Provider) GetAccount(ctx context.Context, accountID string) (*types.Account, error) {
	return nil, fmt.Errorf("apex: GetAccount not yet implemented")
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	return nil, fmt.Errorf("apex: ListAccounts not yet implemented")
}

func (p *Provider) GetPortfolio(ctx context.Context, accountID string) (*types.Portfolio, error) {
	return nil, fmt.Errorf("apex: GetPortfolio not yet implemented")
}

func (p *Provider) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	return nil, fmt.Errorf("apex: CreateOrder not yet implemented")
}

func (p *Provider) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	return nil, fmt.Errorf("apex: ListOrders not yet implemented")
}

func (p *Provider) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	return nil, fmt.Errorf("apex: GetOrder not yet implemented")
}

func (p *Provider) CancelOrder(ctx context.Context, accountID, orderID string) error {
	return fmt.Errorf("apex: CancelOrder not yet implemented")
}

func (p *Provider) CreateTransfer(ctx context.Context, accountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("apex: CreateTransfer not yet implemented")
}

func (p *Provider) ListTransfers(ctx context.Context, accountID string) ([]*types.Transfer, error) {
	return nil, fmt.Errorf("apex: ListTransfers not yet implemented")
}

func (p *Provider) CreateBankRelationship(ctx context.Context, accountID string, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("apex: CreateBankRelationship not yet implemented")
}

func (p *Provider) ListBankRelationships(ctx context.Context, accountID string) ([]*types.BankRelationship, error) {
	return nil, fmt.Errorf("apex: ListBankRelationships not yet implemented")
}

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	return nil, fmt.Errorf("apex: ListAssets not yet implemented")
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	return nil, fmt.Errorf("apex: GetAsset not yet implemented")
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("apex: GetSnapshot not yet implemented")
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("apex: GetSnapshots not yet implemented")
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("apex: GetBars not yet implemented")
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("apex: GetLatestTrades not yet implemented")
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("apex: GetLatestQuotes not yet implemented")
}

func (p *Provider) GetClock(ctx context.Context) (*types.MarketClock, error) {
	return nil, fmt.Errorf("apex: GetClock not yet implemented")
}

func (p *Provider) GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("apex: GetCalendar not yet implemented")
}

// ---------------------------------------------------------------------------
// TradingExtended
// ---------------------------------------------------------------------------

func (p *Provider) ReplaceOrder(ctx context.Context, accountID, orderID string, req *types.ReplaceOrderRequest) (*types.Order, error) {
	return nil, fmt.Errorf("apex: ReplaceOrder not yet implemented")
}

func (p *Provider) CancelAllOrders(ctx context.Context, accountID string) error {
	return fmt.Errorf("apex: CancelAllOrders not yet implemented")
}

func (p *Provider) GetPosition(ctx context.Context, accountID, symbol string) (*types.Position, error) {
	return nil, fmt.Errorf("apex: GetPosition not yet implemented")
}

func (p *Provider) ClosePosition(ctx context.Context, accountID, symbol string, qty *float64) (*types.Order, error) {
	return nil, fmt.Errorf("apex: ClosePosition not yet implemented")
}

func (p *Provider) CloseAllPositions(ctx context.Context, accountID string) ([]*types.Order, error) {
	return nil, fmt.Errorf("apex: CloseAllPositions not yet implemented")
}

func (p *Provider) ListOrdersFiltered(ctx context.Context, accountID string, params *types.ListOrdersParams) ([]*types.Order, error) {
	return nil, fmt.Errorf("apex: ListOrdersFiltered not yet implemented")
}

// ---------------------------------------------------------------------------
// AccountManager
// ---------------------------------------------------------------------------

func (p *Provider) UpdateAccount(ctx context.Context, accountID string, req *types.UpdateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("apex: UpdateAccount not yet implemented")
}

func (p *Provider) CloseAccount(ctx context.Context, accountID string) error {
	return fmt.Errorf("apex: CloseAccount not yet implemented")
}

func (p *Provider) GetAccountActivities(ctx context.Context, accountID string, params *types.ActivityParams) ([]*types.Activity, error) {
	return nil, fmt.Errorf("apex: GetAccountActivities not yet implemented")
}

// ---------------------------------------------------------------------------
// OptionsProvider — full 4-leg support
// ---------------------------------------------------------------------------

func (p *Provider) GetOptionChain(ctx context.Context, symbol, expiration string) (*types.OptionChain, error) {
	return nil, fmt.Errorf("apex: GetOptionChain not yet implemented")
}

func (p *Provider) GetOptionExpirations(ctx context.Context, symbol string) ([]string, error) {
	return nil, fmt.Errorf("apex: GetOptionExpirations not yet implemented")
}

func (p *Provider) GetOptionQuote(ctx context.Context, contractSymbol string) (*types.OptionQuote, error) {
	return nil, fmt.Errorf("apex: GetOptionQuote not yet implemented")
}

func (p *Provider) CreateOptionOrder(ctx context.Context, accountID string, req *types.CreateOptionOrderRequest) (*types.Order, error) {
	return nil, fmt.Errorf("apex: CreateOptionOrder not yet implemented")
}

func (p *Provider) CreateMultiLegOrder(ctx context.Context, accountID string, req *types.CreateMultiLegOrderRequest) (*types.MultiLegOrderResult, error) {
	// Apex supports 1-4 leg option orders natively
	if len(req.Legs) > 4 {
		return nil, fmt.Errorf("apex: maximum 4 legs per order, got %d", len(req.Legs))
	}
	return nil, fmt.Errorf("apex: CreateMultiLegOrder not yet implemented")
}

func (p *Provider) ExerciseOption(ctx context.Context, accountID, contractSymbol string, qty int) error {
	return fmt.Errorf("apex: ExerciseOption not yet implemented")
}

func (p *Provider) DoNotExercise(ctx context.Context, accountID, contractSymbol string) error {
	return fmt.Errorf("apex: DoNotExercise not yet implemented")
}

func (p *Provider) GetOptionPositions(ctx context.Context, accountID string) ([]*types.OptionPosition, error) {
	return nil, fmt.Errorf("apex: GetOptionPositions not yet implemented")
}

// ---------------------------------------------------------------------------
// FuturesProvider
// ---------------------------------------------------------------------------

func (p *Provider) GetFuturesContracts(ctx context.Context, underlying string) ([]*types.FuturesContract, error) {
	return nil, fmt.Errorf("apex: GetFuturesContracts not yet implemented")
}

func (p *Provider) GetFuturesQuote(ctx context.Context, symbol string) (*types.FuturesQuote, error) {
	return nil, fmt.Errorf("apex: GetFuturesQuote not yet implemented")
}

func (p *Provider) CreateFuturesOrder(ctx context.Context, accountID string, req *types.CreateFuturesOrderRequest) (*types.Order, error) {
	return nil, fmt.Errorf("apex: CreateFuturesOrder not yet implemented")
}

func (p *Provider) GetFuturesPositions(ctx context.Context, accountID string) ([]*types.FuturesPosition, error) {
	return nil, fmt.Errorf("apex: GetFuturesPositions not yet implemented")
}

func (p *Provider) CloseFuturesPosition(ctx context.Context, accountID, symbol string, qty *int) (*types.Order, error) {
	return nil, fmt.Errorf("apex: CloseFuturesPosition not yet implemented")
}

func (p *Provider) GetFuturesMargin(ctx context.Context, accountID, symbol string) (*types.FuturesMarginRequirement, error) {
	return nil, fmt.Errorf("apex: GetFuturesMargin not yet implemented")
}

// ---------------------------------------------------------------------------
// MarginProvider
// ---------------------------------------------------------------------------

func (p *Provider) GetMarginRequirements(ctx context.Context, accountID string, req *types.MarginCheckRequest) (*types.MarginRequirements, error) {
	return nil, fmt.Errorf("apex: GetMarginRequirements not yet implemented")
}

func (p *Provider) GetAccountMargin(ctx context.Context, accountID string) (*types.AccountMargin, error) {
	return nil, fmt.Errorf("apex: GetAccountMargin not yet implemented")
}

func (p *Provider) GetOptionsApprovalLevel(ctx context.Context, accountID string) (int, error) {
	return 0, fmt.Errorf("apex: GetOptionsApprovalLevel not yet implemented")
}

func (p *Provider) RequestOptionsUpgrade(ctx context.Context, accountID string, level int) error {
	return fmt.Errorf("apex: RequestOptionsUpgrade not yet implemented")
}
