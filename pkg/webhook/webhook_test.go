package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/taskqueue"
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

// --- New tests ---

// bdEvents are the 16 event types fired by the broker-dealer service.
var bdEvents = []string{
	"order.placed",
	"order.filled",
	"order.canceled",
	"order.rejected",
	"order.partial_fill",
	"order.expired",
	"account.created",
	"account.approved",
	"account.rejected",
	"account.updated",
	"transfer.initiated",
	"transfer.completed",
	"transfer.failed",
	"aml.flagged",
	"aml.cleared",
	"compliance.alert",
}

func TestDeliverWebhook_AllBDEvents(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		et := r.Header.Get("X-Event-Type")
		mu.Lock()
		received[et] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: "sec",
		Events: []string{"*"},
		Active: true,
	})

	for _, ev := range bdEvents {
		Deliver(store, "org1", ev, map[string]string{"test": ev})
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= len(bdEvents) || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, ev := range bdEvents {
		if !received[ev] {
			t.Errorf("event %q was not delivered", ev)
		}
	}
}

func TestDeliverWebhook_SignatureVerification(t *testing.T) {
	secret := "verification-secret-key"
	var capturedBody []byte
	var capturedTS, capturedSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		capturedTS = r.Header.Get("X-Webhook-Timestamp")
		capturedSig = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: secret,
		Events: []string{"order.placed"},
		Active: true,
	})

	Deliver(store, "org1", "order.placed", map[string]string{"order_id": "sig-test"})

	deadline := time.Now().Add(5 * time.Second)
	for capturedSig == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if capturedSig == "" {
		t.Fatal("webhook was not delivered")
	}

	// The signature produced by Deliver must verify with VerifySignature.
	if !VerifySignature(capturedBody, capturedTS, capturedSig, secret) {
		t.Error("signature from Deliver does not pass VerifySignature")
	}

	// Wrong secret must fail.
	if VerifySignature(capturedBody, capturedTS, capturedSig, "wrong-secret") {
		t.Error("wrong secret should not verify")
	}

	// Tampered body must fail.
	if VerifySignature([]byte(`{"tampered":true}`), capturedTS, capturedSig, secret) {
		t.Error("tampered body should not verify")
	}
}

func TestDeliverWebhook_WildcardFiltering(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		et := r.Header.Get("X-Event-Type")
		mu.Lock()
		received[et] = true
		mu.Unlock()
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

	events := []string{"order.placed", "account.created", "aml.flagged", "custom.anything"}
	for _, ev := range events {
		Deliver(store, "org1", ev, map[string]string{"ev": ev})
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= len(events) || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, ev := range events {
		if !received[ev] {
			t.Errorf("wildcard subscriber did not receive %q", ev)
		}
	}
}

func TestDeliverWebhook_EventPrefixFiltering(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		et := r.Header.Get("X-Event-Type")
		mu.Lock()
		received[et] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: "s",
		Events: []string{"order.*"},
		Active: true,
	})

	// Should match.
	Deliver(store, "org1", "order.filled", map[string]string{"id": "1"})
	Deliver(store, "org1", "order.placed", map[string]string{"id": "2"})
	// Should NOT match.
	Deliver(store, "org1", "account.created", map[string]string{"id": "3"})
	Deliver(store, "org1", "aml.flagged", map[string]string{"id": "4"})

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Small grace for non-matching events to arrive (they should not).
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !received["order.filled"] {
		t.Error("order.filled should match order.*")
	}
	if !received["order.placed"] {
		t.Error("order.placed should match order.*")
	}
	if received["account.created"] {
		t.Error("account.created should NOT match order.*")
	}
	if received["aml.flagged"] {
		t.Error("aml.flagged should NOT match order.*")
	}
}

