package pretrade

import (
	"encoding/json"

	"github.com/luxfi/broker/pkg/audit"
)

// AuditLogSink adapts a *audit.Log into the local AuditSink. Use this
// in production wiring; in tests pass a SliceSink.
type AuditLogSink struct {
	Log *audit.Log
}

// RecordPreTrade writes a single audit entry per evaluation. The action
// is the existing risk-decision channel ("route.decision") so existing
// SIEM consumers see pre-trade decisions through one stream; the
// Metadata map carries the structured decision payload.
func (s *AuditLogSink) RecordPreTrade(d *Decision, o *Order) {
	if s == nil || s.Log == nil || d == nil {
		return
	}
	entry := audit.Entry{
		Action:    audit.ActionRouteDecision,
		AccountID: o.AccountID,
		Symbol:    o.Symbol,
		Side:      o.Side,
		Qty:       o.Qty,
		Price:     o.Price,
		RequestID: o.RequestID,
		ClientIP:  o.ClientIP,
		Status:    statusFor(d),
		Metadata: map[string]interface{}{
			"investor_id":      o.InvestorID,
			"offering_id":      o.OfferingID,
			"token_address":    o.TokenAddress,
			"country_code":     o.CountryCode,
			"decision":         marshal(d),
			"required_actions": d.RequiredActions,
			"latency_ns":       d.LatencyNs,
		},
	}
	s.Log.Record(entry)
}

func statusFor(d *Decision) string {
	switch {
	case d.Allow:
		return "allow"
	case d.Deny:
		return "deny"
	case d.Escalate:
		return "escalate"
	default:
		return "unknown"
	}
}

func marshal(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// SliceSink collects decisions into a slice for unit tests. Not safe
// for concurrent producers — use a Mutex if a concurrent test ever
// needs it.
type SliceSink struct {
	Entries []SliceEntry
}

// SliceEntry is one captured pre-trade decision.
type SliceEntry struct {
	Decision *Decision
	Order    *Order
}

// RecordPreTrade satisfies the AuditSink interface.
func (s *SliceSink) RecordPreTrade(d *Decision, o *Order) {
	s.Entries = append(s.Entries, SliceEntry{Decision: d, Order: o})
}
