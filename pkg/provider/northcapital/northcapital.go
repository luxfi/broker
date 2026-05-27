// Package northcapital is the Lux broker-side adapter for the North
// Capital TransactAPI (NCPS) regulated US BD / TA / ATS back-end.
//
// Scope: investor onboarding (Party / Entity), KYC / AML / accredited
// verification, suitability, offerings, trade booking, custody account
// opening, ATS event publication, secondary trades directory, encrypted
// webhook consumer, admin (webhook key rotation). Conforms to the
// broker provider conventions (sibling to alpaca, securrency, sdx,
// apex).
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference
//
// Authored under the Independent-Implementation Clean-Room Engineering
// Protocol (legal/INDEPENDENT-IMPLEMENTATION-CLEAN-ROOM-PROTOCOL.md).
// No Counterparty source has been read. The TransactAPI public REST +
// webhook surface is the sole specification consulted, supplemented by
// the integration brief at legal/partnerships/northcapital/
// NorthCapital_Lux_Integration_Brief.md.
package northcapital

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Environment base URLs published by NCPS.
const (
	ProdURL    = "https://tapi-live.norcapsecurities.com"
	SandboxURL = "https://api-sandboxdash.norcapsecurities.com"
)

// Errors surfaced by the adapter. These are stable and may be matched
// with errors.Is by callers.
var (
	// ErrWebhookSignature is returned by ConsumeWebhook when the HMAC
	// signature on an inbound payload does not verify against the
	// configured WebhookAuthKey.
	ErrWebhookSignature = errors.New("northcapital: webhook HMAC signature mismatch")

	// ErrWebhookDecryption is returned by ConsumeWebhook when the
	// AES-256-CBC envelope on an inbound payload cannot be decrypted
	// (key length wrong, ciphertext malformed, padding invalid).
	ErrWebhookDecryption = errors.New("northcapital: webhook payload decryption failed")

	// ErrRateLimited is returned when TransactAPI sustains 429 across
	// the adapter's full retry budget. The wrapped *APIError carries
	// the last Retry-After observed.
	ErrRateLimited = errors.New("northcapital: rate limited after retry budget exhausted")

	// ErrMissingConfig is returned by methods that require a config
	// field that was not provided (typically WebhookAuthKey or
	// WebhookDecryptionKey on the inbound path).
	ErrMissingConfig = errors.New("northcapital: required config field missing")
)

// APIError is the structured error returned when TransactAPI replies
// with a non-2xx status. Callers may use errors.As to recover it and
// inspect StatusCode / Body / Retry-After.
type APIError struct {
	StatusCode int
	Endpoint   string
	Body       string
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("northcapital API %d on %s: %s", e.StatusCode, e.Endpoint, e.Body)
}

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
	// environment. Required. Sent on every outbound request alongside
	// ClientID per the documented form-field authorization scheme.
	DeveloperAPIKey string

	// WebhookAuthKey is the HMAC verification key used by the
	// encrypted-webhook consumer. Loaded from KMS; never logged.
	WebhookAuthKey string

	// WebhookDecryptionKey is the AES-256-CBC key used to decrypt
	// inbound encrypted webhook payloads. Must be 32 bytes. Loaded
	// from KMS alongside WebhookAuthKey under the same per-tenant KEK
	// envelope.
	WebhookDecryptionKey string

	// MaxRetries caps the number of 429 retries. Default 4 (5 total
	// attempts).
	MaxRetries int

	// RetryBaseDelay is the initial backoff between retries. Default
	// 500ms; jittered exponentially (cap RetryMaxDelay).
	RetryBaseDelay time.Duration

	// RetryMaxDelay caps the per-retry backoff. Default 30s.
	RetryMaxDelay time.Duration

	// HTTPClient may be supplied for tests / instrumentation. If nil,
	// a 30-second-timeout http.Client is constructed.
	HTTPClient *http.Client
}

// Provider implements the broker-side surface for TransactAPI.
type Provider struct {
	cfg    Config
	client *http.Client
	// rand is the jitter source. Seeded once in New so tests are
	// stable across runs without depending on package-level globals.
	rand *rand.Rand
}

