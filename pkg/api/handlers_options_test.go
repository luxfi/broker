package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOptionsExpirations404WhenProviderMissing(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/options/nonexistent/expirations/AAPL")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOptionsChainRoute(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Alpaca is not registered in setupTestServer (empty registry),
	// so this should fail with provider not found.
	resp, err := authedGet(ts.URL + "/v1/options/alpaca/chain/AAPL")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unregistered provider, got %d", resp.StatusCode)
	}
}

func TestOptionsOrderValidation(t *testing.T) {
	ts, srv := setupTestServerWithRef(t)
	defer ts.Close()

	// Register account ownership so the middleware passes.
	srv.Resolver().SetMapping(testUserID, "test-org", "alpaca", "test-acct")

	// Missing action field - should fail because provider not registered (400)
	body := `{"symbol":"AAPL","contract_type":"call","strike":"150","expiration":"2026-04-18","qty":"1","order_type":"limit","limit_price":"5.50","time_in_force":"day"}`
	resp, err := authedPost(ts.URL+"/v1/accounts/alpaca/test-acct/options/orders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestMultiLegValidationTooFewLegs(t *testing.T) {
	ts, srv := setupTestServerWithRef(t)
	defer ts.Close()

	srv.Resolver().SetMapping(testUserID, "test-org", "alpaca", "test-acct")

	body := `{"symbol":"AAPL","legs":[{"contract_type":"call","strike":"150","expiration":"2026-04-18","action":"buy_to_open","qty":"1"}],"order_type":"limit","time_in_force":"day"}`
	resp, err := authedPost(ts.URL+"/v1/accounts/alpaca/test-acct/options/multi-leg", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		t.Fatalf("expected error status, got %d", resp.StatusCode)
	}
}

func TestExerciseValidation(t *testing.T) {
	ts, srv := setupTestServerWithRef(t)
	defer ts.Close()

	srv.Resolver().SetMapping(testUserID, "test-org", "alpaca", "test-acct")

	// Missing contract_symbol
	body := `{"qty":1}`
	resp, err := authedPost(ts.URL+"/v1/accounts/alpaca/test-acct/options/exercise", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected error status, got %d", resp.StatusCode)
	}
}

func TestOptionsRouteEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// No contract param
	resp, err := authedGet(ts.URL + "/v1/options/route")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing contract param, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	errMsg, _ := body["error"].(string)
	if !strings.Contains(errMsg, "contract") {
		t.Errorf("expected error about contract param, got %q", errMsg)
	}
}

// --- Input validation security tests ---

func TestSymbolValidationRejectsInvalid(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// SQL injection attempt in symbol
	resp, err := authedGet(ts.URL + "/v1/options/alpaca/expirations/AAPL'%20OR%201=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("SQL injection in symbol: expected 400, got %d", resp.StatusCode)
	}

	// Path traversal attempt
	resp2, err := authedGet(ts.URL + "/v1/options/alpaca/expirations/../../etc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	// chi router will likely 404 or 301 on path traversal, but if it reaches the handler, it should 400
	if resp2.StatusCode == http.StatusOK || resp2.StatusCode == http.StatusCreated {
		t.Fatalf("path traversal in symbol: expected non-200, got %d", resp2.StatusCode)
	}

	// Numeric injection
	resp3, err := authedGet(ts.URL + "/v1/options/alpaca/expirations/12345")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("numeric symbol: expected 400, got %d", resp3.StatusCode)
	}
}

func TestOCCSymbolValidationInQuoteEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Invalid OCC format
	resp, err := authedGet(ts.URL + "/v1/options/alpaca/quote/NOT_VALID_OCC")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid OCC: expected 400, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	errMsg, _ := body["error"].(string)
	if !strings.Contains(errMsg, "OCC") {
		t.Errorf("expected OCC format error, got %q", errMsg)
	}
}

func TestOCCSymbolValidationInGreeksEndpoint(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Invalid OCC format (but valid URL path characters)
	resp, err := authedGet(ts.URL + "/v1/options/alpaca/greeks/NOT_AN_OCC_SYMBOL")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid OCC in greeks: expected 400, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	errMsg, _ := body["error"].(string)
	if !strings.Contains(errMsg, "OCC") {
		t.Errorf("expected OCC format error, got %q", errMsg)
	}
}

func TestRouteOptionValidation(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Invalid OCC in route query
	resp, err := authedGet(ts.URL + "/v1/options/route?contract=INVALID&side=buy")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid OCC in route: expected 400, got %d", resp.StatusCode)
	}

	// Invalid side
	resp2, err := authedGet(ts.URL + "/v1/options/route?contract=AAPL260418C00150000&side=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid side: expected 400, got %d", resp2.StatusCode)
	}
}

func TestOptionChainExpirationValidation(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Invalid expiration format
	resp, err := authedGet(ts.URL + "/v1/options/alpaca/chain/AAPL?expiration=not-a-date")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid expiration: expected 400, got %d", resp.StatusCode)
	}
}

func TestValidateOCCSymbolFormat(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"AAPL260418C00150000", true},
		{"SPY260620P00500000", true},
		{"X261231C00001000", true},
		{"TSLA260418P00200000", true},
		{"", false},
		{"AAPL", false},
		{"not-valid", false},
		{"AAPL260418X00150000", false}, // X is not C or P
		{"aapl260418C00150000", false}, // lowercase
		{"TOOLONG260418C00150000", false}, // > 6 char root (7 chars)
		{"AAPL26041800150000", false},  // missing C/P
		{"<script>alert(1)</script>", false},
	}
	for _, tt := range tests {
		got := validateOCCSymbol(tt.symbol)
		if got != tt.want {
			t.Errorf("validateOCCSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

func TestValidateUnderlyingSymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"AAPL", true},
		{"SPY", true},
		{"X", true},
		{"TSLA", true},
		{"aapl", true},  // case-insensitive (uppercased internally)
		{"", false},
		{"TOOLONG1", false},  // > 6 chars
		{"123", false},
		{"AAP-L", false},
		{"AA PL", false},
		{"A.B", false},
	}
	for _, tt := range tests {
		got := validateUnderlyingSymbol(tt.symbol)
		if got != tt.want {
			t.Errorf("validateUnderlyingSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

func TestValidateQty(t *testing.T) {
	tests := []struct {
		qty  string
		want bool
	}{
		{"1", true},
		{"100", true},
		{"10000", true},
		{"10001", false},
		{"0", false},
		{"-1", false},
		{"", false},
		{"abc", false},
		{"1.5", false},
	}
	for _, tt := range tests {
		got := validateQty(tt.qty)
		if got != tt.want {
			t.Errorf("validateQty(%q) = %v, want %v", tt.qty, got, tt.want)
		}
	}
}
