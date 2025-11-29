package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	name     string
	snapshot *types.MarketSnapshot
	asset    *types.Asset
	order    *types.Order
	err      error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) GetAsset(_ context.Context, symbol string) (*types.Asset, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.asset != nil {
		return m.asset, nil
	}
	return &types.Asset{Symbol: symbol, Tradable: true}, nil
}

func (m *mockProvider) GetSnapshot(_ context.Context, symbol string) (*types.MarketSnapshot, error) {
	if m.snapshot != nil {
		return m.snapshot, nil
	}
	return &types.MarketSnapshot{Symbol: symbol}, nil
}

func (m *mockProvider) CreateOrder(_ context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	if m.order != nil {
		return m.order, nil
	}
	return &types.Order{
		Provider:   m.name,
		ProviderID: "order-" + m.name,
		Symbol:     req.Symbol,
		Status:     "filled",
		FilledQty:  req.Qty,
	}, nil
}

// Unused methods — minimal stubs to satisfy the interface.
func (m *mockProvider) CreateAccount(context.Context, *types.CreateAccountRequest) (*types.Account, error) {
	return nil, nil
}
func (m *mockProvider) GetAccount(context.Context, string) (*types.Account, error) {
	return nil, nil
}
func (m *mockProvider) ListAccounts(context.Context) ([]*types.Account, error) {
	return nil, nil
}
func (m *mockProvider) GetPortfolio(context.Context, string) (*types.Portfolio, error) {
	return nil, nil
}
func (m *mockProvider) ListOrders(context.Context, string) ([]*types.Order, error) {
	return nil, nil
}
func (m *mockProvider) GetOrder(context.Context, string, string) (*types.Order, error) {
	return nil, nil
}
func (m *mockProvider) CancelOrder(context.Context, string, string) error { return nil }
func (m *mockProvider) CreateTransfer(context.Context, string, *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, nil
}
func (m *mockProvider) ListTransfers(context.Context, string) ([]*types.Transfer, error) {
	return nil, nil
}
func (m *mockProvider) CreateBankRelationship(context.Context, string, string, string, string, string) (*types.BankRelationship, error) {
	return nil, nil
}
func (m *mockProvider) ListBankRelationships(context.Context, string) ([]*types.BankRelationship, error) {
	return nil, nil
}
func (m *mockProvider) ListAssets(context.Context, string) ([]*types.Asset, error) {
	return nil, nil
}
func (m *mockProvider) GetSnapshots(context.Context, []string) (map[string]*types.MarketSnapshot, error) {
	return nil, nil
}
func (m *mockProvider) GetBars(context.Context, string, string, string, string, int) ([]*types.Bar, error) {
	return nil, nil
}
func (m *mockProvider) GetLatestTrades(context.Context, []string) (map[string]*types.Trade, error) {
	return nil, nil
}
func (m *mockProvider) GetLatestQuotes(context.Context, []string) (map[string]*types.Quote, error) {
	return nil, nil
}
func (m *mockProvider) GetClock(context.Context) (*types.MarketClock, error) { return nil, nil }
func (m *mockProvider) GetCalendar(context.Context, string, string) ([]*types.MarketCalendarDay, error) {
	return nil, nil
}

func newTestRegistry(providers ...provider.Provider) *provider.Registry {
	reg := provider.NewRegistry()
	for _, p := range providers {
		reg.Register(p)
	}
	return reg
}

func TestFindBestProviderSingleProvider(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol: "AAPL",
			LatestQuote: &types.Quote{
				BidPrice: 150.0,
				AskPrice: 150.2,
			},
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)

	ctx := context.Background()
	result, err := r.FindBestProvider(ctx, "AAPL", "buy")
	if err != nil {
		t.Fatalf("FindBestProvider error: %v", err)
	}
	if result.Provider != "alpaca" {
		t.Errorf("Provider = %q, want 'alpaca'", result.Provider)
	}
	if result.AskPrice != 150.2 {
		t.Errorf("AskPrice = %f, want 150.2", result.AskPrice)
	}
}

func TestFindBestProviderSelectsCheapestBuy(t *testing.T) {
	// Provider A: higher ask (worse for buyer)
	mpA := &mockProvider{
		name: "expensive",
		snapshot: &types.MarketSnapshot{
			Symbol: "AAPL",
			LatestQuote: &types.Quote{
				BidPrice: 150.0,
				AskPrice: 151.0,
			},
		},
	}
	// Provider B: lower ask (better for buyer)
	mpB := &mockProvider{
		name: "cheap",
		snapshot: &types.MarketSnapshot{
			Symbol: "AAPL",
			LatestQuote: &types.Quote{
				BidPrice: 149.5,
				AskPrice: 150.0,
			},
		},
	}

	reg := newTestRegistry(mpA, mpB)
	r := New(reg)
	// Set zero fees for both to isolate price comparison
	r.SetFees("expensive", 0, 0)
	r.SetFees("cheap", 0, 0)

	ctx := context.Background()
	result, err := r.FindBestProvider(ctx, "AAPL", "buy")
	if err != nil {
		t.Fatalf("FindBestProvider error: %v", err)
	}
	// For buys, lower net price (ask) = lower score = better
	if result.Provider != "cheap" {
		t.Errorf("Provider = %q, want 'cheap' (lower ask)", result.Provider)
	}
}

