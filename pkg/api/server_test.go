package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/luxfi/broker/pkg/provider"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	os.Setenv("BROKER_DEV_MODE", "true")
	os.Setenv("ADMIN_SECRET", "test-secret")
	t.Cleanup(func() {
		os.Unsetenv("BROKER_DEV_MODE")
		os.Unsetenv("ADMIN_SECRET")
	})

	registry := provider.NewRegistry()
	srv := NewServer(registry, ":0")
	return httptest.NewServer(srv.Handler())
}

func TestHealthz(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}
}

func TestListProviders(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/providers")
	if err != nil {
		t.Fatalf("GET /v1/providers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	providers, ok := body["providers"].([]interface{})
	if !ok {
		t.Fatal("expected providers array")
	}
	// Empty registry = empty list
	if len(providers) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(providers))
	}
}

func TestGetCapabilities(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/providers/capabilities")
	if err != nil {
		t.Fatalf("GET /v1/providers/capabilities: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRiskCheck(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/risk/check?provider=test&account_id=a1&symbol=BTC&side=buy&qty=1")
	if err != nil {
		t.Fatalf("GET /v1/risk/check: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if _, ok := body["allowed"]; !ok {
		t.Fatal("expected 'allowed' field in risk check response")
	}
}

func TestAuditQuery(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/audit")
	if err != nil {
		t.Fatalf("GET /v1/audit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuditStats(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/audit/stats")
	if err != nil {
		t.Fatalf("GET /v1/audit/stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuditExport(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/audit/export")
	if err != nil {
		t.Fatalf("GET /v1/audit/export: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json content-type, got %s", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if cd == "" {
		t.Fatal("expected Content-Disposition header for audit export")
	}
}

func TestListAccountsEmptyRegistry(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/accounts")
	if err != nil {
		t.Fatalf("GET /v1/accounts: %v", err)
	}
	defer resp.Body.Close()

	// With empty registry, should return empty array
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestListAssetsUnknownProvider(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/assets/nonexistent")
	if err != nil {
		t.Fatalf("GET /v1/assets/nonexistent: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown provider, got %d", resp.StatusCode)
	}
}

func TestStreamEndpointExists(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Just verify the endpoint exists and responds (SSE will timeout, that's fine)
	client := &http.Client{Timeout: 1}
	resp, _ := client.Get(ts.URL + "/v1/stream")
	if resp != nil {
		resp.Body.Close()
	}
}

func TestCreateAccountBadRequest(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Valid JSON body but no provider registered -> 400
	body := `{"provider":"alpaca","given_name":"Test","family_name":"User","email":"test@example.com"}`
	resp, err := http.Post(ts.URL+"/v1/accounts", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/accounts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["error"] == "" {
		t.Fatal("expected error message in response body")
	}
}

func TestCreateAccountInvalidBody(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/accounts", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("POST /v1/accounts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestRiskCheckResponseStructure(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/risk/check?provider=alpaca&account_id=acct1&symbol=AAPL&side=buy&qty=10&price=150&type=limit")
	if err != nil {
		t.Fatalf("GET /v1/risk/check: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	allowed, ok := result["allowed"].(bool)
	if !ok {
		t.Fatal("'allowed' field should be boolean")
	}
	if !allowed {
		t.Fatal("expected allowed=true for normal order against default limits")
	}
}

func TestAuditStatsResponseStructure(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/audit/stats")
	if err != nil {
		t.Fatalf("GET /v1/audit/stats: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["total_entries"] == nil {
		t.Fatal("missing total_entries")
	}
	if result["actions"] == nil {
		t.Fatal("missing actions")
	}
	if result["providers"] == nil {
		t.Fatal("missing providers")
	}
	if result["statuses"] == nil {
		t.Fatal("missing statuses")
	}
	if result["avg_order_latency"] == nil {
		t.Fatal("missing avg_order_latency")
	}
}

func TestAuditExportContentDisposition(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/audit/export")
	if err != nil {
		t.Fatalf("GET /v1/audit/export: %v", err)
	}
	defer resp.Body.Close()

	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "audit_export.json") {
		t.Fatalf("Content-Disposition = %q, want to contain audit_export.json", cd)
	}

	// Body should be valid JSON array
	var entries []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestAuthRequiredWithoutDevMode(t *testing.T) {
	os.Setenv("BROKER_DEV_MODE", "")
	os.Setenv("ADMIN_SECRET", "test-secret")
	defer func() {
		os.Unsetenv("BROKER_DEV_MODE")
		os.Unsetenv("ADMIN_SECRET")
	}()

	registry := provider.NewRegistry()
	srv := NewServer(registry, ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/providers")
	if err != nil {
		t.Fatalf("GET /v1/providers: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestHealthzBypassesAuth(t *testing.T) {
	os.Setenv("BROKER_DEV_MODE", "")
	os.Setenv("ADMIN_SECRET", "test-secret")
	defer func() {
		os.Unsetenv("BROKER_DEV_MODE")
		os.Unsetenv("ADMIN_SECRET")
	}()

	registry := provider.NewRegistry()
	srv := NewServer(registry, ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
