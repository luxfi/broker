package router

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

func TestTWAPSchedulerStart(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
		order: &types.Order{
			Provider:       "alpaca",
			ProviderID:     "twap-order-1",
			Status:         "filled",
			FilledQty:      "10.00000000",
			FilledAvgPrice: "150.05",
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)
	r.SetFees("alpaca", 0, 0)
	scheduler := NewTWAPScheduler(reg, r)

	ctx := context.Background()
	exec, err := scheduler.Start(ctx, TWAPConfig{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: 30,
		Duration: 300 * time.Millisecond, // short for testing
		Slices:   3,
	}, map[string]string{"alpaca": "acct-1"})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if exec.ID == "" {
		t.Fatal("expected non-empty execution ID")
	}
	if exec.Status != "running" {
		t.Errorf("Status = %q, want 'running'", exec.Status)
	}

	// Wait for completion.
	time.Sleep(600 * time.Millisecond)

	result, err := scheduler.Get(exec.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want 'completed'", result.Status)
	}
	if result.SlicesFilled != 3 {
		t.Errorf("SlicesFilled = %d, want 3", result.SlicesFilled)
	}
	if result.VWAP != 150.05 {
		t.Errorf("VWAP = %f, want 150.05", result.VWAP)
	}
}

func TestTWAPSchedulerCancel(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
		order: &types.Order{
			Provider:       "alpaca",
			ProviderID:     "twap-cancel-1",
			Status:         "filled",
			FilledQty:      "5.00000000",
			FilledAvgPrice: "150.05",
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)
	scheduler := NewTWAPScheduler(reg, r)

	ctx := context.Background()
	exec, err := scheduler.Start(ctx, TWAPConfig{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: 100,
		Duration: 10 * time.Second, // long enough to cancel
		Slices:   100,
	}, map[string]string{"alpaca": "acct-1"})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Let one or two slices execute.
	time.Sleep(200 * time.Millisecond)

	if err := scheduler.Cancel(exec.ID); err != nil {
		t.Fatalf("Cancel error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	result, _ := scheduler.Get(exec.ID)
	if result.Status != "cancelled" {
		t.Errorf("Status = %q, want 'cancelled'", result.Status)
	}
}

func TestTWAPSchedulerInvalidConfig(t *testing.T) {
	reg := newTestRegistry()
	r := New(reg)
	scheduler := NewTWAPScheduler(reg, r)

	ctx := context.Background()

	// Zero quantity
	_, err := scheduler.Start(ctx, TWAPConfig{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: 0,
		Duration: time.Minute,
		Slices:   10,
	}, nil)
	if err == nil {
		t.Fatal("expected error for zero quantity")
	}

	// Zero duration
	_, err = scheduler.Start(ctx, TWAPConfig{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: 100,
		Duration: 0,
		Slices:   10,
	}, nil)
	if err == nil {
		t.Fatal("expected error for zero duration")
	}

	// Too many slices
	_, err = scheduler.Start(ctx, TWAPConfig{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: 100,
		Duration: time.Minute,
		Slices:   2000,
	}, nil)
	if err == nil {
		t.Fatal("expected error for too many slices")
	}
}

func TestTWAPSchedulerList(t *testing.T) {
	mp := &mockProvider{
		name: "alpaca",
		snapshot: &types.MarketSnapshot{
			Symbol:      "AAPL",
			LatestQuote: &types.Quote{BidPrice: 150, AskPrice: 150.1},
		},
		order: &types.Order{
			Provider:       "alpaca",
			ProviderID:     "list-order",
			Status:         "filled",
			FilledQty:      "50.00000000",
			FilledAvgPrice: "150.05",
		},
	}
	reg := newTestRegistry(mp)
	r := New(reg)
	scheduler := NewTWAPScheduler(reg, r)

	ctx := context.Background()
	_, _ = scheduler.Start(ctx, TWAPConfig{
		Symbol:   "AAPL",
		Side:     "buy",
		TotalQty: 100,
		Duration: 100 * time.Millisecond,
		Slices:   2,
	}, map[string]string{"alpaca": "acct-1"})

	execs := scheduler.List()
	if len(execs) != 1 {
		t.Errorf("List() returned %d executions, want 1", len(execs))
	}
}
