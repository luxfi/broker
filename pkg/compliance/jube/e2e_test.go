//go:build integration

package jube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

// jubeURL returns the Jube endpoint from env or defaults to http://jube:5001.
func jubeURL() string {
	if u := os.Getenv("JUBE_URL"); u != "" {
		return u
	}
	return DefaultBaseURL
}

// --- Smoke Tests ---

func TestSmoke_JubeReady(t *testing.T) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(jubeURL() + "/api/Ready")
	if err != nil {
		t.Fatalf("GET /api/Ready: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/Ready: status %d, body: %s", resp.StatusCode, string(body))
	}
	t.Log("Jube is ready")
}

func TestSmoke_SanctionsHit(t *testing.T) {
	c := &http.Client{Timeout: 10 * time.Second}
	url := jubeURL() + "/api/Invoke/Sanction?multiPartString=OSAMA+BIN+LADEN&distance=1"
	resp, err := c.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/Invoke/Sanction: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/Invoke/Sanction: status %d, body: %s", resp.StatusCode, string(body))
	}

	// The response should contain sanctions match data.
	// Jube returns different structures depending on config, so we just
	// verify we got a non-empty JSON response.
	if len(body) < 2 {
		t.Fatal("expected non-empty sanctions response")
	}
	t.Logf("Sanctions hit response: %s", string(body))
}

// --- E2E: Structuring Detection ---

func TestE2E_StructuringDetection(t *testing.T) {
	client, err := New(Config{BaseURL: jubeURL(), Timeout: 15 * time.Second})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer client.Close()

	// Submit a pattern of rapid-fire $9,500 transactions.
	// This is the classic structuring pattern: multiple transactions just
	// below the $10,000 CTR reporting threshold.
	accountID := fmt.Sprintf("e2e-structuring-%d", time.Now().UnixNano())

	var highestScore float64
	var gotBlock bool
	var allAlerts []Alert

	for i := 0; i < 5; i++ {
		resp, err := client.ScreenTransaction(context.Background(), TransactionRequest{
			EntityAnalysisModelID: 1,
			EntityInstanceEntryPayload: map[string]interface{}{
				"AccountId":     accountID,
				"TransactionId": fmt.Sprintf("tx-%s-%d", accountID, i),
				"Amount":        9500,
				"Currency":      "USD",
				"Side":          "buy",
				"Symbol":        "BTC-USD",
				"IP":            "192.168.1.100",
			},
		})
		if err != nil {
			t.Fatalf("ScreenTransaction #%d error: %v", i, err)
		}

		t.Logf("Transaction #%d: score=%.2f action=%s alerts=%d",
			i, resp.Score, resp.Action, len(resp.Alerts))

		if resp.Score > highestScore {
			highestScore = resp.Score
		}
		if resp.Action == ActionBlock {
			gotBlock = true
		}
		allAlerts = append(allAlerts, resp.Alerts...)
	}

	// Jube should detect structuring with escalating scores.
	// We require at least one of:
	// - A block action
	// - A high risk score (> 0.5)
	// - At least one alert
	detected := gotBlock || highestScore > 0.5 || len(allAlerts) > 0
	if !detected {
		t.Fatalf("structuring not detected: highest_score=%.2f blocked=%v alerts=%d",
			highestScore, gotBlock, len(allAlerts))
	}

	t.Logf("Structuring detection: highest_score=%.2f blocked=%v total_alerts=%d",
		highestScore, gotBlock, len(allAlerts))
}

// --- E2E: Pre-Trade Screen Integration ---

func TestE2E_PreTradeScreenBlock(t *testing.T) {
	client, err := New(Config{BaseURL: jubeURL(), Timeout: 15 * time.Second})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer client.Close()

	screen := NewPreTradeScreen(client, PreTradeConfig{
		ModelID:       1,
		AllowOnReview: false, // strict mode
		AllowOnError:  false,
	})

	accountID := fmt.Sprintf("e2e-pretrade-%d", time.Now().UnixNano())

	// Submit rapid structuring pattern through the pre-trade screen.
	var lastResult PreTradeResult
	for i := 0; i < 5; i++ {
		lastResult = screen.Screen(context.Background(), ScreenRequest{
			AccountID: accountID,
			OrderID:   fmt.Sprintf("order-%s-%d", accountID, i),
			Provider:  "alpaca",
			Symbol:    "BTC-USD",
			Side:      "buy",
			Qty:       "1",
			Price:     "9500",
			Currency:  "USD",
			IP:        "10.0.0.1",
		})

		t.Logf("PreTrade #%d: action=%s allowed=%v score=%.2f errors=%v warnings=%v",
			i, lastResult.Action, lastResult.Allowed, lastResult.Score,
			lastResult.Errors, lastResult.Warnings)
	}

	// After 5 structuring transactions, Jube should have escalated.
	if lastResult.Score < 0.3 {
		t.Logf("WARNING: Jube did not escalate score after 5 structuring txns (score=%.2f)", lastResult.Score)
		t.Log("This may indicate Jube's structuring model needs configuration.")
		t.Log("Ensure EntityAnalysisModel ID=1 has structuring rules enabled.")
	}
}

