// Conformance tests for LoadFromKMS. Drive a mock KMSGetter and
// assert:
//
//  - successful load populates clientID, developerAPIKey, webhookAuthKey
//  - missing required key returns ErrKMSKeyMissing
//  - KMS transport/decryption error returns ErrKMSDecryption
//  - missing optional webhookDecryptionKey is tolerated
//  - transport error on optional key is still fatal
//  - env validation rejects unknown env values
//  - nil KMSGetter rejected
//  - empty value treated as missing
//  - exact KMS paths follow shared/northcapital/<env>/<field>
//
// No production KMS traffic. All fixtures are in-process.
//
// Source-of-design: Public-Spec
// Source-ref: legal/partnerships/northcapital/
// NorthCapital_Lux_Integration_Brief.md §2.

package northcapital

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
)

// mockKMS is an in-memory KMSGetter. Configurable per-key value or
// error; tracks the order calls came in so tests can assert path
// shape.
type mockKMS struct {
	mu       sync.Mutex
	values   map[string]string
	errs     map[string]error
	called   []string
	notFound error // returned for paths absent from both maps
}

func newMockKMS() *mockKMS {
	return &mockKMS{
		values:   make(map[string]string),
		errs:     make(map[string]error),
		notFound: errors.New("zapclient: secret not found"),
	}
}

func (m *mockKMS) Get(_ context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = append(m.called, name)
	if err, ok := m.errs[name]; ok {
		return "", err
	}
	if v, ok := m.values[name]; ok {
		return v, nil
	}
	return "", m.notFound
}

func (m *mockKMS) calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.called))
	copy(out, m.called)
	return out
}

// --- happy paths ---

func TestLoadFromKMS_SandboxSuccess(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/clientID"] = "cid-sandbox"
	kms.values["shared/northcapital/sandbox/developerAPIKey"] = "dak-sandbox"
	kms.values["shared/northcapital/sandbox/webhookAuthKey"] = "whk-sandbox"
	kms.values["shared/northcapital/sandbox/webhookDecryptionKey"] = "01234567890123456789012345678901"

	var cfg Config
	if err := cfg.LoadFromKMS(context.Background(), kms, "sandbox"); err != nil {
		t.Fatalf("LoadFromKMS: %v", err)
	}
	if cfg.ClientID != "cid-sandbox" {
		t.Fatalf("ClientID: got %q, want cid-sandbox", cfg.ClientID)
	}
	if cfg.DeveloperAPIKey != "dak-sandbox" {
		t.Fatalf("DeveloperAPIKey: got %q", cfg.DeveloperAPIKey)
	}
	if cfg.WebhookAuthKey != "whk-sandbox" {
		t.Fatalf("WebhookAuthKey: got %q", cfg.WebhookAuthKey)
	}
	if cfg.WebhookDecryptionKey == "" {
		t.Fatal("WebhookDecryptionKey: empty after successful load")
	}

	// Path shape: every fetched name MUST follow
	// shared/northcapital/sandbox/<field>.
	got := kms.calls()
	sort.Strings(got)
	want := []string{
		"shared/northcapital/sandbox/clientID",
		"shared/northcapital/sandbox/developerAPIKey",
		"shared/northcapital/sandbox/webhookAuthKey",
		"shared/northcapital/sandbox/webhookDecryptionKey",
	}
	sort.Strings(want)
	if !equalStrSlice(got, want) {
		t.Fatalf("KMS path shape: got %v, want %v", got, want)
	}
}

func TestLoadFromKMS_ProdSuccess(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/prod/clientID"] = "cid-prod"
	kms.values["shared/northcapital/prod/developerAPIKey"] = "dak-prod"
	kms.values["shared/northcapital/prod/webhookAuthKey"] = "whk-prod"
	// webhookDecryptionKey deliberately absent — should be tolerated.

	var cfg Config
	cfg.WebhookDecryptionKey = "preserved-fallback"
	if err := cfg.LoadFromKMS(context.Background(), kms, "prod"); err != nil {
		t.Fatalf("LoadFromKMS: %v", err)
	}
	if cfg.ClientID != "cid-prod" || cfg.DeveloperAPIKey != "dak-prod" || cfg.WebhookAuthKey != "whk-prod" {
		t.Fatalf("required fields: %+v", cfg)
	}
	// Missing optional must not clobber a caller-supplied value.
	if cfg.WebhookDecryptionKey != "preserved-fallback" {
		t.Fatalf("WebhookDecryptionKey clobbered: %q", cfg.WebhookDecryptionKey)
	}
}

