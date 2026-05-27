// Admin operations — webhook key rotation, key-management metadata.
//
// Endpoints:
//   POST /tapiv3/index.php/v1/updateWebhookAuthKey
//
// Rotation discipline (brief §7): the new key is written into KMS
// *before* this call lands. The adapter accepts both old + new for a
// 24-hour overlap window after rotation so in-flight webhooks signed
// with the old key continue to verify.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference

package northcapital

import (
	"context"
	"errors"
	"net/url"
)

// UpdateWebhookAuthKey rotates the TransactAPI HMAC verification key
// on the NCPS side. The new key must already be persisted in luxfi/kms
// under `shared/northcapital/<env>/webhookAuthKey` (next version) so
// inbound webhooks signed with the new key can be verified immediately
// on issuance. The adapter retains the prior key for the documented
// 24-hour overlap window — the WebhookAuthKey config field accepts a
// comma-separated list during the overlap.
func (p *Provider) UpdateWebhookAuthKey(ctx context.Context, req *UpdateWebhookAuthKeyRequest) error {
	if req == nil || req.NewAuthKey == "" {
		return errors.New("northcapital: NewAuthKey required")
	}
	form := url.Values{}
	form.Set("newAuthKey", req.NewAuthKey)
	const path = "/tapiv3/index.php/v1/updateWebhookAuthKey"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return err
	}
	if _, err := decodeEnvelope(path, raw); err != nil {
		return err
	}
	return nil
}
