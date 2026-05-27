package pretrade

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Config controls the freshness windows and policy knobs of the gate.
// Defaults match the regulatory baseline; offering-specific policies
// can be supplied per call (not yet wired — for the v1 a single Config
// is sufficient as every freshness window is industry-standard).
type Config struct {
	// AccreditationIncomeMaxAge — Reg D 506(b)/(c) self-cert via income
	// or net worth must be renewed at least annually.
	AccreditationIncomeMaxAge time.Duration
	// AccreditationThirdPartyMaxAge — 506(c) third-party verification
	// must be re-performed every 90 days.
	AccreditationThirdPartyMaxAge time.Duration
	// SuitabilityMaxAge — suitability questionnaire freshness window.
	SuitabilityMaxAge time.Duration
	// OFACScreenMaxAge — periodic OFAC / SDN re-screen cadence
	// (monthly is the industry baseline).
	OFACScreenMaxAge time.Duration
	// KYCMaxAge — KYC refresh cadence. 24 months is the FinCEN /
	// FinCEN-CDD baseline; offering policy may shorten.
	KYCMaxAge time.Duration
	// NearExpiryWindow — soft-deny threshold; if a credential will
	// expire within this window, decision is escalate rather than allow.
	NearExpiryWindow time.Duration
}

// DefaultConfig is the recommended regulatory baseline.
func DefaultConfig() Config {
	return Config{
		AccreditationIncomeMaxAge:     365 * 24 * time.Hour,
		AccreditationThirdPartyMaxAge: 90 * 24 * time.Hour,
		SuitabilityMaxAge:             365 * 24 * time.Hour,
		OFACScreenMaxAge:              30 * 24 * time.Hour,
		KYCMaxAge:                     2 * 365 * 24 * time.Hour,
		NearExpiryWindow:              14 * 24 * time.Hour,
	}
}

// Gate is the synchronous pre-trade compliance check (Gap G-37). Compose
// one per broker instance and call Check before submitting any order to
// a provider.
type Gate struct {
	cap   CapTableProvider
	tx    TransferProvider
	aml   AMLProvider
	kyc   KYCProvider
	chain ChainComplianceProvider
	audit AuditSink
	cfg   Config
	now   func() time.Time
}

// New constructs a Gate with all six providers wired. Audit may be nil
// (decisions then drop on the floor — strongly discouraged for prod).
// ChainCompliance may be nil if no on-chain securities flow through
// this gate; the on-chain predicate is skipped when the order's
// TokenAddress is empty regardless.
//
// Returns a non-nil error if any of the required providers (cap, tx,
// aml, kyc) is nil.
func New(cap CapTableProvider, tx TransferProvider, aml AMLProvider, kyc KYCProvider, chain ChainComplianceProvider, audit AuditSink, cfg Config) (*Gate, error) {
	if cap == nil {
		return nil, errors.New("pretrade: cap-table provider is required")
	}
	if tx == nil {
		return nil, errors.New("pretrade: transfer provider is required")
	}
	if aml == nil {
		return nil, errors.New("pretrade: aml provider is required")
	}
	if kyc == nil {
		return nil, errors.New("pretrade: kyc provider is required")
	}
	if cfg == (Config{}) {
		cfg = DefaultConfig()
	}
	return &Gate{
		cap:   cap,
		tx:    tx,
		aml:   aml,
		kyc:   kyc,
		chain: chain,
		audit: audit,
		cfg:   cfg,
		now:   func() time.Time { return time.Now().UTC() },
	}, nil
}

