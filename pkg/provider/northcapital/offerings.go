// Offerings setup — TransactAPI Workstream 3.
//
// Endpoints:
//   POST /createOffering  — new Offering record
//   GET  /getOffering     — read by offeringId
//   GET  /listOfferings   — directory listing, paginated
//
// Each Offering binds 1:1 to an ERC-3643 / T-REX SecurityToken on
// the Lux Network primary EVM (luxfi/captable). Token metadata is
// anchored at issuance; reconciliation between TransactAPI Offering
// state and on-chain token state runs continuously (luxfi/transfer).
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import "context"

// CreateOffering registers a new offering with TransactAPI.
func (p *Provider) CreateOffering(_ context.Context, _ *CreateOfferingRequest) (*Offering, error) {
	// TODO(scaffold/follow-up): POST /createOffering. Caller-supplied
	// IdempotencyKey takes precedence; deterministic fallback via
	// deriveIdempotencyKey(req.OrgID, "/createOffering", body).
	return nil, ErrNotImplemented
}

// GetOffering reads an Offering by TransactAPI offeringId.
func (p *Provider) GetOffering(_ context.Context, _ string) (*Offering, error) {
	// TODO(scaffold/follow-up): GET /getOffering?offeringId=…
	return nil, ErrNotImplemented
}

// ListOfferings returns all Offerings accessible to this clientID.
func (p *Provider) ListOfferings(_ context.Context) ([]*Offering, error) {
	// TODO(scaffold/follow-up): GET /listOfferings + pagination
	return nil, ErrNotImplemented
}
