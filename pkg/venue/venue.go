// Package venue defines the canonical venue adapter interface for the smart
// order routing system. It is intentionally a thin wrapper around the existing
// provider.Provider interface, giving SOR-specific nomenclature while reusing
// the production types.
package venue

import (
	"context"

	"github.com/luxfi/broker/pkg/types"
)

// Venue is the trading-venue interface consumed by the smart order router,
// the price aggregator, and the arbitrage detector. Every exchange or
// broker-dealer backend implements this.
//
// It is a superset of the most-used methods from provider.Provider, surfaced
// under venue-centric naming so routing code reads naturally.
type Venue interface {
	// Name returns a stable identifier for this venue (e.g. "alpaca", "coinbase").
	Name() string

	// GetQuote returns the current best bid/ask for a symbol.
	GetQuote(ctx context.Context, symbol string) (*types.Quote, error)

	// PlaceOrder submits a new order to the venue.
	PlaceOrder(ctx context.Context, order *PlaceOrderRequest) (*types.Order, error)

	// CancelOrder cancels a pending order by provider order ID.
	CancelOrder(ctx context.Context, accountID, orderID string) error

	// GetOrderBook returns the order book for a symbol up to the given depth.
	GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)

	// StreamQuotes opens a streaming channel of quote updates for the given symbols.
	// The returned channel is closed when the context is cancelled.
	StreamQuotes(ctx context.Context, symbols []string) (<-chan *types.Quote, error)
}

// PlaceOrderRequest carries the fields needed to submit an order at a venue.
type PlaceOrderRequest struct {
	AccountID   string `json:"account_id"`
	Symbol      string `json:"symbol"`
	Qty         string `json:"qty,omitempty"`
	Notional    string `json:"notional,omitempty"`
	Side        string `json:"side"`         // buy, sell
	Type        string `json:"type"`         // market, limit, stop, stop_limit
	TimeInForce string `json:"time_in_force"` // day, gtc, ioc, fok
	LimitPrice  string `json:"limit_price,omitempty"`
	StopPrice   string `json:"stop_price,omitempty"`
}

// OrderBook is a depth-of-book snapshot from a single venue.
type OrderBook struct {
	Symbol string       `json:"symbol"`
	Venue  string       `json:"venue"`
	Bids   []BookLevel  `json:"bids"` // sorted best (highest) first
	Asks   []BookLevel  `json:"asks"` // sorted best (lowest) first
}

// BookLevel is a single price/size level in an order book.
type BookLevel struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

// ProviderAdapter wraps a provider.Provider as a Venue.
// This lets the SOR use the venue interface with any existing provider
// implementation without code duplication.
type ProviderAdapter struct {
	P interface {
		Name() string
		GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error)
		CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error)
		CancelOrder(ctx context.Context, accountID, orderID string) error
	}
	DefaultAccountID string
}

func (a *ProviderAdapter) Name() string { return a.P.Name() }

func (a *ProviderAdapter) GetQuote(ctx context.Context, symbol string) (*types.Quote, error) {
	snap, err := a.P.GetSnapshot(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if snap.LatestQuote != nil {
		return snap.LatestQuote, nil
	}
	// Synthesize a quote from the latest trade if no quote is available.
	if snap.LatestTrade != nil {
		return &types.Quote{
			Timestamp: snap.LatestTrade.Timestamp,
			BidPrice:  snap.LatestTrade.Price,
			AskPrice:  snap.LatestTrade.Price,
		}, nil
	}
	return &types.Quote{}, nil
}

func (a *ProviderAdapter) PlaceOrder(ctx context.Context, order *PlaceOrderRequest) (*types.Order, error) {
	accountID := order.AccountID
	if accountID == "" {
		accountID = a.DefaultAccountID
	}
	return a.P.CreateOrder(ctx, accountID, &types.CreateOrderRequest{
		Symbol:      order.Symbol,
		Qty:         order.Qty,
		Notional:    order.Notional,
		Side:        order.Side,
		Type:        order.Type,
		TimeInForce: order.TimeInForce,
		LimitPrice:  order.LimitPrice,
		StopPrice:   order.StopPrice,
	})
}

func (a *ProviderAdapter) CancelOrder(ctx context.Context, accountID, orderID string) error {
	if accountID == "" {
		accountID = a.DefaultAccountID
	}
	return a.P.CancelOrder(ctx, accountID, orderID)
}

// GetOrderBook is not supported by the generic adapter; override in
// venue-specific implementations that have L2 data.
func (a *ProviderAdapter) GetOrderBook(_ context.Context, symbol string, _ int) (*OrderBook, error) {
	return &OrderBook{Symbol: symbol, Venue: a.P.Name()}, nil
}

// StreamQuotes is not supported by the generic adapter. Venue-specific
// implementations should provide real WebSocket streaming.
func (a *ProviderAdapter) StreamQuotes(_ context.Context, _ []string) (<-chan *types.Quote, error) {
	ch := make(chan *types.Quote)
	close(ch)
	return ch, nil
}
