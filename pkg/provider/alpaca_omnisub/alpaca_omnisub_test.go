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

func TestListAssets_IncludesFixedIncome(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/assets": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "eq-1", "symbol": "AAPL", "name": "Apple Inc", "class": "us_equity", "status": "active", "tradable": true},
			})
		},
		"/v1/assets/fixed_income/us_treasuries": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"us_treasuries": []map[string]interface{}{
					{"cusip": "912797QD2", "isin": "US912797QD26", "tradable": true, "bond_status": "outstanding", "subtype": "bill", "coupon_rate": "0", "maturity_date": "2026-07-09"},
					{"cusip": "912797XX9", "isin": "US912797XX99", "tradable": false, "bond_status": "matured", "subtype": "note", "coupon_rate": "2.5", "maturity_date": "2024-01-01"},
				},
			})
		},
		"/v1/assets/fixed_income/us_corporates": func(w http.ResponseWriter, r *http.Request) {
			// Simulate 403 — not subscribed
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"not subscribed"}`))
		},
	})

	// Fetch all: should include equity + FI (corporates 403 is silently skipped)
	assets, err := p.ListAssets(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAssets error: %v", err)
	}
	// 1 equity + 1 tradable treasury (matured one has Tradable=false from bond_status != outstanding)
	var equities, fi int
	for _, a := range assets {
		switch a.Class {
		case "us_equity":
			equities++
		case "fixed_income":
			fi++
		}
	}
	if equities != 1 {
		t.Errorf("equities = %d, want 1", equities)
	}
	if fi != 2 {
		t.Errorf("fixed_income assets = %d, want 2 (both returned, tradable flag varies)", fi)
	}

	// Verify the outstanding treasury is tradable, the matured one is not
	for _, a := range assets {
		if a.Symbol == "912797QD2" {
			if !a.Tradable {
				t.Error("912797QD2 should be tradable (outstanding + tradable=true)")
			}
			if a.Name != "US Treasury bill" {
				t.Errorf("name = %q, want 'US Treasury bill'", a.Name)
			}
			// Verify FI metadata fields are populated
			if a.CUSIP != "912797QD2" {
				t.Errorf("CUSIP = %q, want '912797QD2'", a.CUSIP)
			}
			if a.ISIN != "US912797QD26" {
				t.Errorf("ISIN = %q, want 'US912797QD26'", a.ISIN)
			}
			if a.Subtype != "bill" {
				t.Errorf("Subtype = %q, want 'bill'", a.Subtype)
			}
			if a.MaturityDate != "2026-07-09" {
				t.Errorf("MaturityDate = %q, want '2026-07-09'", a.MaturityDate)
			}
			if a.CouponRate != "0" {
				t.Errorf("CouponRate = %q, want '0'", a.CouponRate)
			}
		}
		if a.Symbol == "912797XX9" {
			if a.Tradable {
				t.Error("912797XX9 should not be tradable (matured)")
			}
		}
	}
}

func TestListAssets_FIDescriptionParsed(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/assets/fixed_income/us_treasuries": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"us_treasuries": []map[string]interface{}{
					{
						"cusip":            "912540ZA2",
						"isin":             "US912540ZA28",
						"tradable":         true,
						"bond_status":      "outstanding",
						"subtype":          "bond",
						"description":      "United States Treasury 0.0%, 01/01/2056",
						"description_short": "UST 0.0% 01/01/2056",
						"coupon":           0,
						"coupon_type":      "zero",
						"coupon_frequency": "zero",
						"maturity_date":    "2056-01-01",
						"issue_date":       "2026-01-01",
					},
				},
			})
		},
		"/v1/assets/fixed_income/us_corporates": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"us_corporates": []map[string]interface{}{
					{
						"cusip":            "001055AQ6",
						"isin":             "US001055AQ60",
						"ticker":           "AFL",
						"tradable":         true,
						"bond_status":      "outstanding",
						"subtype":          "senior",
						"description":      "Aflac 3.6%, 04/01/2030",
						"coupon_rate":      "3.6",
						"coupon_type":      "fixed",
						"coupon_frequency": "semi_annual",
						"maturity_date":    "2030-04-01",
						"issue_date":       "2020-03-17",
					},
				},
			})
		},
	})

	assets, err := p.ListAssets(context.Background(), "fixed_income")
	if err != nil {
		t.Fatalf("ListAssets error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 FI assets, got %d", len(assets))
	}

	for _, a := range assets {
		switch a.CUSIP {
		case "912540ZA2":
			if a.Name != "United States Treasury 0.0%, 01/01/2056" {
				t.Errorf("treasury name = %q, want description", a.Name)
			}
			if a.CouponType != "zero" {
				t.Errorf("CouponType = %q, want 'zero'", a.CouponType)
			}
			if a.CouponFrequency != "zero" {
				t.Errorf("CouponFrequency = %q, want 'zero'", a.CouponFrequency)
			}
			if a.IssueDate != "2026-01-01" {
				t.Errorf("IssueDate = %q, want '2026-01-01'", a.IssueDate)
			}
			if a.CouponRate != "0" {
				t.Errorf("CouponRate = %q, want '0'", a.CouponRate)
			}
		case "001055AQ6":
			if a.Name != "Aflac 3.6%, 04/01/2030" {
				t.Errorf("corporate name = %q, want description", a.Name)
			}
			if a.Ticker != "AFL" {
				t.Errorf("Ticker = %q, want 'AFL'", a.Ticker)
			}
			if a.CouponRate != "3.6" {
				t.Errorf("CouponRate = %q, want '3.6'", a.CouponRate)
			}
			if a.CouponType != "fixed" {
				t.Errorf("CouponType = %q, want 'fixed'", a.CouponType)
			}
			if a.CouponFrequency != "semi_annual" {
				t.Errorf("CouponFrequency = %q, want 'semi_annual'", a.CouponFrequency)
			}
		}
	}
}

func TestListAssets_FixedIncomeOnly(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/assets/fixed_income/us_treasuries": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"us_treasuries": []map[string]interface{}{
					{"cusip": "912797QD2", "tradable": true, "bond_status": "outstanding", "subtype": "bill"},
				},
			})
		},
		"/v1/assets/fixed_income/us_corporates": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"us_corporates": []map[string]interface{}{
					{"cusip": "037833100", "tradable": true, "bond_status": "outstanding", "subtype": "senior"},
				},
			})
		},
	})

	// class=fixed_income should NOT call the standard /v1/assets endpoint
	assets, err := p.ListAssets(context.Background(), "fixed_income")
	if err != nil {
		t.Fatalf("ListAssets error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 FI assets, got %d", len(assets))
	}
	for _, a := range assets {
		if a.Class != "fixed_income" {
			t.Errorf("asset %s class = %q, want 'fixed_income'", a.Symbol, a.Class)
		}
	}
	// Check corporate naming
	for _, a := range assets {
		if a.Symbol == "037833100" && a.Name != "US Corporate senior" {
			t.Errorf("corporate name = %q, want 'US Corporate senior'", a.Name)
		}
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
