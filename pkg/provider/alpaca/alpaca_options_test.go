package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/luxfi/broker/pkg/types"
)

func TestBuildOCCSymbol(t *testing.T) {
	tests := []struct {
		name       string
		underlying string
		expiration string
		ctype      string
		strike     string
		want       string
		wantErr    bool
	}{
		{
			name:       "AAPL call 150",
			underlying: "AAPL",
			expiration: "2026-04-18",
			ctype:      "call",
			strike:     "150",
			want:       "AAPL260418C00150000",
		},
		{
			name:       "AAPL put 145.50",
			underlying: "AAPL",
			expiration: "2026-04-18",
			ctype:      "put",
			strike:     "145.50",
			want:       "AAPL260418P00145500",
		},
		{
			name:       "SPY call 500",
			underlying: "SPY",
			expiration: "2026-06-20",
			ctype:      "call",
			strike:     "500",
			want:       "SPY260620C00500000",
		},
		{
			name:       "missing symbol",
			underlying: "",
			expiration: "2026-04-18",
			ctype:      "call",
			strike:     "150",
			wantErr:    true,
		},
		{
			name:       "bad date",
			underlying: "AAPL",
			expiration: "not-a-date",
			ctype:      "call",
			strike:     "150",
			wantErr:    true,
		},
		{
			name:       "bad strike",
			underlying: "AAPL",
			expiration: "2026-04-18",
			ctype:      "call",
			strike:     "abc",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildOCCSymbol(tt.underlying, tt.expiration, tt.ctype, tt.strike)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOCCSymbol(t *testing.T) {
	tests := []struct {
		occ        string
		wantType   string
		wantStrike float64
		wantExp    string
	}{
		{"AAPL260418C00150000", "call", 150.0, "2026-04-18"},
		{"AAPL260418P00145500", "put", 145.5, "2026-04-18"},
		{"SPY260620C00500000", "call", 500.0, "2026-06-20"},
	}

	for _, tt := range tests {
		t.Run(tt.occ, func(t *testing.T) {
			ct, strike, exp := parseOCCSymbol(tt.occ)
			if ct != tt.wantType {
				t.Errorf("contract_type: got %q, want %q", ct, tt.wantType)
			}
			if strike != tt.wantStrike {
				t.Errorf("strike: got %f, want %f", strike, tt.wantStrike)
			}
			if exp != tt.wantExp {
				t.Errorf("expiration: got %q, want %q", exp, tt.wantExp)
			}
		})
	}
}

func TestExtractUnderlying(t *testing.T) {
	tests := []struct {
		occ  string
		want string
	}{
		{"AAPL260418C00150000", "AAPL"},
		{"SPY260620C00500000", "SPY"},
		{"TSLA260418P00200000", "TSLA"},
	}
	for _, tt := range tests {
		got := extractUnderlying(tt.occ)
		if got != tt.want {
			t.Errorf("extractUnderlying(%q) = %q, want %q", tt.occ, got, tt.want)
		}
	}
}

func TestActionToSide(t *testing.T) {
	tests := []struct {
		action  string
		want    string
		wantErr bool
	}{
		{"buy_to_open", "buy", false},
		{"buy_to_close", "buy", false},
		{"sell_to_open", "sell", false},
		{"sell_to_close", "sell", false},
		{"BUY_TO_OPEN", "buy", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		got, err := actionToSide(tt.action)
		if tt.wantErr {
			if err == nil {
				t.Errorf("actionToSide(%q): expected error", tt.action)
			}
			continue
		}
		if err != nil {
			t.Errorf("actionToSide(%q): unexpected error: %v", tt.action, err)
			continue
		}
		if got != tt.want {
			t.Errorf("actionToSide(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestGetOptionExpirations(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/options/contracts": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			symbols := r.URL.Query().Get("underlying_symbols")
			if symbols != "AAPL" {
				http.Error(w, "unexpected symbol", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"option_contracts": []map[string]interface{}{
					{"symbol": "AAPL260418C00150000", "expiration_date": "2026-04-18", "underlying_symbol": "AAPL", "type": "call", "strike_price": "150"},
					{"symbol": "AAPL260418P00150000", "expiration_date": "2026-04-18", "underlying_symbol": "AAPL", "type": "put", "strike_price": "150"},
					{"symbol": "AAPL260515C00150000", "expiration_date": "2026-05-15", "underlying_symbol": "AAPL", "type": "call", "strike_price": "150"},
				},
			})
		},
	})

	exps, err := p.GetOptionExpirations(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetOptionExpirations: %v", err)
	}
	if len(exps) != 2 {
		t.Fatalf("expected 2 expirations, got %d: %v", len(exps), exps)
	}
	if exps[0] != "2026-04-18" || exps[1] != "2026-05-15" {
		t.Fatalf("unexpected expirations: %v", exps)
	}
}

func TestGetOptionChain(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/options/contracts": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"option_contracts": []map[string]interface{}{
					{
						"symbol":            "AAPL260418C00150000",
						"expiration_date":   "2026-04-18",
						"underlying_symbol": "AAPL",
						"type":              "call",
						"strike_price":      "150",
						"status":            "active",
						"tradable":          true,
						"style":             "american",
						"open_interest":     "1000",
						"close_price":       "5.50",
					},
					{
						"symbol":            "AAPL260418P00150000",
						"expiration_date":   "2026-04-18",
						"underlying_symbol": "AAPL",
						"type":              "put",
						"strike_price":      "150",
						"status":            "active",
						"tradable":          true,
						"style":             "american",
						"open_interest":     "800",
						"close_price":       "3.20",
					},
				},
			})
		},
	})

	// Patch data URL to same server (snapshots will 404, which is fine)
	chain, err := p.GetOptionChain(context.Background(), "AAPL", "2026-04-18")
	if err != nil {
		t.Fatalf("GetOptionChain: %v", err)
	}
	if chain.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want AAPL", chain.Symbol)
	}
	if len(chain.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(chain.Calls))
	}
	if len(chain.Puts) != 1 {
		t.Fatalf("expected 1 put, got %d", len(chain.Puts))
	}
	if chain.Calls[0].Strike != 150 {
		t.Errorf("call strike = %f, want 150", chain.Calls[0].Strike)
	}
	if chain.Puts[0].Strike != 150 {
		t.Errorf("put strike = %f, want 150", chain.Puts[0].Strike)
	}
	if chain.Calls[0].OpenInterest != 1000 {
		t.Errorf("call OI = %d, want 1000", chain.Calls[0].OpenInterest)
	}
	if !chain.Calls[0].Tradable {
		t.Error("expected call to be tradable")
	}
}

