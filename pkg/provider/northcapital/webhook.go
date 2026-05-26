// Encrypted-webhook consumer — TransactAPI Workstream 1-8 (cross-cutting).
//
// Inbound webhook handling proceeds in five steps (per the integration
// brief §7):
//
//  1. Lux ZIP-served endpoint at
//       https://api.lux.financial/v1/northcapital/webhook
//     terminates the HTTPS connection (no /api/ prefix per the global
//     convention; everything under /v1/).
//  2. ZIP Basic-Auth gate verifies the per-provider sentinel (the
//     interim defence pattern carried over from the currencycloud
//     audit). Failure → 401 without revealing any state.
//  3. HMAC signature is verified against WebhookAuthKey loaded from
//     luxfi/kms (unwrap under per-tenant KEK; the unwrap call is
//     logged to the KMS audit trail).
//  4. Payload is decrypted with WebhookDecryptionKey (separate sub-key
//     in the same KMS envelope). The TransactAPI encrypted-webhook
//     scheme — symmetric envelope + HMAC — is documented at
//     https://transactapi.readme.io/ ("Setting Up Encrypted Webhooks").
//  5. Decrypted event maps into the broker-wide WebhookEvent shape
//     (aliased in types.go) and is delivered onto the brokerd event
//     bus; downstream listeners (captable, transfer, amld, reporting)
//     react idempotently keyed on WebhookEvent.EventID.
//
// The TransactAPI Webhook Auth Key rotates quarterly. The adapter
// accepts both old + new for a 24-hour overlap window after rotation
// is written into KMS — both keys live in the same envelope for that
// window, with key_version distinguishing them.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/

package northcapital

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
)

// HMAC header name on inbound TransactAPI webhooks. Reserved here as
// the single source of truth for the adapter; pinned to the documented
// value in the follow-up pass once the canonical name is confirmed
// against the public guide.
const webhookHMACHeader = "X-TransactAPI-Signature"

// ErrWebhookSignature is returned by ConsumeWebhook when the HMAC
// signature on an inbound payload does not verify against the
// configured WebhookAuthKey.
var ErrWebhookSignature = errors.New("northcapital: webhook HMAC signature mismatch")

// ConsumeWebhook decrypts + verifies an inbound TransactAPI webhook
// payload and returns a normalized WebhookEvent. Step 3 (HMAC
// verification) is implemented here as the wire-skeleton; Step 4
// (payload decryption) is TODO and deliberately left as a clear
// follow-up — the symmetric-envelope decryption requires the published
// TransactAPI key-derivation/ciphersuite confirmation against the
// "Setting Up Encrypted Webhooks" guide before being committed.
//
// The caller (the ZIP-served webhook handler in brokerd) is responsible
// for steps 1 + 2 (HTTPS termination, ZIP Basic-Auth) and for delivery
// of the returned WebhookEvent onto the event bus.
func (p *Provider) ConsumeWebhook(_ context.Context, headers http.Header, body []byte) (*WebhookEvent, error) {
	// Step 3 — HMAC signature verification (scaffolding-pass implementation).
	if p.cfg.WebhookAuthKey == "" {
		// Refuse to proceed without a key. Easier to detect a
		// mis-provisioning failure than to silently degrade.
		return nil, errors.New("northcapital: webhook auth key not configured")
	}
	got := headers.Get(webhookHMACHeader)
	if got == "" {
		return nil, ErrWebhookSignature
	}
	mac := hmac.New(sha256.New, []byte(p.cfg.WebhookAuthKey))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	// Constant-time compare; never branch on the key material.
	gotBytes, err := hex.DecodeString(got)
	if err != nil {
		return nil, ErrWebhookSignature
	}
	wantBytes, _ := hex.DecodeString(want)
	if !hmac.Equal(gotBytes, wantBytes) {
		return nil, ErrWebhookSignature
	}

	// Step 4 — payload decryption: TODO(scaffold/follow-up).
	// The TransactAPI encrypted-webhook scheme uses a symmetric-key
	// envelope; the exact ciphersuite + key-derivation must be pinned
	// against the public "Setting Up Encrypted Webhooks" guide before
	// implementation lands. Until then this method intentionally
	// returns ErrNotImplemented so callers cannot accidentally treat
	// an unverified-decryption path as success.
	return nil, ErrNotImplemented
}
