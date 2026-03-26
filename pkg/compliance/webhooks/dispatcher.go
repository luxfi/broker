package webhooks

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

	"github.com/rs/zerolog/log"
)

// Event type constants for the webhook system.
const (
	EventTradeExecuted = "trade.executed"
)

// maxRetries is the maximum number of delivery attempts per recipient.
const maxRetries = 5

// initialBackoff is the base delay for exponential backoff.
const initialBackoff = 500 * time.Millisecond

// WebhookTarget is a registered webhook endpoint for a broker-dealer or transfer agent.
type WebhookTarget struct {
	RecipientID string // unique ID of the recipient
	Name        string // firm name
	Role        string // buyer_broker_dealer, seller_broker_dealer, transfer_agent
	URL         string // POST endpoint
	HMACSecret  string // HMAC-SHA256 signing secret
}

// Dispatcher sends trade.executed webhooks to cross-listed broker-dealers and transfer agents.
type Dispatcher struct {
	httpClient *http.Client
}

// NewDispatcher creates a webhook dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// FireTradeExecuted sends a trade.executed webhook to all provided targets.
// Each target is delivered independently with retries. Errors are logged but
// do not prevent delivery to other targets.
func (d *Dispatcher) FireTradeExecuted(ctx context.Context, payload *TradeWebhookPayload, targets []WebhookTarget) error {
	payload.WebhookEvent.EventType = EventTradeExecuted
	payload.WebhookEvent.Endpoint = "POST /v1/webhooks/trade"
	payload.WebhookEvent.Version = "2.0.0"
	if payload.WebhookEvent.Timestamp.IsZero() {
		payload.WebhookEvent.Timestamp = time.Now().UTC()
	}

	// Build recipients list from targets.
	payload.WebhookEvent.Recipients = make([]Recipient, len(targets))
	for i, t := range targets {
		payload.WebhookEvent.Recipients[i] = Recipient{
			RecipientID: t.RecipientID,
			Name:        t.Name,
			Role:        t.Role,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhooks: marshal trade.executed payload: %w", err)
	}

	var firstErr error
	for i, target := range targets {
		sig := signPayload(body, target.HMACSecret)

		deliveredAt, deliveryErr := d.deliverWithRetry(ctx, target.URL, body, sig)
		if deliveryErr != nil {
			log.Error().
				Err(deliveryErr).
				Str("recipient", target.RecipientID).
				Str("role", target.Role).
				Msg("webhooks: trade.executed delivery failed")
			payload.WebhookEvent.Recipients[i].DeliveryStatus = "failed"
			if firstErr == nil {
				firstErr = deliveryErr
			}
		} else {
			payload.WebhookEvent.Recipients[i].DeliveredAt = deliveredAt.Format(time.RFC3339Nano)
			payload.WebhookEvent.Recipients[i].DeliveryStatus = "delivered"
		}
	}

	return firstErr
}

// deliverWithRetry attempts delivery with exponential backoff.
func (d *Dispatcher) deliverWithRetry(ctx context.Context, url string, body []byte, signature string) (time.Time, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(float64(initialBackoff) * math.Pow(2, float64(attempt-1)))
			select {
			case <-ctx.Done():
				return time.Time{}, fmt.Errorf("context cancelled after %d attempts: %w", attempt, ctx.Err())
			case <-time.After(backoff):
			}
		}

		lastErr = d.send(ctx, url, body, signature)
		if lastErr == nil {
			return time.Now().UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("delivery failed after %d attempts: %w", maxRetries, lastErr)
}

// send performs a single webhook POST with HMAC signature.
func (d *Dispatcher) send(ctx context.Context, url string, body []byte, signature string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)

	resp, err := d.httpClient.Do(req)
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