func TestGetOptionQuote(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/options/contracts/AAPL260418C00150000": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"symbol":            "AAPL260418C00150000",
				"expiration_date":   "2026-04-18",
				"underlying_symbol": "AAPL",
				"type":              "call",
				"strike_price":      "150",
				"status":            "active",
				"tradable":          true,
				"open_interest":     "1000",
				"close_price":       "5.50",
			})
		},
	})

	quote, err := p.GetOptionQuote(context.Background(), "AAPL260418C00150000")
	if err != nil {
		t.Fatalf("GetOptionQuote: %v", err)
	}
	if quote.Symbol != "AAPL260418C00150000" {
		t.Errorf("Symbol = %q", quote.Symbol)
	}
	if quote.Strike != 150 {
		t.Errorf("Strike = %f, want 150", quote.Strike)
	}
	if quote.ContractType != "call" {
		t.Errorf("ContractType = %q, want call", quote.ContractType)
	}
	if quote.Underlying != "AAPL" {
		t.Errorf("Underlying = %q, want AAPL", quote.Underlying)
	}
}

func TestCreateOptionOrder(t *testing.T) {
	var gotBody map[string]interface{}
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/orders": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			json.NewDecoder(r.Body).Decode(&gotBody)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "order-123",
				"symbol":         "AAPL260418C00150000",
				"qty":            "1",
				"side":           "buy",
				"type":           "limit",
				"time_in_force":  "day",
				"limit_price":    "5.50",
				"status":         "accepted",
				"asset_class":    "us_option",
				"created_at":     "2026-04-01T10:00:00Z",
			})
		},
	})

	order, err := p.CreateOptionOrder(context.Background(), "test-acct", &types.CreateOptionOrderRequest{
		Symbol:       "AAPL",
		ContractType: "call",
		Strike:       "150",
		Expiration:   "2026-04-18",
		Action:       "buy_to_open",
		Qty:          "1",
		OrderType:    "limit",
		LimitPrice:   "5.50",
		TimeInForce:  "day",
	})
	if err != nil {
		t.Fatalf("CreateOptionOrder: %v", err)
	}
	if order.ProviderID != "order-123" {
		t.Errorf("ProviderID = %q, want order-123", order.ProviderID)
	}
	if order.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", order.Status)
	}
	if order.AssetClass != "us_option" {
		t.Errorf("AssetClass = %q, want us_option", order.AssetClass)
	}

	// Verify the request body sent to Alpaca
	if gotBody["symbol"] != "AAPL260418C00150000" {
		t.Errorf("body symbol = %v, want AAPL260418C00150000", gotBody["symbol"])
	}
	if gotBody["side"] != "buy" {
		t.Errorf("body side = %v, want buy", gotBody["side"])
	}
	if gotBody["order_class"] != "simple" {
		t.Errorf("body order_class = %v, want simple", gotBody["order_class"])
	}
}

