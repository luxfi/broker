package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/luxfi/broker/pkg/types"
)

func TestListCorporateBonds(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"GET /v1/assets/fixed_income/us_corporates": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":           "bond-001",
					"cusip":        "037833100",
					"symbol":       "AAPL4.5-2030",
					"name":         "Apple Inc 4.5% 2030",
					"status":       "active",
					"tradable":     true,
					"fractionable": false,
				},
				{
					"id":           "bond-002",
					"cusip":        "594918104",
					"name":         "Microsoft Corp 3.0% 2028",
					"status":       "active",
					"tradable":     true,
					"fractionable": false,
				},
			})
		},
	})

	assets, err := p.ListCorporateBonds(context.Background())
	if err != nil {
		t.Fatalf("ListCorporateBonds: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}

	// First asset has explicit symbol
	if assets[0].Symbol != "AAPL4.5-2030" {
		t.Errorf("asset[0].Symbol = %q, want AAPL4.5-2030", assets[0].Symbol)
	}
	if assets[0].Class != "fixed_income" {
		t.Errorf("asset[0].Class = %q, want fixed_income", assets[0].Class)
	}
	if !assets[0].Tradable {
		t.Error("asset[0].Tradable = false, want true")
	}

	// Second asset falls back to CUSIP as symbol
	if assets[1].Symbol != "594918104" {
		t.Errorf("asset[1].Symbol = %q, want 594918104 (CUSIP fallback)", assets[1].Symbol)
	}
}

func TestListTreasuryBonds(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"GET /v1/assets/fixed_income/us_treasuries": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":       "treas-001",
					"cusip":    "912828ZT6",
					"name":     "US Treasury 2.5% 2027",
					"status":   "active",
					"tradable": true,
				},
			})
		},
	})

	assets, err := p.ListTreasuryBonds(context.Background())
	if err != nil {
		t.Fatalf("ListTreasuryBonds: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if assets[0].Symbol != "912828ZT6" {
		t.Errorf("symbol = %q, want 912828ZT6", assets[0].Symbol)
	}
	if assets[0].Class != "fixed_income" {
		t.Errorf("class = %q, want fixed_income", assets[0].Class)
	}
}

func TestCreateFixedIncomeOrder_Buy(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"POST /v1/trading/accounts/acc-1/orders": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)

			if body["type"] != "market" {
				t.Errorf("order type = %v, want market", body["type"])
			}
			if body["time_in_force"] != "day" {
				t.Errorf("TIF = %v, want day", body["time_in_force"])
			}
			if _, ok := body["extended_hours"]; ok {
				t.Error("extended_hours should not be set for buy orders")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "order-fi-1",
				"symbol":         "037833100",
				"side":           "buy",
				"type":           "market",
				"time_in_force":  "day",
				"status":         "accepted",
				"qty":            "5000",
				"asset_class":    "us_equity",
				"created_at":     "2026-04-05T10:00:00Z",
			})
		},
	})

	order, err := p.CreateFixedIncomeOrder(context.Background(), "acc-1", &types.CreateOrderRequest{
		Symbol:      "037833100",
		Side:        "buy",
		Type:        "market",
		TimeInForce: "day",
		Qty:         "5000",
	})
	if err != nil {
		t.Fatalf("CreateFixedIncomeOrder: %v", err)
	}
	if order.AssetClass != "fixed_income" {
		t.Errorf("AssetClass = %q, want fixed_income", order.AssetClass)
	}
	if order.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", order.Status)
	}
}

func TestCreateFixedIncomeOrder_SellExtendedHours(t *testing.T) {
	var gotExtended bool
	_, p := testServer(t, map[string]http.HandlerFunc{
		"POST /v1/trading/accounts/acc-1/orders": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)

			if eh, ok := body["extended_hours"]; ok && eh == true {
				gotExtended = true
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "order-fi-2",
				"symbol":        "037833100",
				"side":          "sell",
				"type":          "market",
				"time_in_force": "day",
				"status":        "pending_new",
				"qty":           "2000",
				"created_at":    "2026-04-05T20:00:00Z",
			})
		},
	})

	order, err := p.CreateFixedIncomeOrder(context.Background(), "acc-1", &types.CreateOrderRequest{
		Symbol:        "037833100",
		Side:          "sell",
		Qty:           "2000",
		ExtendedHours: true,
	})
	if err != nil {
		t.Fatalf("CreateFixedIncomeOrder sell: %v", err)
	}
	if !gotExtended {
		t.Error("extended_hours not sent for sell order with ExtendedHours=true")
	}
	if order.Status != "pending_new" {
		t.Errorf("Status = %q, want pending_new (out-of-hours)", order.Status)
	}
}

