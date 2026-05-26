// Package northcapital is the Lux broker-side adapter for the North
// Capital TransactAPI (NCPS) regulated US BD / TA / ATS back-end.
//
// Scope: investor onboarding (Party / Entity), KYC / AML / accredited
// verification, offerings, trade booking, custody account opening,
// ATS event publication, secondary trades directory, encrypted
// webhook consumer. Conforms to the broker provider conventions
// (sibling to alpaca, securrency, sdx, apex).
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/
//
// Authored under the Independent-Implementation Clean-Room Engineering
// Protocol (legal/INDEPENDENT-IMPLEMENTATION-CLEAN-ROOM-PROTOCOL.md).
// No Counterparty source has been read. The TransactAPI public REST +
// webhook surface is the sole specification consulted, supplemented by
// the integration brief at legal/partnerships/northcapital/
// NorthCapital_Lux_Integration_Brief.md.
package northcapital

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Environment base URLs published by NCPS.
const (
	ProdURL    = "https://tapi-live.norcapsecurities.com"
	SandboxURL = "https://api-sandboxdash.norcapsecurities.com"
)

// ErrNotImplemented is returned by every endpoint method whose body
// is intentionally left as scaffolding in this pass. Each method is
// individually filled in subsequent passes against the corresponding
// TransactAPI endpoint surface.
var ErrNotImplemented = errors.New("northcapital: method not yet implemented (scaffolding pass)")

// Config carries the per-environment credentials and tuning for the
// TransactAPI adapter. Credentials are sealed under the per-tenant
// KEK in luxfi/kms; see LoadFromKMS below for the documented
// integration hook.
type Config struct {
	// BaseURL is one of ProdURL or SandboxURL. Required.
	BaseURL string

	// ClientID is the TransactAPI clientID for this environment. Required.
	ClientID string

	// DeveloperAPIKey is the TransactAPI developerAPIKey for this
	// environment. Required. Used to authenticate outbound requests
	// per TransactAPI's documented Authorization-header scheme.
	DeveloperAPIKey string

	// WebhookAuthKey is the HMAC verification key used by the
	// encrypted-webhook consumer. Loaded from KMS; never logged.
	WebhookAuthKey string

	// WebhookDecryptionKey is the symmetric key used to decrypt
	// inbound encrypted webhook payloads. Loaded from KMS alongside
	// WebhookAuthKey. Held under the same per-tenant KEK envelope.
	WebhookDecryptionKey string

	// HTTPClient may be supplied for tests / instrumentation. If nil,
	// a 30-second-timeout http.Client is constructed.
	HTTPClient *http.Client
}

// Provider implements the broker-side surface for TransactAPI. Each
// method's body is filled in a follow-up scaffolding pass against the
// corresponding TransactAPI endpoint.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New constructs a TransactAPI Provider. BaseURL must be set
// (typically to SandboxURL during development; ProdURL once NCPS
// issues production keys following sandbox conformance).
func New(cfg Config) *Provider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Provider{cfg: cfg, client: cfg.HTTPClient}
}

// Name returns the provider identifier used by the Lux smart-order
// router and the broker registry.
func (p *Provider) Name() string { return "northcapital" }

// LoadFromKMS is the documented hook for hydrating Config from
// luxfi/kms. The follow-up pass wires this through to the real
// luxfi/kms client; this placeholder defines the integration point
// (one tenant id, four KMS paths under shared/northcapital/<env>/*)
// so the call-site in brokerd can already be written to it.
//
// Expected KMS layout (per the integration brief §2):
//
//	shared/northcapital/sandbox/clientID
//	shared/northcapital/sandbox/developerAPIKey
//	shared/northcapital/sandbox/webhookAuthKey
//	shared/northcapital/sandbox/webhookDecryptionKey
//	shared/northcapital/prod/clientID
//	shared/northcapital/prod/developerAPIKey
//	shared/northcapital/prod/webhookAuthKey
//	shared/northcapital/prod/webhookDecryptionKey
//
// Each value is the per-tenant-KEK-wrapped envelope. The KMS client
// unwraps under the tenant's KEK, returning plaintext only inside this
// process's address space, never persisted.
func (cfg *Config) LoadFromKMS(_ context.Context, _ string) error {
	// TODO(scaffold/follow-up): wire luxfi/kms client; for now caller
	// constructs Config directly. The integration brief §7 enumerates
	// the exact unwrap path and the audit-trail requirements.
	return ErrNotImplemented
}

// --- HTTP helper (modeled on stripe.go's doRequestIdem) ---

// doRequestIdem issues a JSON-bodied TransactAPI call. An empty
// idemKey means "do not send an idempotency header" — used for GETs
// and for endpoints that do not accept idempotency. The TransactAPI
// idempotency-header name is reserved here as a single source of
// truth; follow-up passes per-endpoint may move to documented
// per-endpoint forms if NCPS uses a different convention there.
const idempotencyHeader = "X-Idempotency-Key"

func (p *Provider) doRequestIdem(ctx context.Context, method, path string, body any, idemKey string) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("northcapital: marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}

	// TransactAPI's documented authorization carries clientID +
	// developerAPIKey on every outbound. The exact header / body
	// placement per endpoint is pinned in the follow-up pass; using
	// the documented Authorization-header form here as the default.
	req.Header.Set("Authorization", "Bearer "+p.cfg.DeveloperAPIKey)
	req.Header.Set("X-Client-Id", p.cfg.ClientID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set(idempotencyHeader, idemKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("northcapital API %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// deriveIdempotencyKey is the broker-local mirror of
// treasury/pkg/provider.DeriveIdempotencyKey. It is intentionally
// duplicated here (rather than imported from the treasury package)
// so the broker module does not gain a transitive dep on the
// treasury module for a one-function helper. When the broker
// provider package introduces its own provider.go with a
// DeriveIdempotencyKey of its own (planned in a follow-up), this
// helper collapses to a call into that.
func deriveIdempotencyKey(orgID, endpoint string, canonicalBody []byte) string {
	h := sha256.New()
	h.Write([]byte(orgID))
	h.Write([]byte{0})
	h.Write([]byte(endpoint))
	h.Write([]byte{0})
	h.Write(canonicalBody)
	return hex.EncodeToString(h.Sum(nil))
}

// Capabilities declares what this adapter supports. Mirrored to the
// broker registry surface; consumed by the smart-order router and by
// the public capabilities-discovery endpoint.
func (p *Provider) Capabilities() *BrokerCapability {
	return &BrokerCapability{
		Name:            "northcapital",
		PaymentTypes:    []string{"ach", "wire", "credit_card", "check", "ira"},
		Features:        []string{"bd", "ta", "ats", "custody", "kyc", "aml", "accredited", "offerings", "secondary_trades"},
		Countries:       []string{"US"},
		SettlementSpeed: "t+0_to_t+2",
		Status:          "active",
	}
}