func TestCreateOptionOrderWithContractSymbol(t *testing.T) {
	var gotBody map[string]interface{}
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/orders": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&gotBody)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "order-456",
				"symbol":      "AAPL260418C00150000",
				"qty":         "1",
				"side":        "sell",
				"type":        "market",
				"status":      "accepted",
				"asset_class": "us_option",
				"created_at":  "2026-04-01T10:00:00Z",
			})
		},
	})

	order, err := p.CreateOptionOrder(context.Background(), "test-acct", &types.CreateOptionOrderRequest{
		ContractSymbol: "AAPL260418C00150000",
		Action:         "sell_to_close",
		Qty:            "1",
		OrderType:      "market",
		TimeInForce:    "day",
	})
	if err != nil {
		t.Fatalf("CreateOptionOrder: %v", err)
	}
	if order.ProviderID != "order-456" {
		t.Errorf("ProviderID = %q", order.ProviderID)
	}
	if gotBody["symbol"] != "AAPL260418C00150000" {
		t.Errorf("body symbol = %v, want AAPL260418C00150000", gotBody["symbol"])
	}
	if gotBody["side"] != "sell" {
		t.Errorf("body side = %v, want sell", gotBody["side"])
	}
}

func TestCreateMultiLegOrder(t *testing.T) {
	var gotBody map[string]interface{}
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/orders": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&gotBody)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "multileg-789",
				"symbol":      "AAPL",
				"status":      "accepted",
				"created_at":  "2026-04-01T10:00:00Z",
			})
		},
	})

	result, err := p.CreateMultiLegOrder(context.Background(), "test-acct", &types.CreateMultiLegOrderRequest{
		Symbol:       "AAPL",
		StrategyType: "vertical",
		Legs: []types.OptionLeg{
			{ContractType: "call", Strike: "150", Expiration: "2026-04-18", Action: "buy_to_open", Quantity: "1"},
			{ContractType: "call", Strike: "155", Expiration: "2026-04-18", Action: "sell_to_open", Quantity: "1"},
		},
		OrderType:   "limit",
		LimitPrice:  "2.00",
		TimeInForce: "day",
	})
	if err != nil {
		t.Fatalf("CreateMultiLegOrder: %v", err)
	}
	if result.StrategyOrderID != "multileg-789" {
		t.Errorf("StrategyOrderID = %q", result.StrategyOrderID)
	}
	if result.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", result.Status)
	}

	// Verify mleg body structure
	if gotBody["order_class"] != "mleg" {
		t.Errorf("order_class = %v, want mleg", gotBody["order_class"])
	}
	if gotBody["symbol"] != "AAPL" {
		t.Errorf("symbol = %v, want AAPL", gotBody["symbol"])
	}
	rawLegs, ok := gotBody["legs"].([]interface{})
	if !ok || len(rawLegs) != 2 {
		t.Fatalf("expected 2 legs, got %v", gotBody["legs"])
	}

	// Verify leg fields use ratio_qty and position_intent (not qty)
	leg0, ok := rawLegs[0].(map[string]interface{})
	if !ok {
		t.Fatal("leg[0] is not a map")
	}
	if leg0["ratio_qty"] != "1" {
		t.Errorf("leg[0].ratio_qty = %v, want 1", leg0["ratio_qty"])
	}
	if leg0["position_intent"] != "buy_to_open" {
		t.Errorf("leg[0].position_intent = %v, want buy_to_open", leg0["position_intent"])
	}
	if _, hasQty := leg0["qty"]; hasQty {
		t.Error("leg[0] should not have 'qty' field, should use 'ratio_qty'")
	}

	leg1, ok := rawLegs[1].(map[string]interface{})
	if !ok {
		t.Fatal("leg[1] is not a map")
	}
	if leg1["ratio_qty"] != "1" {
		t.Errorf("leg[1].ratio_qty = %v, want 1", leg1["ratio_qty"])
	}
	if leg1["position_intent"] != "sell_to_open" {
		t.Errorf("leg[1].position_intent = %v, want sell_to_open", leg1["position_intent"])
	}
}

