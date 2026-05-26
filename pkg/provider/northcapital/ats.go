// ATS / Secondary trades — TransactAPI Workstream 7.
//
// Endpoints / channels:
//   POST  /atsEvent              — publish a Lux-originated ATS event
//   GET   /getSecondaryTrades    — Secondary Trades Directory listing
//
// The ATS webhook channel is the inbound complement, handled by the
// Provider.ConsumeWebhook entry point (see webhook.go). Events flow
// outbound (Lux → TransactAPI) on order placement / cancellation /
// match / clear / settle so the NCPS-side ATS record stays in lockstep
// with the Lux on-chain compliance-module state.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import "context"

// PublishATSEvent posts an ATS lifecycle event to TransactAPI's ATS
// channel. The adapter is responsible for deterministic idempotency
// on retry — duplicate EventID MUST collapse on the NCPS side.
func (p *Provider) PublishATSEvent(_ context.Context, _ *ATSEvent) error {
	// TODO(scaffold/follow-up): POST /atsEvent with the EventID as the
	// idempotency-key input (so retries of the same logical event are
	// indistinguishable from a single send).
	return ErrNotImplemented
}

// GetSecondaryTrades reads the Secondary Trades Directory for an Offering.
func (p *Provider) GetSecondaryTrades(_ context.Context, _ string) ([]*SecondaryTrade, error) {
	// TODO(scaffold/follow-up): GET /getSecondaryTrades?offeringId=…
	return nil, ErrNotImplemented
}
