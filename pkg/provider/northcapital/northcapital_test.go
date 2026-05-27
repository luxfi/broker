// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference

package northcapital

import (
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
		WebhookDecryptionKey: "0123456789abcdef0123456789abcdef", // 32 bytes
		HTTPClient:           srv.Client(),
		MaxRetries:           1,
		RetryBaseDelay:       1, // 1ns — make backoff trivial for tests
		RetryMaxDelay:        1,
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
	// Endpoint and body must each affect the key.
	if d := deriveIdempotencyKey("org-A", "/createTrade", []byte(`{"x":1}`)); d == a {
		t.Fatalf("different endpoint must produce different key")
	}
	if d := deriveIdempotencyKey("org-A", "/createParty", []byte(`{"x":2}`)); d == a {
		t.Fatalf("different body must produce different key")
	}
}