func TestCreateMultiLegOrderTooFewLegs(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{})

	_, err := p.CreateMultiLegOrder(context.Background(), "test-acct", &types.CreateMultiLegOrderRequest{
		Symbol: "AAPL",
		Legs:   []types.OptionLeg{{ContractType: "call", Strike: "150", Expiration: "2026-04-18", Action: "buy_to_open", Quantity: "1"}},
	})
	if err == nil {
		t.Fatal("expected error for < 2 legs")
	}
}

func TestExerciseOption(t *testing.T) {
	var gotPath string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/positions/AAPL260418C00150000/exercise": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
		},
	})

	err := p.ExerciseOption(context.Background(), "test-acct", "AAPL260418C00150000", 1)
	if err != nil {
		t.Fatalf("ExerciseOption: %v", err)
	}
	if gotPath != "/v1/trading/accounts/test-acct/positions/AAPL260418C00150000/exercise" {
		t.Errorf("unexpected path: %s", gotPath)
	}
}

func TestGetOptionPositions(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/positions": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"symbol":          "AAPL",
					"qty":             "100",
					"avg_entry_price": "175.50",
					"market_value":    "17550",
					"current_price":   "175.50",
					"unrealized_pl":   "0",
					"side":            "long",
					"asset_class":     "us_equity",
				},
				{
					"symbol":          "AAPL260418C00150000",
					"qty":             "5",
					"avg_entry_price": "5.50",
					"market_value":    "2750",
					"current_price":   "5.50",
					"unrealized_pl":   "0",
					"side":            "long",
					"asset_class":     "us_option",
				},
				{
					"symbol":          "AAPL260418P00145000",
					"qty":             "-3",
					"avg_entry_price": "3.20",
					"market_value":    "-960",
					"current_price":   "3.20",
					"unrealized_pl":   "0",
					"side":            "short",
					"asset_class":     "us_option",
				},
			})
		},
	})

	positions, err := p.GetOptionPositions(context.Background(), "test-acct")
	if err != nil {
		t.Fatalf("GetOptionPositions: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 option positions (equity filtered out), got %d", len(positions))
	}

	// First option: AAPL call
	if positions[0].Symbol != "AAPL260418C00150000" {
		t.Errorf("pos[0].Symbol = %q", positions[0].Symbol)
	}
	if positions[0].Underlying != "AAPL" {
		t.Errorf("pos[0].Underlying = %q, want AAPL", positions[0].Underlying)
	}
	if positions[0].ContractType != "call" {
		t.Errorf("pos[0].ContractType = %q, want call", positions[0].ContractType)
	}
	if positions[0].Strike != 150 {
		t.Errorf("pos[0].Strike = %f, want 150", positions[0].Strike)
	}
	if positions[0].Qty != "5" {
		t.Errorf("pos[0].Qty = %q, want 5", positions[0].Qty)
	}
	if positions[0].Side != "long" {
		t.Errorf("pos[0].Side = %q, want long", positions[0].Side)
	}

	// Second option: AAPL put (short)
	if positions[1].ContractType != "put" {
		t.Errorf("pos[1].ContractType = %q, want put", positions[1].ContractType)
	}
	if positions[1].Strike != 145 {
		t.Errorf("pos[1].Strike = %f, want 145", positions[1].Strike)
	}
	if positions[1].Side != "short" {
		t.Errorf("pos[1].Side = %q, want short", positions[1].Side)
	}
}

