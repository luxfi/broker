package alpaca_omnisub

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

const testOmnibusID = "omni-master-001"

func testServer(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, *Provider) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		mux.HandleFunc(pattern, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := New(Config{
		BaseURL:          srv.URL,
		APIKey:           "test-key",
		APISecret:        "test-secret",
		OmnibusAccountID: testOmnibusID,
	})
	return srv, p
}

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

func TestName(t *testing.T) {
	p := New(Config{})
	if p.Name() != "alpaca_omnisub" {
		t.Fatalf("Name() = %q, want 'alpaca_omnisub'", p.Name())
	}
}

func TestDefaultsToSandbox(t *testing.T) {
	p := New(Config{})
	if p.cfg.BaseURL != SandboxURL {
		t.Fatalf("BaseURL = %q, want %q", p.cfg.BaseURL, SandboxURL)
	}
	if p.dataURL != DataSandboxURL {
		t.Fatalf("dataURL = %q, want %q", p.dataURL, DataSandboxURL)
	}
}

func TestProductionDataURL(t *testing.T) {
	p := New(Config{BaseURL: ProductionURL})
	if p.dataURL != DataURL {
		t.Fatalf("dataURL = %q, want %q", p.dataURL, DataURL)
	}
}

func TestCreateSubAccount(t *testing.T) {
	var gotBody map[string]interface{}

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			user, pass, ok := r.BasicAuth()
			if !ok || user != "test-key" || pass != "test-secret" {
				t.Error("missing or wrong basic auth")
			}

			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "sub-uuid-1",
				"account_number": "SUB001",
				"status":         "ACTIVE",
				"currency":       "USD",
				"account_type":   "omnibus_sub_account",
				"created_at":     "2026-04-01T10:00:00Z",
				"identity": map[string]interface{}{
					"given_name":  "Alice",
					"family_name": "Smith",
				},
				"contact": map[string]interface{}{
					"email_address": "alice@example.com",
				},
			})
		},
	})

	ctx := context.Background()
	acct, err := p.CreateAccount(ctx, &types.CreateAccountRequest{
		Identity: &types.Identity{
			GivenName:  "Alice",
			FamilyName: "Smith",
			TaxID:      "123-45-6789",
		},
		Contact: &types.Contact{
			Email: "alice@example.com",
			Phone: "555-0001",
		},
		IPAddress: "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("CreateAccount error: %v", err)
	}

	if acct.ProviderID != "sub-uuid-1" {
		t.Errorf("ProviderID = %q, want 'sub-uuid-1'", acct.ProviderID)
	}
	if acct.Provider != "alpaca_omnisub" {
		t.Errorf("Provider = %q, want 'alpaca_omnisub'", acct.Provider)
	}
	if acct.AccountType != "omnibus_sub_account" {
		t.Errorf("AccountType = %q, want 'omnibus_sub_account'", acct.AccountType)
	}

	// Verify body includes omnisub fields
	if gotBody["account_type"] != "omnibus_sub_account" {
		t.Errorf("body account_type = %v, want 'omnibus_sub_account'", gotBody["account_type"])
	}
	if gotBody["omnibus_master_id"] != testOmnibusID {
		t.Errorf("body omnibus_master_id = %v, want %q", gotBody["omnibus_master_id"], testOmnibusID)
	}
}

func TestCreateSubAccountMissingIP(t *testing.T) {
	_, p := testServer(t, nil)
	_, err := p.CreateAccount(context.Background(), &types.CreateAccountRequest{
		Identity: &types.Identity{GivenName: "Bob"},
		Contact:  &types.Contact{Email: "bob@example.com"},
	})
	if err == nil {
		t.Fatal("expected error for missing IP")
	}
	if !strings.Contains(err.Error(), "IP address") {
		t.Errorf("error = %q, want to contain 'IP address'", err.Error())
	}
}

