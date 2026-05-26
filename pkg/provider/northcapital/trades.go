// Trades / Subscriptions — TransactAPI Workstream 4.
//
// Endpoints:
//   POST /createTrade        — book a subscription / trade ticket
//   GET  /getTrade           — read by tradeId
//   POST /uploadBulkTrades   — CSV bulk-trade upload
//
// A Trade settled means the corresponding ERC-3643 SecurityToken
// movement settled on Lux Network (mint for primary, transfer for
// secondary). Parity is enforced by luxfi/transfer; divergence
// triggers an amld alert and freezes new transfers (brief §5).
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import "context"

// CreateTrade books a subscription / trade ticket against an Offering.
func (p *Provider) CreateTrade(_ context.Context, _ *CreateTradeRequest) (*Trade, error) {
	// TODO(scaffold/follow-up): POST /createTrade. Caller-supplied
	// IdempotencyKey takes precedence; deterministic fallback via
	// deriveIdempotencyKey(req.OrgID, "/createTrade", body). Trade
	// MUST be idempotent on retry — a duplicate POST must return the
	// existing tradeId, not double-book.
	return nil, ErrNotImplemented
}

// GetTrade reads a Trade by TransactAPI tradeId.
func (p *Provider) GetTrade(_ context.Context, _ string) (*Trade, error) {
	// TODO(scaffold/follow-up): GET /getTrade?tradeId=…
	return nil, ErrNotImplemented
}

// BulkUploadTrades posts a CSV batch of trades. Returns a batch result
// with accepted / rejected counts and per-row errors.
func (p *Provider) BulkUploadTrades(_ context.Context, _ []byte) (*BulkTradeResult, error) {
	// TODO(scaffold/follow-up): POST /uploadBulkTrades (multipart/form-data)
	return nil, ErrNotImplemented
}
