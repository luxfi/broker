package audit

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestNewLog(t *testing.T) {
	l := NewLog()
	if l == nil {
		t.Fatal("NewLog returned nil")
	}
	if l.entries == nil {
		t.Fatal("entries slice not initialized")
	}
	if len(l.entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(l.entries))
	}
}

func TestRecordSingleEntry(t *testing.T) {
	l := NewLog()
	l.Record(Entry{
		Action:    ActionOrderCreate,
		Provider:  "alpaca",
		AccountID: "acct1",
		Symbol:    "AAPL",
		Status:    "success",
	})

	entries := l.Query(Filter{})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Action != ActionOrderCreate {
		t.Fatalf("action = %q, want %q", e.Action, ActionOrderCreate)
	}
	if e.Provider != "alpaca" {
		t.Fatalf("provider = %q, want %q", e.Provider, "alpaca")
	}
	if e.ID == "" {
		t.Fatal("entry ID should be auto-generated")
	}
	if e.Timestamp.IsZero() {
		t.Fatal("entry timestamp should be auto-set")
	}
}

func TestRecordMultipleEntries(t *testing.T) {
	l := NewLog()
	for i := 0; i < 5; i++ {
		l.Record(Entry{
			Action:   ActionOrderCreate,
			Provider: "alpaca",
			Status:   "success",
		})
	}

	entries := l.Query(Filter{})
	if len(entries) != 5 {
		t.Fatalf("entries len = %d, want 5", len(entries))
	}
}

func TestRecordPreservesExplicitTimestampAndID(t *testing.T) {
	l := NewLog()
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	l.Record(Entry{
		ID:        "custom-id",
		Timestamp: ts,
		Action:    ActionAuth,
		Status:    "success",
	})

	entries := l.Query(Filter{})
	if entries[0].ID != "custom-id" {
		t.Fatalf("ID = %q, want %q", entries[0].ID, "custom-id")
	}
	if !entries[0].Timestamp.Equal(ts) {
		t.Fatalf("Timestamp = %v, want %v", entries[0].Timestamp, ts)
	}
}

func TestRecordOrder(t *testing.T) {
	l := NewLog()
	l.RecordOrder(nil, ActionOrderCreate, "alpaca", "acct1", "AAPL", "buy", "10", "ord1", "smart_route", "success", 5*time.Millisecond, nil)

	entries := l.Query(Filter{})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Action != ActionOrderCreate {
		t.Fatalf("action = %q, want %q", e.Action, ActionOrderCreate)
	}
	if e.Symbol != "AAPL" {
		t.Fatalf("symbol = %q, want AAPL", e.Symbol)
	}
	if e.Side != "buy" {
		t.Fatalf("side = %q, want buy", e.Side)
	}
	if e.OrderID != "ord1" {
		t.Fatalf("orderID = %q, want ord1", e.OrderID)
	}
	if e.Status != "success" {
		t.Fatalf("status = %q, want success", e.Status)
	}
	if e.Latency != 5*time.Millisecond {
		t.Fatalf("latency = %v, want 5ms", e.Latency)
	}
}

func TestRecordOrderWithError(t *testing.T) {
	l := NewLog()
	l.RecordOrder(nil, ActionOrderCreate, "alpaca", "acct1", "AAPL", "buy", "10", "", "", "success", 0, &testError{"connection timeout"})

	entries := l.Query(Filter{})
	e := entries[0]
	if e.Status != "failure" {
		t.Fatalf("status = %q, want failure", e.Status)
	}
	if e.Error != "connection timeout" {
		t.Fatalf("error = %q, want %q", e.Error, "connection timeout")
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestQueryNoFilter(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Action: ActionOrderCreate, Provider: "alpaca", Status: "success"})
	l.Record(Entry{Action: ActionOrderCancel, Provider: "ibkr", Status: "success"})
	l.Record(Entry{Action: ActionTransfer, Provider: "alpaca", Status: "success"})

	entries := l.Query(Filter{})
	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3", len(entries))
	}
}

func TestQueryByAction(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Action: ActionOrderCreate, Status: "success"})
	l.Record(Entry{Action: ActionOrderCancel, Status: "success"})
	l.Record(Entry{Action: ActionOrderCreate, Status: "success"})

	entries := l.Query(Filter{Action: ActionOrderCreate})
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
}

func TestQueryByProvider(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Provider: "alpaca", Status: "success"})
	l.Record(Entry{Provider: "ibkr", Status: "success"})
	l.Record(Entry{Provider: "alpaca", Status: "success"})

	entries := l.Query(Filter{Provider: "ibkr"})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
}

func TestQueryByAccountID(t *testing.T) {
	l := NewLog()
	l.Record(Entry{AccountID: "acct1", Status: "success"})
	l.Record(Entry{AccountID: "acct2", Status: "success"})
	l.Record(Entry{AccountID: "acct1", Status: "success"})

	entries := l.Query(Filter{AccountID: "acct1"})
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
}

func TestQueryBySymbol(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Symbol: "AAPL", Status: "success"})
	l.Record(Entry{Symbol: "GOOG", Status: "success"})
	l.Record(Entry{Symbol: "AAPL", Status: "success"})

	entries := l.Query(Filter{Symbol: "GOOG"})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
}