func TestGetAccount(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/sub-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "sub-1",
				"account_number": "S100",
				"status":         "ACTIVE",
				"currency":       "USD",
				"account_type":   "omnibus_sub_account",
				"created_at":     "2026-04-01T10:00:00Z",
			})
		},
	})

	acct, err := p.GetAccount(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("GetAccount error: %v", err)
	}
	if acct.ProviderID != "sub-1" {
		t.Errorf("ProviderID = %q, want 'sub-1'", acct.ProviderID)
	}
	if acct.AccountNumber != "S100" {
		t.Errorf("AccountNumber = %q, want 'S100'", acct.AccountNumber)
	}
}

func TestListAccounts(t *testing.T) {
	var gotQuery string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts": func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "s1", "status": "ACTIVE", "account_type": "omnibus_sub_account", "created_at": "2026-04-01T10:00:00Z"},
				{"id": "s2", "status": "ACTIVE", "account_type": "omnibus_sub_account", "created_at": "2026-04-02T10:00:00Z"},
			})
		},
	})

	accts, err := p.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts error: %v", err)
	}
	if len(accts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accts))
	}
	// Verify query filters by omnibus master
	if !strings.Contains(gotQuery, testOmnibusID) {
		t.Errorf("query = %q, want to contain omnibus master ID %q", gotQuery, testOmnibusID)
	}
}

func TestPlaceOrder(t *testing.T) {
	var gotBody map[string]interface{}
	var gotPath string

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/sub-1/orders": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "order-uuid-1",
				"symbol":        "AAPL",
				"qty":           "10",
				"side":          "buy",
				"type":          "limit",
				"time_in_force": "day",
				"limit_price":   "150.00",
				"status":        "accepted",
				"created_at":    "2026-04-01T10:00:00Z",
			})
		},
	})

	order, err := p.CreateOrder(context.Background(), "sub-1", &types.CreateOrderRequest{
		Symbol:      "AAPL",
		Qty:         "10",
		Side:        "buy",
		Type:        "limit",
		TimeInForce: "day",
		LimitPrice:  "150.00",
	})
	if err != nil {
		t.Fatalf("CreateOrder error: %v", err)
	}

	if gotPath != "/v1/trading/accounts/sub-1/orders" {
		t.Errorf("path = %s, want /v1/trading/accounts/sub-1/orders", gotPath)
	}
	if order.ProviderID != "order-uuid-1" {
		t.Errorf("ProviderID = %q, want 'order-uuid-1'", order.ProviderID)
	}
	if order.Provider != "alpaca_omnisub" {
		t.Errorf("Provider = %q, want 'alpaca_omnisub'", order.Provider)
	}
	if order.Status != "accepted" {
		t.Errorf("Status = %q, want 'accepted'", order.Status)
	}
	if gotBody["symbol"] != "AAPL" {
		t.Errorf("body symbol = %v, want 'AAPL'", gotBody["symbol"])
	}
}

func TestGetOrder(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/sub-1/orders/o1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         "o1",
				"symbol":     "AAPL",
				"status":     "filled",
				"created_at": "2026-04-01T10:00:00Z",
			})
		},
	})

	order, err := p.GetOrder(context.Background(), "sub-1", "o1")
	if err != nil {
		t.Fatalf("GetOrder error: %v", err)
	}
	if order.ProviderID != "o1" {
		t.Errorf("ProviderID = %q, want 'o1'", order.ProviderID)
	}
}

func TestCancelOrder(t *testing.T) {
	var gotMethod string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/sub-1/orders/o1": func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			w.WriteHeader(http.StatusNoContent)
		},
	})

	err := p.CancelOrder(context.Background(), "sub-1", "o1")
	if err != nil {
		t.Fatalf("CancelOrder error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
}

func TestListOrders(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/sub-1/orders": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "o1", "symbol": "AAPL", "status": "filled", "created_at": "2026-04-01T10:00:00Z"},
				{"id": "o2", "symbol": "GOOG", "status": "accepted", "created_at": "2026-04-01T11:00:00Z"},
			})
		},
	})

	orders, err := p.ListOrders(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("ListOrders error: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}
}

func TestGetPortfolio(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/sub-1/account": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cash":            "5000.00",
				"equity":          "25000.00",
				"buying_power":    "50000.00",
				"portfolio_value": "25000.00",
			})
		},
		"/v1/trading/accounts/sub-1/positions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"symbol":          "AAPL",
					"qty":             "20",
					"avg_entry_price": "150.00",
					"market_value":    "3100.00",
					"current_price":   "155.00",
					"unrealized_pl":   "100.00",
					"side":            "long",
					"asset_class":     "us_equity",
				},
			})
		},
	})

	port, err := p.GetPortfolio(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("GetPortfolio error: %v", err)
	}
	if port.Cash != "5000.00" {
		t.Errorf("Cash = %q, want '5000.00'", port.Cash)
	}
	if len(port.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(port.Positions))
	}
	if port.Positions[0].Symbol != "AAPL" {
		t.Errorf("position symbol = %q, want 'AAPL'", port.Positions[0].Symbol)
	}
}

