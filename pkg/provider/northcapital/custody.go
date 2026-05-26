// Custody account opening — TransactAPI Workstream 6.
//
// Endpoint:
//   POST /openCustodyAccount
//
// The custody-account-opening workflow involves NCPS-side compliance
// review; the adapter exposes it as a single call and surfaces the
// status via webhook (account.opened / account.rejected). Reconciled
// against the Lux captable identity registry once open.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import "context"

// OpenCustodyAccount opens a custody account for the given Party (or
// Entity), of the requested account_type (individual / joint / entity / ira).
func (p *Provider) OpenCustodyAccount(_ context.Context, _ string, _ *CustodyOpenRequest) (*CustodyAccount, error) {
	// TODO(scaffold/follow-up): POST /openCustodyAccount. Caller-
	// supplied IdempotencyKey takes precedence; deterministic
	// fallback via deriveIdempotencyKey(req.OrgID,
	// "/openCustodyAccount", body).
	return nil, ErrNotImplemented
}