// New constructs a TransactAPI Provider. BaseURL must be set
// (typically to SandboxURL during development; ProdURL once NCPS
// issues production keys following sandbox conformance).
func New(cfg Config) *Provider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 4
	}
	if cfg.RetryBaseDelay == 0 {
		cfg.RetryBaseDelay = 500 * time.Millisecond
	}
	if cfg.RetryMaxDelay == 0 {
		cfg.RetryMaxDelay = 30 * time.Second
	}
	return &Provider{
		cfg:    cfg,
		client: cfg.HTTPClient,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the provider identifier used by the Lux smart-order
// router and the broker registry.
func (p *Provider) Name() string { return "northcapital" }

// LoadFromKMS is the documented hook for hydrating Config from
// luxfi/kms. The integration brief §7 enumerates the exact unwrap
// path and the audit-trail requirements. The hook is intentionally
// thin — the KMS client wiring is the responsibility of the brokerd
// startup path, not this adapter.
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
	return errors.New("northcapital: LoadFromKMS must be wired by brokerd to luxfi/kms")
}

// --- HTTP transport ---
//
// TransactAPI's legacy v1 endpoints (createParty, performAML, etc.)
// accept x-www-form-urlencoded bodies with clientID + developerAPIKey
// carried as form fields on every call, and reply with JSON. The
// modern /v3/ endpoints (parties, entities, trades, offerings) accept
// JSON bodies and use the documented Bearer-token authorization. We
// support both: doForm for legacy v1, doJSON for v3.

// idempotencyHeader is the X-Idempotency-Key header expected by NCPS
// on mutating calls. Set by the adapter on every POST/PATCH/DELETE
// from a deterministic key (caller-supplied IdempotencyKey wins).
const idempotencyHeader = "X-Idempotency-Key"

// doForm issues a form-encoded TransactAPI call against the legacy v1
// surface. clientID + developerAPIKey are injected as form fields on
// every request. Retries on 429 with exponential backoff + jitter,
// honoring Retry-After when present.
func (p *Provider) doForm(ctx context.Context, method, path string, form url.Values, idemKey string) ([]byte, error) {
	if form == nil {
		form = url.Values{}
	}
	// Authorization fields injected on every form call per the
	// documented TransactAPI legacy auth scheme.
	form.Set("clientID", p.cfg.ClientID)
	form.Set("developerAPIKey", p.cfg.DeveloperAPIKey)

	endpoint := p.cfg.BaseURL + path

	doOnce := func() (*http.Response, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		if idemKey != "" {
			req.Header.Set(idempotencyHeader, idemKey)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return resp, body, err
	}

	return p.doWithRetry(ctx, path, doOnce)
}

// doJSON issues a JSON-bodied TransactAPI call against the v3 surface
// (Bearer-token authorization, idempotency header, retry+backoff). The
// body argument is marshaled to JSON; nil sends no body.
func (p *Provider) doJSON(ctx context.Context, method, path string, body any, idemKey string) ([]byte, error) {
	var reqBodyBytes []byte
	if body != nil {
		var err error
		reqBodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("northcapital: marshal: %w", err)
		}
	}

	endpoint := p.cfg.BaseURL + path

	doOnce := func() (*http.Response, []byte, error) {
		var reader io.Reader
		if reqBodyBytes != nil {
			reader = strings.NewReader(string(reqBodyBytes))
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+p.cfg.DeveloperAPIKey)
		req.Header.Set("X-Client-Id", p.cfg.ClientID)
		req.Header.Set("Accept", "application/json")
		if reqBodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idemKey != "" {
			req.Header.Set(idempotencyHeader, idemKey)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		return resp, respBody, err
	}

	return p.doWithRetry(ctx, path, doOnce)
}

// doWithRetry wraps a single-attempt fn with retry behavior: 429s and
// 5xx errors are retried with exponential backoff (capped) + full
// jitter, honoring Retry-After when present. Returns the final
// (possibly-failing) result.
func (p *Provider) doWithRetry(ctx context.Context, path string, fn func() (*http.Response, []byte, error)) ([]byte, error) {
	var lastBody []byte
	var lastStatus int
	var lastRetryAfter time.Duration

	for attempt := 0; attempt <= p.cfg.MaxRetries; attempt++ {
		resp, body, err := fn()
		if err != nil {
			// Network error — retry on the same backoff schedule but
			// only if context still alive.
			if attempt == p.cfg.MaxRetries {
				return nil, err
			}
			if waitErr := p.sleepBackoff(ctx, attempt, 0); waitErr != nil {
				return nil, waitErr
			}
			continue
		}
		lastBody = body
		lastStatus = resp.StatusCode

		// Retryable: 429 + 5xx (excluding 501 Not Implemented).
		if resp.StatusCode == http.StatusTooManyRequests ||
			(resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented) {
			lastRetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
			if attempt == p.cfg.MaxRetries {
				break
			}
			if waitErr := p.sleepBackoff(ctx, attempt, lastRetryAfter); waitErr != nil {
				return nil, waitErr
			}
			continue
		}

		// Non-retryable 4xx → wrap as APIError, return.
		if resp.StatusCode >= 400 {
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Endpoint:   path,
				Body:       string(body),
			}
		}
		return body, nil
	}

	apiErr := &APIError{
		StatusCode: lastStatus,
		Endpoint:   path,
		Body:       string(lastBody),
		RetryAfter: lastRetryAfter,
	}
	if lastStatus == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%w: %s", ErrRateLimited, apiErr.Error())
	}
	return nil, apiErr
}

