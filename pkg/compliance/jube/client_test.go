package jube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- Client Tests ---

func TestNewClientDefaults(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	if c.baseURL != DefaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Fatalf("timeout = %v, want %v", c.httpClient.Timeout, DefaultTimeout)
	}
	if c.natsConn != nil {
		t.Fatal("natsConn should be nil when NATSAddr not set")
	}
}

func TestNewClientCustomBaseURL(t *testing.T) {
	c, err := New(Config{BaseURL: "http://localhost:9999", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	if c.baseURL != "http://localhost:9999" {
		t.Fatalf("baseURL = %q, want http://localhost:9999", c.baseURL)
	}
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestNewClientBadNATS(t *testing.T) {
	_, err := New(Config{NATSAddr: "nats://127.0.0.1:19999"})
	if err == nil {
		t.Fatal("expected error connecting to bad NATS address")
	}
	if !strings.Contains(err.Error(), "nats connect") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScreenTransaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/EntityAnalysisModel/Invoke" {
			t.Errorf("path = %s, want /api/EntityAnalysisModel/Invoke", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}

		var req TransactionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.EntityAnalysisModelID != 1 {
			t.Errorf("modelId = %d, want 1", req.EntityAnalysisModelID)
		}

		json.NewEncoder(w).Encode(TransactionResponse{
			Score:  0.85,
			Action: ActionBlock,
			Alerts: []Alert{{ID: "a1", RuleName: "high-value", Severity: "high"}},
		})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	resp, err := c.ScreenTransaction(context.Background(), TransactionRequest{
		EntityAnalysisModelID: 1,
		EntityInstanceEntryPayload: map[string]interface{}{
			"AccountId": "acct-123",
			"Amount":    50000,
			"Currency":  "USD",
		},
	})
	if err != nil {
		t.Fatalf("ScreenTransaction() error: %v", err)
	}
	if resp.Score != 0.85 {
		t.Fatalf("score = %f, want 0.85", resp.Score)
	}
	if resp.Action != ActionBlock {
		t.Fatalf("action = %q, want %q", resp.Action, ActionBlock)
	}
	if len(resp.Alerts) != 1 {
		t.Fatalf("alerts len = %d, want 1", len(resp.Alerts))
	}
	if resp.Alerts[0].RuleName != "high-value" {
		t.Fatalf("alert ruleName = %q, want high-value", resp.Alerts[0].RuleName)
	}
}

func TestCheckSanctions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/Sanction") {
			t.Errorf("path = %s, want /api/Sanction", r.URL.Path)
		}
		name := r.URL.Query().Get("name")
		if name != "John Doe" {
			t.Errorf("name param = %q, want 'John Doe'", name)
		}
		country := r.URL.Query().Get("country")
		if country != "US" {
			t.Errorf("country param = %q, want 'US'", country)
		}

		json.NewEncoder(w).Encode(SanctionResult{
			Hit: true,
			Matches: []SanctionMatch{
				{ListName: "OFAC SDN", EntityName: "John Doe", Score: 0.95, Country: "US"},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	result, err := c.CheckSanctions(context.Background(), "John Doe", "US")
	if err != nil {
		t.Fatalf("CheckSanctions() error: %v", err)
	}
	if !result.Hit {
		t.Fatal("expected sanctions hit")
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(result.Matches))
	}
	if result.Matches[0].ListName != "OFAC SDN" {
		t.Fatalf("listName = %q, want 'OFAC SDN'", result.Matches[0].ListName)
	}
}

func TestCheckSanctionsNoCountry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("country") != "" {
			t.Error("expected no country param when empty string passed")
		}
		json.NewEncoder(w).Encode(SanctionResult{Hit: false})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	result, err := c.CheckSanctions(context.Background(), "Clean Person", "")
	if err != nil {
		t.Fatalf("CheckSanctions() error: %v", err)
	}
	if result.Hit {
		t.Fatal("expected no sanctions hit")
	}
}

func TestCreateCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/CaseManagement" {
			t.Errorf("path = %s, want /api/CaseManagement", r.URL.Path)
		}

		var req CaseRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Case{
			ID:          "case-001",
			AccountID:   req.AccountID,
			Type:        req.Type,
			Severity:    req.Severity,
			Status:      "open",
			Description: req.Description,
			CreatedAt:   time.Now(),
		})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	cas, err := c.CreateCase(context.Background(), CaseRequest{
		AccountID:   "acct-123",
		Type:        "aml",
		Severity:    "high",
		Description: "Suspicious large wire transfer",
	})
	if err != nil {
		t.Fatalf("CreateCase() error: %v", err)
	}
	if cas.ID != "case-001" {
		t.Fatalf("case ID = %q, want case-001", cas.ID)
	}
	if cas.Status != "open" {
		t.Fatalf("case status = %q, want open", cas.Status)
	}
}

func TestGetCases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Query().Get("accountId") != "acct-123" {
			t.Errorf("accountId = %q, want acct-123", r.URL.Query().Get("accountId"))
		}
		if r.URL.Query().Get("type") != "aml" {
			t.Errorf("type = %q, want aml", r.URL.Query().Get("type"))
		}

		json.NewEncoder(w).Encode([]Case{
			{ID: "case-001", AccountID: "acct-123", Type: "aml", Status: "open"},
			{ID: "case-002", AccountID: "acct-123", Type: "aml", Status: "investigating"},
		})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	cases, err := c.GetCases(context.Background(), CaseFilter{
		AccountID: "acct-123",
		Type:      "aml",
	})
	if err != nil {
		t.Fatalf("GetCases() error: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases len = %d, want 2", len(cases))
	}
}

func TestGetCasesNoFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query params, got %q", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode([]Case{})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	cases, err := c.GetCases(context.Background(), CaseFilter{})
	if err != nil {
		t.Fatalf("GetCases() error: %v", err)
	}
	if len(cases) != 0 {
		t.Fatalf("cases len = %d, want 0", len(cases))
	}
}

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/ExhaustiveSearchInstance" {
			t.Errorf("path = %s, want /api/ExhaustiveSearchInstance", r.URL.Path)
		}

		json.NewEncoder(w).Encode([]SearchResult{
			{EntityID: "e1", EntityType: "account", Score: 0.9},
		})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	results, err := c.Search(context.Background(), SearchRequest{
		Query: "suspicious activity",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].EntityID != "e1" {
		t.Fatalf("entityId = %q, want e1", results[0].EntityID)
	}
}

func TestScreenTransactionHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	_, err = c.ScreenTransaction(context.Background(), TransactionRequest{EntityAnalysisModelID: 999})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("error should mention status 500, got: %v", err)
	}
}

func TestScreenTransactionContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = c.ScreenTransaction(ctx, TransactionRequest{EntityAnalysisModelID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSubscribeAlertsNoNATS(t *testing.T) {
	c, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()

	err = c.SubscribeAlerts(func(a Alert) {})
	if err == nil {
		t.Fatal("expected error when NATS not configured")
	}
	if !strings.Contains(err.Error(), "nats not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Webhook Tests ---

func TestFireWebhookSuccess(t *testing.T) {
	var receivedSig string
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Webhook-Signature")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	event := WebhookEvent{
		Event:     EventAMLFlagged,
		Timestamp: time.Now(),
		Data: AMLFlaggedData{
			AccountID:     "acct-123",
			TransactionID: "tx-456",
			RiskScore:     0.92,
			SanctionsHit:  true,
			Action:        ActionBlock,
		},
	}

	err := FireWebhook(context.Background(), event, srv.URL, "test-secret")
	if err != nil {
		t.Fatalf("FireWebhook() error: %v", err)
	}

	if receivedSig == "" {
		t.Fatal("expected X-Webhook-Signature header")
	}

	if !VerifySignature(receivedBody, receivedSig, "test-secret") {
		t.Fatal("signature verification failed")
	}
}

func TestFireWebhookRetries(t *testing.T) {
	var attempts int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	event := WebhookEvent{
		Event:     EventAMLCleared,
		Timestamp: time.Now(),
		Data:      AMLClearedData{AccountID: "acct-123", CaseID: "case-001"},
	}

	err := FireWebhook(context.Background(), event, srv.URL, "secret")
	if err != nil {
		t.Fatalf("FireWebhook() error: %v (attempts: %d)", err, atomic.LoadInt32(&attempts))
	}

	got := atomic.LoadInt32(&attempts)
	if got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestFireWebhookAllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	event := WebhookEvent{
		Event:     EventAMLFlagged,
		Timestamp: time.Now(),
		Data:      AMLFlaggedData{AccountID: "acct-123"},
	}

	err := FireWebhook(context.Background(), event, srv.URL, "secret")
	if err == nil {
		t.Fatal("expected error when all retries fail")
	}
	if !strings.Contains(err.Error(), "delivery failed after") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFireWebhookContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	event := WebhookEvent{
		Event:     EventAMLFlagged,
		Timestamp: time.Now(),
		Data:      AMLFlaggedData{AccountID: "acct-123"},
	}

	err := FireWebhook(ctx, event, srv.URL, "secret")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestVerifySignature(t *testing.T) {
	payload := []byte(`{"event":"aml.flagged","data":{"accountId":"123"}}`)
	secret := "my-hmac-secret"

	sig := signPayload(payload, secret)

	if !VerifySignature(payload, sig, secret) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature(payload, sig, "wrong-secret") {
		t.Fatal("invalid secret accepted")
	}
	if VerifySignature(payload, "deadbeef", secret) {
		t.Fatal("invalid signature accepted")
	}
	if VerifySignature([]byte("tampered"), sig, secret) {
		t.Fatal("tampered payload accepted")
	}
}

func TestActionConstants(t *testing.T) {
	if ActionAllow != "allow" {
		t.Fatalf("ActionAllow = %q", ActionAllow)
	}
	if ActionBlock != "block" {
		t.Fatalf("ActionBlock = %q", ActionBlock)
	}
	if ActionReview != "review" {
		t.Fatalf("ActionReview = %q", ActionReview)
	}
}

func TestEventConstants(t *testing.T) {
	if EventAMLFlagged != "aml.flagged" {
		t.Fatalf("EventAMLFlagged = %q", EventAMLFlagged)
	}
	if EventAMLCleared != "aml.cleared" {
		t.Fatalf("EventAMLCleared = %q", EventAMLCleared)
	}
	if EventKYCApproved != "kyc.approved" {
		t.Fatalf("EventKYCApproved = %q", EventKYCApproved)
	}
}
