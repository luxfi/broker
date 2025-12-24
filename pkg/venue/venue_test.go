package venue

import (
	"context"
	"testing"

	"github.com/luxfi/broker/pkg/types"
)

type stubProvider struct {
	name     string
	snapshot *types.MarketSnapshot
	order    *types.Order
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return s.snapshot, nil
}
func (s *stubProvider) CreateOrder(_ context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	if s.order != nil {
		return s.order, nil
	}
	return &types.Order{Symbol: req.Symbol, Status: "new"}, nil
}
func (s *stubProvider) CancelOrder(_ context.Context, _, _ string) error { return nil }

func TestProviderAdapterGetQuote(t *testing.T) {
	sp := &stubProvider{
		name: "test",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150.0, AskPrice: 150.2},
		},
	}
	adapter := &ProviderAdapter{P: sp}
	q, err := adapter.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote error: %v", err)
	}
	if q.BidPrice != 150.0 {
		t.Errorf("BidPrice = %f, want 150.0", q.BidPrice)
	}
	if q.AskPrice != 150.2 {
		t.Errorf("AskPrice = %f, want 150.2", q.AskPrice)
	}
}

func TestProviderAdapterGetQuoteFallsBackToTrade(t *testing.T) {
	sp := &stubProvider{
		name: "test",
		snapshot: &types.MarketSnapshot{
			Symbol:      "BTC/USD",
			LatestTrade: &types.Trade{Price: 50000},
		},
	}
	adapter := &ProviderAdapter{P: sp}
	q, err := adapter.GetQuote(context.Background(), "BTC/USD")
	if err != nil {
		t.Fatalf("GetQuote error: %v", err)
	}
	if q.BidPrice != 50000 {
		t.Errorf("BidPrice = %f, want 50000", q.BidPrice)
	}
}

func TestProviderAdapterPlaceOrder(t *testing.T) {
	sp := &stubProvider{
		name: "test",
		order: &types.Order{
			ProviderID: "order-123",
			Symbol:     "AAPL",
			Status:     "filled",
		},
	}
	adapter := &ProviderAdapter{P: sp, DefaultAccountID: "acct-1"}
	order, err := adapter.PlaceOrder(context.Background(), &PlaceOrderRequest{
		Symbol:      "AAPL",
		Qty:         "10",
		Side:        "buy",
		Type:        "market",
		TimeInForce: "day",
	})
	if err != nil {
		t.Fatalf("PlaceOrder error: %v", err)
	}
	if order.ProviderID != "order-123" {
		t.Errorf("ProviderID = %q, want 'order-123'", order.ProviderID)
	}
}

func TestProviderAdapterName(t *testing.T) {
	adapter := &ProviderAdapter{P: &stubProvider{name: "alpaca"}}
	if adapter.Name() != "alpaca" {
		t.Errorf("Name() = %q, want 'alpaca'", adapter.Name())
	}
}
