package alpaca

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luxfi/broker/pkg/types"
)

// testServer creates a mock Alpaca API server with route handlers.
func testServer(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, *Provider) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := New(Config{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		APISecret: "test-secret",
	})
	return srv, p
}

// testDataServer creates a separate data server and patches the provider's dataURL.
func testDataServer(t *testing.T, p *Provider, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p.dataURL = srv.URL
	return srv
}

func TestNewAlpacaCreatesValidClient(t *testing.T) {
	p := New(Config{
		BaseURL:   "https://broker-api.sandbox.alpaca.markets",
		APIKey:    "ak",
		APISecret: "as",
	})
	if p.Name() != "alpaca" {
		t.Fatalf("Name() = %q, want 'alpaca'", p.Name())
	}
	if p.cfg.APIKey != "ak" {
		t.Fatalf("APIKey = %q, want 'ak'", p.cfg.APIKey)
	}
	if p.dataURL != DataSandboxURL {
		t.Fatalf("dataURL = %q, want sandbox data URL", p.dataURL)
	}
}

func TestNewAlpacaDefaultsToSandbox(t *testing.T) {
	p := New(Config{})
	if p.cfg.BaseURL != SandboxURL {
		t.Fatalf("BaseURL = %q, want %q", p.cfg.BaseURL, SandboxURL)
	}
}

func TestNewAlpacaProductionDataURL(t *testing.T) {
	p := New(Config{BaseURL: ProductionURL})
	if p.dataURL != DataURL {
		t.Fatalf("dataURL = %q, want %q", p.dataURL, DataURL)
	}
}

func TestCreateAccount(t *testing.T) {
	var gotBody map[string]interface{}

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}

			// Verify basic auth
			user, pass, ok := r.BasicAuth()
			if !ok || user != "test-key" || pass != "test-secret" {
				t.Error("missing or wrong basic auth")
			}

			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "acct-uuid",
				"account_number": "ACC123",
				"status":         "ACTIVE",
				"currency":       "USD",
				"created_at":     "2024-01-15T10:00:00Z",
				"identity": map[string]interface{}{
					"given_name":  "John",
					"family_name": "Doe",
				},
				"contact": map[string]interface{}{
					"email_address": "john@example.com",
				},
			})
		},
	})

	ctx := context.Background()
	acct, err := p.CreateAccount(ctx, &types.CreateAccountRequest{
		Identity: &types.Identity{
			GivenName:  "John",
			FamilyName: "Doe",
			TaxID:      "123-45-6789",
		},
		Contact: &types.Contact{
			Email: "john@example.com",
			Phone: "555-1234",
			City:  "NYC",
		},
		IPAddress: "203.0.113.42",
	})
	if err != nil {
		t.Fatalf("CreateAccount error: %v", err)
	}

	if acct.ProviderID != "acct-uuid" {
		t.Errorf("ProviderID = %q, want 'acct-uuid'", acct.ProviderID)
	}
	if acct.Provider != "alpaca" {
		t.Errorf("Provider = %q, want 'alpaca'", acct.Provider)
	}
	if acct.Status != "ACTIVE" {
		t.Errorf("Status = %q, want 'ACTIVE'", acct.Status)
	}
	if acct.Identity == nil || acct.Identity.GivenName != "John" {
		t.Error("Identity.GivenName not parsed correctly")
	}
	if acct.Contact == nil || acct.Contact.Email != "john@example.com" {
		t.Error("Contact.Email not parsed correctly")
	}

	// Verify body sent to API
	identity, ok := gotBody["identity"].(map[string]interface{})
	if !ok {
		t.Fatal("body missing identity")
	}
	if identity["given_name"] != "John" {
		t.Errorf("body identity.given_name = %v, want 'John'", identity["given_name"])
	}
}