// Check evaluates the pre-trade compliance gate for one order.
//
// Predicate order is deterministic and chosen to short-circuit on the
// cheapest predicates first when possible, but every applicable
// predicate is evaluated so that the Decision carries every actionable
// reason in a single round (avoiding the "fix one thing, get rejected
// for the next" UX).
//
// The returned Decision is never nil. A non-nil error indicates an
// upstream-provider failure that prevented evaluation (e.g., AML service
// unreachable); the caller must hold the order and surface the error,
// because trading under an unknown compliance state is the failure mode
// G-37 exists to prevent.
func (g *Gate) Check(ctx context.Context, o *Order) (*Decision, error) {
	if o == nil {
		return nil, errors.New("pretrade: nil order")
	}
	if o.InvestorID == "" {
		return nil, errors.New("pretrade: order.InvestorID is required")
	}
	if o.OfferingID == "" {
		return nil, errors.New("pretrade: order.OfferingID is required")
	}
	start := g.now()
	dec := &Decision{EvaluatedAt: start}

	// --- Predicate 1: KYC freshness ---
	if reason, err := g.checkKYC(ctx, o); err != nil {
		return nil, fmt.Errorf("kyc: %w", err)
	} else if reason != nil {
		appendReason(dec, *reason)
	}

	// --- Predicate 2: Accreditation currency ---
	if reason, err := g.checkAccreditation(ctx, o); err != nil {
		return nil, fmt.Errorf("accreditation: %w", err)
	} else if reason != nil {
		appendReason(dec, *reason)
	}

	// --- Predicate 3: Suitability freshness ---
	if reason, err := g.checkSuitability(ctx, o); err != nil {
		return nil, fmt.Errorf("suitability: %w", err)
	} else if reason != nil {
		appendReason(dec, *reason)
	}

	// --- Predicate 4: AML status + OFAC re-screen cadence ---
	if reason, err := g.checkAML(ctx, o); err != nil {
		return nil, fmt.Errorf("aml: %w", err)
	} else if reason != nil {
		appendReason(dec, *reason)
	}

	// --- Predicate 5: Restriction-check (lockup / Rule 144 / country /
	//                  max-holder / transfer-restriction agreement) ---
	if reason, err := g.checkRestrictions(ctx, o); err != nil {
		return nil, fmt.Errorf("restrictions: %w", err)
	} else if reason != nil {
		appendReason(dec, *reason)
	}

	// --- Predicate 6: On-chain ERC-3643 compliance ---
	if reason, err := g.checkChain(ctx, o); err != nil {
		return nil, fmt.Errorf("chain: %w", err)
	} else if reason != nil {
		appendReason(dec, *reason)
	}

	finalize(dec)
	dec.LatencyNs = g.now().Sub(start).Nanoseconds()
	if g.audit != nil {
		g.audit.RecordPreTrade(dec, redact(o))
	}
	return dec, nil
}

// --- Predicates ---

func (g *Gate) checkKYC(ctx context.Context, o *Order) (*Reason, error) {
	s, err := g.kyc.Status(ctx, o.InvestorID)
	if err != nil {
		return nil, err
	}
	if !s.Verified {
		return &Reason{Code: "kyc_not_verified", Severity: SeverityDeny, Message: "investor has not completed KYC"}, nil
	}
	age := g.now().Sub(s.VerifiedAt)
	if age > g.cfg.KYCMaxAge {
		return &Reason{
			Code:     "kyc_stale",
			Severity: SeverityDeny,
			Message:  fmt.Sprintf("kyc last verified %s ago (max %s)", age.Round(24*time.Hour), g.cfg.KYCMaxAge),
		}, nil
	}
	if age > g.cfg.KYCMaxAge-g.cfg.NearExpiryWindow {
		return &Reason{
			Code:     "kyc_near_expiry",
			Severity: SeverityEscalate,
			Message:  fmt.Sprintf("kyc expires within %s", g.cfg.NearExpiryWindow),
		}, nil
	}
	return nil, nil
}

func (g *Gate) checkAccreditation(ctx context.Context, o *Order) (*Reason, error) {
	a, err := g.cap.Accreditation(ctx, o.InvestorID)
	if err != nil {
		return nil, err
	}
	if !a.Verified {
		return &Reason{Code: "not_accredited", Severity: SeverityDeny, Message: "investor is not accredited"}, nil
	}
	window := g.cfg.AccreditationIncomeMaxAge
	if strings.EqualFold(a.Method, "third_party") {
		window = g.cfg.AccreditationThirdPartyMaxAge
	}
	age := g.now().Sub(a.VerifiedAt)
	if age > window {
		return &Reason{
			Code:     "accreditation_stale",
			Severity: SeverityDeny,
			Message:  fmt.Sprintf("%s accreditation verified %s ago (max %s)", a.Method, age.Round(24*time.Hour), window),
		}, nil
	}
	if age > window-g.cfg.NearExpiryWindow {
		return &Reason{
			Code:     "accreditation_near_expiry",
			Severity: SeverityEscalate,
			Message:  fmt.Sprintf("%s accreditation expires within %s", a.Method, g.cfg.NearExpiryWindow),
		}, nil
	}
	return nil, nil
}