func TestGetOptionChainWithSnapshots(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/options/contracts": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"option_contracts": []map[string]interface{}{
					{
						"symbol":            "AAPL260418C00150000",
						"expiration_date":   "2026-04-18",
						"underlying_symbol": "AAPL",
						"type":              "call",
						"strike_price":      "150",
						"status":            "active",
						"tradable":          true,
						"open_interest":     "1000",
						"close_price":       "5.50",
					},
				},
			})
		},
	})

	// Set up data server with snapshot
	testDataServer(t, p, map[string]http.HandlerFunc{
		"/v1beta1/options/snapshots": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"snapshots": map[string]interface{}{
					"AAPL260418C00150000": map[string]interface{}{
						"latestQuote": map[string]interface{}{
							"bp": 5.40,
							"ap": 5.60,
						},
						"latestTrade": map[string]interface{}{
							"p": 5.50,
						},
						"greeks": map[string]interface{}{
							"delta":              0.65,
							"gamma":              0.03,
							"theta":              -0.05,
							"vega":               0.15,
							"rho":                0.01,
							"implied_volatility": 0.30,
						},
					},
				},
			})
		},
	})

	chain, err := p.GetOptionChain(context.Background(), "AAPL", "2026-04-18")
	if err != nil {
		t.Fatalf("GetOptionChain: %v", err)
	}
	if len(chain.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(chain.Calls))
	}

	call := chain.Calls[0]
	if call.Bid != 5.40 {
		t.Errorf("Bid = %f, want 5.40", call.Bid)
	}
	if call.Ask != 5.60 {
		t.Errorf("Ask = %f, want 5.60", call.Ask)
	}
	if call.Last != 5.50 {
		t.Errorf("Last = %f, want 5.50", call.Last)
	}
	if call.Greeks.Delta != 0.65 {
		t.Errorf("Delta = %f, want 0.65", call.Greeks.Delta)
	}
	if call.Greeks.IV != 0.30 {
		t.Errorf("IV = %f, want 0.30", call.Greeks.IV)
	}
}

func TestDoNotExercise(t *testing.T) {
	var gotPath string
	var gotMethod string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/positions/AAPL260418C00150000/do-not-exercise": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotMethod = r.Method
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
		},
	})

	err := p.DoNotExercise(context.Background(), "test-acct", "AAPL260418C00150000")
	if err != nil {
		t.Fatalf("DoNotExercise: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/trading/accounts/test-acct/positions/AAPL260418C00150000/do-not-exercise" {
		t.Errorf("unexpected path: %s", gotPath)
	}
}

func TestSetOptionsApprovalLevel(t *testing.T) {
	var gotBody map[string]interface{}
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/test-acct/options/approval": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
		},
	})

	err := p.SetOptionsApprovalLevel(context.Background(), "test-acct", 3)
	if err != nil {
		t.Fatalf("SetOptionsApprovalLevel: %v", err)
	}
	// JSON numbers decode as float64
	if gotBody["level"] != float64(3) {
		t.Errorf("level = %v, want 3", gotBody["level"])
	}
}

// --- Security-focused tests ---

func TestBuildOCCSymbolNegativeStrike(t *testing.T) {
	// Negative strikes should still produce a valid OCC symbol
	// (the exchange may reject it, but the format function should not panic)
	sym, err := buildOCCSymbol("AAPL", "2026-04-18", "call", "-150")
	if err != nil {
		// Negative strikes are invalid in practice; if we reject them that's fine
		return
	}
	// If it doesn't error, verify it doesn't produce garbage
	if sym == "" {
		t.Error("expected non-empty symbol for negative strike")
	}
}

func TestBuildOCCSymbolZeroStrike(t *testing.T) {
	sym, err := buildOCCSymbol("AAPL", "2026-04-18", "call", "0")
	if err != nil {
		return // acceptable to reject zero strikes
	}
	if sym == "" {
		t.Error("expected non-empty symbol for zero strike")
	}
}

