package pretrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- mocks ---

type mockCap struct {
	acc     AccreditationStatus
	accErr  error
	suit    SuitabilityStatus
	suitErr error
	rst     RestrictionStatus
	rstErr  error
}

func (m *mockCap) Accreditation(context.Context, string) (AccreditationStatus, error) {
	return m.acc, m.accErr
}
func (m *mockCap) Suitability(context.Context, string, string) (SuitabilityStatus, error) {
	return m.suit, m.suitErr
}
func (m *mockCap) Restrictions(context.Context, string, string) (RestrictionStatus, error) {
	return m.rst, m.rstErr
}

type mockTx struct {
	allow      bool
	violations []string
	err        error
}

func (m *mockTx) AllowTransfer(context.Context, string, string, string, int64) (bool, []string, error) {
	return m.allow, m.violations, m.err
}

type mockAML struct {
	s   AMLStatus
	err error
}

func (m *mockAML) Status(context.Context, string) (AMLStatus, error) { return m.s, m.err }

type mockKYC struct {
	s   KYCStatus
	err error
}

func (m *mockKYC) Status(context.Context, string) (KYCStatus, error) { return m.s, m.err }

type mockChain struct {
	s   ChainComplianceStatus
	err error
}

func (m *mockChain) CanTransfer(context.Context, string, string, string, int64) (ChainComplianceStatus, error) {
	return m.s, m.err
}

func fixedNow() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) }

func happyOrder() *Order {
	return &Order{
		AccountID:   "acct1",
		InvestorID:  "inv1",
		OfferingID:  "off1",
		Symbol:      "OSAGE",
		Side:        "buy",
		Qty:         "10",
		Price:       "100",
		OrderType:   "limit",
		CountryCode: "US",
	}
}

func happyProviders() (*mockCap, *mockTx, *mockAML, *mockKYC, *mockChain) {
	now := fixedNow()
	return &mockCap{
			acc: AccreditationStatus{
				Verified:   true,
				Method:     "income",
				VerifiedAt: now.Add(-60 * 24 * time.Hour),
			},
			suit: SuitabilityStatus{OnFile: true, LastUpdatedAt: now.Add(-30 * 24 * time.Hour)},
			rst:  RestrictionStatus{},
		},
		&mockTx{allow: true},
		&mockAML{s: AMLStatus{Clean: true, OFACLastScreenedAt: now.Add(-7 * 24 * time.Hour)}},
		&mockKYC{s: KYCStatus{Verified: true, VerifiedAt: now.Add(-90 * 24 * time.Hour)}},
		&mockChain{s: ChainComplianceStatus{CanTransfer: true}}
}

