package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Log provides an append-only audit trail for all trading operations.
// Required for institutional compliance (SEC, FINRA, MiFID II).
type Log struct {
	mu      sync.Mutex
	entries []Entry
	hooks   []Hook
}

// Entry is a single audit log record.
type Entry struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Action      Action                 `json:"action"`
	Provider    string                 `json:"provider"`
	AccountID   string                 `json:"account_id"`
	Symbol      string                 `json:"symbol,omitempty"`
	Side        string                 `json:"side,omitempty"`
	Qty         string                 `json:"qty,omitempty"`
	Price       string                 `json:"price,omitempty"`
	OrderID     string                 `json:"order_id,omitempty"`
	Algorithm   string                 `json:"algorithm,omitempty"`
	Status      string                 `json:"status"`      // success, failure, pending
	Latency     time.Duration          `json:"latency_ns"`  // execution latency
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ClientIP    string                 `json:"client_ip,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
}

// Action categorizes audit events.
type Action string

const (
	ActionOrderCreate   Action = "order.create"
	ActionOrderCancel   Action = "order.cancel"
	ActionOrderFill     Action = "order.fill"
	ActionOrderReject   Action = "order.reject"
	ActionTransfer      Action = "transfer.create"
	ActionAccountCreate Action = "account.create"
	ActionAccountRead   Action = "account.read"
	ActionRouteDecision Action = "route.decision"
	ActionSplitPlan     Action = "route.split_plan"
	ActionSplitExecute  Action = "route.split_execute"
	ActionMarketData    Action = "market_data.query"
	ActionAuth          Action = "auth"
	ActionError         Action = "error"
)

// Hook is called for every audit entry (e.g., for external logging).
type Hook func(Entry)

func NewLog() *Log {
	return &Log{
		entries: make([]Entry, 0, 1024),
	}
}

// AddHook registers a callback for all audit entries.
func (l *Log) AddHook(h Hook) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks = append(l.hooks, h)
}

// Record adds an audit entry.
func (l *Log) Record(e Entry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.ID == "" {
		e.ID = generateID()
	}

	l.mu.Lock()
	l.entries = append(l.entries, e)
	hooks := make([]Hook, len(l.hooks))
	copy(hooks, l.hooks)
	l.mu.Unlock()

	for _, h := range hooks {
		h(e)
	}
}

// RecordOrder is a convenience for order audit entries.
func (l *Log) RecordOrder(ctx context.Context, action Action, provider, accountID, symbol, side, qty, orderID, algo, status string, latency time.Duration, err error) {
	e := Entry{
		Action:    action,
		Provider:  provider,
		AccountID: accountID,
		Symbol:    symbol,
		Side:      side,
		Qty:       qty,
		OrderID:   orderID,
		Algorithm: algo,
		Status:    status,
		Latency:   latency,
	}
	if err != nil {
		e.Error = err.Error()
		e.Status = "failure"
	}
	l.Record(e)
}

// Query returns audit entries matching the filter.
func (l *Log) Query(filter Filter) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var results []Entry
	for _, e := range l.entries {
		if filter.Matches(e) {
			results = append(results, e)
		}
	}
	return results
}

// Filter for querying audit entries.
type Filter struct {
	Action    Action    `json:"action,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	AccountID string    `json:"account_id,omitempty"`
	Symbol    string    `json:"symbol,omitempty"`
	Since     time.Time `json:"since,omitempty"`
	Until     time.Time `json:"until,omitempty"`
	Limit     int       `json:"limit,omitempty"`
}

func (f Filter) Matches(e Entry) bool {
	if f.Action != "" && e.Action != f.Action {
		return false
	}
	if f.Provider != "" && e.Provider != f.Provider {
		return false
	}
	if f.AccountID != "" && e.AccountID != f.AccountID {
		return false
	}
	if f.Symbol != "" && e.Symbol != f.Symbol {
		return false
	}
	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
		return false
	}
	return true
}

// Export returns all entries as JSON for compliance reporting.
func (l *Log) Export() ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return json.Marshal(l.entries)
}

// Stats returns aggregate statistics for the audit log.
func (l *Log) Stats() map[string]interface{} {
	l.mu.Lock()
	defer l.mu.Unlock()

	actionCounts := make(map[Action]int)
	providerCounts := make(map[string]int)
	statusCounts := make(map[string]int)
	var totalLatency time.Duration
	orderCount := 0

	for _, e := range l.entries {
		actionCounts[e.Action]++
		if e.Provider != "" {
			providerCounts[e.Provider]++
		}
		statusCounts[e.Status]++
		if e.Action == ActionOrderCreate || e.Action == ActionOrderFill {
			totalLatency += e.Latency
			orderCount++
		}
	}

	var avgLatency time.Duration
	if orderCount > 0 {
		avgLatency = totalLatency / time.Duration(orderCount)
	}

	return map[string]interface{}{
		"total_entries":     len(l.entries),
		"actions":           actionCounts,
		"providers":         providerCounts,
		"statuses":          statusCounts,
		"avg_order_latency": avgLatency.String(),
	}
}

func generateID() string {
	return time.Now().UTC().Format("20060102150405.000000")
}