func TestFindBestProviderSelectsBestSell(t *testing.T) {
	// Provider A: lower bid (worse for seller)
	mpA := &mockProvider{
		name: "low_bid",
		snapshot: &types.MarketSnapshot{
			Symbol: "AAPL",
			LatestQuote: &types.Quote{
				BidPrice: 149.0,
				AskPrice: 150.0,
			},
		},
	}
	// Provider B: higher bid (better for seller)
	mpB := &mockProvider{
		name: "high_bid",
		snapshot: &types.MarketSnapshot{
			Symbol: "AAPL",
			LatestQuote: &types.Quote{
				BidPrice: 150.5,
				AskPrice: 151.0,
			},
		},
	}

	reg := newTestRegistry(mpA, mpB)
	r := New(reg)
	r.SetFees("low_bid", 0, 0)
	r.SetFees("high_bid", 0, 0)

	ctx := context.Background()
	result, err := r.FindBestProvider(ctx, "AAPL", "sell")
	if err != nil {
		t.Fatalf("FindBestProvider error: %v", err)
	}
	// For sells, score = -netPrice, so higher bid => more negative score => sorted first
	if result.Provider != "high_bid" {
		t.Errorf("Provider = %q, want 'high_bid' (higher bid)", result.Provider)
	}
}

func TestFindBestProviderNoProviders(t *testing.T) {
	reg := provider.NewRegistry()
	r := New(reg)

	ctx := context.Background()
	_, err := r.FindBestProvider(ctx, "AAPL", "buy")
	if err == nil {
		t.Fatal("expected error for no providers")
	}
}

func TestFindBestProviderUntradableAsset(t *testing.T) {
	mp := &mockProvider{
		name:  "alpaca",
		asset: &types.Asset{Symbol: "DELIST", Tradable: false},
	}
	reg := newTestRegistry(mp)
	r := New(reg)

	ctx := context.Background()
	_, err := r.FindBestProvider(ctx, "DELIST", "buy")
	if err == nil {
		t.Fatal("expected error for untradable asset")
	}
}

func TestFindBestProviderAssetError(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		err:  fmt.Errorf("asset not found"),
	}
	reg := newTestRegistry(mp)
	r := New(reg)

	ctx := context.Background()
	_, err := r.FindBestProvider(ctx, "INVALID", "buy")
	if err == nil {
		t.Fatal("expected error for asset error")
	}
}

func TestGetAllRoutesRankedByScore(t *testing.T) {
	providers := []*mockProvider{
		{
			name: "best",
			snapshot: &types.MarketSnapshot{
				Symbol:      "AAPL",
				LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
			},
		},
		{
			name: "worst",
			snapshot: &types.MarketSnapshot{
				Symbol:      "AAPL",
				LatestQuote: &types.Quote{BidPrice: 149, AskPrice: 152.0},
			},
		},
	}
	reg := newTestRegistry(&mockProvider{name: providers[0].name, snapshot: providers[0].snapshot}, &mockProvider{name: providers[1].name, snapshot: providers[1].snapshot})
	r := New(reg)
	r.SetFees("best", 0, 0)
	r.SetFees("worst", 0, 0)

	ctx := context.Background()
	routes, err := r.GetAllRoutes(ctx, "AAPL", "buy")
	if err != nil {
		t.Fatalf("GetAllRoutes error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	// First route should be the one with lower ask (lower score for buy)
	if routes[0].Provider != "best" {
		t.Errorf("routes[0].Provider = %q, want 'best'", routes[0].Provider)
	}
}

func TestBuildSplitPlan(t *testing.T) {
	mpA := &mockProvider{
		name: "provA",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
	}
	mpB := &mockProvider{
		name: "provB",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 149.5, AskPrice: 150.5},
		},
	}

	reg := newTestRegistry(mpA, mpB)
	r := New(reg)
	r.SetFees("provA", 0, 0)
	r.SetFees("provB", 0, 0)

	ctx := context.Background()
	plan, err := r.BuildSplitPlan(ctx, "AAPL", "buy", "100")
	if err != nil {
		t.Fatalf("BuildSplitPlan error: %v", err)
	}
	if plan.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want 'AAPL'", plan.Symbol)
	}
	if len(plan.Legs) == 0 {
		t.Fatal("expected at least 1 leg")
	}
	if plan.Algorithm != "split" {
		t.Errorf("Algorithm = %q, want 'split'", plan.Algorithm)
	}
	if plan.EstimatedVWAP <= 0 {
		t.Errorf("EstimatedVWAP = %f, want > 0", plan.EstimatedVWAP)
	}
}

func TestBuildSplitPlanInvalidQty(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)

	ctx := context.Background()
	_, err := r.BuildSplitPlan(ctx, "AAPL", "buy", "0")
	if err == nil {
		t.Fatal("expected error for zero qty")
	}
}

