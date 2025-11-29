package risk

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultLimits(t *testing.T) {
	dl := DefaultLimits()
	if dl.MaxOrderValue != 1_000_000 {
		t.Fatalf("MaxOrderValue = %f, want 1000000", dl.MaxOrderValue)
	}
	if dl.MaxDailyVolume != 10_000_000 {
		t.Fatalf("MaxDailyVolume = %f, want 10000000", dl.MaxDailyVolume)
	}
	if dl.MaxOpenOrders != 100 {
		t.Fatalf("MaxOpenOrders = %d, want 100", dl.MaxOpenOrders)
	}
	if dl.MaxPositionValue != 5_000_000 {
		t.Fatalf("MaxPositionValue = %f, want 5000000", dl.MaxPositionValue)
	}
	if dl.RateLimitPerMin != 60 {
		t.Fatalf("RateLimitPerMin = %d, want 60", dl.RateLimitPerMin)
	}
}

func TestNewEngine(t *testing.T) {
	gl := DefaultLimits()
	e := NewEngine(gl)
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.global.MaxOrderValue != gl.MaxOrderValue {
		t.Fatalf("engine global.MaxOrderValue = %f, want %f", e.global.MaxOrderValue, gl.MaxOrderValue)
	}
}

func TestPreTradeCheckNormalOrder(t *testing.T) {
	e := NewEngine(DefaultLimits())
	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Side:      "buy",
		Qty:       "10",
		Price:     "150",
		OrderType: "limit",
	})
	if !result.Allowed {
		t.Fatalf("expected allowed, got errors: %v", result.Errors)
	}
}

func TestPreTradeCheckExceedsMaxOrderValue(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxOrderValue = 10_000 // $10k limit
	e := NewEngine(limits)

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "100",
		Price:     "200", // $20k order
		OrderType: "limit",
	})
	if result.Allowed {
		t.Fatal("expected rejected for exceeding max order value")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "exceeds limit") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'exceeds limit' error, got: %v", result.Errors)
	}
}

func TestPreTradeCheckDailyVolumeExceeded(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDailyVolume = 5_000
	limits.MaxOrderValue = 100_000
	e := NewEngine(limits)

	// Pre-fill usage with volume close to limit
	e.RecordOrder("alpaca", "acct1", 4_500)

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "10",
		Price:     "100", // $1000 order, would bring total to $5500
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for daily volume exceeded")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "daily volume") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected daily volume error, got: %v", result.Errors)
	}
}

func TestPreTradeCheckRateLimited(t *testing.T) {
	limits := DefaultLimits()
	limits.RateLimitPerMin = 3
	e := NewEngine(limits)

	// Place 3 orders in the last minute
	key := "alpaca/acct1"
	e.mu.Lock()
	e.usage[key] = &AccountUsage{
		LastReset:       time.Now(),
		OrderTimestamps: []time.Time{time.Now(), time.Now(), time.Now()},
	}
	e.mu.Unlock()

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "1",
		Price:     "100",
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for rate limit")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "rate limit") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rate limit error, got: %v", result.Errors)
	}
}

func TestPreTradeCheckBlockedSymbol(t *testing.T) {
	limits := DefaultLimits()
	limits.BlockedSymbols = []string{"GME", "AMC"}
	e := NewEngine(limits)

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "GME",
		Qty:       "1",
		Price:     "20",
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for blocked symbol")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "globally blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocked symbol error, got: %v", result.Errors)
	}
}

func TestPreTradeCheckAccountBlockedSymbol(t *testing.T) {
	e := NewEngine(DefaultLimits())
	e.SetAccountLimits("alpaca", "acct1", AccountLimits{
		BlockedSymbols: []string{"TSLA"},
	})

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "TSLA",
		Qty:       "1",
		Price:     "100",
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for account blocked symbol")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected blocked error, got: %v", result.Errors)
	}
}

func TestPreTradeCheckAllowedSymbolsRestriction(t *testing.T) {
	e := NewEngine(DefaultLimits())
	e.SetAccountLimits("alpaca", "acct1", AccountLimits{
		AllowedSymbols: []string{"AAPL", "GOOG"},
	})

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "MSFT",
		Qty:       "1",
		Price:     "100",
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for symbol not in allowed list")
	}
}

func TestPreTradeCheckProviderAllowlist(t *testing.T) {
	limits := DefaultLimits()
	limits.AllowedProviders = []string{"alpaca"}
	e := NewEngine(limits)

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "ibkr",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "1",
		Price:     "100",
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for provider not in allowlist")
	}
}

func TestPreTradeCheckOpenOrdersLimit(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxOpenOrders = 2
	e := NewEngine(limits)

	// Record 2 open orders
	e.RecordOrder("alpaca", "acct1", 100)
	e.RecordOrder("alpaca", "acct1", 100)

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "1",
		Price:     "100",
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected for max open orders")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "open orders") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected open orders error, got: %v", result.Errors)
	}
}