func (g *Gate) checkSuitability(ctx context.Context, o *Order) (*Reason, error) {
	s, err := g.cap.Suitability(ctx, o.InvestorID, o.OfferingID)
	if err != nil {
		return nil, err
	}
	if !s.OnFile {
		return &Reason{Code: "suitability_missing", Severity: SeverityDeny, Message: "no suitability questionnaire on file"}, nil
	}
	if s.Ambiguous {
		return &Reason{
			Code:     "suitability_ambiguous",
			Severity: SeverityEscalate,
			Message:  "suitability response pattern requires human review",
		}, nil
	}
	age := g.now().Sub(s.LastUpdatedAt)
	if age > g.cfg.SuitabilityMaxAge {
		return &Reason{
			Code:     "suitability_stale",
			Severity: SeverityDeny,
			Message:  fmt.Sprintf("suitability last updated %s ago (max %s)", age.Round(24*time.Hour), g.cfg.SuitabilityMaxAge),
		}, nil
	}
	return nil, nil
}

func (g *Gate) checkAML(ctx context.Context, o *Order) (*Reason, error) {
	s, err := g.aml.Status(ctx, o.InvestorID)
	if err != nil {
		return nil, err
	}
	if !s.Clean {
		return &Reason{Code: "aml_not_clean", Severity: SeverityDeny, Message: "open aml finding"}, nil
	}
	if s.HasOpenSAR {
		return &Reason{Code: "open_sar", Severity: SeverityDeny, Message: "open suspicious activity report"}, nil
	}
	age := g.now().Sub(s.OFACLastScreenedAt)
	if age > g.cfg.OFACScreenMaxAge {
		return &Reason{
			Code:     "ofac_screen_stale",
			Severity: SeverityDeny,
			Message:  fmt.Sprintf("ofac re-screen %s ago (max %s)", age.Round(24*time.Hour), g.cfg.OFACScreenMaxAge),
		}, nil
	}
	return nil, nil
}

func (g *Gate) checkRestrictions(ctx context.Context, o *Order) (*Reason, error) {
	r, err := g.cap.Restrictions(ctx, o.InvestorID, o.OfferingID)
	if err != nil {
		return nil, err
	}
	if r.LockupRemainingDays > 0 {
		return &Reason{
			Code:     "lockup_active",
			Severity: SeverityDeny,
			Message:  fmt.Sprintf("lockup remaining: %d days", r.LockupRemainingDays),
		}, nil
	}
	if !r.Rule144EligibleAt.IsZero() && r.Rule144EligibleAt.After(g.now()) {
		return &Reason{
			Code:     "rule144_holding_period",
			Severity: SeverityDeny,
			Message:  fmt.Sprintf("rule 144 eligibility on %s", r.Rule144EligibleAt.Format("2006-01-02")),
		}, nil
	}
	if o.CountryCode != "" {
		for _, c := range r.RestrictedCountries {
			if strings.EqualFold(c, o.CountryCode) {
				return &Reason{
					Code:     "country_restricted",
					Severity: SeverityDeny,
					Message:  fmt.Sprintf("country %s restricted on offering", o.CountryCode),
				}, nil
			}
		}
	}
	if strings.EqualFold(o.Side, "buy") && r.MaxHolders > 0 && !r.InvestorAlreadyHolds {
		if r.CurrentHolders >= r.MaxHolders {
			return &Reason{
				Code:     "max_holders",
				Severity: SeverityDeny,
				Message:  fmt.Sprintf("offering at max holders (%d)", r.MaxHolders),
			}, nil
		}
	}
	// Cross-check the cap-table-side restriction view against the
	// transfer-of-record service. The two should agree; any disagreement
	// is a hard deny because we cannot tell which view is canonical at
	// order-entry time and either way the order is unsafe.
	qty, _ := strconv.ParseInt(strings.Split(o.Qty, ".")[0], 10, 64)
	if qty <= 0 {
		// Quantity may legitimately be a sub-unit decimal (fractional);
		// pass-through with a sentinel 1 so the transfer service still
		// gets the chance to deny based on per-holder per-class state.
		qty = 1
	}
	allowed, violations, err := g.tx.AllowTransfer(ctx, o.InvestorID, o.OfferingID, o.Side, qty)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return &Reason{
			Code:     "transfer_restricted",
			Severity: SeverityDeny,
			Message:  "transfer-of-record service refused: " + strings.Join(violations, "; "),
		}, nil
	}
	return nil, nil
}

