package risk

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
)

// Engine performs pre-trade risk checks before orders reach providers.
// Required for institutional compliance and risk management.
type Engine struct {
	mu     sync.RWMutex
	limits map[string]*AccountLimits // accountKey -> limits
	usage  map[string]*AccountUsage  // accountKey -> current usage
	global GlobalLimits
}

// GlobalLimits are platform-wide risk limits.
type GlobalLimits struct {
	MaxOrderValue     float64       `json:"max_order_value"`      // max single order in USD
	MaxDailyVolume    float64       `json:"max_daily_volume"`     // max daily volume per account in USD
	MaxOpenOrders     int           `json:"max_open_orders"`      // max concurrent open orders per account
	MaxPositionValue  float64       `json:"max_position_value"`   // max position size in USD
	RateLimitPerMin   int           `json:"rate_limit_per_min"`   // max orders per minute per account
	AllowedProviders  []string      `json:"allowed_providers"`    // whitelist of providers
	BlockedSymbols    []string      `json:"blocked_symbols"`      // symbols that cannot be traded
	CooldownAfterLoss time.Duration `json:"cooldown_after_loss"`  // cooldown after significant loss
}

// AccountLimits are per-account risk limits (override global).
type AccountLimits struct {
	MaxOrderValue    float64  `json:"max_order_value"`
	MaxDailyVolume   float64  `json:"max_daily_volume"`
	MaxOpenOrders    int      `json:"max_open_orders"`
	MaxPositionValue float64  `json:"max_position_value"`
	RateLimitPerMin  int      `json:"rate_limit_per_min"`
	AllowedSymbols   []string `json:"allowed_symbols,omitempty"`   // if set, only these
	BlockedSymbols   []string `json:"blocked_symbols,omitempty"`
}

// AccountUsage tracks current resource usage for an account.
type AccountUsage struct {
	DailyVolume    float64
	OpenOrders     int
	OrderTimestamps []time.Time // for rate limiting
	LastReset      time.Time
}

// CheckRequest contains the order details to validate.
type CheckRequest struct {
	Provider  string
	AccountID string
	Symbol    string
	Side      string
	Qty       string
	Price     string // estimated fill price
	OrderType string
}

