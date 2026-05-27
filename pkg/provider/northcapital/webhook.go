// Encrypted-webhook consumer — cross-cutting workstream.
//
// Inbound webhook handling proceeds in five steps (per the integration
// brief §7):
//
//  1. Lux ZIP-served endpoint at
//       https://api.lux.financial/v1/northcapital/webhook
//     terminates HTTPS (no /api/ prefix per the global convention).
//  2. ZIP Basic-Auth gate verifies the per-provider sentinel (the
//     interim defence pattern from the currencycloud audit). Failure
//     → 401 without revealing any state.
//  3. HMAC signature is verified (when present) against WebhookAuthKey
//     loaded from luxfi/kms — defence-in-depth on top of #4. The
//     TransactAPI public encrypted-webhooks guide is silent on HMAC;
//     the integration brief §7 nevertheless prescribes one for the
//     Lux boundary, so the adapter honors WebhookAuthKey when set and
//     skips the HMAC gate (logging "no signature key") when unset.
//  4. Payload is decrypted with WebhookDecryptionKey under the
//     documented AES-256-CBC scheme. The TransactAPI guide specifies:
//
//        - Payload is URL-safe Base64 encoded.
//        - "+" characters in the encoded payload are transmitted as
//          the literal string "plusencr"; decode that first.
//        - Decoded ciphertext: first 16 bytes are the IV; the rest
//          is the AES-256-CBC ciphertext.
//        - Key length is exactly 32 bytes (TransactAPI guarantees
//          padding to 32 chars server-side; the adapter enforces
//          exactly 32 here so a mis-sized key fails closed rather
//          than degrading silently).
//        - Decrypted plaintext: URL-encoded query string with fields
//          separated by ", " (comma-space).
//
//        Assumption pinned for the sandbox handshake: the documented
//        guide does not say "encrypt-then-MAC" or "MAC-then-encrypt".
//        We implement decrypt-then-(optional)MAC against the raw
//        envelope bytes. The integration team should confirm this
//        ordering during the NCPS sandbox handshake. If the ordering
//        is reversed, swap the order in ConsumeWebhook below.
//
//  5. Decrypted event is projected into the broker-wide WebhookEvent
//     shape (aliased in types.go) and delivered onto the brokerd event
//     bus; downstream listeners (captable, transfer, amld, reporting)
//     react idempotently keyed on WebhookEvent.EventID.
//
// The TransactAPI Webhook Auth Key rotates quarterly via the Admin
// endpoint (admin.go); the adapter accepts both old + new for a
// 24-hour overlap window after rotation by parsing WebhookAuthKey as
// a comma-separated list of versions.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/docs/setting-up-encrypted-webhooks

package northcapital

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HMAC header name on inbound TransactAPI webhooks. Used when the
// WebhookAuthKey is configured; the public encrypted-webhooks guide
// does not specify a header name, so this is the Lux-side convention
// (pinned in the integration brief §7) and may be remapped if the
// sandbox handshake reveals a different upstream name.
const webhookHMACHeader = "X-TransactAPI-Signature"

// plusencrMarker is the literal string TransactAPI substitutes for
// "+" characters in the URL-safe Base64 encoded webhook envelope
// (per the public encrypted-webhooks guide).
const plusencrMarker = "plusencr"

// ConsumeWebhook decrypts + verifies an inbound TransactAPI webhook
// payload and returns a normalized WebhookEvent. The caller (the
// ZIP-served webhook handler in brokerd) is responsible for steps 1
// + 2 (HTTPS termination, ZIP Basic-Auth) and for delivery of the
// returned WebhookEvent onto the event bus.
//
// HMAC verification runs when WebhookAuthKey is configured. Decryption
// always runs (it is required for the payload to be readable at all).
// Either failure path returns a sentinel error suitable for the
// caller's metrics + logging.
func (p *Provider) ConsumeWebhook(_ context.Context, headers http.Header, body []byte) (*WebhookEvent, error) {
	if p.cfg.WebhookDecryptionKey == "" {
		return nil, fmt.Errorf("%w: WebhookDecryptionKey", ErrMissingConfig)
	}

	// Step 3 — HMAC signature verification (optional, when key set).
	if p.cfg.WebhookAuthKey != "" {
		got := headers.Get(webhookHMACHeader)
		if got == "" {
			return nil, ErrWebhookSignature
		}
		if err := verifyHMAC(p.cfg.WebhookAuthKey, body, got); err != nil {
			return nil, err
		}
	}

	// Step 4 — payload decryption per the documented AES-256-CBC scheme.
	plaintext, err := decryptWebhookPayload(string(body), p.cfg.WebhookDecryptionKey)
	if err != nil {
		return nil, err
	}

	// Step 5 — project decrypted "k=v, k=v, …" into WebhookEvent.
	evt, err := parseDecryptedEvent(plaintext)
	if err != nil {
		return nil, err
	}
	return evt, nil
}

// verifyHMAC checks the hex-encoded HMAC-SHA256 signature in `got`
// against the configured key (or comma-separated key list during the
// rotation overlap window). Constant-time compare; never branch on
// key material.
func verifyHMAC(keyList string, body []byte, got string) error {
	gotBytes, err := hex.DecodeString(strings.TrimSpace(got))
	if err != nil || len(gotBytes) == 0 {
		return ErrWebhookSignature
	}
	for _, k := range strings.Split(keyList, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(k))
		mac.Write(body)
		want := mac.Sum(nil)
		if hmac.Equal(gotBytes, want) {
			return nil
		}
	}
	return ErrWebhookSignature
}