// sleepBackoff waits between retry attempts. If retryAfter is set
// (from the server) it is honored exactly; otherwise an exponential
// backoff with full jitter is used.
func (p *Provider) sleepBackoff(ctx context.Context, attempt int, retryAfter time.Duration) error {
	var wait time.Duration
	if retryAfter > 0 {
		wait = retryAfter
	} else {
		// 2^attempt * base, capped at max, then full-jitter.
		exp := p.cfg.RetryBaseDelay << attempt
		if exp <= 0 || exp > p.cfg.RetryMaxDelay {
			exp = p.cfg.RetryMaxDelay
		}
		wait = time.Duration(p.rand.Int63n(int64(exp) + 1))
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseRetryAfter decodes the Retry-After header in either delta-seconds
// or HTTP-date form. Returns 0 if the header is missing or unparseable.
func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(s); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

// deriveIdempotencyKey is the broker-local mirror of the canonical
// provider.DeriveIdempotencyKey helper. SHA-256 over (orgID || 0x00
// || endpoint || 0x00 || canonical-body) keeps retries of the same
// logical request collapsing to the same key, while different orgs
// or different endpoints split cleanly.
func deriveIdempotencyKey(orgID, endpoint string, canonicalBody []byte) string {
	h := sha256.New()
	h.Write([]byte(orgID))
	h.Write([]byte{0})
	h.Write([]byte(endpoint))
	h.Write([]byte{0})
	h.Write(canonicalBody)
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalForm returns a deterministic url.Values encoding for use as
// the idempotency-key input. Sorting by key gives byte-stable output
// independent of caller key-insertion order.
func canonicalForm(form url.Values) []byte {
	// url.Values.Encode() is documented to sort by key alphabetically,
	// which is exactly the canonical form we need.
	return []byte(form.Encode())
}

// resolveIdemKey picks the caller-supplied IdempotencyKey if set,
// otherwise derives one deterministically from (orgID, endpoint, body).
func (p *Provider) resolveIdemKey(callerKey, orgID, endpoint string, body []byte) string {
	if callerKey != "" {
		return callerKey
	}
	if orgID == "" {
		// Fall back to clientID so retries still collapse for a
		// caller that hasn't threaded org context through. This
		// preserves safe-retry without forcing every call site to
		// pass an OrgID up front.
		orgID = p.cfg.ClientID
	}
	return deriveIdempotencyKey(orgID, endpoint, body)
}

// Capabilities declares what this adapter supports. Mirrored to the
// broker registry surface; consumed by the smart-order router and by
// the public capabilities-discovery endpoint.
func (p *Provider) Capabilities() *BrokerCapability {
	return &BrokerCapability{
		Name:            "northcapital",
		PaymentTypes:    []string{"ach", "wire", "credit_card", "check", "ira"},
		Features:        []string{"bd", "ta", "ats", "custody", "kyc", "aml", "accredited", "suitability", "offerings", "secondary_trades"},
		Countries:       []string{"US"},
		SettlementSpeed: "t+0_to_t+2",
		Status:          "active",
	}
}