func TestCreateOrder(t *testing.T) {
	var gotBody map[string]interface{}
	var gotPath, gotMethod string

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/orders": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotMethod = r.Method
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "order-uuid",
				"symbol":        "AAPL",
				"qty":           "10",
				"side":          "buy",
				"type":          "limit",
				"time_in_force": "gtc",
				"limit_price":   "150.00",
				"status":        "accepted",
				"created_at":    "2024-01-15T10:00:00Z",
			})
		},
	})

	ctx := context.Background()
	order, err := p.CreateOrder(ctx, "acct-1", &types.CreateOrderRequest{
		Symbol:        "AAPL",
		Qty:           "10",
		Side:          "buy",
		Type:          "limit",
		TimeInForce:   "gtc",
		LimitPrice:    "150.00",
		StopPrice:     "145.00",
		ClientOrderID: "my-order-1",
		TrailPrice:    "5.00",
		TrailPercent:  "2.5",
		ExtendedHours: true,
		OrderClass:    "bracket",
		TakeProfit:    &types.TakeProfit{LimitPrice: "160.00"},
		StopLoss:      &types.StopLoss{StopPrice: "140.00", LimitPrice: "139.00"},
	})
	if err != nil {
		t.Fatalf("CreateOrder error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/v1/trading/accounts/acct-1/orders" {
		t.Errorf("path = %s, want /v1/trading/accounts/acct-1/orders", gotPath)
	}
	if order.ProviderID != "order-uuid" {
		t.Errorf("ProviderID = %q, want 'order-uuid'", order.ProviderID)
	}
	if order.Status != "accepted" {
		t.Errorf("Status = %q, want 'accepted'", order.Status)
	}

	// Verify all fields in body
	fieldChecks := map[string]interface{}{
		"symbol":          "AAPL",
		"qty":             "10",
		"side":            "buy",
		"type":            "limit",
		"time_in_force":   "gtc",
		"limit_price":     "150.00",
		"stop_price":      "145.00",
		"client_order_id": "my-order-1",
		"trail_price":     "5.00",
		"trail_percent":   "2.5",
		"extended_hours":  true,
		"order_class":     "bracket",
	}
	for k, want := range fieldChecks {
		got, ok := gotBody[k]
		if !ok {
			t.Errorf("body missing key %q", k)
			continue
		}
		if got != want {
			t.Errorf("body[%q] = %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}

	// Check nested take_profit
	tp, ok := gotBody["take_profit"].(map[string]interface{})
	if !ok {
		t.Fatal("body missing take_profit")
	}
	if tp["limit_price"] != "160.00" {
		t.Errorf("take_profit.limit_price = %v, want '160.00'", tp["limit_price"])
	}

	// Check nested stop_loss
	sl, ok := gotBody["stop_loss"].(map[string]interface{})
	if !ok {
		t.Fatal("body missing stop_loss")
	}
	if sl["stop_price"] != "140.00" {
		t.Errorf("stop_loss.stop_price = %v, want '140.00'", sl["stop_price"])
	}
	if sl["limit_price"] != "139.00" {
		t.Errorf("stop_loss.limit_price = %v, want '139.00'", sl["limit_price"])
	}
}

func TestReplaceOrder(t *testing.T) {
	var gotBody map[string]interface{}
	var gotMethod string

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/orders/order-1": func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "order-1",
				"symbol":        "AAPL",
				"qty":           "20",
				"side":          "buy",
				"type":          "limit",
				"time_in_force": "gtc",
				"limit_price":   "155.00",
				"status":        "accepted",
				"created_at":    "2024-01-15T10:00:00Z",
			})
		},
	})

	qty := 20.0
	limitPrice := 155.0
	ctx := context.Background()
	order, err := p.ReplaceOrder(ctx, "acct-1", "order-1", &types.ReplaceOrderRequest{
		Qty:           &qty,
		LimitPrice:    &limitPrice,
		TimeInForce:   "gtc",
		ClientOrderID: "replaced-1",
	})
	if err != nil {
		t.Fatalf("ReplaceOrder error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if order.ProviderID != "order-1" {
		t.Errorf("ProviderID = %q, want 'order-1'", order.ProviderID)
	}

	// Verify body fields
	if gotBody["qty"] != "20" {
		t.Errorf("body qty = %v, want '20'", gotBody["qty"])
	}
	if gotBody["limit_price"] != "155" {
		t.Errorf("body limit_price = %v, want '155'", gotBody["limit_price"])
	}
	if gotBody["time_in_force"] != "gtc" {
		t.Errorf("body time_in_force = %v, want 'gtc'", gotBody["time_in_force"])
	}
	if gotBody["client_order_id"] != "replaced-1" {
		t.Errorf("body client_order_id = %v, want 'replaced-1'", gotBody["client_order_id"])
	}
}

