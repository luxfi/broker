// KMS hydration for the broker-side TransactAPI adapter. The
// canonical luxfi/kms client (github.com/luxfi/kms) returns the
// caller a plaintext value per name; the per-tenant-KEK unwrap
// happens server-side in the KMS process. The adapter therefore
// holds a small KMSGetter interface — narrowly the surface
// luxfi/kms.GetWith satisfies — so the adapter has zero direct
// dependency on luxfi/kms while remaining trivially wireable from
// brokerd startup.
//
// brokerd wiring sketch (kept out of this file to keep the
// dependency direction one-way):
//
//	import "github.com/luxfi/kms"
//
//	type kmsAdapter struct{ cfg kms.Config }
//	func (a *kmsAdapter) Get(ctx context.Context, name string) (string, error) {
//	    return kms.GetWith(ctx, a.cfg, name)
//	}
//
//	cfg := northcapital.Config{BaseURL: northcapital.SandboxURL}
//	if err := cfg.LoadFromKMS(ctx, &kmsAdapter{cfg: kms.Config{Env: env}}, "sandbox"); err != nil {
//	    log.Fatalf("northcapital LoadFromKMS: %v", err)
//	}
//	p := northcapital.New(cfg)
//
// Source-of-design: Public-Spec
// Source-ref: legal/partnerships/northcapital/
// NorthCapital_Lux_Integration_Brief.md §2; github.com/luxfi/kms
// public Go surface (kms.Get / kms.GetWith).
//
// Authored under the Independent-Implementation Clean-Room Engineering
// Protocol (legal/INDEPENDENT-IMPLEMENTATION-CLEAN-ROOM-PROTOCOL.md).

package northcapital

import (
	"context"
	"errors"
	"fmt"
)

// KMSGetter is the minimal surface LoadFromKMS depends on. The
// canonical implementation is a one-line wrapper around
// luxfi/kms.GetWith; callers free to substitute any secret-store
// surface that returns plaintext values per name. The interface
// returns the plaintext value of name — the unwrap-under-the-per-
// tenant-KEK happens server-side in the KMS process, never inside
// the adapter.
type KMSGetter interface {
	// Get returns the plaintext secret value for name. Implementations
	// MUST return a typed sentinel (or wrap one) for "not found" so
	// LoadFromKMS can distinguish that from a transport / decryption
	// failure; the adapter uses errors.Is against ErrKMSKeyMissing.
	Get(ctx context.Context, name string) (string, error)
}

// ErrKMSKeyMissing is the sentinel LoadFromKMS returns when one of
// the required KMS paths is absent. Callers MUST treat this as a
// hard error — the adapter cannot operate without the credential.
// Wraps the underlying KMSGetter error so callers can errors.Is it
// AND inspect the underlying for diagnostics.
var ErrKMSKeyMissing = errors.New("northcapital: required KMS key missing")

// ErrKMSDecryption is the sentinel LoadFromKMS returns when the KMS
// retrieval failed for a transport or decryption reason (anything
// other than "not found"). Wraps the underlying error.
var ErrKMSDecryption = errors.New("northcapital: KMS decryption / transport error")

// kmsPath returns the canonical KMS key name for (env, field) per
// the integration brief §2:
//
//	shared/northcapital/<env>/<field>
//
// env is "sandbox" or "prod"; field is one of
// clientID / developerAPIKey / webhookAuthKey / webhookDecryptionKey.
func kmsPath(env, field string) string {
	return "shared/northcapital/" + env + "/" + field
}

// loadOne fetches one KMS path with the documented error mapping.
// "not found" → ErrKMSKeyMissing; anything else → ErrKMSDecryption.
// The underlying error is wrapped so errors.Unwrap reaches it.
func loadOne(ctx context.Context, kms KMSGetter, env, field string) (string, error) {
	path := kmsPath(env, field)
	v, err := kms.Get(ctx, path)
	if err != nil {
		if isMissingErr(err) {
			return "", fmt.Errorf("%w: %s: %w", ErrKMSKeyMissing, path, err)
		}
		return "", fmt.Errorf("%w: %s: %w", ErrKMSDecryption, path, err)
	}
	if v == "" {
		return "", fmt.Errorf("%w: %s (empty value)", ErrKMSKeyMissing, path)
	}
	return v, nil
}