func TestQueryByTimeRange(t *testing.T) {
	l := NewLog()
	t1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC)

	l.Record(Entry{Timestamp: t1, Status: "success"})
	l.Record(Entry{Timestamp: t2, Status: "success"})
	l.Record(Entry{Timestamp: t3, Status: "success"})

	since := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)
	until := time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC)
	entries := l.Query(Filter{Since: since, Until: until})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if !entries[0].Timestamp.Equal(t2) {
		t.Fatalf("entry timestamp = %v, want %v", entries[0].Timestamp, t2)
	}
}

func TestQueryCombinedFilters(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Action: ActionOrderCreate, Provider: "alpaca", AccountID: "acct1", Symbol: "AAPL", Status: "success"})
	l.Record(Entry{Action: ActionOrderCreate, Provider: "alpaca", AccountID: "acct2", Symbol: "AAPL", Status: "success"})
	l.Record(Entry{Action: ActionOrderCancel, Provider: "alpaca", AccountID: "acct1", Symbol: "AAPL", Status: "success"})
	l.Record(Entry{Action: ActionOrderCreate, Provider: "ibkr", AccountID: "acct1", Symbol: "AAPL", Status: "success"})

	entries := l.Query(Filter{
		Action:    ActionOrderCreate,
		Provider:  "alpaca",
		AccountID: "acct1",
	})
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
}

func TestStats(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Action: ActionOrderCreate, Provider: "alpaca", Status: "success", Latency: 10 * time.Millisecond})
	l.Record(Entry{Action: ActionOrderFill, Provider: "alpaca", Status: "success", Latency: 20 * time.Millisecond})
	l.Record(Entry{Action: ActionOrderCreate, Provider: "ibkr", Status: "failure", Latency: 30 * time.Millisecond})
	l.Record(Entry{Action: ActionTransfer, Provider: "alpaca", Status: "success"})

	stats := l.Stats()
	if stats["total_entries"].(int) != 4 {
		t.Fatalf("total_entries = %v, want 4", stats["total_entries"])
	}

	actions := stats["actions"].(map[Action]int)
	if actions[ActionOrderCreate] != 2 {
		t.Fatalf("ActionOrderCreate count = %d, want 2", actions[ActionOrderCreate])
	}
	if actions[ActionOrderFill] != 1 {
		t.Fatalf("ActionOrderFill count = %d, want 1", actions[ActionOrderFill])
	}

	providers := stats["providers"].(map[string]int)
	if providers["alpaca"] != 3 {
		t.Fatalf("alpaca count = %d, want 3", providers["alpaca"])
	}

	statuses := stats["statuses"].(map[string]int)
	if statuses["success"] != 3 {
		t.Fatalf("success count = %d, want 3", statuses["success"])
	}
	if statuses["failure"] != 1 {
		t.Fatalf("failure count = %d, want 1", statuses["failure"])
	}

	// avg latency: (10+20+30)/3 = 20ms
	avgStr := stats["avg_order_latency"].(string)
	if avgStr != "20ms" {
		t.Fatalf("avg_order_latency = %q, want %q", avgStr, "20ms")
	}
}

func TestExportEmpty(t *testing.T) {
	l := NewLog()
	data, err := l.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(entries))
	}
}

func TestExportWithEntries(t *testing.T) {
	l := NewLog()
	l.Record(Entry{Action: ActionOrderCreate, Provider: "alpaca", Symbol: "AAPL", Status: "success"})
	l.Record(Entry{Action: ActionOrderCancel, Provider: "ibkr", Symbol: "GOOG", Status: "success"})

	data, err := l.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0].Symbol != "AAPL" {
		t.Fatalf("first entry symbol = %q, want AAPL", entries[0].Symbol)
	}
	if entries[1].Symbol != "GOOG" {
		t.Fatalf("second entry symbol = %q, want GOOG", entries[1].Symbol)
	}
}

func TestConcurrentRecordAndQuery(t *testing.T) {
	l := NewLog()
	var wg sync.WaitGroup
	n := 100

	// Concurrent writers
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Record(Entry{Action: ActionOrderCreate, Provider: "alpaca", Status: "success"})
		}()
	}

	// Concurrent readers
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Query(Filter{})
		}()
	}

	wg.Wait()

	entries := l.Query(Filter{})
	if len(entries) != n {
		t.Fatalf("entries len = %d, want %d", len(entries), n)
	}
}

func TestAddHookReceivesEntries(t *testing.T) {
	l := NewLog()
	var received []Entry
	var mu sync.Mutex

	l.AddHook(func(e Entry) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	l.Record(Entry{Action: ActionOrderCreate, Status: "success"})
	l.Record(Entry{Action: ActionOrderCancel, Status: "success"})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("hook received %d entries, want 2", len(received))
	}
	if received[0].Action != ActionOrderCreate {
		t.Fatalf("first hook entry action = %q, want %q", received[0].Action, ActionOrderCreate)
	}
	if received[1].Action != ActionOrderCancel {
		t.Fatalf("second hook entry action = %q, want %q", received[1].Action, ActionOrderCancel)
	}
}

func TestAddMultipleHooks(t *testing.T) {
	l := NewLog()
	count1 := 0
	count2 := 0

	l.AddHook(func(e Entry) { count1++ })
	l.AddHook(func(e Entry) { count2++ })

	l.Record(Entry{Action: ActionOrderCreate, Status: "success"})

	if count1 != 1 {
		t.Fatalf("hook1 count = %d, want 1", count1)
	}
	if count2 != 1 {
		t.Fatalf("hook2 count = %d, want 1", count2)
	}
}