func TestRecordOrderUpdatesDailyVolume(t *testing.T) {
	e := NewEngine(DefaultLimits())
	e.RecordOrder("alpaca", "acct1", 5000)

	e.mu.RLock()
	usage := e.usage["alpaca/acct1"]
	e.mu.RUnlock()

	if usage == nil {
		t.Fatal("expected usage to be created")
	}
	if usage.DailyVolume != 5000 {
		t.Fatalf("DailyVolume = %f, want 5000", usage.DailyVolume)
	}
	if usage.OpenOrders != 1 {
		t.Fatalf("OpenOrders = %d, want 1", usage.OpenOrders)
	}
	if len(usage.OrderTimestamps) != 1 {
		t.Fatalf("OrderTimestamps len = %d, want 1", len(usage.OrderTimestamps))
	}
}

func TestRecordOrderAccumulates(t *testing.T) {
	e := NewEngine(DefaultLimits())
	e.RecordOrder("alpaca", "acct1", 1000)
	e.RecordOrder("alpaca", "acct1", 2000)

	e.mu.RLock()
	usage := e.usage["alpaca/acct1"]
	e.mu.RUnlock()

	if usage.DailyVolume != 3000 {
		t.Fatalf("DailyVolume = %f, want 3000", usage.DailyVolume)
	}
	if usage.OpenOrders != 2 {
		t.Fatalf("OpenOrders = %d, want 2", usage.OpenOrders)
	}
}

func TestRecordFillDecrementsOpenOrders(t *testing.T) {
	e := NewEngine(DefaultLimits())
	e.RecordOrder("alpaca", "acct1", 1000)
	e.RecordOrder("alpaca", "acct1", 2000)

	e.mu.RLock()
	before := e.usage["alpaca/acct1"].OpenOrders
	e.mu.RUnlock()

	if before != 2 {
		t.Fatalf("OpenOrders before fill = %d, want 2", before)
	}

	e.RecordFill("alpaca", "acct1")

	e.mu.RLock()
	after := e.usage["alpaca/acct1"].OpenOrders
	e.mu.RUnlock()

	if after != 1 {
		t.Fatalf("OpenOrders after fill = %d, want 1", after)
	}
}

func TestRecordFillDoesNotGoBelowZero(t *testing.T) {
	e := NewEngine(DefaultLimits())
	// Record one order, fill twice
	e.RecordOrder("alpaca", "acct1", 100)
	e.RecordFill("alpaca", "acct1")
	e.RecordFill("alpaca", "acct1")

	e.mu.RLock()
	open := e.usage["alpaca/acct1"].OpenOrders
	e.mu.RUnlock()

	if open != 0 {
		t.Fatalf("OpenOrders = %d, want 0 (should not go negative)", open)
	}
}

func TestRecordFillUnknownAccountNoPanic(t *testing.T) {
	e := NewEngine(DefaultLimits())
	// Should not panic on unknown account
	e.RecordFill("alpaca", "nonexistent")
}

func TestEstimateOrderValue(t *testing.T) {
	tests := []struct {
		name  string
		qty   string
		price string
		want  float64
	}{
		{"normal", "10", "150.50", 1505.0},
		{"zero price uses 1", "100", "0", 100.0},
		{"negative qty abs", "-5", "200", 1000.0},
		{"empty strings", "", "", 0.0},
		{"fractional", "0.5", "60000", 30000.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateOrderValue(tt.qty, tt.price)
			if got != tt.want {
				t.Fatalf("estimateOrderValue(%q, %q) = %f, want %f", tt.qty, tt.price, got, tt.want)
			}
		})
	}
}

func TestPreTradeCheckLargeOrderWarning(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxOrderValue = 1000
	e := NewEngine(limits)

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "6",
		Price:     "100", // $600, >50% of $1000
		OrderType: "market",
	})
	if !result.Allowed {
		t.Fatalf("expected allowed, got errors: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings for large order, got none")
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "large order") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'large order' warning, got: %v", result.Warnings)
	}
}

func TestPreTradeCheckDailyResetAfter24Hours(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDailyVolume = 1_000
	limits.MaxOrderValue = 100_000
	e := NewEngine(limits)

	// Set usage from >24h ago
	key := "alpaca/acct1"
	e.mu.Lock()
	e.usage[key] = &AccountUsage{
		DailyVolume: 999,
		LastReset:   time.Now().Add(-25 * time.Hour),
	}
	e.mu.Unlock()

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "5",
		Price:     "100", // $500, under $1000 limit
		OrderType: "market",
	})
	if !result.Allowed {
		t.Fatalf("expected allowed after daily reset, got errors: %v", result.Errors)
	}
}

func TestSetAccountLimitsOverridesGlobal(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxOrderValue = 100_000
	e := NewEngine(limits)

	// Set tighter account limits
	e.SetAccountLimits("alpaca", "acct1", AccountLimits{
		MaxOrderValue: 500, // $500 limit
	})

	result := e.PreTradeCheck(CheckRequest{
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       "10",
		Price:     "100", // $1000, exceeds $500 account limit
		OrderType: "market",
	})
	if result.Allowed {
		t.Fatal("expected rejected by account-level limit")
	}
}