func TestSmartOrderRoutes(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
		order: &types.Order{
			Provider:   "alpaca",
			ProviderID: "smart-order-1",
			Symbol:     "AAPL",
			Status:     "filled",
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)

	ctx := context.Background()
	order, err := r.SmartOrder(ctx, map[string]string{"alpaca": "acct-1"}, &types.CreateOrderRequest{
		Symbol:      "AAPL",
		Qty:         "10",
		Side:        "buy",
		Type:        "market",
		TimeInForce: "day",
	})
	if err != nil {
		t.Fatalf("SmartOrder error: %v", err)
	}
	if order.ProviderID != "smart-order-1" {
		t.Errorf("ProviderID = %q, want 'smart-order-1'", order.ProviderID)
	}
}

func TestSmartOrderNoAccount(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)

	ctx := context.Background()
	_, err := r.SmartOrder(ctx, map[string]string{}, &types.CreateOrderRequest{
		Symbol: "AAPL",
		Side:   "buy",
	})
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestSetFeesAndGetCapabilities(t *testing.T) {
	reg := provider.NewRegistry()
	r := New(reg)

	r.SetFees("test-prov", 5.0, 10.0)
	r.SetCapability(&types.ProviderCapability{
		Name:         "test-prov",
		Status:       "active",
		AssetClasses: []string{"crypto"},
	})

	caps := r.GetCapabilities()
	found := false
	for _, c := range caps {
		if c.Name == "test-prov" {
			found = true
			if c.MakerFee != 5.0 {
				t.Errorf("MakerFee = %f, want 5.0", c.MakerFee)
			}
			if c.TakerFee != 10.0 {
				t.Errorf("TakerFee = %f, want 10.0", c.TakerFee)
			}
			if c.Status != "active" {
				t.Errorf("Status = %q, want 'active'", c.Status)
			}
		}
	}
	if !found {
		t.Error("test-prov not found in capabilities")
	}
}

func TestGetCapabilitiesReturnsSortedByName(t *testing.T) {
	reg := provider.NewRegistry()
	r := New(reg)

	caps := r.GetCapabilities()
	for i := 1; i < len(caps); i++ {
		if caps[i].Name < caps[i-1].Name {
			t.Errorf("capabilities not sorted: %q < %q", caps[i].Name, caps[i-1].Name)
		}
	}
}

func TestFindBestProviderAccountsForFees(t *testing.T) {
	// Provider with higher fees should score worse
	mpCheap := &mockProvider{
		name: "cheap",
		snapshot: &types.MarketSnapshot{
			Symbol: "BTC",
			LatestQuote: &types.Quote{
				BidPrice: 50000,
				AskPrice: 50010,
			},
		},
	}
	mpExpensive := &mockProvider{
		name: "expensive",
		snapshot: &types.MarketSnapshot{
			Symbol: "BTC",
			LatestQuote: &types.Quote{
				BidPrice: 50000,
				AskPrice: 50010, // Same price
			},
		},
	}

	reg := newTestRegistry(mpCheap, mpExpensive)
	r := New(reg)
	r.SetFees("cheap", 0, 5)      // 5 bps taker
	r.SetFees("expensive", 0, 100) // 100 bps taker

	ctx := context.Background()
	result, err := r.FindBestProvider(ctx, "BTC", "buy")
	if err != nil {
		t.Fatalf("FindBestProvider error: %v", err)
	}
	// Same ask price but cheap has lower fees → lower net price → lower score
	if result.Provider != "cheap" {
		t.Errorf("Provider = %q, want 'cheap' (lower fees)", result.Provider)
	}
}

func TestExecuteSplitPlan(t *testing.T) {
	mpA := &mockProvider{
		name: "provA",
		order: &types.Order{
			Provider:       "provA",
			ProviderID:     "orderA",
			Status:         "filled",
			FilledQty:      "50",
			FilledAvgPrice: "150.00",
		},
	}
	mpB := &mockProvider{
		name: "provB",
		order: &types.Order{
			Provider:       "provB",
			ProviderID:     "orderB",
			Status:         "filled",
			FilledQty:      "50",
			FilledAvgPrice: "150.50",
		},
	}

	reg := newTestRegistry(mpA, mpB)
	r := New(reg)

	plan := &types.SplitPlan{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: "100",
		Legs: []types.SplitLeg{
			{Provider: "provA", Qty: "50"},
			{Provider: "provB", Qty: "50"},
		},
	}

	ctx := context.Background()
	result, err := r.ExecuteSplitPlan(ctx, plan, map[string]string{
		"provA": "acctA",
		"provB": "acctB",
	})
	if err != nil {
		t.Fatalf("ExecuteSplitPlan error: %v", err)
	}
	if result.Status != "filled" {
		t.Errorf("Status = %q, want 'filled'", result.Status)
	}
	if len(result.Legs) != 2 {
		t.Fatalf("expected 2 legs, got %d", len(result.Legs))
	}
	if result.VWAP <= 0 {
		t.Errorf("VWAP = %f, want > 0", result.VWAP)
	}
}
