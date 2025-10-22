package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/luxfi/broker/pkg/types"
)

// TradingExtended is an optional interface for providers that support
// advanced trading operations beyond the base Provider interface.
type TradingExtended interface {
	ReplaceOrder(ctx context.Context, accountID, orderID string, req *types.ReplaceOrderRequest) (*types.Order, error)
	CancelAllOrders(ctx context.Context, accountID string) error
	GetPosition(ctx context.Context, accountID, symbol string) (*types.Position, error)
	ClosePosition(ctx context.Context, accountID, symbol string, qty *float64) (*types.Order, error)
	CloseAllPositions(ctx context.Context, accountID string) ([]*types.Order, error)
	ListOrdersFiltered(ctx context.Context, accountID string, params *types.ListOrdersParams) ([]*types.Order, error)
}

// AccountManager is an optional interface for account lifecycle management.
type AccountManager interface {
	UpdateAccount(ctx context.Context, accountID string, req *types.UpdateAccountRequest) (*types.Account, error)
	CloseAccount(ctx context.Context, accountID string) error
	GetAccountActivities(ctx context.Context, accountID string, params *types.ActivityParams) ([]*types.Activity, error)
}

// DocumentManager is an optional interface for document upload/retrieval.
type DocumentManager interface {
	UploadDocument(ctx context.Context, accountID string, doc *types.DocumentUpload) (*types.Document, error)
	ListDocuments(ctx context.Context, accountID string, params *types.DocumentParams) ([]*types.Document, error)
	GetDocument(ctx context.Context, accountID, documentID string) (*types.Document, error)
	DownloadDocument(ctx context.Context, accountID, documentID string) ([]byte, string, error)
}

// JournalManager is an optional interface for inter-account journal transfers.
type JournalManager interface {
	CreateJournal(ctx context.Context, req *types.CreateJournalRequest) (*types.Journal, error)
	ListJournals(ctx context.Context, params *types.JournalParams) ([]*types.Journal, error)
	GetJournal(ctx context.Context, journalID string) (*types.Journal, error)
	DeleteJournal(ctx context.Context, journalID string) error
	CreateBatchJournal(ctx context.Context, req *types.BatchJournalRequest) ([]*types.Journal, error)
	ReverseBatchJournal(ctx context.Context, req *types.ReverseBatchJournalRequest) ([]*types.Journal, error)
}

// TransferExtended is an optional interface for extended transfer operations.
type TransferExtended interface {
	CancelTransfer(ctx context.Context, accountID, transferID string) error
	DeleteACHRelationship(ctx context.Context, accountID, achID string) error
	CreateRecipientBank(ctx context.Context, accountID string, req *types.CreateBankRequest) (*types.RecipientBank, error)
	ListRecipientBanks(ctx context.Context, accountID string) ([]*types.RecipientBank, error)
	DeleteRecipientBank(ctx context.Context, accountID, bankID string) error
}

// CryptoDataProvider is an optional interface for crypto-specific market data.
type CryptoDataProvider interface {
	GetCryptoBars(ctx context.Context, req *types.CryptoBarsRequest) (*types.BarsResponse, error)
	GetCryptoQuotes(ctx context.Context, req *types.CryptoQuotesRequest) (*types.QuotesResponse, error)
	GetCryptoTrades(ctx context.Context, req *types.CryptoTradesRequest) (*types.TradesResponse, error)
	GetCryptoSnapshots(ctx context.Context, symbols []string) (map[string]*types.CryptoSnapshot, error)
}

// EventStreamer is an optional interface for server-sent event streaming.
type EventStreamer interface {
	StreamTradeEvents(ctx context.Context, since string) (<-chan *types.TradeEvent, error)
	StreamAccountEvents(ctx context.Context, since string) (<-chan *types.AccountEvent, error)
	StreamTransferEvents(ctx context.Context, since string) (<-chan *types.TransferEvent, error)
	StreamJournalEvents(ctx context.Context, since string) (<-chan *types.JournalEvent, error)
}

// PortfolioAnalyzer is an optional interface for portfolio history.
type PortfolioAnalyzer interface {
	GetPortfolioHistory(ctx context.Context, accountID string, params *types.HistoryParams) (*types.PortfolioHistory, error)
}

// WatchlistManager is an optional interface for watchlist operations.
type WatchlistManager interface {
	CreateWatchlist(ctx context.Context, accountID string, req *types.CreateWatchlistRequest) (*types.Watchlist, error)
	ListWatchlists(ctx context.Context, accountID string) ([]*types.Watchlist, error)
	GetWatchlist(ctx context.Context, accountID, watchlistID string) (*types.Watchlist, error)
	UpdateWatchlist(ctx context.Context, accountID, watchlistID string, req *types.UpdateWatchlistRequest) (*types.Watchlist, error)
	DeleteWatchlist(ctx context.Context, accountID, watchlistID string) error
	AddWatchlistAsset(ctx context.Context, accountID, watchlistID, symbol string) (*types.Watchlist, error)
	RemoveWatchlistAsset(ctx context.Context, accountID, watchlistID, symbol string) error
}

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
