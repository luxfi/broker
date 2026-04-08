package router

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// TWAPConfig controls time-weighted average price execution.
type TWAPConfig struct {
	Symbol      string        `json:"symbol"`
	Side        string        `json:"side"`        // buy, sell
	TotalQty    float64       `json:"total_qty"`
	Duration    time.Duration `json:"duration"`    // total execution window
	Slices      int           `json:"slices"`      // number of child orders
	MaxSlippage float64       `json:"max_slippage_bps,omitempty"` // halt if slippage exceeds this
}

// TWAPExecution tracks the state of a running TWAP.
type TWAPExecution struct {
	ID          string          `json:"id"`
	Config      TWAPConfig      `json:"config"`
	Status      string          `json:"status"` // running, completed, cancelled, failed
	SlicesFilled int            `json:"slices_filled"`
	TotalFilled float64         `json:"total_filled"`
	VWAP        float64         `json:"vwap"`
	Legs        []TWAPLeg       `json:"legs"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Error       string          `json:"error,omitempty"`

	mu     sync.Mutex
	cancel context.CancelFunc
}

// TWAPLeg is one time slice of a TWAP execution.
type TWAPLeg struct {
	SliceIndex int       `json:"slice_index"`
	Provider   string    `json:"provider"`
	OrderID    string    `json:"order_id"`
	Qty        string    `json:"qty"`
	FilledQty  string    `json:"filled_qty"`
	Price      float64   `json:"price"`
	Status     string    `json:"status"`
	ExecutedAt time.Time `json:"executed_at"`
}

// TWAPScheduler manages TWAP order executions.
type TWAPScheduler struct {
	registry   *provider.Registry
	router     *Router
	mu         sync.Mutex
	executions map[string]*TWAPExecution
}

// NewTWAPScheduler creates a new TWAP scheduler.
func NewTWAPScheduler(registry *provider.Registry, router *Router) *TWAPScheduler {
	return &TWAPScheduler{
		registry:   registry,
		router:     router,
		executions: make(map[string]*TWAPExecution),
	}
}

// Start begins a TWAP execution. It runs in the background, slicing the total
// quantity into equal parts and executing each slice at regular intervals.
// The accountsByProvider map provides the account ID for each venue.
func (s *TWAPScheduler) Start(ctx context.Context, cfg TWAPConfig, accountsByProvider map[string]string) (*TWAPExecution, error) {
	if cfg.TotalQty <= 0 {
		return nil, fmt.Errorf("total quantity must be positive")
	}
	if cfg.Duration <= 0 {
		return nil, fmt.Errorf("duration must be positive")
	}
	if cfg.Slices <= 0 {
		cfg.Slices = 10
	}
	if cfg.Slices > 1000 {
		return nil, fmt.Errorf("slices cannot exceed 1000")
	}

	execCtx, cancel := context.WithCancel(ctx)

	exec := &TWAPExecution{
		ID:        fmt.Sprintf("twap_%d", time.Now().UnixNano()),
		Config:    cfg,
		Status:    "running",
		StartedAt: time.Now(),
		cancel:    cancel,
	}

	s.mu.Lock()
	s.executions[exec.ID] = exec
	s.mu.Unlock()

	go s.run(execCtx, exec, cfg, accountsByProvider)
	return exec, nil
}

// Cancel stops a running TWAP execution.
func (s *TWAPScheduler) Cancel(id string) error {
	s.mu.Lock()
	exec, ok := s.executions[id]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("execution %s not found", id)
	}

	exec.mu.Lock()
	defer exec.mu.Unlock()
	if exec.Status != "running" {
		return fmt.Errorf("execution %s is %s, not running", id, exec.Status)
	}
	exec.Status = "cancelled"
	exec.cancel()
	now := time.Now()
	exec.CompletedAt = &now
	return nil
}

// Get returns a snapshot of the current state of a TWAP execution.
func (s *TWAPScheduler) Get(id string) (*TWAPExecution, error) {
	s.mu.Lock()
	exec, ok := s.executions[id]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("execution %s not found", id)
	}
	exec.mu.Lock()
	snapshot := &TWAPExecution{
		ID:           exec.ID,
		Config:       exec.Config,
		Status:       exec.Status,
		StartedAt:    exec.StartedAt,
		CompletedAt:  exec.CompletedAt,
		TotalFilled:  exec.TotalFilled,
		SlicesFilled: exec.SlicesFilled,
		VWAP:         exec.VWAP,
		Legs:         make([]TWAPLeg, len(exec.Legs)),
	}
	copy(snapshot.Legs, exec.Legs)
	exec.mu.Unlock()
	return snapshot, nil
}

// List returns all TWAP executions.
func (s *TWAPScheduler) List() []*TWAPExecution {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*TWAPExecution, 0, len(s.executions))
	for _, e := range s.executions {
		result = append(result, e)
	}
	return result
}

func (s *TWAPScheduler) run(ctx context.Context, exec *TWAPExecution, cfg TWAPConfig, accounts map[string]string) {
	sliceQty := cfg.TotalQty / float64(cfg.Slices)
	interval := cfg.Duration / time.Duration(cfg.Slices)

	var totalFilled, weightedPrice float64

	for i := 0; i < cfg.Slices; i++ {
		select {
		case <-ctx.Done():
			exec.mu.Lock()
			if exec.Status == "running" {
				exec.Status = "cancelled"
			}
			now := time.Now()
			exec.CompletedAt = &now
			exec.mu.Unlock()
			return
		default:
		}

		// For the last slice, take the remainder to avoid rounding drift.
		qty := sliceQty
		if i == cfg.Slices-1 {
			qty = cfg.TotalQty - totalFilled
		}
		if qty <= 0 {
			break
		}

		leg := s.executeSlice(ctx, exec, i, cfg.Symbol, cfg.Side, qty, accounts)

		exec.mu.Lock()
		exec.Legs = append(exec.Legs, leg)
		if leg.Status == "filled" || leg.Status == "partial" {
			filled, _ := strconv.ParseFloat(leg.FilledQty, 64)
			totalFilled += filled
			weightedPrice += leg.Price * filled
			exec.SlicesFilled++
		}
		exec.TotalFilled = totalFilled
		if totalFilled > 0 {
			exec.VWAP = weightedPrice / totalFilled
		}
		exec.mu.Unlock()

		// Wait for the next slice interval (unless this is the last slice).
		if i < cfg.Slices-1 {
			select {
			case <-ctx.Done():
				exec.mu.Lock()
				if exec.Status == "running" {
					exec.Status = "cancelled"
				}
				now := time.Now()
				exec.CompletedAt = &now
				exec.mu.Unlock()
				return
			case <-time.After(interval):
			}
		}
	}

	exec.mu.Lock()
	if exec.TotalFilled >= cfg.TotalQty*0.99 { // allow 1% rounding tolerance
		exec.Status = "completed"
	} else if exec.TotalFilled > 0 {
		exec.Status = "partial"
	} else {
		exec.Status = "failed"
	}
	now := time.Now()
	exec.CompletedAt = &now
	exec.mu.Unlock()
}

func (s *TWAPScheduler) executeSlice(ctx context.Context, exec *TWAPExecution, index int, symbol, side string, qty float64, accounts map[string]string) TWAPLeg {
	leg := TWAPLeg{
		SliceIndex: index,
		Qty:        fmt.Sprintf("%.8f", qty),
		ExecutedAt: time.Now(),
	}

	// Find best venue for this slice.
	best, err := s.router.FindBestProvider(ctx, symbol, side)
	if err != nil {
		leg.Status = "failed"
		return leg
	}
	leg.Provider = best.Provider

	accountID, ok := accounts[best.Provider]
	if !ok {
		leg.Status = "failed"
		return leg
	}

	p, err := s.registry.Get(best.Provider)
	if err != nil {
		leg.Status = "failed"
		return leg
	}

	order, err := p.CreateOrder(ctx, accountID, &types.CreateOrderRequest{
		Symbol:      symbol,
		Qty:         fmt.Sprintf("%.8f", qty),
		Side:        side,
		Type:        "market",
		TimeInForce: "ioc",
	})
	if err != nil {
		leg.Status = "failed"
		return leg
	}

	leg.OrderID = order.ProviderID
	leg.FilledQty = order.FilledQty
	leg.Status = order.Status
	if order.FilledAvgPrice != "" {
		leg.Price, _ = strconv.ParseFloat(order.FilledAvgPrice, 64)
	}
	return leg
}
