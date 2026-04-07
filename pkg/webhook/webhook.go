// Package webhook provides HMAC-SHA256 signed webhook delivery with
// exponential backoff retry for the broker-dealer service.
//
// Consistent with the ATS (client_routes.go) and TA (webhook.go) patterns:
// same headers, same signature format, same retry semantics.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
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

// maxRetries is the number of delivery attempts before giving up.
const maxRetries = 3

// backoffBase is the base delay for exponential backoff: 1s, 4s, 16s.
const backoffBase = 1 * time.Second

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Deliver sends a webhook event to all registered subscribers for the given
// org and event type. Each subscriber is delivered independently; failures
// on one do not block others. Delivery is logged via the store.
//
// Headers match the ATS/TA convention:
//   - X-Webhook-Signature: sha256=<hex>
//   - X-Event-Type: <event>
//   - X-Event-ID: <id>
//   - X-Webhook-Timestamp: <unix>
func Deliver(store Store, orgID, eventType string, payload any) {
	if store == nil {
		return
	}

	hooks, err := store.ListByEvent(orgID, eventType)
	if err != nil || len(hooks) == 0 {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Str("event", eventType).Msg("webhook: marshal payload")
		return
	}

	eventID := generateEventID()

	for _, wh := range hooks {
		go deliver(store, wh, eventType, eventID, body)
	}
}

// deliver sends a single webhook with retries.
func deliver(store Store, wh Webhook, eventType, eventID string, body []byte) {
	delivery := &Delivery{
		WebhookID: wh.ID,
		EventType: eventType,
		EventID:   eventID,
		Status:    "pending",
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(4, float64(attempt))) * backoffBase
			time.Sleep(backoff)
		}

		delivery.Attempts = attempt + 1
		code, err := send(wh.URL, wh.Secret, eventType, eventID, body)
		delivery.ResponseCode = code

		if err == nil && code >= 200 && code < 300 {
			delivery.Status = "delivered"
			if logErr := store.LogDelivery(delivery); logErr != nil {
				log.Error().Err(logErr).Str("webhook", wh.ID).Msg("webhook: log delivery")
			}
			log.Info().
				Str("webhook", wh.ID).
				Str("event", eventType).
				Str("event_id", eventID).
				Int("attempts", attempt+1).
				Msg("webhook: delivered")
			return
		}

		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("HTTP %d", code)
		}
	}

	delivery.Status = "failed"
	delivery.Error = lastErr.Error()
	if logErr := store.LogDelivery(delivery); logErr != nil {
		log.Error().Err(logErr).Str("webhook", wh.ID).Msg("webhook: log delivery")
	}
	log.Error().
		Err(lastErr).
		Str("webhook", wh.ID).
		Str("event", eventType).
		Str("event_id", eventID).
		Int("attempts", maxRetries).
		Msg("webhook: delivery failed")
}

// send performs a single HTTP POST with HMAC-SHA256 signature.
func send(url, secret, eventType, eventID string, body []byte) (int, error) {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signatureBody := timestamp + "." + string(body)
	signature := hmacSHA256(secret, signatureBody)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	req.Header.Set("X-Event-Type", eventType)
	req.Header.Set("X-Event-ID", eventID)
	req.Header.Set("X-Webhook-Timestamp", timestamp)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

// hmacSHA256 computes HMAC-SHA256 of msg using the given key.
func hmacSHA256(key, msg string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks that a received signature matches the expected
// HMAC-SHA256. Receivers of BD webhooks use this for validation.
func VerifySignature(payload []byte, timestamp, signature, secret string) bool {
	signatureBody := timestamp + "." + string(payload)
	expected := "sha256=" + hmacSHA256(secret, signatureBody)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func generateEventID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