func TestListOrdersFiltered(t *testing.T) {
	var gotQuery string

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/orders": func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":            "o1",
					"symbol":        "AAPL",
					"side":          "buy",
					"type":          "market",
					"time_in_force": "day",
					"status":        "filled",
					"created_at":    "2024-01-15T10:00:00Z",
				},
			})
		},
	})

	ctx := context.Background()
	orders, err := p.ListOrdersFiltered(ctx, "acct-1", &types.ListOrdersParams{
		Status:    "closed",
		Limit:     10,
		Direction: "desc",
		Nested:    true,
	})
	if err != nil {
		t.Fatalf("ListOrdersFiltered error: %v", err)
	}

	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}

	// Check query params
	if !strings.Contains(gotQuery, "status=closed") {
		t.Errorf("query missing status=closed: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "limit=10") {
		t.Errorf("query missing limit=10: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "direction=desc") {
		t.Errorf("query missing direction=desc: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "nested=true") {
		t.Errorf("query missing nested=true: %s", gotQuery)
	}
}

func TestGetPosition(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/positions/AAPL": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"symbol":          "AAPL",
				"qty":             "100",
				"avg_entry_price": "150.00",
				"market_value":    "15500.00",
				"current_price":   "155.00",
				"unrealized_pl":   "500.00",
				"side":            "long",
				"asset_class":     "us_equity",
			})
		},
	})

	ctx := context.Background()
	pos, err := p.GetPosition(ctx, "acct-1", "AAPL")
	if err != nil {
		t.Fatalf("GetPosition error: %v", err)
	}

	if pos.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want 'AAPL'", pos.Symbol)
	}
	if pos.Qty != "100" {
		t.Errorf("Qty = %q, want '100'", pos.Qty)
	}
	if pos.AvgEntryPrice != "150.00" {
		t.Errorf("AvgEntryPrice = %q, want '150.00'", pos.AvgEntryPrice)
	}
	if pos.UnrealizedPL != "500.00" {
		t.Errorf("UnrealizedPL = %q, want '500.00'", pos.UnrealizedPL)
	}
	if pos.Side != "long" {
		t.Errorf("Side = %q, want 'long'", pos.Side)
	}
}

func TestClosePositionWithQty(t *testing.T) {
	var gotQuery string
	var gotMethod string

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/positions/AAPL": func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotQuery = r.URL.RawQuery

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "close-order-1",
				"symbol":        "AAPL",
				"side":          "sell",
				"type":          "market",
				"time_in_force": "day",
				"status":        "accepted",
				"created_at":    "2024-01-15T10:00:00Z",
			})
		},
	})

	qty := 50.0
	ctx := context.Background()
	order, err := p.ClosePosition(ctx, "acct-1", "AAPL", &qty)
	if err != nil {
		t.Fatalf("ClosePosition error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if !strings.Contains(gotQuery, "qty=50") {
		t.Errorf("query = %q, want 'qty=50'", gotQuery)
	}
	if order.ProviderID != "close-order-1" {
		t.Errorf("ProviderID = %q, want 'close-order-1'", order.ProviderID)
	}
}