func newGateT(t *testing.T, cap CapTableProvider, tx TransferProvider, aml AMLProvider, kyc KYCProvider, chain ChainComplianceProvider) (*Gate, *SliceSink) {
	t.Helper()
	sink := &SliceSink{}
	g, err := New(cap, tx, aml, kyc, chain, sink, DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	g.now = fixedNow
	return g, sink
}

// --- happy path ---

func TestCheck_HappyPath(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	g, sink := newGateT(t, c, tx, a, k, ch)
	d, err := g.Check(context.Background(), happyOrder())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !d.Allow || d.Deny || d.Escalate {
		t.Fatalf("expected allow, got %+v", d)
	}
	if len(d.Reasons) != 0 {
		t.Fatalf("expected no reasons, got %v", d.Reasons)
	}
	if len(sink.Entries) != 1 {
		t.Fatalf("audit: expected 1 entry, got %d", len(sink.Entries))
	}
	if d.LatencyNs < 0 {
		t.Fatalf("latency must be >= 0, got %d", d.LatencyNs)
	}
}

// --- deny paths ---

type denyCase struct {
	name string
	code string
	// mutate happy state into a failing state for this predicate.
	mut func(*mockCap, *mockTx, *mockAML, *mockKYC, *mockChain)
}

func denyCases() []denyCase {
	return []denyCase{
		{"kyc_not_verified", "kyc_not_verified", func(_ *mockCap, _ *mockTx, _ *mockAML, k *mockKYC, _ *mockChain) {
			k.s.Verified = false
		}},
		{"kyc_stale", "kyc_stale", func(_ *mockCap, _ *mockTx, _ *mockAML, k *mockKYC, _ *mockChain) {
			k.s.VerifiedAt = fixedNow().Add(-3 * 365 * 24 * time.Hour)
		}},
		{"not_accredited", "not_accredited", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.acc.Verified = false
		}},
		{"accreditation_stale_income", "accreditation_stale", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.acc.VerifiedAt = fixedNow().Add(-400 * 24 * time.Hour)
		}},
		{"accreditation_stale_third_party", "accreditation_stale", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.acc.Method = "third_party"
			c.acc.VerifiedAt = fixedNow().Add(-120 * 24 * time.Hour)
		}},
		{"suitability_missing", "suitability_missing", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.suit.OnFile = false
		}},
		{"suitability_stale", "suitability_stale", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.suit.LastUpdatedAt = fixedNow().Add(-400 * 24 * time.Hour)
		}},
		{"aml_not_clean", "aml_not_clean", func(_ *mockCap, _ *mockTx, a *mockAML, _ *mockKYC, _ *mockChain) {
			a.s.Clean = false
		}},
		{"open_sar", "open_sar", func(_ *mockCap, _ *mockTx, a *mockAML, _ *mockKYC, _ *mockChain) {
			a.s.HasOpenSAR = true
		}},
		{"ofac_screen_stale", "ofac_screen_stale", func(_ *mockCap, _ *mockTx, a *mockAML, _ *mockKYC, _ *mockChain) {
			a.s.OFACLastScreenedAt = fixedNow().Add(-60 * 24 * time.Hour)
		}},
		{"lockup_active", "lockup_active", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.rst.LockupRemainingDays = 45
		}},
		{"rule144_holding_period", "rule144_holding_period", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.rst.Rule144EligibleAt = fixedNow().Add(15 * 24 * time.Hour)
		}},
		{"country_restricted", "country_restricted", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.rst.RestrictedCountries = []string{"IR", "KP", "US"}
		}},
		{"max_holders", "max_holders", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.rst.MaxHolders = 1000
			c.rst.CurrentHolders = 1000
			c.rst.InvestorAlreadyHolds = false
		}},
		{"transfer_restricted", "transfer_restricted", func(_ *mockCap, tx *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			tx.allow = false
			tx.violations = []string{"affiliate restriction"}
		}},
		{"chain_compliance_refused", "chain_compliance_refused", func(_ *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, ch *mockChain) {
			ch.s = ChainComplianceStatus{CanTransfer: false, Reason: "module blocked"}
		}},
	}
}

func TestCheck_DenyPaths(t *testing.T) {
	for _, tc := range denyCases() {
		t.Run(tc.name, func(t *testing.T) {
			c, tx, a, k, ch := happyProviders()
			tc.mut(c, tx, a, k, ch)
			g, sink := newGateT(t, c, tx, a, k, ch)
			o := happyOrder()
			// chain check only runs when token address is set
			if tc.name == "chain_compliance_refused" {
				o.TokenAddress = "0xdeadbeef"
			}
			d, err := g.Check(context.Background(), o)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if !d.Deny {
				t.Fatalf("expected deny, got %+v", d)
			}
			if !hasReason(d, tc.code) {
				t.Fatalf("expected reason %q, got %+v", tc.code, d.Reasons)
			}
			if len(sink.Entries) != 1 || sink.Entries[0].Decision != d {
				t.Fatalf("audit: missing or wrong entry")
			}
		})
	}
}

// --- escalate paths ---

func TestCheck_EscalatePaths(t *testing.T) {
	now := fixedNow()
	cases := []struct {
		name string
		code string
		mut  func(*mockCap, *mockTx, *mockAML, *mockKYC, *mockChain)
	}{
		{"suitability_ambiguous", "suitability_ambiguous", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			c.suit.Ambiguous = true
		}},
		{"kyc_near_expiry", "kyc_near_expiry", func(_ *mockCap, _ *mockTx, _ *mockAML, k *mockKYC, _ *mockChain) {
			// KYC max = 24 months; near-expiry window = 14 days; place
			// last verification just inside the near-expiry window.
			k.s.VerifiedAt = now.Add(-(2*365*24*time.Hour - 7*24*time.Hour))
		}},
		{"accreditation_near_expiry", "accreditation_near_expiry", func(c *mockCap, _ *mockTx, _ *mockAML, _ *mockKYC, _ *mockChain) {
			// income window = 365d; near-expiry = 14d.
			c.acc.VerifiedAt = now.Add(-(365*24*time.Hour - 7*24*time.Hour))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, tx, a, k, ch := happyProviders()
			tc.mut(c, tx, a, k, ch)
			g, _ := newGateT(t, c, tx, a, k, ch)
			d, err := g.Check(context.Background(), happyOrder())
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if !d.Escalate {
				t.Fatalf("expected escalate, got %+v", d)
			}
			if !hasReason(d, tc.code) {
				t.Fatalf("expected reason %q, got %+v", tc.code, d.Reasons)
			}
		})
	}
}