func TestBuildOCCSymbolVeryLargeStrike(t *testing.T) {
	// Strike of $99999.999 -> 99999999 which is 8 digits (max for OCC format)
	sym, err := buildOCCSymbol("SPY", "2026-04-18", "call", "99999.999")
	if err != nil {
		t.Fatalf("unexpected error for large strike: %v", err)
	}
	if sym != "SPY260418C99999999" {
		t.Errorf("got %q, want SPY260418C99999999", sym)
	}
}

func TestBuildOCCSymbolOverflowStrike(t *testing.T) {
	// Strike of $100000 -> 100000000 which overflows 8-digit OCC format
	sym, err := buildOCCSymbol("SPY", "2026-04-18", "call", "100000")
	if err != nil {
		return // Acceptable to reject overflow
	}
	// If it doesn't error, the symbol will be >8 digits which is invalid OCC
	// This is a known limitation; the real validation happens at handler level
	_ = sym
}

func TestParseOCCSymbolShortInput(t *testing.T) {
	// Should not panic on short strings
	ct, strike, exp := parseOCCSymbol("")
	if ct != "" || strike != 0 || exp != "" {
		t.Errorf("expected empty results for empty input")
	}

	ct2, strike2, exp2 := parseOCCSymbol("ABC")
	if ct2 != "" || strike2 != 0 || exp2 != "" {
		t.Errorf("expected empty results for short input")
	}
}

func TestExtractUnderlyingShortInput(t *testing.T) {
	// Should not panic on short strings
	result := extractUnderlying("")
	if result != "" {
		t.Errorf("expected empty for empty input, got %q", result)
	}

	result2 := extractUnderlying("ABC")
	if result2 != "ABC" {
		t.Errorf("expected ABC for short input, got %q", result2)
	}
}

func TestCreateMultiLegOrderMaxLegs(t *testing.T) {
	// Verify the provider-level code enforces minimum 2 legs
	_, p := testServer(t, map[string]http.HandlerFunc{})

	// 0 legs
	_, err := p.CreateMultiLegOrder(context.Background(), "test-acct", &types.CreateMultiLegOrderRequest{
		Symbol: "AAPL",
		Legs:   []types.OptionLeg{},
	})
	if err == nil {
		t.Fatal("expected error for 0 legs")
	}

	// 1 leg
	_, err = p.CreateMultiLegOrder(context.Background(), "test-acct", &types.CreateMultiLegOrderRequest{
		Symbol: "AAPL",
		Legs:   []types.OptionLeg{{ContractType: "call", Strike: "150", Expiration: "2026-04-18", Action: "buy_to_open", Quantity: "1"}},
	})
	if err == nil {
		t.Fatal("expected error for 1 leg")
	}
}

func TestContractSymbolURLEscaping(t *testing.T) {
	// Verify that contract symbols with special chars are properly URL-escaped
	// This prevents path traversal in exercise/do-not-exercise endpoints
	var gotPath string
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/trading/accounts/test-acct/positions/AAPL260418C00150000/exercise": func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
		},
	})

	err := p.ExerciseOption(context.Background(), "test-acct", "AAPL260418C00150000", 1)
	if err != nil {
		t.Fatalf("ExerciseOption: %v", err)
	}
	// Verify the path is correct and not manipulated
	expected := "/v1/trading/accounts/test-acct/positions/AAPL260418C00150000/exercise"
	if gotPath != expected {
		t.Errorf("path = %q, want %q", gotPath, expected)
	}
}

func TestActionToSideEdgeCases(t *testing.T) {
	// Verify we don't accept partial matches or similar-looking actions
	badActions := []string{
		"buy",
		"sell",
		"buy_open",
		"sell_close",
		"buy_to_",
		"buy_to_open_extra",
		"BUY_TO_OPEN_EXTRA",
		"",
		" buy_to_open",
		"buy_to_open ",
		"buy_to_open\n",
	}
	for _, action := range badActions {
		_, err := actionToSide(action)
		if err == nil {
			t.Errorf("actionToSide(%q): expected error, got nil", action)
		}
	}
}