func TestCreateFixedIncomeOrder_ValidationErrors(t *testing.T) {
	p := New(Config{BaseURL: "http://localhost:1", APIKey: "k", APISecret: "s"})

	tests := []struct {
		name string
		req  *types.CreateOrderRequest
	}{
		{"missing symbol", &types.CreateOrderRequest{Side: "buy", Qty: "1000"}},
		{"bad side", &types.CreateOrderRequest{Symbol: "X", Side: "short", Qty: "1000"}},
		{"non-market type", &types.CreateOrderRequest{Symbol: "X", Side: "buy", Type: "limit", Qty: "1000"}},
		{"non-day TIF", &types.CreateOrderRequest{Symbol: "X", Side: "buy", TimeInForce: "gtc", Qty: "1000"}},
		{"notional too many decimals", &types.CreateOrderRequest{Symbol: "X", Side: "buy", Notional: "100.123"}},
		{"qty too many decimals", &types.CreateOrderRequest{Symbol: "X", Side: "buy", Qty: "1000.1234567890"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.CreateFixedIncomeOrder(context.Background(), "acc-1", tt.req)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestGetFixedIncomeActivities_MaturityFilter(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"GET /v1/accounts/acc-1/activities": func(w http.ResponseWriter, r *http.Request) {
			got := r.URL.Query().Get("activity_type")
			if got != "maturity" {
				t.Errorf("activity_type filter = %q, want maturity", got)
			}
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":            "act-1",
					"account_id":    "acc-1",
					"activity_type": "maturity",
					"symbol":        "037833100",
					"net_amount":    "5000.00",
					"date":          "2026-04-01",
				},
			})
		},
	})

	activities, err := p.GetFixedIncomeActivities(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("GetFixedIncomeActivities: %v", err)
	}
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity, got %d", len(activities))
	}
	if activities[0].ActivityType != "maturity" {
		t.Errorf("ActivityType = %q, want maturity", activities[0].ActivityType)
	}
}

func TestGetFixedIncomePositions_FiltersFixedIncome(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"GET /v1/trading/accounts/acc-1/positions": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"symbol":          "AAPL",
					"qty":             "10",
					"avg_entry_price": "150.00",
					"market_value":    "1500.00",
					"current_price":   "155.00",
					"unrealized_pl":   "50.00",
					"side":            "long",
					"asset_class":     "us_equity",
				},
				{
					"symbol":          "037833100",
					"qty":             "5000.000000000",
					"avg_entry_price": "98.50",
					"market_value":    "4925.00",
					"current_price":   "99.00",
					"unrealized_pl":   "25.00",
					"side":            "long",
					"asset_class":     "fixed_income",
				},
			})
		},
	})

	positions, err := p.GetFixedIncomePositions(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("GetFixedIncomePositions: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 FI position (filtered), got %d", len(positions))
	}
	if positions[0].Symbol != "037833100" {
		t.Errorf("Symbol = %q, want 037833100", positions[0].Symbol)
	}
	if positions[0].AssetClass != "fixed_income" {
		t.Errorf("AssetClass = %q, want fixed_income", positions[0].AssetClass)
	}
	// qty is par value ($1000/bond), displayed with 9dp
	if positions[0].Qty != "5000.000000000" {
		t.Errorf("Qty = %q, want 5000.000000000 (par value, 9dp)", positions[0].Qty)
	}
}

func TestCloseFixedIncomePosition(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/trading/accounts/acc-1/positions/037833100": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "order-close-1",
				"symbol":        "037833100",
				"side":          "sell",
				"type":          "market",
				"time_in_force": "day",
				"status":        "accepted",
				"created_at":    "2026-04-05T10:00:00Z",
			})
		},
	})

	order, err := p.CloseFixedIncomePosition(context.Background(), "acc-1", "037833100", nil)
	if err != nil {
		t.Fatalf("CloseFixedIncomePosition: %v", err)
	}
	if order.AssetClass != "fixed_income" {
		t.Errorf("AssetClass = %q, want fixed_income", order.AssetClass)
	}
	if order.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", order.Status)
	}
}

func TestGetFixedIncomeOrder(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"GET /v1/trading/accounts/acc-1/orders/order-fi-1": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "order-fi-1",
				"symbol":        "037833100",
				"side":          "buy",
				"type":          "market",
				"time_in_force": "day",
				"status":        "filled",
				"filled_qty":    "5000",
				"qty":           "5000",
				"created_at":    "2026-04-05T10:00:00Z",
				"filled_at":     "2026-04-05T10:00:01Z",
			})
		},
	})

	order, err := p.GetFixedIncomeOrder(context.Background(), "acc-1", "order-fi-1")
	if err != nil {
		t.Fatalf("GetFixedIncomeOrder: %v", err)
	}
	if order.AssetClass != "fixed_income" {
		t.Errorf("AssetClass = %q, want fixed_income", order.AssetClass)
	}
	if order.Status != "filled" {
		t.Errorf("Status = %q, want filled", order.Status)
	}
}

func TestCancelFixedIncomeOrder(t *testing.T) {
	var called bool
	_, p := testServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/trading/accounts/acc-1/orders/order-fi-1": func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		},
	})

	err := p.CancelFixedIncomeOrder(context.Background(), "acc-1", "order-fi-1")
	if err != nil {
		t.Fatalf("CancelFixedIncomeOrder: %v", err)
	}
	if !called {
		t.Error("cancel endpoint was not called")
	}
}
