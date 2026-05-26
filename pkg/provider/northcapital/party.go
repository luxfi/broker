// Party / Entity onboarding — TransactAPI Workstream 1.
//
// Endpoints (per the integration brief §1 row 1):
//   POST /createParty    — natural-person / joint onboarding
//   POST /createEntity   — legal-entity onboarding
//   GET  /getParty       — read by partyId
//   GET  /listParties    — directory listing, paginated
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import "context"

// CreateParty onboards a natural-person (or joint) investor.
func (p *Provider) CreateParty(_ context.Context, _ *CreatePartyRequest) (*Party, error) {
	// TODO(scaffold/follow-up): POST /createParty. Caller-supplied
	// IdempotencyKey takes precedence; deterministic fallback via
	// deriveIdempotencyKey(req.OrgID, "/createParty", body).
	return nil, ErrNotImplemented
}

// CreateEntity onboards a legal-entity (LLC / corp / trust / partnership /
// SPV) investor. Requires beneficial-owner + control-person partyIds.
func (p *Provider) CreateEntity(_ context.Context, _ *CreateEntityRequest) (*Entity, error) {
	// TODO(scaffold/follow-up): POST /createEntity. Caller-supplied
	// IdempotencyKey takes precedence; deterministic fallback via
	// deriveIdempotencyKey(req.OrgID, "/createEntity", body).
	return nil, ErrNotImplemented
}

// GetParty reads a Party by TransactAPI partyId.
func (p *Provider) GetParty(_ context.Context, _ string) (*Party, error) {
	// TODO(scaffold/follow-up): GET /getParty?partyId=…
	return nil, ErrNotImplemented
}

// ListParties returns all Parties accessible to this clientID. Caller
// should expect the adapter to handle pagination uniformly (per the
// "Working with List Endpoints" guide referenced in §10 of the brief).
func (p *Provider) ListParties(_ context.Context) ([]*Party, error) {
	// TODO(scaffold/follow-up): GET /listParties + pagination
	return nil, ErrNotImplemented
}