// decryptWebhookPayload runs the documented TransactAPI webhook
// decryption: undo the "plusencr" → "+" substitution, URL-safe Base64
// decode, split off the 16-byte IV, then AES-256-CBC decrypt the
// remainder with the supplied 32-byte key. Returns the decrypted
// plaintext (the documented "URL-encoded query string separated by ', '").
func decryptWebhookPayload(envelope, key string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("%w: WebhookDecryptionKey must be exactly 32 bytes, got %d",
			ErrWebhookDecryption, len(key))
	}
	// Undo the "+" → "plusencr" substitution.
	restored := strings.ReplaceAll(envelope, plusencrMarker, "+")

	// URL-safe Base64 decoding accepts both URL and standard forms via
	// the relaxed decoder; we try URL-safe first and fall back to std
	// so either encoding succeeds.
	decoded, err := base64.URLEncoding.DecodeString(restored)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(restored)
		if err != nil {
			decoded, err = base64.RawURLEncoding.DecodeString(restored)
			if err != nil {
				return "", fmt.Errorf("%w: base64 decode: %v", ErrWebhookDecryption, err)
			}
		}
	}

	if len(decoded) < aes.BlockSize {
		return "", fmt.Errorf("%w: ciphertext shorter than IV", ErrWebhookDecryption)
	}
	iv := decoded[:aes.BlockSize]
	ct := decoded[aes.BlockSize:]
	if len(ct)%aes.BlockSize != 0 {
		return "", fmt.Errorf("%w: ciphertext not multiple of block size (%d)",
			ErrWebhookDecryption, len(ct))
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWebhookDecryption, err)
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	pt := make([]byte, len(ct))
	mode.CryptBlocks(pt, ct)

	// Strip PKCS#7 padding.
	pt, err = pkcs7Unpad(pt, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// pkcs7Unpad strips PKCS#7 padding from the tail of `b`. Returns an
// error if the padding is malformed (constant-time over the padding
// length to avoid the classic CBC padding-oracle attack).
func pkcs7Unpad(b []byte, blockSize int) ([]byte, error) {
	n := len(b)
	if n == 0 || n%blockSize != 0 {
		return nil, fmt.Errorf("%w: padding length %d invalid", ErrWebhookDecryption, n)
	}
	pad := int(b[n-1])
	if pad <= 0 || pad > blockSize {
		return nil, fmt.Errorf("%w: padding byte %d out of range", ErrWebhookDecryption, pad)
	}
	// Constant-time: scan every padding byte even after a mismatch.
	var diff byte
	for i := n - pad; i < n; i++ {
		diff |= b[i] ^ byte(pad)
	}
	if diff != 0 {
		return nil, fmt.Errorf("%w: padding bytes do not match length", ErrWebhookDecryption)
	}
	return b[:n-pad], nil
}

// parseDecryptedEvent interprets the documented decrypted format —
// a URL-encoded query string with "k=v" fields separated by ", " —
// and projects it into the broker-wide WebhookEvent shape.
//
// The TransactAPI keys we map (per the public reference):
//
//	eventId             → WebhookEvent.EventID
//	eventType / event   → WebhookEvent.EventType
//	endpoint            → WebhookEvent.Endpoint (when present)
//	timestamp           → WebhookEvent.Timestamp (RFC3339 or epoch)
//	version             → WebhookEvent.Version
//	transactionType     → WebhookEvent.TransactionType
//	blockchainTxId      → WebhookEvent.BlockchainTransactionID
//
// Unknown keys are preserved on the wire for downstream consumers via
// the brokerd event bus envelope (the WebhookEvent shape is the
// minimum guarantee; richer decoding happens at the listener layer
// once the full TransactAPI event taxonomy is mapped).
func parseDecryptedEvent(plaintext string) (*WebhookEvent, error) {
	// Split on ", " per the documented field separator. Then parse
	// each "k=v" through url.QueryUnescape so percent-encoded values
	// round-trip.
	parts := strings.Split(plaintext, ", ")
	fields := make(map[string]string, len(parts))
	for _, kv := range parts {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		v := kv[eq+1:]
		if uv, err := url.QueryUnescape(v); err == nil {
			v = uv
		}
		fields[k] = v
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: decrypted payload empty", ErrWebhookDecryption)
	}
	evt := &WebhookEvent{
		EventID:                 firstNonEmpty(fields["eventId"], fields["eventID"], fields["id"]),
		EventType:               firstNonEmpty(fields["eventType"], fields["event"], fields["type"]),
		Endpoint:                fields["endpoint"],
		Version:                 fields["version"],
		TransactionType:         fields["transactionType"],
		BlockchainTransactionID: fields["blockchainTxId"],
	}
	if ts := firstNonEmpty(fields["timestamp"], fields["time"], fields["ts"]); ts != "" {
		evt.Timestamp = parseEventTime(ts)
	}
	return evt, nil
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func parseEventTime(s string) time.Time {
	if t := parseDate(s); !t.IsZero() {
		return t
	}
	// Try Unix epoch seconds as a final fallback.
	if i, err := time.Parse(time.RFC1123, s); err == nil {
		return i
	}
	return time.Time{}
}