func TestGetBarsPagination(t *testing.T) {
	callCount := 0
	_, p := testServer(t, nil) // base server not used for data

	testDataServer(t, p, map[string]http.HandlerFunc{
		"/v2/stocks/AAPL/bars": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				// First page with next_page_token
				json.NewEncoder(w).Encode(map[string]interface{}{
					"bars": []map[string]interface{}{
						{"t": "2024-01-15T10:00:00Z", "o": 150.0, "h": 155.0, "l": 149.0, "c": 153.0, "v": 1000.0},
					},
					"next_page_token": "page2token",
				})
			} else {
				// Second page, no token
				json.NewEncoder(w).Encode(map[string]interface{}{
					"bars": []map[string]interface{}{
						{"t": "2024-01-16T10:00:00Z", "o": 153.0, "h": 158.0, "l": 152.0, "c": 157.0, "v": 2000.0},
					},
					"next_page_token": "",
				})
			}
		},
	})

	ctx := context.Background()
	bars, err := p.GetBars(ctx, "AAPL", "1Day", "2024-01-15", "2024-01-17", 0)
	if err != nil {
		t.Fatalf("GetBars error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 API calls (pagination), got %d", callCount)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	if bars[0].Close != 153.0 {
		t.Errorf("bars[0].Close = %f, want 153.0", bars[0].Close)
	}
	if bars[1].Close != 157.0 {
		t.Errorf("bars[1].Close = %f, want 157.0", bars[1].Close)
	}
}

func TestIsCryptoSymbol(t *testing.T) {
	tests := []struct {
		symbol string
		want   bool
	}{
		{"BTC/USD", true},
		{"ETH/USD", true},
		{"AAPL", false},
		{"GOOG", false},
		{"SOL/USD", true},
		{"", false},
	}
	for _, tt := range tests {
		got := isCryptoSymbol(tt.symbol)
		if got != tt.want {
			t.Errorf("isCryptoSymbol(%q) = %v, want %v", tt.symbol, got, tt.want)
		}
	}
}

func TestStocksOrCryptoPath(t *testing.T) {
	tests := []struct {
		symbol string
		want   string
	}{
		{"AAPL", "/v2/stocks"},
		{"BTC/USD", "/v1beta3/crypto/us"},
	}
	for _, tt := range tests {
		got := stocksOrCryptoPath(tt.symbol)
		if got != tt.want {
			t.Errorf("stocksOrCryptoPath(%q) = %q, want %q", tt.symbol, got, tt.want)
		}
	}
}

func TestGetSnapshotStock(t *testing.T) {
	_, p := testServer(t, nil)
	testDataServer(t, p, map[string]http.HandlerFunc{
		"/v2/stocks/AAPL/snapshot": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"latestTrade": map[string]interface{}{
					"t": "2024-01-15T10:00:00Z",
					"p": 155.0,
					"s": 100.0,
					"x": "V",
				},
				"latestQuote": map[string]interface{}{
					"t":  "2024-01-15T10:00:00Z",
					"bp": 154.9,
					"bs": 200.0,
					"ap": 155.1,
					"as": 150.0,
				},
			})
		},
	})

	ctx := context.Background()
	snap, err := p.GetSnapshot(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetSnapshot error: %v", err)
	}

	if snap.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want 'AAPL'", snap.Symbol)
	}
	if snap.LatestTrade == nil || snap.LatestTrade.Price != 155.0 {
		t.Errorf("LatestTrade.Price = %v, want 155.0", snap.LatestTrade)
	}
	if snap.LatestQuote == nil || snap.LatestQuote.BidPrice != 154.9 {
		t.Errorf("LatestQuote.BidPrice = %v, want 154.9", snap.LatestQuote)
	}
}

