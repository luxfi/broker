// Package pretrade implements the synchronous pre-trade compliance gate
// required at order-entry time (Gap G-37, Stage 6.5).
//
// The gate fetches state from the consumed providers (cap-table, transfer-
// restrictions, AML, KYC, on-chain ERC-3643 compliance) and decides
// allow / deny / escalate. Without this gate, a trade can settle and
// only then fail compliance — creating a rescission obligation.
//
// Each predicate is composable: a Gate runs the bundle in order and
// short-circuits on the first hard deny; soft-deny conditions become
// escalate decisions for human review.
package pretrade

import (
	"time"
)

// Order is the minimal pre-trade view of an order. The router supplies
// this from its native types.CreateOrderRequest; we keep the gate
// dependency-light so it does not have to import the order surface and
// can be unit-tested with fixtures.
type Order struct {
	// AccountID identifies the brokerage account placing the order.
	AccountID string
	// InvestorID identifies the natural / legal person behind the account
	// for KYC / suitability / accreditation purposes.
	InvestorID string
	// OfferingID identifies the security being traded — drives the
	// restriction, max-holder, lockup, and per-offering policy lookups.
	OfferingID string
	// Symbol is the human-readable security identifier.
	Symbol string
	// Side: "buy" or "sell".
	Side string
	// Qty is the order quantity as a decimal string.
	Qty string
	// Price is the estimated fill price as a decimal string (optional for
	// market orders).
	Price string
	// OrderType: "market", "limit", "stop", etc.
	OrderType string
	// TokenAddress is the ERC-3643 token contract address (optional —
	// only set for on-chain securities; gate skips the on-chain check
	// when empty).
	TokenAddress string
	// CountryCode is the investor's country of residence (ISO 3166-1
	// alpha-2). Drives country-restriction predicate.
	CountryCode string
	// ClientIP is captured for the audit log.
	ClientIP string
	// RequestID is the caller-supplied request id for audit correlation.
	RequestID string
}

// Decision is the outcome of the pre-trade compliance gate.
type Decision struct {
	// Allow is true iff every predicate cleared. Mutually exclusive with
	// Deny and Escalate.
	Allow bool `json:"allow"`
	// Deny is true iff at least one hard-deny predicate fired. Hard deny
	// is irrecoverable in the current order — the caller must reject.
	Deny bool `json:"deny"`
	// Escalate is true iff at least one soft-deny predicate fired and no
	// hard-deny did. The caller must hold the order and route it to a
	// human reviewer.
	Escalate bool `json:"escalate"`
	// Reasons enumerates every predicate that did not clear, with code,
	// severity, and a human-readable message.
	Reasons []Reason `json:"reasons,omitempty"`
	// RequiredActions enumerates the operational steps that would allow
	// the order to clear (e.g., "renew_accreditation", "complete_kyc").
	RequiredActions []string `json:"required_actions,omitempty"`
	// EvaluatedAt is the timestamp at which the gate ran.
	EvaluatedAt time.Time `json:"evaluated_at"`
	// LatencyNs is the wall-clock latency of the gate evaluation.
	LatencyNs int64 `json:"latency_ns"`
}

// Reason is a single predicate outcome.
type Reason struct {
	// Code is a stable machine-readable identifier (e.g.,
	// "accreditation_stale", "ofac_screening_stale").
	Code string `json:"code"`
	// Severity is one of "deny" (hard) or "escalate" (soft).
	Severity Severity `json:"severity"`
	// Message is the human-readable explanation, PII-redacted.
	Message string `json:"message"`
}

// Severity captures whether a predicate result is recoverable.
type Severity string

const (
	// SeverityDeny is a hard deny — the order cannot proceed under the
	// current state, and no human review can override at order-entry
	// time (operational remediation required first).
	SeverityDeny Severity = "deny"
	// SeverityEscalate is a soft deny — the order is held and routed to
	// a human reviewer (e.g., suitability questionnaire ambiguity, near-
	// expiry forms).
	SeverityEscalate Severity = "escalate"
)

// AccreditationStatus is the investor's accreditation snapshot returned
// by the cap-table provider.
type AccreditationStatus struct {
	// Verified is true iff a verification method completed successfully.
	Verified bool
	// Method is the verification method used (drives the freshness window):
	//   "income"     — Reg D 506(b)/(c) self-cert via income
	//   "networth"   — Reg D 506(b)/(c) self-cert via net worth
	//   "third_party"— 506(c) third-party verification (90-day window)
	//   "qualified_purchaser" — 3(c)(7) qualified purchaser certification
	Method string
	// VerifiedAt is the timestamp of the most recent verification event.
	VerifiedAt time.Time
}

// SuitabilityStatus is the investor's suitability snapshot.
type SuitabilityStatus struct {
	// OnFile is true iff a suitability questionnaire has ever been
	// submitted for the investor.
	OnFile bool
	// LastUpdatedAt is the timestamp of the most recent questionnaire
	// version on file.
	LastUpdatedAt time.Time
	// Ambiguous is set when the suitability service marks the response
	// pattern as requiring human review (e.g., investor flags risk
	// tolerance "low" + product flagged "high risk").
	Ambiguous bool
}

// RestrictionStatus is the offering-level restriction snapshot.
type RestrictionStatus struct {
	// LockupRemainingDays is the number of days remaining on any active
	// lockup affecting the investor's position. Zero means no lockup.
	LockupRemainingDays int
	// Rule144EligibleAt is the earliest date the investor's holdings are
	// Rule 144 resale-eligible. Zero value means the investor is already
	// eligible (or Rule 144 does not apply to this offering).
	Rule144EligibleAt time.Time
	// RestrictedCountries is the per-offering list of ISO 3166-1 alpha-2
	// country codes blocked from holding this security.
	RestrictedCountries []string
	// MaxHolders is the per-offering cap on holder-of-record count (e.g.,
	// 1933 Act §12(g) thresholds). Zero means no cap.
	MaxHolders int
	// CurrentHolders is the current holder-of-record count for the
	// offering. Compared against MaxHolders only for buy-side orders that
	// would add a new holder.
	CurrentHolders int
	// InvestorAlreadyHolds is true iff the investor is already a holder
	// of record on the offering (so a buy-side order does not change the
	// holder count).
	InvestorAlreadyHolds bool
}

// AMLStatus is the AML / OFAC snapshot.
type AMLStatus struct {
	// Clean is true iff there are no open AML findings.
	Clean bool
	// OFACLastScreenedAt is the timestamp of the most recent OFAC / SDN
	// re-screen.
	OFACLastScreenedAt time.Time
	// HasOpenSAR is true iff a Suspicious Activity Report is open.
	HasOpenSAR bool
}

// KYCStatus is the KYC freshness snapshot.
type KYCStatus struct {
	// Verified is true iff the investor has cleared KYC.
	Verified bool
	// VerifiedAt is the timestamp of the most recent KYC pass.
	VerifiedAt time.Time
}

// ChainComplianceStatus is the on-chain ERC-3643 compliance module
// response. The compliance module is delegated-to: this gate forwards
// the order parameters to the on-chain canTransfer check and collects
// the result.
type ChainComplianceStatus struct {
	// CanTransfer is the modular compliance module's allow/deny answer.
	CanTransfer bool
	// Reason captures the contract revert reason (or off-chain
	// short-circuit reason) when CanTransfer is false.
	Reason string
}