// --- upstream-error propagation ---

func TestCheck_UpstreamError(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	a.err = errors.New("aml service down")
	g, _ := newGateT(t, c, tx, a, k, ch)
	d, err := g.Check(context.Background(), happyOrder())
	if err == nil {
		t.Fatalf("expected error, got decision %+v", d)
	}
}

// --- input validation ---

func TestCheck_NilOrder(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	g, _ := newGateT(t, c, tx, a, k, ch)
	if _, err := g.Check(context.Background(), nil); err == nil {
		t.Fatal("expected error on nil order")
	}
}

func TestCheck_MissingInvestorID(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	g, _ := newGateT(t, c, tx, a, k, ch)
	o := happyOrder()
	o.InvestorID = ""
	if _, err := g.Check(context.Background(), o); err == nil {
		t.Fatal("expected error on missing investor id")
	}
}

func TestCheck_MissingOfferingID(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	g, _ := newGateT(t, c, tx, a, k, ch)
	o := happyOrder()
	o.OfferingID = ""
	if _, err := g.Check(context.Background(), o); err == nil {
		t.Fatal("expected error on missing offering id")
	}
}

func TestNew_NilProviders(t *testing.T) {
	cases := []struct {
		name string
		c    CapTableProvider
		tx   TransferProvider
		a    AMLProvider
		k    KYCProvider
	}{
		{"nil_cap", nil, &mockTx{}, &mockAML{}, &mockKYC{}},
		{"nil_tx", &mockCap{}, nil, &mockAML{}, &mockKYC{}},
		{"nil_aml", &mockCap{}, &mockTx{}, nil, &mockKYC{}},
		{"nil_kyc", &mockCap{}, &mockTx{}, &mockAML{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.c, tc.tx, tc.a, tc.k, nil, nil, DefaultConfig()); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// --- chain check skipped when no token address ---

func TestCheck_ChainSkippedWithoutToken(t *testing.T) {
	c, tx, a, k, _ := happyProviders()
	ch := &mockChain{err: errors.New("should not be called")}
	g, _ := newGateT(t, c, tx, a, k, ch)
	d, err := g.Check(context.Background(), happyOrder())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !d.Allow {
		t.Fatalf("expected allow, got %+v", d)
	}
}

// --- holder check skipped for sells and for existing holders ---

func TestCheck_MaxHoldersIgnoredForSell(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	c.rst.MaxHolders = 10
	c.rst.CurrentHolders = 10
	g, _ := newGateT(t, c, tx, a, k, ch)
	o := happyOrder()
	o.Side = "sell"
	d, err := g.Check(context.Background(), o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !d.Allow {
		t.Fatalf("expected allow for sell, got %+v", d)
	}
}

func TestCheck_MaxHoldersIgnoredForExistingHolder(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	c.rst.MaxHolders = 10
	c.rst.CurrentHolders = 10
	c.rst.InvestorAlreadyHolds = true
	g, _ := newGateT(t, c, tx, a, k, ch)
	d, err := g.Check(context.Background(), happyOrder())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !d.Allow {
		t.Fatalf("expected allow for existing holder, got %+v", d)
	}
}

// --- redaction ---

func TestRedact_IPv4(t *testing.T) {
	got := maskIP("192.168.1.42")
	if got != "192.168.1.0" {
		t.Fatalf("got %q, want %q", got, "192.168.1.0")
	}
}

func TestRedact_IPv6(t *testing.T) {
	got := maskIP("2001:db8:abcd:1234:5678:9abc:def0:1111")
	if !strings.HasPrefix(got, "2001:db8:abcd:") {
		t.Fatalf("got %q, want prefix 2001:db8:abcd:", got)
	}
	if !strings.HasSuffix(got, ":0:0:0:0:0") {
		t.Fatalf("got %q, want suffix :0:0:0:0:0", got)
	}
}

func TestRedact_Empty(t *testing.T) {
	if maskIP("") != "" {
		t.Fatal("empty stays empty")
	}
}

// --- multi-reason decisions ---

func TestCheck_MultipleReasons(t *testing.T) {
	c, tx, a, k, ch := happyProviders()
	c.acc.Verified = false
	c.suit.OnFile = false
	g, _ := newGateT(t, c, tx, a, k, ch)
	d, err := g.Check(context.Background(), happyOrder())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !d.Deny {
		t.Fatalf("expected deny, got %+v", d)
	}
	if len(d.Reasons) < 2 {
		t.Fatalf("expected >=2 reasons, got %d", len(d.Reasons))
	}
}

// --- helper ---

func hasReason(d *Decision, code string) bool {
	for _, r := range d.Reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}
