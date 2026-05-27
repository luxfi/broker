package pretrade

import "context"

// CapTableProvider returns accreditation, suitability, and offering-level
// restriction state for an investor / offering pair. Backed in production
// by github.com/luxfi/captable. Defined here so the gate can be
// constructed with mocks in tests and so the consumed surface is
// minimal and stable.
type CapTableProvider interface {
	// Accreditation returns the investor's accreditation snapshot, or an
	// error if the investor is unknown.
	Accreditation(ctx context.Context, investorID string) (AccreditationStatus, error)
	// Suitability returns the investor's suitability snapshot for the
	// offering, or an error if the investor is unknown.
	Suitability(ctx context.Context, investorID, offeringID string) (SuitabilityStatus, error)
	// Restrictions returns the offering-level restriction snapshot for
	// the investor.
	Restrictions(ctx context.Context, investorID, offeringID string) (RestrictionStatus, error)
}

// TransferProvider exposes the canonical transfer-restriction Check for
// the cap-table-of-record. Backed in production by github.com/luxfi/
// transfer/pkg/restrictions. The gate uses this as a cross-check against
// the offering-level RestrictionStatus: any disagreement is a hard deny.
type TransferProvider interface {
	// AllowTransfer answers whether the proposed quantity of an offering
	// can be transferred to or from the investor (depending on side).
	AllowTransfer(ctx context.Context, investorID, offeringID, side string, qty int64) (bool, []string, error)
}

// AMLProvider returns the AML / OFAC snapshot for an investor.
type AMLProvider interface {
	// Status returns the AML snapshot or an error if the investor is
	// unknown to the AML service.
	Status(ctx context.Context, investorID string) (AMLStatus, error)
}

// KYCProvider returns the KYC snapshot for an investor.
type KYCProvider interface {
	// Status returns the KYC snapshot or an error if the investor is
	// unknown.
	Status(ctx context.Context, investorID string) (KYCStatus, error)
}

// ChainComplianceProvider delegates the on-chain ERC-3643 modular
// compliance check. Backed in production by github.com/luxfi/securities/
// erc3643/compliance.go. The provider is queried only when the order
// carries a non-empty TokenAddress.
type ChainComplianceProvider interface {
	// CanTransfer asks the on-chain compliance module whether the
	// transfer is permitted under the token's attached modules.
	CanTransfer(ctx context.Context, tokenAddr, fromInvestorID, toInvestorID string, qty int64) (ChainComplianceStatus, error)
}

// AuditSink receives a Decision for every Check evaluation. Implementations
// must be non-blocking and concurrency-safe. The broker wires this to the
// canonical broker/pkg/audit log; tests pass a slice-collector.
type AuditSink interface {
	RecordPreTrade(decision *Decision, order *Order)
}