func TestWebhookStore_OrgIsolation(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string][]string) // orgID -> []eventType

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We embed orgID in the URL as a query param to distinguish.
		org := r.URL.Query().Get("org")
		et := r.Header.Get("X-Event-Type")
		mu.Lock()
		received[org] = append(received[org], et)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL + "?org=org1",
		Secret: "s1",
		Events: []string{"*"},
		Active: true,
	})
	store.Save(&Webhook{
		OrgID:  "org2",
		URL:    srv.URL + "?org=org2",
		Secret: "s2",
		Events: []string{"*"},
		Active: true,
	})

	Deliver(store, "org1", "order.placed", map[string]string{"id": "1"})
	Deliver(store, "org2", "account.created", map[string]string{"id": "2"})

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		total := len(received["org1"]) + len(received["org2"])
		mu.Unlock()
		if total >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received["org1"]) != 1 || received["org1"][0] != "order.placed" {
		t.Errorf("org1 should receive exactly [order.placed], got %v", received["org1"])
	}
	if len(received["org2"]) != 1 || received["org2"][0] != "account.created" {
		t.Errorf("org2 should receive exactly [account.created], got %v", received["org2"])
	}
}

func TestTaskqueueFallback(t *testing.T) {
	var delivered atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    srv.URL,
		Secret: "s",
		Events: []string{"order.placed"},
		Active: true,
	})

	// tq is nil — should fall back to direct delivery.
	DeliverWithQueue(store, nil, "org1", "order.placed", map[string]string{"id": "1"})

	deadline := time.Now().Add(5 * time.Second)
	for delivered.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if delivered.Load() != 1 {
		t.Errorf("expected 1 direct delivery when tq=nil, got %d", delivered.Load())
	}
}

func TestDeliverWebhook_PayloadSerialization(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
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

	payload := map[string]interface{}{
		"order_id":  "ord_123",
		"symbol":    "AAPL",
		"qty":       10.5,
		"side":      "buy",
		"filled_at": "2026-04-07T00:00:00Z",
	}
	Deliver(store, "org1", "order.filled", payload)

	deadline := time.Now().Add(5 * time.Second)
	for capturedBody == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if capturedBody == nil {
		t.Fatal("webhook was not delivered")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(capturedBody, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if decoded["order_id"] != "ord_123" {
		t.Errorf("order_id: expected ord_123, got %v", decoded["order_id"])
	}
	if decoded["symbol"] != "AAPL" {
		t.Errorf("symbol: expected AAPL, got %v", decoded["symbol"])
	}
	if decoded["qty"] != 10.5 {
		t.Errorf("qty: expected 10.5, got %v", decoded["qty"])
	}
	if decoded["side"] != "buy" {
		t.Errorf("side: expected buy, got %v", decoded["side"])
	}
}

func TestDeliverWithQueue_TaskqueueEnqueue(t *testing.T) {
	var mu sync.Mutex
	var enqueued []taskqueue.Task

	// Fake Hanzo Tasks server.
	tasksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tasks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var task taskqueue.Task
		json.Unmarshal(body, &task)
		mu.Lock()
		enqueued = append(enqueued, task)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer tasksSrv.Close()

	tq := taskqueue.NewWithURL(tasksSrv.URL)

	store := NewMemoryStore()
	store.Save(&Webhook{
		OrgID:  "org1",
		URL:    "https://example.com/hook",
		Secret: "sec",
		Events: []string{"order.placed"},
		Active: true,
	})

	DeliverWithQueue(store, tq, "org1", "order.placed", map[string]string{"id": "1"})

	// DeliverWithQueue enqueues synchronously (not in a goroutine) when tq is set.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(enqueued) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enqueued))
	}
	if enqueued[0].Type != "webhook.deliver" {
		t.Errorf("task type: expected webhook.deliver, got %q", enqueued[0].Type)
	}
}

// --- Additional route tests ---

func TestWebhookRoutes_CreateValidation(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	cases := []struct {
		name   string
		body   string
		orgID  string
		status int
	}{
		{"missing url", `{"events":["order.placed"]}`, "org1", http.StatusBadRequest},
		{"empty url", `{"url":"","events":["order.placed"]}`, "org1", http.StatusBadRequest},
		{"missing events", `{"url":"https://example.com/hook"}`, "org1", http.StatusBadRequest},
		{"empty events", `{"url":"https://example.com/hook","events":[]}`, "org1", http.StatusBadRequest},
		{"missing org", `{"url":"https://example.com/hook","events":["*"]}`, "", http.StatusUnauthorized},
		{"invalid json", `{bad json`, "org1", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if tc.orgID != "" {
				req.Header.Set("X-Org-Id", tc.orgID)
			}
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.status {
				t.Errorf("%s: expected %d, got %d: %s", tc.name, tc.status, w.Code, w.Body.String())
			}
		})
	}
}

