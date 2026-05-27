package pretrade

import (
	"context"
	"errors"
	"fmt"
)

// OrderEnricher maps a raw broker-side order (whatever shape the caller
// uses) into the pretrade.Order shape. Wiring into broker/pkg/router is
// done by the consumer because the router does not know about the
// investor / offering binding — that lives in the cap-table.
type OrderEnricher func(ctx context.Context, accountID string, raw any) (*Order, error)

// SubmitFunc is the underlying order-placement function the broker
// already exposes (e.g., Router.SmartOrder). The middleware wraps it.
type SubmitFunc[Req any, Resp any] func(ctx context.Context, accountID string, req Req) (Resp, error)

// GateError is returned when the pre-trade gate denies or escalates an
// order. It carries the full Decision so callers can surface the
// reasons + RequiredActions to the user.
type GateError struct {
	Decision *Decision
}

// Error satisfies the error interface.
func (e *GateError) Error() string {
	if e == nil || e.Decision == nil {
		return "pretrade: gate denied"
	}
	if e.Decision.Deny {
		return fmt.Sprintf("pretrade: denied — %s", joinCodes(e.Decision.Reasons))
	}
	if e.Decision.Escalate {
		return fmt.Sprintf("pretrade: escalate — %s", joinCodes(e.Decision.Reasons))
	}
	return "pretrade: unknown gate outcome"
}

// IsDeny reports whether the underlying decision is a hard deny.
func (e *GateError) IsDeny() bool { return e != nil && e.Decision != nil && e.Decision.Deny }

// IsEscalate reports whether the underlying decision is an escalation.
func (e *GateError) IsEscalate() bool {
	return e != nil && e.Decision != nil && e.Decision.Escalate
}

// IsGateError unwraps an error that may wrap a GateError.
func IsGateError(err error) (*GateError, bool) {
	var ge *GateError
	if errors.As(err, &ge) {
		return ge, true
	}
	return nil, false
}

// Wrap returns a SubmitFunc that runs the pretrade gate before delegating
// to inner. If the gate denies or escalates, inner is not called and a
// *GateError is returned. If the gate's upstream provider errors, the
// upstream error is propagated and inner is not called.
//
// Generics keep the wrapper type-safe over whatever concrete order /
// response shapes the caller uses, without locking pretrade into the
// broker/pkg/types surface.
func Wrap[Req any, Resp any](g *Gate, enricher OrderEnricher, inner SubmitFunc[Req, Resp]) SubmitFunc[Req, Resp] {
	return func(ctx context.Context, accountID string, req Req) (Resp, error) {
		var zero Resp
		o, err := enricher(ctx, accountID, req)
		if err != nil {
			return zero, fmt.Errorf("pretrade enrich: %w", err)
		}
		dec, err := g.Check(ctx, o)
		if err != nil {
			return zero, fmt.Errorf("pretrade check: %w", err)
		}
		if !dec.Allow {
			return zero, &GateError{Decision: dec}
		}
		return inner(ctx, accountID, req)
	}
}

func joinCodes(rs []Reason) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, r.Code)
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
