package provider

import (
	"context"

	"github.com/luxfi/broker/pkg/types"
)

// OptionsProvider is an optional interface for providers that support options trading.
// Providers like Alpaca, IBKR, and Tradier implement this interface.
type OptionsProvider interface {
	// GetOptionChain returns all contracts for a symbol and expiration date.
	GetOptionChain(ctx context.Context, symbol string, expiration string) (*types.OptionChain, error)

	// GetOptionExpirations returns available expiration dates for a symbol.
	GetOptionExpirations(ctx context.Context, symbol string) ([]string, error)

	// GetOptionQuote returns a real-time quote for a specific contract.
	GetOptionQuote(ctx context.Context, contractSymbol string) (*types.OptionQuote, error)

	// CreateOptionOrder places a single-leg option order.
	CreateOptionOrder(ctx context.Context, accountID string, req *types.CreateOptionOrderRequest) (*types.Order, error)

	// CreateMultiLegOrder places a multi-leg strategy order (spread, straddle, etc).
	CreateMultiLegOrder(ctx context.Context, accountID string, req *types.CreateMultiLegOrderRequest) (*types.MultiLegOrderResult, error)

	// ExerciseOption exercises an option contract early.
	ExerciseOption(ctx context.Context, accountID string, contractSymbol string, qty int) error

	// DoNotExercise marks an option contract as do-not-exercise at expiration.
	DoNotExercise(ctx context.Context, accountID string, contractSymbol string) error

	// GetOptionPositions returns all option positions for an account.
	GetOptionPositions(ctx context.Context, accountID string) ([]*types.OptionPosition, error)
}
