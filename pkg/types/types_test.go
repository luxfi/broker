package types

import (
	"encoding/json"
	"testing"
)

func TestCreateOrderRequestMarshalAllFields(t *testing.T) {
	req := CreateOrderRequest{
		AccountID:     "acct-123",
		Symbol:        "AAPL",
		Qty:           "10",
		Notional:      "1500.00",
		Side:          "buy",
		Type:          "limit",
		TimeInForce:   "gtc",
		LimitPrice:    "150.00",
		StopPrice:     "145.00",
		ClientOrderID: "client-1",
		TrailPrice:    "5.00",
		TrailPercent:  "2.5",
		ExtendedHours: true,
		OrderClass:    "bracket",
		TakeProfit:    &TakeProfit{LimitPrice: "160.00"},
		StopLoss:      &StopLoss{StopPrice: "140.00", LimitPrice: "139.00"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	checks := map[string]string{
		"account_id":      "acct-123",
		"symbol":          "AAPL",
		"qty":             "10",
		"notional":        "1500.00",
		"side":            "buy",
		"type":            "limit",
		"time_in_force":   "gtc",
		"limit_price":     "150.00",
		"stop_price":      "145.00",
		"client_order_id": "client-1",
		"trail_price":     "5.00",
		"trail_percent":   "2.5",
		"order_class":     "bracket",
	}

	for key, want := range checks {
		got, ok := m[key]
		if !ok {
			t.Errorf("missing key %q in JSON", key)
			continue
		}
		if got != want {
			t.Errorf("key %q = %v, want %v", key, got, want)
		}
	}

	// Check extended_hours bool
	if eh, ok := m["extended_hours"]; !ok || eh != true {
		t.Errorf("extended_hours = %v, want true", eh)
	}

	// Check take_profit nested
	tp, ok := m["take_profit"].(map[string]interface{})
	if !ok {
		t.Fatal("take_profit not a map")
	}
	if tp["limit_price"] != "160.00" {
		t.Errorf("take_profit.limit_price = %v, want '160.00'", tp["limit_price"])
	}

	// Check stop_loss nested
	sl, ok := m["stop_loss"].(map[string]interface{})
	if !ok {
		t.Fatal("stop_loss not a map")
	}
	if sl["stop_price"] != "140.00" {
		t.Errorf("stop_loss.stop_price = %v, want '140.00'", sl["stop_price"])
	}
	if sl["limit_price"] != "139.00" {
		t.Errorf("stop_loss.limit_price = %v, want '139.00'", sl["limit_price"])
	}
}

func TestCreateOrderRequestOmitsEmptyFields(t *testing.T) {
	req := CreateOrderRequest{
		Symbol:      "AAPL",
		Side:        "buy",
		Type:        "market",
		TimeInForce: "day",
		Qty:         "10",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	omitKeys := []string{"notional", "limit_price", "stop_price", "client_order_id",
		"trail_price", "trail_percent", "order_class", "take_profit", "stop_loss"}

	for _, key := range omitKeys {
		if _, ok := m[key]; ok {
			t.Errorf("expected key %q to be omitted, but it was present", key)
		}
	}

	// extended_hours defaults to false, omitempty on bool omits false
	if _, ok := m["extended_hours"]; ok {
		t.Error("expected extended_hours to be omitted when false")
	}
}

func TestReplaceOrderRequestOmitsZeroValues(t *testing.T) {
	req := ReplaceOrderRequest{
		TimeInForce: "gtc",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// nil pointers should be omitted
	omitKeys := []string{"qty", "limit_price", "stop_price", "trail_price", "trail_percent", "client_order_id"}
	for _, key := range omitKeys {
		if _, ok := m[key]; ok {
			t.Errorf("expected key %q to be omitted, but it was present", key)
		}
	}

	if m["time_in_force"] != "gtc" {
		t.Errorf("time_in_force = %v, want 'gtc'", m["time_in_force"])
	}
}

func TestReplaceOrderRequestWithPointers(t *testing.T) {
	qty := 50.0
	limit := 155.5
	stop := 140.0
	trail := 3.0
	trailPct := 1.5

	req := ReplaceOrderRequest{
		Qty:           &qty,
		LimitPrice:    &limit,
		StopPrice:     &stop,
		TrailPrice:    &trail,
		TrailPercent:  &trailPct,
		TimeInForce:   "gtc",
		ClientOrderID: "replace-1",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if m["qty"] != 50.0 {
		t.Errorf("qty = %v, want 50", m["qty"])
	}
	if m["limit_price"] != 155.5 {
		t.Errorf("limit_price = %v, want 155.5", m["limit_price"])
	}
	if m["stop_price"] != 140.0 {
		t.Errorf("stop_price = %v, want 140", m["stop_price"])
	}
	if m["trail_price"] != 3.0 {
		t.Errorf("trail_price = %v, want 3", m["trail_price"])
	}
	if m["trail_percent"] != 1.5 {
		t.Errorf("trail_percent = %v, want 1.5", m["trail_percent"])
	}
	if m["client_order_id"] != "replace-1" {
		t.Errorf("client_order_id = %v, want 'replace-1'", m["client_order_id"])
	}
}

func TestAccountDisclosuresDefaults(t *testing.T) {
	// All fields are *bool — default (nil) should be omitted
	d := AccountDisclosures{}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// All fields should be omitted since they're nil pointers with omitempty
	if len(m) != 0 {
		t.Errorf("expected empty map for default disclosures, got %v", m)
	}
}

func TestAccountDisclosuresWithValues(t *testing.T) {
	tr := true
	fa := false
	d := AccountDisclosures{
		IsControlPerson:           &tr,
		IsAffiliatedExchangeFinra: &fa,
		IsPoliticallyExposed:      &fa,
		ImmediateFamilyExposed:    &tr,
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if m["is_control_person"] != true {
		t.Errorf("is_control_person = %v, want true", m["is_control_person"])
	}
	if m["is_affiliated_exchange_or_finra"] != false {
		t.Errorf("is_affiliated_exchange_or_finra = %v, want false", m["is_affiliated_exchange_or_finra"])
	}
	if m["is_politically_exposed"] != false {
		t.Errorf("is_politically_exposed = %v, want false", m["is_politically_exposed"])
	}
	if m["immediate_family_exposed"] != true {
		t.Errorf("immediate_family_exposed = %v, want true", m["immediate_family_exposed"])
	}
}

func TestBarJSONRoundTrip(t *testing.T) {
	bar := Bar{
		Timestamp:  "2024-01-15T10:30:00Z",
		Open:       150.0,
		High:       155.5,
		Low:        149.0,
		Close:      153.2,
		Volume:     1000000,
		VWAP:       152.1,
		TradeCount: 5000,
	}

	data, err := json.Marshal(bar)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Bar
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Timestamp != bar.Timestamp {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, bar.Timestamp)
	}
	if decoded.Close != bar.Close {
		t.Errorf("Close = %v, want %v", decoded.Close, bar.Close)
	}
	if decoded.VWAP != bar.VWAP {
		t.Errorf("VWAP = %v, want %v", decoded.VWAP, bar.VWAP)
	}
}

func TestPositionJSONRoundTrip(t *testing.T) {
	pos := Position{
		Symbol:        "AAPL",
		Qty:           "100",
		AvgEntryPrice: "150.00",
		MarketValue:   "15500.00",
		CurrentPrice:  "155.00",
		UnrealizedPL:  "500.00",
		Side:          "long",
		AssetClass:    "us_equity",
	}

	data, err := json.Marshal(pos)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Position
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Symbol != pos.Symbol {
		t.Errorf("Symbol = %v, want %v", decoded.Symbol, pos.Symbol)
	}
	if decoded.UnrealizedPL != pos.UnrealizedPL {
		t.Errorf("UnrealizedPL = %v, want %v", decoded.UnrealizedPL, pos.UnrealizedPL)
	}
}

func TestListOrdersParamsOmitEmpty(t *testing.T) {
	params := ListOrdersParams{
		Status: "open",
		Limit:  50,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if m["status"] != "open" {
		t.Errorf("status = %v, want 'open'", m["status"])
	}

	// These should be omitted
	for _, key := range []string{"after", "until", "direction"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected %q to be omitted", key)
		}
	}
}
