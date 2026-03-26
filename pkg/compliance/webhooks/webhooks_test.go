package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestTradeWebhookPayloadUnmarshal(t *testing.T) {
	schemaDir := filepath.Join("schemas")
	files := []string{
		"trade__corporation.json",
		"trade__llc.json",
		"trade__trust.json",
		"trade__partnership.json",
		"trade__sole_proprietorship.json",
		"trade__public_company.json",
		"trade__spv.json",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(schemaDir, f))
			if err != nil {
				t.Fatalf("read schema file: %v", err)
			}

			var payload TradeWebhookPayload
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshal %s: %v", f, err)
			}

			// Verify key fields are populated.
			if payload.WebhookEvent.EventType != "trade.executed" {
				t.Errorf("expected event_type trade.executed, got %q", payload.WebhookEvent.EventType)
			}
			if payload.WebhookEvent.EventID == "" {
				t.Error("event_id is empty")
			}
			if len(payload.WebhookEvent.Recipients) != 3 {
				t.Errorf("expected 3 recipients, got %d", len(payload.WebhookEvent.Recipients))
			}
			if payload.Transaction.TransactionID == "" {
				t.Error("transaction_id is empty")
			}
			if payload.Transaction.Security.AssetID == "" {
				t.Error("asset_id is empty")
			}
			if payload.Transaction.Security.GrossTradeAmount <= 0 {
				t.Error("gross_trade_amount should be positive")
			}
			if payload.Buyer.InvestorID == "" {
				t.Error("buyer investor_id is empty")
			}
			if payload.Seller.InvestorID == "" {
				t.Error("seller investor_id is empty")
			}
			if payload.TransferAgent.FirmName == "" {
				t.Error("transfer_agent firm_name is empty")
			}
		})
	}
}

func TestTradeWebhookPayloadRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	payload := TradeWebhookPayload{
		WebhookEvent: WebhookEvent{
			EventID:         "evt_test_001",
			EventType:       EventTradeExecuted,
			Endpoint:        "POST /v1/webhooks/trade",
			Timestamp:       now,
			Version:         "2.0.0",
			TransactionType: "trade",
		},
		Transaction: Transaction{
			TransactionID:   "txn_test_001",
			TransactionType: "secondary_market_transfer",
			Status:          "pending_compliance_clearance",
			InitiatedAt:     now,
			Security: Security{
				AssetID:          "asset_test_001",
				NumberOfShares:   100,
				PricePerShare:    50.0,
				GrossTradeAmount: 5000.0,
				Currency:         "USD",
			},
		},
		Buyer: Party{
			InvestorID: "inv_buyer_001",
			AccountID:  "acct_buyer_001",
		},
		Seller: Party{
			InvestorID: "inv_seller_001",
			AccountID:  "acct_seller_001",
		},
		TransferAgent: TransferAgent{
			FirmName:      "Test TA",
			SECRegistered: true,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded TradeWebhookPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.WebhookEvent.EventID != "evt_test_001" {
		t.Errorf("roundtrip event_id mismatch: %q", decoded.WebhookEvent.EventID)
	}
	if decoded.Transaction.Security.GrossTradeAmount != 5000.0 {
		t.Errorf("roundtrip gross_trade_amount mismatch: %v", decoded.Transaction.Security.GrossTradeAmount)
	}
}

func TestDispatcherFireTradeExecuted(t *testing.T) {
	var deliveries atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		sig := r.Header.Get("X-Webhook-Signature")
		if sig == "" {
			t.Error("missing X-Webhook-Signature header")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if len(body) == 0 {
			t.Error("empty body")
		}

		// Verify signature.
		if !VerifySignature(body, sig, "test-secret") {
			t.Error("signature verification failed")
		}

		deliveries.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher()
	payload := &TradeWebhookPayload{
		WebhookEvent: WebhookEvent{
			EventID:         "evt_test_dispatch",
			TransactionType: "trade",
		},
		Transaction: Transaction{
			TransactionID: "txn_dispatch_001",
			Security: Security{
				AssetID:          "asset_dispatch_001",
				GrossTradeAmount: 10000.0,
			},
		},
		Buyer: Party{
			InvestorID: "inv_buyer_dispatch",
		},
		Seller: Party{
			InvestorID: "inv_seller_dispatch",
		},
		TransferAgent: TransferAgent{
			FirmName: "Test TA",
		},
	}

	targets := []WebhookTarget{
		{RecipientID: "rec_001", Name: "BD Alpha", Role: "buyer_broker_dealer", URL: srv.URL, HMACSecret: "test-secret"},
		{RecipientID: "rec_002", Name: "BD Beta", Role: "seller_broker_dealer", URL: srv.URL, HMACSecret: "test-secret"},
		{RecipientID: "rec_003", Name: "TA Gamma", Role: "transfer_agent", URL: srv.URL, HMACSecret: "test-secret"},
	}

	ctx := context.Background()
	err := d.FireTradeExecuted(ctx, payload, targets)
	if err != nil {
		t.Fatalf("FireTradeExecuted: %v", err)
	}

	if deliveries.Load() != 3 {
		t.Errorf("expected 3 deliveries, got %d", deliveries.Load())
	}

	// Verify the payload was enriched.
	if payload.WebhookEvent.EventType != EventTradeExecuted {
		t.Errorf("event_type not set: %q", payload.WebhookEvent.EventType)
	}
	for _, r := range payload.WebhookEvent.Recipients {
		if r.DeliveryStatus != "delivered" {
			t.Errorf("recipient %s status: %q", r.RecipientID, r.DeliveryStatus)
		}
	}
}

func TestDispatcherRetryOnFailure(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDispatcher()
	payload := &TradeWebhookPayload{
		WebhookEvent: WebhookEvent{EventID: "evt_retry"},
		Transaction:  Transaction{TransactionID: "txn_retry"},
		Buyer:        Party{InvestorID: "inv_retry_buyer"},
		Seller:       Party{InvestorID: "inv_retry_seller"},
		TransferAgent: TransferAgent{FirmName: "Retry TA"},
	}

	targets := []WebhookTarget{
		{RecipientID: "rec_retry", Name: "Retry BD", Role: "buyer_broker_dealer", URL: srv.URL, HMACSecret: "s"},
	}

	err := d.FireTradeExecuted(context.Background(), payload, targets)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}

	if attempts.Load() < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts.Load())
	}
}

func TestVerifySignature(t *testing.T) {
	payload := []byte(`{"event":"trade.executed","data":{}}`)
	secret := "my-secret-key"

	sig := signPayload(payload, secret)

	if !VerifySignature(payload, sig, secret) {
		t.Error("valid signature rejected")
	}
	if VerifySignature(payload, "bad-sig", secret) {
		t.Error("invalid signature accepted")
	}
	if VerifySignature(payload, sig, "wrong-secret") {
		t.Error("wrong secret accepted")
	}
}