// CheckResult contains the risk check outcome.
type CheckResult struct {
	Allowed  bool     `json:"allowed"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}

// DefaultLimits returns sensible defaults for a new platform.
func DefaultLimits() GlobalLimits {
	return GlobalLimits{
		MaxOrderValue:    1_000_000, // $1M max single order
		MaxDailyVolume:   10_000_000, // $10M daily per account
		MaxOpenOrders:    100,
		MaxPositionValue: 5_000_000, // $5M max position
		RateLimitPerMin:  60,        // 1 order/sec avg
	}
}

func NewEngine(global GlobalLimits) *Engine {
	return &Engine{
		limits: make(map[string]*AccountLimits),
		usage:  make(map[string]*AccountUsage),
		global: global,
	}
}

// SetAccountLimits configures per-account limits.
func (e *Engine) SetAccountLimits(provider, accountID string, limits AccountLimits) {
	key := provider + "/" + accountID
	e.mu.Lock()
	defer e.mu.Unlock()
	e.limits[key] = &limits
}

// PreTradeCheck validates an order against all risk limits.
func (e *Engine) PreTradeCheck(req CheckRequest) CheckResult {
	result := CheckResult{Allowed: true}
	key := req.Provider + "/" + req.AccountID

	e.mu.Lock()
	defer e.mu.Unlock()

	// Get or create usage tracker
	usage, ok := e.usage[key]
	if !ok {
		usage = &AccountUsage{LastReset: time.Now()}
		e.usage[key] = usage
	}

	// Reset daily counters if needed
	if time.Since(usage.LastReset) > 24*time.Hour {
		usage.DailyVolume = 0
		usage.OrderTimestamps = nil
		usage.LastReset = time.Now()
	}

	// Get effective limits (account overrides global)
	maxOrderVal := e.global.MaxOrderValue
	maxDailyVol := e.global.MaxDailyVolume
	maxOpen := e.global.MaxOpenOrders
	rateLimit := e.global.RateLimitPerMin

	if acctLimits, ok := e.limits[key]; ok {
		if acctLimits.MaxOrderValue > 0 {
			maxOrderVal = acctLimits.MaxOrderValue
		}
		if acctLimits.MaxDailyVolume > 0 {
			maxDailyVol = acctLimits.MaxDailyVolume
		}
		if acctLimits.MaxOpenOrders > 0 {
			maxOpen = acctLimits.MaxOpenOrders
		}
		if acctLimits.RateLimitPerMin > 0 {
			rateLimit = acctLimits.RateLimitPerMin
		}

		// Check allowed/blocked symbols
		if len(acctLimits.AllowedSymbols) > 0 {
			found := false
			for _, s := range acctLimits.AllowedSymbols {
				if s == req.Symbol {
					found = true
					break
				}
			}
			if !found {
				result.Allowed = false
				result.Errors = append(result.Errors, fmt.Sprintf("symbol %s not in allowed list", req.Symbol))
			}
		}
		for _, s := range acctLimits.BlockedSymbols {
			if s == req.Symbol {
				result.Allowed = false
				result.Errors = append(result.Errors, fmt.Sprintf("symbol %s is blocked", req.Symbol))
			}
		}
	}

	// Check global blocked symbols
	for _, s := range e.global.BlockedSymbols {
		if s == req.Symbol {
			result.Allowed = false
			result.Errors = append(result.Errors, fmt.Sprintf("symbol %s is globally blocked", req.Symbol))
		}
	}

	// Check provider allowlist
	if len(e.global.AllowedProviders) > 0 {
		found := false
		for _, p := range e.global.AllowedProviders {
			if p == req.Provider {
				found = true
				break
			}
		}
		if !found {
			result.Allowed = false
			result.Errors = append(result.Errors, fmt.Sprintf("provider %s not allowed", req.Provider))
		}
	}

	// Estimate order value
	orderValue := estimateOrderValue(req.Qty, req.Price)

	// Max order value check
	if orderValue > maxOrderVal {
		result.Allowed = false
		result.Errors = append(result.Errors, fmt.Sprintf("order value $%.2f exceeds limit $%.2f", orderValue, maxOrderVal))
	}

	// Daily volume check
	if usage.DailyVolume+orderValue > maxDailyVol {
		result.Allowed = false
		result.Errors = append(result.Errors, fmt.Sprintf("daily volume would exceed limit $%.2f", maxDailyVol))
	}

	// Open orders check
	if usage.OpenOrders >= maxOpen {
		result.Allowed = false
		result.Errors = append(result.Errors, fmt.Sprintf("open orders %d at limit %d", usage.OpenOrders, maxOpen))
	}

	// Rate limit check
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	recent := 0
	for _, ts := range usage.OrderTimestamps {
		if ts.After(cutoff) {
			recent++
		}
	}
	if recent >= rateLimit {
		result.Allowed = false
		result.Errors = append(result.Errors, fmt.Sprintf("rate limit: %d orders in last minute (limit %d)", recent, rateLimit))
	}

	// Warnings for large orders
	if orderValue > maxOrderVal*0.5 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("large order: $%.2f (%.0f%% of limit)", orderValue, orderValue/maxOrderVal*100))
	}
	if usage.DailyVolume+orderValue > maxDailyVol*0.8 {
		result.Warnings = append(result.Warnings, "approaching daily volume limit")
	}

	return result
}

// RecordOrder updates usage after an order is placed.
func (e *Engine) RecordOrder(provider, accountID string, orderValue float64) {
	key := provider + "/" + accountID
	e.mu.Lock()
	defer e.mu.Unlock()

	usage, ok := e.usage[key]
	if !ok {
		usage = &AccountUsage{LastReset: time.Now()}
		e.usage[key] = usage
	}
	usage.DailyVolume += orderValue
	usage.OpenOrders++
	usage.OrderTimestamps = append(usage.OrderTimestamps, time.Now())
}

// RecordFill updates usage when an order fills or is cancelled.
func (e *Engine) RecordFill(provider, accountID string) {
	key := provider + "/" + accountID
	e.mu.Lock()
	defer e.mu.Unlock()
	if usage, ok := e.usage[key]; ok {
		if usage.OpenOrders > 0 {
			usage.OpenOrders--
		}
	}
}

func estimateOrderValue(qty, price string) float64 {
	q, _ := strconv.ParseFloat(qty, 64)
	p, _ := strconv.ParseFloat(price, 64)
	if p == 0 {
		p = 1 // market orders without price estimate
	}
	return math.Abs(q * p)
}
