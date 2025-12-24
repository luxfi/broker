package jube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- PreTradeScreen Unit Tests ---

func newTestScreen(t *testing.T, handler http.HandlerFunc, cfg PreTradeConfig) (*PreTradeScreen, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	screen := NewPreTradeScreen(c, cfg)
	return screen, func() {
		c.Close()
		srv.Close()
	}
}

func TestScreenAllowCleanTransaction(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.1,
			Action: ActionAllow,
		})
	}, PreTradeConfig{})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-clean",
		Symbol:    "AAPL",
		Side:      "buy",
		Qty:       "10",
		Price:     "150.00",
		Currency:  "USD",
	})

	if !result.Allowed {
		t.Fatalf("expected allowed=true, got false; errors: %v", result.Errors)
	}
	if result.Action != PreTradeAllow {
		t.Fatalf("action = %q, want %q", result.Action, PreTradeAllow)
	}
	if result.Score != 0.1 {
		t.Fatalf("score = %f, want 0.1", result.Score)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestScreenBlockHighRisk(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.95,
			Action: ActionBlock,
			Alerts: []Alert{
				{ID: "a1", RuleName: "structuring", Severity: "critical", Score: 0.95},
			},
		})
	}, PreTradeConfig{})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-sus",
		Symbol:    "BTC-USD",
		Side:      "buy",
		Qty:       "1",
		Price:     "9500",
		Currency:  "USD",
	})

	if result.Allowed {
		t.Fatal("expected allowed=false for blocked transaction")
	}
	if result.Action != PreTradeBlock {
		t.Fatalf("action = %q, want %q", result.Action, PreTradeBlock)
	}
	if result.Score != 0.95 {
		t.Fatalf("score = %f, want 0.95", result.Score)
	}
	if len(result.Errors) < 2 {
		t.Fatalf("expected at least 2 errors (block msg + alert), got %d: %v", len(result.Errors), result.Errors)
	}
	if len(result.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(result.Alerts))
	}
}

func TestScreenReviewAllowedByDefault(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.65,
			Action: ActionReview,
			Alerts: []Alert{
				{ID: "a1", RuleName: "velocity", Severity: "medium", Score: 0.65},
			},
		})
	}, PreTradeConfig{AllowOnReview: true})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-review",
		Symbol:    "MSFT",
		Side:      "buy",
		Qty:       "100",
		Price:     "300",
		Currency:  "USD",
	})

	if !result.Allowed {
		t.Fatal("expected allowed=true when AllowOnReview=true")
	}
	if result.Action != PreTradeReview {
		t.Fatalf("action = %q, want %q", result.Action, PreTradeReview)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for review action")
	}
}

func TestScreenReviewBlockedWhenConfigured(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.70,
			Action: ActionReview,
			Alerts: []Alert{
				{ID: "a1", RuleName: "suspicious-pattern", Severity: "high", Score: 0.70},
			},
		})
	}, PreTradeConfig{AllowOnReview: false})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-strict",
		Symbol:    "ETH-USD",
		Side:      "sell",
		Qty:       "50",
		Price:     "2000",
		Currency:  "USD",
	})

	if result.Allowed {
		t.Fatal("expected allowed=false when AllowOnReview=false")
	}
	if result.Action != PreTradeReview {
		t.Fatalf("action = %q, want %q", result.Action, PreTradeReview)
	}
}

func TestScreenFailOpenOnError(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}, PreTradeConfig{AllowOnError: true})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-err",
		Symbol:    "GOOG",
		Side:      "buy",
		Qty:       "5",
		Price:     "100",
		Currency:  "USD",
	})

	if !result.Allowed {
		t.Fatal("expected allowed=true (fail-open) when Jube returns error")
	}
	if result.Action != PreTradeAllow {
		t.Fatalf("action = %q, want %q", result.Action, PreTradeAllow)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning about Jube unavailability")
	}
}

func TestScreenFailClosedOnError(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	}, PreTradeConfig{AllowOnError: false})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-err",
		Symbol:    "GOOG",
		Side:      "buy",
		Qty:       "5",
		Price:     "100",
		Currency:  "USD",
	})

	if result.Allowed {
		t.Fatal("expected allowed=false (fail-closed) when Jube returns error")
	}
	if result.Action != PreTradeBlock {
		t.Fatalf("action = %q, want %q", result.Action, PreTradeBlock)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error about compliance unavailability")
	}
}

