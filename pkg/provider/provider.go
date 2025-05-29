package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/broker/pkg/types"
)

// Provider is the unified interface every broker backend must implement.
type Provider interface {
	// Name returns the provider identifier (e.g. "alpaca", "ibkr").
	Name() string

	// Accounts
	CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error)
	GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error)
	ListAccounts(ctx context.Context) ([]*types.Account, error)

	// Portfolio & Positions
	GetPortfolio(ctx context.Context, providerAccountID string) (*types.Portfolio, error)

	// Orders
	CreateOrder(ctx context.Context, providerAccountID string, req *types.CreateOrderRequest) (*types.Order, error)
	ListOrders(ctx context.Context, providerAccountID string) ([]*types.Order, error)
	GetOrder(ctx context.Context, providerAccountID, providerOrderID string) (*types.Order, error)
	CancelOrder(ctx context.Context, providerAccountID, providerOrderID string) error

	// Transfers
	CreateTransfer(ctx context.Context, providerAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error)
	ListTransfers(ctx context.Context, providerAccountID string) ([]*types.Transfer, error)

	// Bank Relationships
	CreateBankRelationship(ctx context.Context, providerAccountID string, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error)
	ListBankRelationships(ctx context.Context, providerAccountID string) ([]*types.BankRelationship, error)

	// Assets
	ListAssets(ctx context.Context, class string) ([]*types.Asset, error)
	GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error)

	// Market Data
	GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error)
	GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error)
	GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error)
	GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error)
	GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error)
	GetClock(ctx context.Context) (*types.MarketClock, error)
	GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error)
}

// Registry holds all registered providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return p, nil
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	return names
}