// --- missing required keys ---

func TestLoadFromKMS_MissingClientID(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/developerAPIKey"] = "dak"
	kms.values["shared/northcapital/sandbox/webhookAuthKey"] = "whk"

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSKeyMissing) {
		t.Fatalf("missing clientID: got %v, want errors.Is ErrKMSKeyMissing", err)
	}
}

func TestLoadFromKMS_MissingDeveloperAPIKey(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/clientID"] = "cid"
	kms.values["shared/northcapital/sandbox/webhookAuthKey"] = "whk"

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSKeyMissing) {
		t.Fatalf("got %v, want ErrKMSKeyMissing", err)
	}
}

func TestLoadFromKMS_MissingWebhookAuthKey(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/clientID"] = "cid"
	kms.values["shared/northcapital/sandbox/developerAPIKey"] = "dak"

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSKeyMissing) {
		t.Fatalf("got %v, want ErrKMSKeyMissing", err)
	}
}

func TestLoadFromKMS_EmptyValueTreatedAsMissing(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/clientID"] = ""
	kms.values["shared/northcapital/sandbox/developerAPIKey"] = "dak"
	kms.values["shared/northcapital/sandbox/webhookAuthKey"] = "whk"

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSKeyMissing) {
		t.Fatalf("empty clientID: got %v, want ErrKMSKeyMissing", err)
	}
}

// --- transport / decryption errors ---

func TestLoadFromKMS_DecryptionError(t *testing.T) {
	kms := newMockKMS()
	kms.errs["shared/northcapital/sandbox/clientID"] = fmt.Errorf("aead: decrypt: bad tag")

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSDecryption) {
		t.Fatalf("decryption error: got %v, want errors.Is ErrKMSDecryption", err)
	}
}

func TestLoadFromKMS_TransportError(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/clientID"] = "cid"
	kms.values["shared/northcapital/sandbox/developerAPIKey"] = "dak"
	kms.errs["shared/northcapital/sandbox/webhookAuthKey"] = fmt.Errorf("dial tcp: i/o timeout")

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSDecryption) {
		t.Fatalf("transport error: got %v, want errors.Is ErrKMSDecryption", err)
	}
}

func TestLoadFromKMS_TransportErrorOnOptionalKeyIsFatal(t *testing.T) {
	kms := newMockKMS()
	kms.values["shared/northcapital/sandbox/clientID"] = "cid"
	kms.values["shared/northcapital/sandbox/developerAPIKey"] = "dak"
	kms.values["shared/northcapital/sandbox/webhookAuthKey"] = "whk"
	// Transport error on the optional key — should still be fatal.
	// "Optional" means missing is tolerated; corruption / transport
	// failure on a key that exists must not be silently ignored.
	kms.errs["shared/northcapital/sandbox/webhookDecryptionKey"] = fmt.Errorf("kms: dial unreachable")

	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "sandbox")
	if !errors.Is(err, ErrKMSDecryption) {
		t.Fatalf("transport error on optional: got %v, want errors.Is ErrKMSDecryption", err)
	}
}

// --- argument validation ---

func TestLoadFromKMS_NilGetter(t *testing.T) {
	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), nil, "sandbox")
	if err == nil {
		t.Fatal("nil KMSGetter: expected error, got nil")
	}
}

func TestLoadFromKMS_EmptyEnv(t *testing.T) {
	kms := newMockKMS()
	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "")
	if err == nil {
		t.Fatal("empty env: expected error, got nil")
	}
}

func TestLoadFromKMS_UnknownEnv(t *testing.T) {
	kms := newMockKMS()
	var cfg Config
	err := cfg.LoadFromKMS(context.Background(), kms, "staging")
	if err == nil {
		t.Fatal("unknown env: expected error, got nil")
	}
}

// --- helpers ---

func equalStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestKMSPath(t *testing.T) {
	cases := []struct {
		env, field, want string
	}{
		{"sandbox", "clientID", "shared/northcapital/sandbox/clientID"},
		{"prod", "developerAPIKey", "shared/northcapital/prod/developerAPIKey"},
		{"prod", "webhookAuthKey", "shared/northcapital/prod/webhookAuthKey"},
	}
	for _, tc := range cases {
		if got := kmsPath(tc.env, tc.field); got != tc.want {
			t.Fatalf("kmsPath(%q,%q)=%q want %q", tc.env, tc.field, got, tc.want)
		}
	}
}