func TestTransferGoesToOmnibus(t *testing.T) {
	var gotPath string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/" + testOmnibusID + "/transfers": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         "xfer-1",
				"account_id": testOmnibusID,
				"type":       "ach",
				"direction":  "INCOMING",
				"amount":     "1000.00",
				"currency":   "USD",
				"status":     "QUEUED",
				"created_at": "2026-04-01T10:00:00Z",
				"updated_at": "2026-04-01T10:00:00Z",
			})
		},
	})

	xfer, err := p.CreateTransfer(context.Background(), "sub-1", &types.CreateTransferRequest{
		Type:      "ach",
		Direction: "INCOMING",
		Amount:    "1000.00",
	})
	if err != nil {
		t.Fatalf("CreateTransfer error: %v", err)
	}

	// Verify the transfer went to the omnibus, not the sub
	if gotPath != "/v1/accounts/"+testOmnibusID+"/transfers" {
		t.Errorf("path = %s, want /v1/accounts/%s/transfers", gotPath, testOmnibusID)
	}
	if xfer.ProviderID != "xfer-1" {
		t.Errorf("ProviderID = %q, want 'xfer-1'", xfer.ProviderID)
	}
	if xfer.Provider != "alpaca_omnisub" {
		t.Errorf("Provider = %q, want 'alpaca_omnisub'", xfer.Provider)
	}
}

func TestListTransfersOnOmnibus(t *testing.T) {
	var gotPath string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/" + testOmnibusID + "/transfers": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "x1", "amount": "500.00", "status": "COMPLETE", "created_at": "2026-04-01T10:00:00Z", "updated_at": "2026-04-01T10:00:00Z"},
			})
		},
	})

	xfers, err := p.ListTransfers(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("ListTransfers error: %v", err)
	}
	if len(xfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(xfers))
	}
	if gotPath != "/v1/accounts/"+testOmnibusID+"/transfers" {
		t.Errorf("path = %s, want omnibus transfers path", gotPath)
	}
}

func TestCreateJournal(t *testing.T) {
	var gotBody map[string]interface{}

	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/journals": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "j-1",
				"entry_type":   "JNLC",
				"from_account": testOmnibusID,
				"to_account":   "sub-1",
				"net_amount":   "500.00",
				"status":       "executed",
				"created_at":   "2026-04-01T10:00:00Z",
			})
		},
	})

	j, err := p.CreateJournal(context.Background(), &types.CreateJournalRequest{
		EntryType:   "JNLC",
		FromAccount: testOmnibusID,
		ToAccount:   "sub-1",
		Amount:      "500.00",
		Description: "fund sub-account",
	})
	if err != nil {
		t.Fatalf("CreateJournal error: %v", err)
	}
	if j.ID != "j-1" {
		t.Errorf("ID = %q, want 'j-1'", j.ID)
	}
	if j.Amount != "500.00" {
		t.Errorf("Amount = %q, want '500.00'", j.Amount)
	}
	if gotBody["from_account"] != testOmnibusID {
		t.Errorf("body from_account = %v, want %q", gotBody["from_account"], testOmnibusID)
	}
	if gotBody["description"] != "fund sub-account" {
		t.Errorf("body description = %v, want 'fund sub-account'", gotBody["description"])
	}
}