func (g *Gate) checkChain(ctx context.Context, o *Order) (*Reason, error) {
	// On-chain compliance is only relevant for tokenised securities; off-
	// chain offerings carry no token address and skip this predicate.
	if o.TokenAddress == "" || g.chain == nil {
		return nil, nil
	}
	qty, _ := strconv.ParseInt(strings.Split(o.Qty, ".")[0], 10, 64)
	if qty <= 0 {
		qty = 1
	}
	// For buy: counter-party (seller) view is "from" — we do not know it
	// here; the on-chain module is symmetric over the investor/token pair
	// so we pass the investor as the receiving side. For sell: investor
	// is the sending side. The provider implementation must accept the
	// "" placeholder when the counter-party is not known at this stage.
	from, to := o.InvestorID, ""
	if strings.EqualFold(o.Side, "buy") {
		from, to = "", o.InvestorID
	}
	s, err := g.chain.CanTransfer(ctx, o.TokenAddress, from, to, qty)
	if err != nil {
		return nil, err
	}
	if !s.CanTransfer {
		msg := "erc-3643 compliance module refused"
		if s.Reason != "" {
			msg += ": " + s.Reason
		}
		return &Reason{Code: "chain_compliance_refused", Severity: SeverityDeny, Message: msg}, nil
	}
	return nil, nil
}

// --- helpers ---

func appendReason(dec *Decision, r Reason) {
	dec.Reasons = append(dec.Reasons, r)
	if action := requiredActionFor(r.Code); action != "" {
		dec.RequiredActions = append(dec.RequiredActions, action)
	}
}

func finalize(dec *Decision) {
	hasDeny := false
	hasEscalate := false
	for _, r := range dec.Reasons {
		switch r.Severity {
		case SeverityDeny:
			hasDeny = true
		case SeverityEscalate:
			hasEscalate = true
		}
	}
	switch {
	case hasDeny:
		dec.Deny = true
	case hasEscalate:
		dec.Escalate = true
	default:
		dec.Allow = true
	}
}

// requiredActionFor maps a reason code to the operational remediation
// the caller can present to the investor / operator.
func requiredActionFor(code string) string {
	switch code {
	case "kyc_not_verified", "kyc_stale", "kyc_near_expiry":
		return "complete_kyc"
	case "not_accredited", "accreditation_stale", "accreditation_near_expiry":
		return "renew_accreditation"
	case "suitability_missing", "suitability_stale":
		return "submit_suitability"
	case "suitability_ambiguous":
		return "review_suitability"
	case "aml_not_clean", "open_sar":
		return "contact_compliance"
	case "ofac_screen_stale":
		return "rescreen_ofac"
	case "lockup_active":
		return "wait_lockup_expiry"
	case "rule144_holding_period":
		return "wait_rule144_eligibility"
	case "country_restricted":
		return "ineligible_country"
	case "max_holders":
		return "offering_capacity"
	case "transfer_restricted", "chain_compliance_refused":
		return "review_restrictions"
	}
	return ""
}

// redact returns a shallow copy of the order with PII stripped for the
// audit trail. AccountID and InvestorID are stable identifiers (not
// PII) — they're retained; ClientIP is masked to /24 (IPv4) per audit-
// minimisation best-practice.
func redact(o *Order) *Order {
	if o == nil {
		return nil
	}
	cp := *o
	cp.ClientIP = maskIP(cp.ClientIP)
	return &cp
}

func maskIP(ip string) string {
	if ip == "" {
		return ""
	}
	// IPv4: keep first three octets.
	if i := strings.LastIndex(ip, "."); i > 0 && strings.Count(ip, ".") == 3 {
		return ip[:i] + ".0"
	}
	// IPv6: keep first /48 (three groups), zero the rest.
	parts := strings.Split(ip, ":")
	if len(parts) >= 4 {
		for i := 3; i < len(parts); i++ {
			parts[i] = "0"
		}
		return strings.Join(parts, ":")
	}
	return ip
}