func TestScreenSendsCorrectPayload(t *testing.T) {
	var received TransactionRequest
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		json.NewEncoder(w).Encode(TransactionResponse{Action: ActionAllow, Score: 0.05})
	}, PreTradeConfig{ModelID: 42})
	defer cleanup()

	screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-payload",
		OrderID:   "order-789",
		Provider:  "alpaca",
		Symbol:    "AAPL",
		Side:      "buy",
		Qty:       "10",
		Price:     "150",
		Currency:  "USD",
		IP:        "10.0.0.1",
	})

	if received.EntityAnalysisModelID != 42 {
		t.Fatalf("modelId = %d, want 42", received.EntityAnalysisModelID)
	}
	payload := received.EntityInstanceEntryPayload
	if payload["AccountId"] != "acct-payload" {
		t.Fatalf("AccountId = %v, want acct-payload", payload["AccountId"])
	}
	if payload["TransactionId"] != "order-789" {
		t.Fatalf("TransactionId = %v, want order-789", payload["TransactionId"])
	}
	if payload["Symbol"] != "AAPL" {
		t.Fatalf("Symbol = %v, want AAPL", payload["Symbol"])
	}
	// Amount should be 10 * 150 = 1500
	if amt, ok := payload["Amount"].(float64); !ok || amt != 1500 {
		t.Fatalf("Amount = %v, want 1500", payload["Amount"])
	}
}

func TestScreenWebhookFiredOnBlock(t *testing.T) {
	var webhookReceived bool
	var webhookBody WebhookEvent
	var mu sync.Mutex

	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		webhookReceived = true
		json.NewDecoder(r.Body).Decode(&webhookBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.90,
			Action: ActionBlock,
			Alerts: []Alert{{ID: "a1", RuleName: "high-risk", Severity: "critical"}},
		})
	}, PreTradeConfig{
		WebhookURL:        webhookSrv.URL,
		WebhookHMACSecret: "test-webhook-secret",
	})
	defer cleanup()

	screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-webhook",
		OrderID:   "order-wh",
		Symbol:    "BTC-USD",
		Side:      "buy",
		Qty:       "1",
		Price:     "50000",
		Currency:  "USD",
	})

	// Wait for async webhook delivery.
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !webhookReceived {
		t.Fatal("expected webhook to be fired on block")
	}
	if webhookBody.Event != EventAMLFlagged {
		t.Fatalf("webhook event = %q, want %q", webhookBody.Event, EventAMLFlagged)
	}
}

func TestScreenDefaultModelID(t *testing.T) {
	screen := NewPreTradeScreen(&Client{}, PreTradeConfig{})
	if screen.cfg.ModelID != 1 {
		t.Fatalf("default ModelID = %d, want 1", screen.cfg.ModelID)
	}
}

func TestScreenCustomModelID(t *testing.T) {
	screen := NewPreTradeScreen(&Client{}, PreTradeConfig{ModelID: 99})
	if screen.cfg.ModelID != 99 {
		t.Fatalf("ModelID = %d, want 99", screen.cfg.ModelID)
	}
}

func TestScreenRequestAmount(t *testing.T) {
	tests := []struct {
		qty, price string
		want       float64
	}{
		{"10", "150", 1500},
		{"0.5", "100", 50},
		{"100", "", 100},  // no price = market order, p defaults to 1
		{"", "100", 0},    // no qty = 0
		{"abc", "100", 0}, // invalid qty
	}

	for _, tt := range tests {
		r := ScreenRequest{Qty: tt.qty, Price: tt.price}
		got := r.Amount()
		if got != tt.want {
			t.Errorf("Amount(%q, %q) = %f, want %f", tt.qty, tt.price, got, tt.want)
		}
	}
}

func TestScreenMultipleAlerts(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.88,
			Action: ActionBlock,
			Alerts: []Alert{
				{ID: "a1", RuleName: "structuring", Severity: "critical", Score: 0.88},
				{ID: "a2", RuleName: "velocity", Severity: "high", Score: 0.75},
				{ID: "a3", RuleName: "geography", Severity: "medium", Score: 0.60},
			},
		})
	}, PreTradeConfig{})
	defer cleanup()

	result := screen.Screen(context.Background(), ScreenRequest{
		AccountID: "acct-multi",
		Symbol:    "BTC-USD",
		Side:      "buy",
		Qty:       "1",
		Price:     "9500",
		Currency:  "USD",
	})

	if result.Allowed {
		t.Fatal("expected blocked")
	}
	if len(result.Alerts) != 3 {
		t.Fatalf("expected 3 alerts, got %d", len(result.Alerts))
	}
	// 1 block message + 3 alert details = 4 errors
	if len(result.Errors) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

func TestScreenContextCancelled(t *testing.T) {
	screen, cleanup := newTestScreen(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}, PreTradeConfig{AllowOnError: false})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := screen.Screen(ctx, ScreenRequest{
		AccountID: "acct-timeout",
		Symbol:    "AAPL",
		Side:      "buy",
		Qty:       "1",
		Price:     "100",
		Currency:  "USD",
	})

	if result.Allowed {
		t.Fatal("expected blocked on timeout with fail-closed")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error message")
	}
}