func TestGetOmnibusSnapshot(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/" + testOmnibusID + "/account": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cash":            "100000.00",
				"equity":          "500000.00",
				"buying_power":    "1000000.00",
				"portfolio_value": "500000.00",
			})
		},
		"/v1/trading/accounts/" + testOmnibusID + "/positions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"symbol":          "AAPL",
					"qty":             "1000",
					"avg_entry_price": "150.00",
					"market_value":    "155000.00",
					"current_price":   "155.00",
					"unrealized_pl":   "5000.00",
					"side":            "long",
					"asset_class":     "us_equity",
				},
				{
					"symbol":          "GOOG",
					"qty":             "200",
					"avg_entry_price": "2800.00",
					"market_value":    "580000.00",
					"current_price":   "2900.00",
					"unrealized_pl":   "20000.00",
					"side":            "long",
					"asset_class":     "us_equity",
				},
			})
		},
	})

	snap, err := p.GetOmnibusSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetOmnibusSnapshot error: %v", err)
	}
	if snap.AccountID != testOmnibusID {
		t.Errorf("AccountID = %q, want %q", snap.AccountID, testOmnibusID)
	}
	if snap.Cash != "100000.00" {
		t.Errorf("Cash = %q, want '100000.00'", snap.Cash)
	}
	if snap.Equity != "500000.00" {
		t.Errorf("Equity = %q, want '500000.00'", snap.Equity)
	}
	if len(snap.Positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(snap.Positions))
	}
	if snap.Positions[0].Symbol != "AAPL" {
		t.Errorf("positions[0].Symbol = %q, want 'AAPL'", snap.Positions[0].Symbol)
	}
}

func TestBankRelationshipOnOmnibus(t *testing.T) {
	var gotPath string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/" + testOmnibusID + "/ach_relationships": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":                 "ach-1",
					"account_id":         testOmnibusID,
					"account_owner_name": "Omnibus LLC",
					"bank_account_type":  "CHECKING",
					"status":             "APPROVED",
				})
			} else {
				json.NewEncoder(w).Encode([]map[string]interface{}{
					{
						"id":                 "ach-1",
						"account_id":         testOmnibusID,
						"account_owner_name": "Omnibus LLC",
						"bank_account_type":  "CHECKING",
						"status":             "APPROVED",
					},
				})
			}
		},
	})

	// Create
	br, err := p.CreateBankRelationship(context.Background(), "sub-1", "Omnibus LLC", "CHECKING", "123456789", "021000021")
	if err != nil {
		t.Fatalf("CreateBankRelationship error: %v", err)
	}
	if br.ProviderID != "ach-1" {
		t.Errorf("ProviderID = %q, want 'ach-1'", br.ProviderID)
	}
	if gotPath != "/v1/accounts/"+testOmnibusID+"/ach_relationships" {
		t.Errorf("path = %s, want omnibus ach path", gotPath)
	}

	// List
	rels, err := p.ListBankRelationships(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("ListBankRelationships error: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}
}

func TestGetSnapshotStock(t *testing.T) {
	_, p := testServer(t, nil)
	testDataServer(t, p, map[string]http.HandlerFunc{
		"/v2/stocks/AAPL/snapshot": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"latestTrade": map[string]interface{}{
					"t": "2026-04-01T10:00:00Z",
					"p": 155.0,
					"s": 100.0,
					"x": "V",
				},
				"latestQuote": map[string]interface{}{
					"t":  "2026-04-01T10:00:00Z",
					"bp": 154.9,
					"bs": 200.0,
					"ap": 155.1,
					"as": 150.0,
				},
			})
		},
	})

	snap, err := p.GetSnapshot(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetSnapshot error: %v", err)
	}
	if snap.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want 'AAPL'", snap.Symbol)
	}
	if snap.LatestTrade == nil || snap.LatestTrade.Price != 155.0 {
		t.Errorf("LatestTrade.Price = %v, want 155.0", snap.LatestTrade)
	}
}

func TestErrorHandling(t *testing.T) {
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

	_, err := p.GetAccount(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "account not found") {
		t.Errorf("error = %q, want to contain 'account not found'", err.Error())
	}
	if !strings.Contains(err.Error(), "alpaca_omnisub") {
		t.Errorf("error = %q, want to contain provider name", err.Error())
	}
}