func TestGetSnapshotCrypto(t *testing.T) {
	_, p := testServer(t, nil)
	testDataServer(t, p, map[string]http.HandlerFunc{
		"/v1beta3/crypto/us/snapshots": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"snapshots": map[string]interface{}{
					"BTC/USD": map[string]interface{}{
						"latestTrade": map[string]interface{}{
							"t": "2024-01-15T10:00:00Z",
							"p": 42000.0,
							"s": 0.5,
						},
					},
				},
			})
		},
	})

	ctx := context.Background()
	snap, err := p.GetSnapshot(ctx, "BTC/USD")
	if err != nil {
		t.Fatalf("GetSnapshot crypto error: %v", err)
	}

	if snap.Symbol != "BTC/USD" {
		t.Errorf("Symbol = %q, want 'BTC/USD'", snap.Symbol)
	}
	if snap.LatestTrade == nil || snap.LatestTrade.Price != 42000.0 {
		t.Errorf("LatestTrade.Price = %v, want 42000.0", snap.LatestTrade)
	}
}

func TestErrorHandling4xx(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/bad-id": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    40410000,
				"message": "account not found",
			})
		},
	})

	ctx := context.Background()
	_, err := p.GetAccount(ctx, "bad-id")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "account not found") {
		t.Errorf("error = %q, want to contain 'account not found'", err.Error())
	}
}

func TestErrorHandlingGeneric(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/server-err": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		},
	})

	ctx := context.Background()
	_, err := p.GetAccount(ctx, "server-err")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain '500'", err.Error())
	}
}

func TestGetAccount(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "acct-1",
				"account_number": "A100",
				"status":         "ACTIVE",
				"currency":       "USD",
				"created_at":     "2024-01-15T10:00:00Z",
			})
		},
	})

	ctx := context.Background()
	acct, err := p.GetAccount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("GetAccount error: %v", err)
	}
	if acct.ProviderID != "acct-1" {
		t.Errorf("ProviderID = %q, want 'acct-1'", acct.ProviderID)
	}
	if acct.AccountNumber != "A100" {
		t.Errorf("AccountNumber = %q, want 'A100'", acct.AccountNumber)
	}
}

func TestListAccounts(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "a1", "status": "ACTIVE", "created_at": "2024-01-15T10:00:00Z"},
				{"id": "a2", "status": "ACTIVE", "created_at": "2024-01-16T10:00:00Z"},
			})
		},
	})

	ctx := context.Background()
	accts, err := p.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts error: %v", err)
	}
	if len(accts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accts))
	}
}

func TestGetPortfolio(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/account": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cash":            "10000.00",
				"equity":          "50000.00",
				"buying_power":    "100000.00",
				"portfolio_value": "50000.00",
			})
		},
		"/v1/trading/accounts/acct-1/positions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"symbol":          "AAPL",
					"qty":             "50",
					"avg_entry_price": "150.00",
					"market_value":    "7750.00",
					"current_price":   "155.00",
					"unrealized_pl":   "250.00",
					"side":            "long",
					"asset_class":     "us_equity",
				},
			})
		},
	})

	ctx := context.Background()
	port, err := p.GetPortfolio(ctx, "acct-1")
	if err != nil {
		t.Fatalf("GetPortfolio error: %v", err)
	}
	if port.Cash != "10000.00" {
		t.Errorf("Cash = %q, want '10000.00'", port.Cash)
	}
	if len(port.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(port.Positions))
	}
	if port.Positions[0].Symbol != "AAPL" {
		t.Errorf("Position symbol = %q, want 'AAPL'", port.Positions[0].Symbol)
	}
}

func TestCancelOrder(t *testing.T) {
	var gotMethod string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/acct-1/orders/order-1": func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			w.WriteHeader(http.StatusNoContent)
		},
	})

	ctx := context.Background()
	err := p.CancelOrder(ctx, "acct-1", "order-1")
	if err != nil {
		t.Fatalf("CancelOrder error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
}
