// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestProvider returns a Provider whose HTTP client targets the
// supplied httptest.Server. No production HTTP traffic ever leaves
// the test process.
func newTestProvider(t *testing.T, h http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(Config{
		BaseURL:              srv.URL,
		ClientID:             "test-client",
		DeveloperAPIKey:      "test-key",
		WebhookAuthKey:       "test-webhook-key",
		WebhookDecryptionKey: "test-decrypt-key",
		HTTPClient:           srv.Client(),
	})
}

// TestNew verifies that a Provider can be constructed against the
// sandbox base URL with the documented credential fields, and that
// the Name() + Capabilities() surface is wired up.
func TestNew(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "sandbox",
			cfg:  Config{BaseURL: SandboxURL, ClientID: "cid", DeveloperAPIKey: "dak"},
		},
		{
			name: "prod",
			cfg:  Config{BaseURL: ProdURL, ClientID: "cid", DeveloperAPIKey: "dak"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := New(tc.cfg)
			if p == nil {
				t.Fatal("New returned nil")
			}
			if p.Name() != "northcapital" {
				t.Fatalf("Name: got %q, want %q", p.Name(), "northcapital")
			}
			caps := p.Capabilities()
			if caps == nil || caps.Name != "northcapital" {
				t.Fatalf("Capabilities: got %+v", caps)
			}
			if caps.Status != "active" {
				t.Fatalf("Capabilities.Status: got %q, want %q", caps.Status, "active")
			}
			if len(caps.PaymentTypes) == 0 || len(caps.Features) == 0 {
				t.Fatal("Capabilities must declare PaymentTypes + Features")
			}
		})
	}
}

// TestStubsReturnNotImplemented locks in the scaffolding contract:
// every endpoint method returns ErrNotImplemented until its follow-up
// implementation pass lands. Catching this regression early is the
// whole point of the stub harness.
func TestStubsReturnNotImplemented(t *testing.T) {
	p := newTestProvider(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("scaffolding stubs must not issue HTTP traffic")
	})
	ctx := context.Background()

	if _, err := p.CreateParty(ctx, &CreatePartyRequest{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("CreateParty: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.CreateEntity(ctx, &CreateEntityRequest{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("CreateEntity: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.GetParty(ctx, "p_1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("GetParty: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.ListParties(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("ListParties: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.PerformKYC(ctx, "p_1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("PerformKYC: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.PerformAML(ctx, "p_1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("PerformAML: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.PerformAccredited(ctx, "p_1", AccreditedByIncome); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("PerformAccredited: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.CreateOffering(ctx, &CreateOfferingRequest{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("CreateOffering: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.GetOffering(ctx, "o_1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("GetOffering: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.ListOfferings(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("ListOfferings: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.CreateTrade(ctx, &CreateTradeRequest{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("CreateTrade: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.GetTrade(ctx, "t_1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("GetTrade: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.BulkUploadTrades(ctx, nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("BulkUploadTrades: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.OpenCustodyAccount(ctx, "p_1", &CustodyOpenRequest{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("OpenCustodyAccount: got %v, want ErrNotImplemented", err)
	}
	if err := p.PublishATSEvent(ctx, &ATSEvent{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("PublishATSEvent: got %v, want ErrNotImplemented", err)
	}
	if _, err := p.GetSecondaryTrades(ctx, "o_1"); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("GetSecondaryTrades: got %v, want ErrNotImplemented", err)
	}
}

// TestDeriveIdempotencyKey_Deterministic locks the (orgID, endpoint,
// canonical-body) → key contract for the broker-local helper.
func TestDeriveIdempotencyKey_Deterministic(t *testing.T) {
	a := deriveIdempotencyKey("org-A", "/createParty", []byte(`{"x":1}`))
	b := deriveIdempotencyKey("org-A", "/createParty", []byte(`{"x":1}`))
	c := deriveIdempotencyKey("org-B", "/createParty", []byte(`{"x":1}`))
	if a == "" {
		t.Fatal("deriveIdempotencyKey must produce a non-empty key")
	}
	if a != b {
		t.Fatalf("deterministic call must collapse to the same key: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("different orgIDs must produce different keys; both got %q", a)
	}
}

// TestConsumeWebhook_RejectsMissingSignature exercises the HMAC
// gate: a webhook with no signature header is rejected before any
// decryption is attempted.
func TestConsumeWebhook_RejectsMissingSignature(t *testing.T) {
	p := newTestProvider(t, nil)
	_, err := p.ConsumeWebhook(context.Background(), http.Header{}, []byte(`{}`))
	if !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("ConsumeWebhook: got %v, want ErrWebhookSignature", err)
	}
}

// TestConsumeWebhook_RejectsBadSignature: a webhook with a present
// but wrong signature is rejected. (Correct-signature path returns
// ErrNotImplemented in this scaffolding pass.)
func TestConsumeWebhook_RejectsBadSignature(t *testing.T) {
	p := newTestProvider(t, nil)
	h := http.Header{}
	h.Set(webhookHMACHeader, "00aa")
	_, err := p.ConsumeWebhook(context.Background(), h, []byte(`{}`))
	if !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("ConsumeWebhook: got %v, want ErrWebhookSignature", err)
	}
}
