package jube

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// Webhook event type constants.
const (
	EventAMLFlagged    = "aml.flagged"
	EventAMLCleared    = "aml.cleared"
	EventKYCApproved   = "kyc.approved"
	EventTradeExecuted = "trade.executed"
)

// maxWebhookRetries is the maximum number of delivery attempts.
const maxWebhookRetries = 5

// webhookInitialBackoff is the base delay for exponential backoff.
const webhookInitialBackoff = 500 * time.Millisecond

// WebhookEvent is the envelope sent to webhook targets.
type WebhookEvent struct {
	Event     string      `json:"event"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// AMLFlaggedData is the payload for aml.flagged events.
type AMLFlaggedData struct {
	AccountID     string  `json:"accountId"`
	TransactionID string  `json:"transactionId"`
	RiskScore     float64 `json:"riskScore"`
	Alerts        []Alert `json:"alerts"`
	SanctionsHit  bool    `json:"sanctionsHit"`
	PEPMatch      bool    `json:"pepMatch"`
	Action        string  `json:"action"`
}

// AMLClearedData is the payload for aml.cleared events.
type AMLClearedData struct {
	AccountID     string `json:"accountId"`
	TransactionID string `json:"transactionId"`
	CaseID        string `json:"caseId"`
	ClearedBy     string `json:"clearedBy"`
}

// KYCApprovedData is the payload for kyc.approved events.
type KYCApprovedData struct {
	AccountID  string `json:"accountId"`
	UserID     string `json:"userId"`
	Provider   string `json:"provider"`
	VerifiedAt string `json:"verifiedAt"`
}

// FireWebhook sends a signed webhook event to the target URL.
// The payload is signed with HMAC-SHA256 using hmacSecret, and the signature
// is sent in the X-Webhook-Signature header. Delivery is retried up to
// maxWebhookRetries times with exponential backoff on failure.
func FireWebhook(ctx context.Context, event WebhookEvent, targetURL string, hmacSecret string) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("jube webhook: marshal event: %w", err)
	}

	sig := signPayload(payload, hmacSecret)

	var lastErr error
	for attempt := 0; attempt < maxWebhookRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(float64(webhookInitialBackoff) * math.Pow(2, float64(attempt-1)))
			select {
			case <-ctx.Done():
				return fmt.Errorf("jube webhook: context cancelled after %d attempts: %w", attempt, ctx.Err())
			case <-time.After(backoff):
			}
		}

		lastErr = sendWebhook(ctx, targetURL, payload, sig)
		if lastErr == nil {
			return nil
		}
	}

	return fmt.Errorf("jube webhook: delivery failed after %d attempts: %w", maxWebhookRetries, lastErr)
}

// sendWebhook performs a single webhook delivery attempt.
func sendWebhook(ctx context.Context, targetURL string, payload []byte, signature string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("target returned status %d", resp.StatusCode)
	}
	return nil
}

// signPayload computes HMAC-SHA256 of payload using the given secret.
func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks that the given signature matches the expected
// HMAC-SHA256 of payload. Use this on the receiving end of webhooks.
func VerifySignature(payload []byte, signature string, secret string) bool {
	expected := signPayload(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}
