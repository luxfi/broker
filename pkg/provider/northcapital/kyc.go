// KYC / AML / Accredited verification — TransactAPI Workstream 2.
//
// Endpoints:
//   POST /performKYC          — identity + document verification
//   POST /performAML          — sanctions / OFAC / SDN / PEP screening
//   POST /performAccredited   — Reg D 506(c) accredited-investor check
//
// Lux's amld overlay (luxfi/amld) layers an independent screen on top
// of these results, per §6 of the integration brief. Where the two
// disagree, the conservative path (block + escalate) wins.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import "context"

// PerformKYC runs identity + document verification on a Party.
func (p *Provider) PerformKYC(_ context.Context, _ string) (*KYCResult, error) {
	// TODO(scaffold/follow-up): POST /performKYC?partyId=…
	return nil, ErrNotImplemented
}

// PerformAML runs sanctions / OFAC / SDN / PEP screening on a Party.
func (p *Provider) PerformAML(_ context.Context, _ string) (*AMLResult, error) {
	// TODO(scaffold/follow-up): POST /performAML?partyId=…
	return nil, ErrNotImplemented
}

// PerformAccredited runs Reg D 506(c) accredited-investor verification
// using the documented pathway (income / net-worth / third-party letter
// / professional license).
func (p *Provider) PerformAccredited(_ context.Context, _ string, _ AccreditedMethod) (*AccreditedResult, error) {
	// TODO(scaffold/follow-up): POST /performAccredited
	return nil, ErrNotImplemented
}