func TestWebhookRoutes_ListPagination(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	// Create 5 webhooks.
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"url":"https://example.com/hook/%d","events":["order.placed"]}`, i)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("X-Org-Id", "org1")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d: expected 201, got %d", i, w.Code)
		}
	}

	// List all.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Org-Id", "org1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	var hooks []Webhook
	json.Unmarshal(w.Body.Bytes(), &hooks)
	if len(hooks) != 5 {
		t.Errorf("expected 5 webhooks, got %d", len(hooks))
	}

	// Verify each has a unique URL.
	urls := make(map[string]bool)
	for _, h := range hooks {
		urls[h.URL] = true
	}
	if len(urls) != 5 {
		t.Errorf("expected 5 unique URLs, got %d", len(urls))
	}
}

func TestWebhookRoutes_DeleteNonExistent(t *testing.T) {
	store := NewMemoryStore()
	r := NewRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/wh_nonexistent", nil)
	req.Header.Set("X-Org-Id", "org1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWebhookRoutes_DeliveryHistory(t *testing.T) {
	var delivered atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewMemoryStore()
	r := NewRouter(store)

	// Create webhook via route.
	body := fmt.Sprintf(`{"url":"%s","events":["order.placed"]}`, srv.URL)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Org-Id", "org1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	whID := created["id"].(string)

	// Fire 3 events.
	for i := 0; i < 3; i++ {
		Deliver(store, "org1", "order.placed", map[string]string{"i": fmt.Sprintf("%d", i)})
	}

	deadline := time.Now().Add(5 * time.Second)
	for delivered.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if delivered.Load() < 3 {
		t.Fatalf("expected 3 deliveries, got %d", delivered.Load())
	}

	// Query delivery history via route.
	req = httptest.NewRequest(http.MethodGet, "/"+whID+"/deliveries", nil)
	req.Header.Set("X-Org-Id", "org1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("deliveries: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var deliveries []Delivery
	json.Unmarshal(w.Body.Bytes(), &deliveries)
	if len(deliveries) != 3 {
		t.Errorf("expected 3 delivery records, got %d", len(deliveries))
	}
	for _, d := range deliveries {
		if d.Status != "delivered" {
			t.Errorf("delivery %s: expected status delivered, got %s", d.ID, d.Status)
		}
		if d.EventType != "order.placed" {
			t.Errorf("delivery %s: expected event order.placed, got %s", d.ID, d.EventType)
		}
	}
}

func TestMatchEvent(t *testing.T) {
	cases := []struct {
		pattern, event string
		want           bool
	}{
		{"*", "order.placed", true},
		{"*", "anything", true},
		{"order.placed", "order.placed", true},
		{"order.placed", "order.filled", false},
		{"order.*", "order.placed", true},
		{"order.*", "order.filled", true},
		{"order.*", "account.created", false},
		{"order.*", "order", false},         // "order" has no dot after prefix
		{"aml.*", "aml.flagged", true},
		{"aml.*", "aml.cleared", true},
		{"aml.*", "compliance.alert", false},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.pattern, tc.event), func(t *testing.T) {
			got := matchEvent(tc.pattern, tc.event)
			if got != tc.want {
				t.Errorf("matchEvent(%q, %q) = %v, want %v", tc.pattern, tc.event, got, tc.want)
			}
		})
	}
}