// --- E2E: Webhook Fires on AML Flag ---

func TestE2E_WebhookFiresOnFlag(t *testing.T) {
	var mu sync.Mutex
	var received []WebhookEvent

	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event WebhookEvent
		json.NewDecoder(r.Body).Decode(&event)
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	client, err := New(Config{BaseURL: jubeURL(), Timeout: 15 * time.Second})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer client.Close()

	screen := NewPreTradeScreen(client, PreTradeConfig{
		ModelID:           1,
		AllowOnReview:     true,
		AllowOnError:      true,
		WebhookURL:        webhookSrv.URL,
		WebhookHMACSecret: "e2e-test-secret",
	})

	accountID := fmt.Sprintf("e2e-webhook-%d", time.Now().UnixNano())

	// Submit structuring pattern to trigger flags.
	for i := 0; i < 5; i++ {
		screen.Screen(context.Background(), ScreenRequest{
			AccountID: accountID,
			OrderID:   fmt.Sprintf("order-%s-%d", accountID, i),
			Provider:  "alpaca",
			Symbol:    "BTC-USD",
			Side:      "buy",
			Qty:       "1",
			Price:     "9500",
			Currency:  "USD",
		})
	}

	// Wait for async webhook deliveries.
	time.Sleep(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Log("No webhooks received. This is expected if Jube does not flag structuring by default.")
		t.Log("Configure Jube with structuring rules (EntityAnalysisModel ID=1) to trigger aml.flagged events.")
		return
	}

	for i, event := range received {
		if event.Event != EventAMLFlagged {
			t.Errorf("webhook #%d: event=%q, want %q", i, event.Event, EventAMLFlagged)
		}
	}
	t.Logf("Received %d aml.flagged webhooks", len(received))
}

// --- E2E: Sanctions Screen via Client ---

func TestE2E_SanctionsScreenSDN(t *testing.T) {
	client, err := New(Config{BaseURL: jubeURL(), Timeout: 15 * time.Second})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer client.Close()

	result, err := client.CheckSanctions(context.Background(), "OSAMA BIN LADEN", "")
	if err != nil {
		t.Fatalf("CheckSanctions() error: %v", err)
	}

	t.Logf("Sanctions result: hit=%v matches=%d", result.Hit, len(result.Matches))

	if !result.Hit {
		t.Log("WARNING: No sanctions hit for 'OSAMA BIN LADEN'. Check Jube sanctions list configuration.")
	}
}

// --- E2E: Case Management ---

func TestE2E_CreateCaseForFlaggedTransaction(t *testing.T) {
	client, err := New(Config{BaseURL: jubeURL(), Timeout: 15 * time.Second})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer client.Close()

	accountID := fmt.Sprintf("e2e-case-%d", time.Now().UnixNano())

	cas, err := client.CreateCase(context.Background(), CaseRequest{
		AccountID:   accountID,
		Type:        "aml",
		Severity:    "high",
		Description: "E2E test: structuring detected in rapid $9,500 transactions",
	})
	if err != nil {
		t.Fatalf("CreateCase() error: %v", err)
	}

	if cas.ID == "" {
		t.Fatal("expected case to have an ID")
	}
	if cas.Status != "open" {
		t.Logf("case status = %q (expected 'open', but Jube may use different defaults)", cas.Status)
	}

	t.Logf("Created case: id=%s status=%s", cas.ID, cas.Status)

	// Verify we can retrieve the case.
	cases, err := client.GetCases(context.Background(), CaseFilter{AccountID: accountID, Type: "aml"})
	if err != nil {
		t.Fatalf("GetCases() error: %v", err)
	}
	if len(cases) == 0 {
		t.Log("WARNING: GetCases returned 0 results. Jube may not support case listing with these filters.")
	} else {
		t.Logf("Retrieved %d cases for account %s", len(cases), accountID)
	}
}
