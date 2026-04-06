package provider

import (
	"context"

	"github.com/luxfi/broker/pkg/types"
)

// FuturesProvider is an optional interface for providers that support futures trading.
// Providers like Apex Futures and IBKR implement this interface.
type FuturesProvider interface {
	// GetFuturesContracts returns available contracts for an underlying (e.g. "ES", "CL").
	GetFuturesContracts(ctx context.Context, underlying string) ([]*types.FuturesContract, error)

	// GetFuturesQuote returns a real-time quote for a specific contract.
	GetFuturesQuote(ctx context.Context, symbol string) (*types.FuturesQuote, error)

	// CreateFuturesOrder places a futures order.
	CreateFuturesOrder(ctx context.Context, accountID string, req *types.CreateFuturesOrderRequest) (*types.Order, error)

	// GetFuturesPositions returns all futures positions for an account.
	GetFuturesPositions(ctx context.Context, accountID string) ([]*types.FuturesPosition, error)

	// CloseFuturesPosition closes a specific futures position.
	CloseFuturesPosition(ctx context.Context, accountID, symbol string, qty *int) (*types.Order, error)

	// GetFuturesMargin returns margin requirements for a contract.
	GetFuturesMargin(ctx context.Context, accountID, symbol string) (*types.FuturesMarginRequirement, error)
}

// FXProvider is an optional interface for providers that support FX/forex trading.
// Providers like CurrencyCloud and LMAX implement this interface.
type FXProvider interface {
	// GetFXPairs returns available currency pairs.
	GetFXPairs(ctx context.Context) ([]*types.FXPair, error)

	// GetFXQuote returns a real-time quote for a currency pair.
	GetFXQuote(ctx context.Context, pair string) (*types.FXQuote, error)

	// CreateFXOrder places an FX order.
	CreateFXOrder(ctx context.Context, accountID string, req *types.CreateFXOrderRequest) (*types.Order, error)

	// GetFXPositions returns all FX positions for an account.
	GetFXPositions(ctx context.Context, accountID string) ([]*types.FXPosition, error)

	// CloseFXPosition closes a specific FX position.
	CloseFXPosition(ctx context.Context, accountID, pair string) (*types.Order, error)

	// GetFXRate returns the current exchange rate for a pair.
	GetFXRate(ctx context.Context, baseCurrency, quoteCurrency string) (*types.FXRate, error)
}

// MarginProvider is an optional interface for margin account management.
type MarginProvider interface {
	// GetMarginRequirements returns margin requirements for a proposed order.
	GetMarginRequirements(ctx context.Context, accountID string, req *types.MarginCheckRequest) (*types.MarginRequirements, error)

	// GetAccountMargin returns current margin status for an account.
	GetAccountMargin(ctx context.Context, accountID string) (*types.AccountMargin, error)

	// GetOptionsApprovalLevel returns the options trading approval level.
	GetOptionsApprovalLevel(ctx context.Context, accountID string) (int, error)

	// RequestOptionsUpgrade requests an upgrade to options approval level.
	RequestOptionsUpgrade(ctx context.Context, accountID string, level int) error
}