// isMissingErr classifies an error as "not found". luxfi/kms surfaces
// not-found via the zapclient.ErrNotFound sentinel, which is
// reachable from errors.Is. We also accept plain errors carrying
// "not found" in the message text so any KMSGetter wrapper without
// an explicit sentinel works out of the box.
func isMissingErr(err error) bool {
	if err == nil {
		return false
	}
	// String-form check covers wrapping layers that lose the typed
	// sentinel; the canonical luxfi/kms client preserves the typed
	// error through GetWith so errors.Is works first. We deliberately
	// do not import luxfi/kms here to keep the dep direction clean.
	msg := err.Error()
	return contains(msg, "not found") || contains(msg, "NotFound")
}

// contains is a strings.Contains alias that avoids the package
// import in this file (which would force every consumer of the
// adapter to compile in strings just for the kms wiring).
func contains(s, substr string) bool {
	if substr == "" {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// LoadFromKMS hydrates cfg from the documented KMS paths under
//
//	shared/northcapital/<env>/clientID
//	shared/northcapital/<env>/developerAPIKey
//	shared/northcapital/<env>/webhookAuthKey
//	shared/northcapital/<env>/webhookDecryptionKey   (optional)
//
// env is one of "sandbox" or "prod". The clientID, developerAPIKey,
// and webhookAuthKey are required; webhookDecryptionKey is optional
// (only required when the inbound webhook pipeline is active —
// adapters that only call outbound endpoints can omit it). A missing
// required key returns ErrKMSKeyMissing; a KMS transport / decryption
// failure returns ErrKMSDecryption.
//
// LoadFromKMS does NOT set BaseURL — that is environment-shape, not
// secret-shape — so the caller decides between SandboxURL and
// ProdURL. Typical wiring:
//
//	cfg := northcapital.Config{BaseURL: northcapital.SandboxURL}
//	if err := cfg.LoadFromKMS(ctx, kmsAdapter, "sandbox"); err != nil { ... }
//	p := northcapital.New(cfg)
//
// On success every required field on cfg is populated; the optional
// webhookDecryptionKey is populated when present in KMS and left
// untouched when absent (treated as not-required).
func (cfg *Config) LoadFromKMS(ctx context.Context, kms KMSGetter, env string) error {
	if kms == nil {
		return errors.New("northcapital: LoadFromKMS requires a non-nil KMSGetter")
	}
	if env == "" {
		return errors.New("northcapital: LoadFromKMS requires env (\"sandbox\" or \"prod\")")
	}
	if env != "sandbox" && env != "prod" {
		return fmt.Errorf("northcapital: LoadFromKMS env %q not recognized (want \"sandbox\" or \"prod\")", env)
	}

	clientID, err := loadOne(ctx, kms, env, "clientID")
	if err != nil {
		return err
	}
	developerAPIKey, err := loadOne(ctx, kms, env, "developerAPIKey")
	if err != nil {
		return err
	}
	webhookAuthKey, err := loadOne(ctx, kms, env, "webhookAuthKey")
	if err != nil {
		return err
	}

	// webhookDecryptionKey is optional — only required when the
	// inbound encrypted-webhook pipeline is active. If absent, leave
	// cfg.WebhookDecryptionKey as-is (typically empty); the webhook
	// consumer path will refuse to operate without it and surfaces
	// ErrMissingConfig at that point. Any error other than missing
	// is fatal — a transport / decryption error against a key that
	// exists is a hard failure.
	webhookDecryptionKey, err := loadOne(ctx, kms, env, "webhookDecryptionKey")
	if err != nil && !errors.Is(err, ErrKMSKeyMissing) {
		return err
	}

	cfg.ClientID = clientID
	cfg.DeveloperAPIKey = developerAPIKey
	cfg.WebhookAuthKey = webhookAuthKey
	if webhookDecryptionKey != "" {
		cfg.WebhookDecryptionKey = webhookDecryptionKey
	}
	return nil
}
