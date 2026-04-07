package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeliver_Success(t *testing.T) {
	var deliveries atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Webhook-Signature") == "" {
			t.Error("missing X-Webhook-Signature")
		}
		if r.Header.Get("X-Event-Type") != "order.placed" {
			t.Errorf("expected X-Event-Type order.placed, got %s", r.Header.Get("X-Event-Type"))
		}
		if r.Header.Get("X-Event-ID") == "" {
			t.Error("missing X-Event-ID")
		}
		if r.Header.Get("X-Webhook-Timestamp") == "" {
			t.Error("missing X-Webhook-Timestamp")
		}

		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("empty body")
		}

		// Verify HMAC signature.
		ts := r.Header.Get("X-Webhook-Timestamp")
		sig := r.Header.Get("X-Webhook-Signature")
		if !VerifySignature(body, ts, sig, "test-secret") {
			t.Error("signature verification failed")
		}

		deliveries.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: "test-secret",
		Events: []string{"order.placed"},
		Active: true,
	})

	Deliver(store, "org1", "order.placed", map[string]string{"order_id": "123"})

	// Wait for async delivery.
	deadline := time.Now().Add(5 * time.Second)
	for deliveries.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if deliveries.Load() != 1 {
		t.Errorf("expected 1 delivery, got %d", deliveries.Load())
	}

	// Verify delivery was logged.
	dels, _ := store.ListDeliveries(store.webhooks["wh_1"].ID, 10)
	if len(dels) != 1 {
		t.Fatalf("expected 1 delivery log, got %d", len(dels))
	}
	if dels[0].Status != "delivered" {
		t.Errorf("expected status delivered, got %s", dels[0].Status)
	}
}

func TestDeliver_RetryOnFailure(t *testing.T) {
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

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: "s",
		Events: []string{"order.filled"},
		Active: true,
	})

	Deliver(store, "org1", "order.filled", map[string]string{"order_id": "456"})

	// Wait for at least 2 attempts (proves retry works).
	deadline := time.Now().Add(10 * time.Second)
	for attempts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}

	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts (retry), got %d", attempts.Load())
	}
}

func TestDeliver_NoMatchingWebhooks(t *testing.T) {
	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    "http://example.com",
		Secret: "s",
		Events: []string{"order.placed"},
		Active: true,
	})

	// Different event — should not deliver.
	Deliver(store, "org1", "transfer.completed", nil)

	time.Sleep(50 * time.Millisecond)
	dels, _ := store.ListDeliveries("wh_1", 10)
	if len(dels) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(dels))
	}
}

func TestDeliver_WildcardSubscription(t *testing.T) {
	var deliveries atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveries.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: "s",
		Events: []string{"*"},
		Active: true,
	})

	Deliver(store, "org1", "anything.here", map[string]string{"key": "val"})

	deadline := time.Now().Add(5 * time.Second)
	for deliveries.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if deliveries.Load() != 1 {
		t.Errorf("expected 1 delivery, got %d", deliveries.Load())
	}
}

func TestVerifySignature(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	ts := "1700000000"
	secret := "my-secret"

	signatureBody := ts + "." + string(payload)
	sig := "sha256=" + hmacSHA256(secret, signatureBody)

	if !VerifySignature(payload, ts, sig, secret) {
		t.Error("valid signature rejected")
	}
	if VerifySignature(payload, ts, "sha256=bad", secret) {
		t.Error("invalid signature accepted")
	}
	if VerifySignature(payload, ts, sig, "wrong-secret") {
		t.Error("wrong secret accepted")
	}
}

// --- Route tests ---

func TestRoutes_CreateAndList(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	// Create a webhook.
	body := `{"url":"https://example.com/hook","events":["order.placed","order.filled"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Org-Id", "org1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created["secret"] == nil || created["secret"] == "" {
		t.Error("create: secret not returned")
	}
	if created["id"] == nil || created["id"] == "" {
		t.Error("create: id not returned")
	}

	// List webhooks.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-Id", "org1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var hooks []Webhook
	json.Unmarshal(w.Body.Bytes(), &hooks)
	if len(hooks) != 1 {
		t.Fatalf("list: expected 1 webhook, got %d", len(hooks))
	}
	if hooks[0].URL != "https://example.com/hook" {
		t.Errorf("list: unexpected URL %s", hooks[0].URL)
	}
}

func TestRoutes_Delete(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	// Create.
	body := `{"url":"https://example.com/hook","events":["*"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Org-Id", "org1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/"+id, nil)
	req.Header.Set("X-Org-Id", "org1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List should be empty.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-Id", "org1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var hooks []Webhook
	json.Unmarshal(w.Body.Bytes(), &hooks)
	if len(hooks) != 0 {
		t.Errorf("list after delete: expected 0, got %d", len(hooks))
	}
}

func TestRoutes_OrgIsolation(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	// Create webhook for org1.
	body := `{"url":"https://example.com/hook","events":["*"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Org-Id", "org1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// List as org2 — should see nothing.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-Id", "org2")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var hooks []Webhook
	json.Unmarshal(w.Body.Bytes(), &hooks)
	if len(hooks) != 0 {
		t.Errorf("org2 should see 0 webhooks, got %d", len(hooks))
	}
}

func TestRoutes_Validation(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	// Missing URL.
	body := `{"events":["order.placed"]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Org-Id", "org1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing url: expected 400, got %d", w.Code)
	}

	// Missing events.
	body = `{"url":"https://example.com/hook"}`
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Org-Id", "org1")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("missing events: expected 400, got %d", w.Code)
	}

	// Missing org.
	body = `{"url":"https://example.com/hook","events":["*"]}`
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing org: expected 401, got %d", w.Code)
	}
}
