package settlement

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// KYC tier names. Higher tiers have higher instant-buy limits because
// the user has been more thoroughly verified, reducing fraud risk.
const (
	TierBasic         = "basic"
	TierStandard      = "standard"
	TierEnhanced      = "enhanced"
	TierInstitutional = "institutional"
)

// Default instant-buy limits per KYC tier (USD).
// These represent the maximum total outstanding prefunded credit per account.
var defaultTierLimits = map[string]float64{
	TierBasic:         250,
	TierStandard:      5_000,
	TierEnhanced:      25_000,
	TierInstitutional: 250_000,
}

// SettlementEventType categorizes events in the ACH settlement lifecycle.
type SettlementEventType string

const (
	EventACHInitiated SettlementEventType = "ach_initiated"
	EventACHPending   SettlementEventType = "ach_pending"
	EventACHCleared   SettlementEventType = "ach_cleared"
	EventACHFailed    SettlementEventType = "ach_failed"
	EventMarginCall   SettlementEventType = "margin_call"
	EventLiquidated   SettlementEventType = "liquidated"
)

// InstantBuyRequest is the input for executing an instant buy backed by pool capital.
type InstantBuyRequest struct {
	AccountID string  `json:"account_id"`
	Symbol    string  `json:"symbol"`
	Qty       float64 `json:"qty"`
	Side      string  `json:"side"`     // buy or sell
	KYCTier   string  `json:"kyc_tier"` // basic, standard, enhanced, institutional
}

// InstantBuyResult is the output after a successful instant buy.
type InstantBuyResult struct {
	ReservationID       string    `json:"reservation_id"`
	OrderID             string    `json:"order_id"` // placeholder — real order ID comes from execution provider
	ExecutionPrice      float64   `json:"execution_price"`
	EstimatedSettlement time.Time `json:"estimated_settlement"` // when ACH is expected to clear
	Status              string    `json:"status"`
}

// SettlementEvent represents a state change in the settlement process.
type SettlementEvent struct {
	Type      SettlementEventType `json:"type"`
	Timestamp time.Time           `json:"timestamp"`
}

// Exposure summarizes an account's total outstanding prefunded credit.
type Exposure struct {
	AccountID          string  `json:"account_id"`
	TotalOutstanding   float64 `json:"total_outstanding"`   // sum of pending reservation amounts
	ActiveReservations int     `json:"active_reservations"` // count of pending reservations
	TierLimit          float64 `json:"tier_limit"`          // max allowed for this KYC tier
	AvailableCredit    float64 `json:"available_credit"`    // tier_limit - total_outstanding
}

// ServiceStats provides aggregate settlement statistics.
type ServiceStats struct {
	TotalReservations    int     `json:"total_reservations"`
	PendingReservations  int     `json:"pending_reservations"`
	SettledReservations  int     `json:"settled_reservations"`
	FailedReservations   int     `json:"failed_reservations"`
	LiquidatedReservations int  `json:"liquidated_reservations"`
	MarginCalledReservations int `json:"margin_called_reservations"`
	TotalVolume          float64 `json:"total_volume"`    // sum of all reservation amounts ever
	PendingVolume        float64 `json:"pending_volume"`  // sum of currently pending amounts
}

// Service orchestrates the full instant-buy flow:
//
//  1. Validate KYC tier limits
//  2. Reserve pool funds
//  3. Execute trade (caller handles actual execution)
//  4. Initiate ACH (caller handles actual ACH)
//  5. Track settlement lifecycle via events
//
// The Service does not execute trades or initiate ACH directly — those are
// handled by the broker's execution and funding services. This service manages
// the credit reservation lifecycle.
type Service struct {
	mu         sync.RWMutex
	pool       *Pool
	tierLimits map[string]float64
	policy     *MarginPolicy
}

// NewService creates a settlement service backed by the given pool.
func NewService(pool *Pool) *Service {
	limits := make(map[string]float64)
	for k, v := range defaultTierLimits {
		limits[k] = v
	}
	return &Service{
		pool:       pool,
		tierLimits: limits,
		policy:     DefaultMarginPolicy(),
	}
}

// SetTierLimit configures the instant-buy limit for a KYC tier.
func (s *Service) SetTierLimit(tier string, limit float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tierLimits[tier] = limit
}

// SetMarginPolicy replaces the margin monitoring policy.
func (s *Service) SetMarginPolicy(policy *MarginPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policy = policy
}

// InstantBuy validates the request, reserves pool funds, and returns a result
// indicating the trade can proceed. The caller is responsible for:
//   - Executing the actual trade via the broker provider
//   - Initiating the ACH transfer
//   - Calling ProcessSettlement with lifecycle events
//
// The ctx parameter is accepted for future use (e.g., tracing, cancellation).
func (s *Service) InstantBuy(_ context.Context, req *InstantBuyRequest) (*InstantBuyResult, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	if req.Symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if req.Qty <= 0 {
		return nil, fmt.Errorf("qty must be positive")
	}
	if req.Side != "buy" {
		return nil, fmt.Errorf("instant buy only supports side=buy, got %q", req.Side)
	}

	// Look up KYC tier limit.
	s.mu.RLock()
	tierLimit, ok := s.tierLimits[req.KYCTier]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown KYC tier %q", req.KYCTier)
	}

	// Calculate the order value. For simplicity, we use qty as the USD amount.
	// In production, this would be qty * current market price.
	orderValue := req.Qty

	// Check if this order would exceed the account's KYC tier limit.
	// We sum all pending reservations for this account.
	existing := s.pool.ListReservations(req.AccountID)
	var outstanding float64
	for _, r := range existing {
		if r.Status == StatusPendingSettlement || r.Status == StatusMarginCalled {
			outstanding += r.Amount
		}
	}
	if outstanding+orderValue > tierLimit {
		return nil, fmt.Errorf("order $%.2f would exceed %s tier limit $%.2f (outstanding $%.2f, available $%.2f)",
			orderValue, req.KYCTier, tierLimit, outstanding, tierLimit-outstanding)
	}

	// Reserve pool funds. The pool enforces its own limits (per-user, per-tx, utilization).
	// We use orderValue as both the amount and price since qty represents USD value here.
	reservation, err := s.pool.Reserve(req.AccountID, req.Symbol, orderValue, orderValue/req.Qty)
	if err != nil {
		return nil, fmt.Errorf("pool reservation failed: %w", err)
	}

	// Estimated settlement: ACH typically clears in 2-3 business days.
	estimatedSettlement := time.Now().UTC().Add(3 * 24 * time.Hour)

	return &InstantBuyResult{
		ReservationID:       reservation.ID,
		OrderID:             "ord_" + reservation.ID[4:], // derive order ID from reservation ID
		ExecutionPrice:      reservation.Price,
		EstimatedSettlement: estimatedSettlement,
		Status:              string(StatusPendingSettlement),
	}, nil
}

// ProcessSettlement handles settlement lifecycle events for a reservation.
// Called by the funding service when ACH status changes.
func (s *Service) ProcessSettlement(reservationID string, event SettlementEvent) error {
	switch event.Type {
	case EventACHInitiated, EventACHPending:
		// Informational — no state change needed. The reservation stays pending.
		return nil

	case EventACHCleared:
		// Happy path: user's real funds arrived. Release the pool credit.
		return s.pool.Settle(reservationID)

	case EventACHFailed:
		// ACH bounced. Mark for liquidation.
		return s.pool.Fail(reservationID)

	case EventMarginCall:
		return s.pool.MarginCall(reservationID)

	case EventLiquidated:
		// Force-sell completed. Recover whatever we got.
		// In production, recoveredAmount would come from the liquidation execution.
		// Here we pass 0 since the actual recovery is handled externally.
		return s.pool.Liquidate(reservationID, 0)

	default:
		return fmt.Errorf("unknown settlement event type %q", event.Type)
	}
}

// CheckMarginHealth scans all pending reservations and returns alerts for
// positions that have depreciated beyond policy thresholds.
//
// The caller must provide current prices via a price feed. This method uses
// a simple approach: it checks each reservation's asset against its entry price.
// In production, you'd pass a price lookup function.
func (s *Service) CheckMarginHealth(priceFunc func(asset string) float64) []MarginAlert {
	s.mu.RLock()
	policy := s.policy
	s.mu.RUnlock()

	// Get all reservations from the pool.
	s.pool.mu.RLock()
	reservations := make([]*Reservation, 0, len(s.pool.reservations))
	for _, r := range s.pool.reservations {
		if r.Status == StatusPendingSettlement || r.Status == StatusMarginCalled {
			copy := *r
			reservations = append(reservations, &copy)
		}
	}
	s.pool.mu.RUnlock()

	var alerts []MarginAlert
	for _, r := range reservations {
		currentPrice := priceFunc(r.Asset)
		if alert := CheckMargin(r, currentPrice, policy); alert != nil {
			alerts = append(alerts, *alert)
		}
	}
	return alerts
}

// GetAccountExposure returns the total outstanding credit exposure for an account.
func (s *Service) GetAccountExposure(accountID string) *Exposure {
	s.mu.RLock()
	tierLimit := 0.0
	// Find the highest applicable tier limit for this account.
	// In production, you'd look up the account's actual KYC tier.
	// Here we return the max configured tier as the default.
	for _, limit := range s.tierLimits {
		if limit > tierLimit {
			tierLimit = limit
		}
	}
	s.mu.RUnlock()

	reservations := s.pool.ListReservations(accountID)
	var outstanding float64
	var active int
	for _, r := range reservations {
		if r.Status == StatusPendingSettlement || r.Status == StatusMarginCalled {
			outstanding += r.Amount
			active++
		}
	}

	return &Exposure{
		AccountID:          accountID,
		TotalOutstanding:   outstanding,
		ActiveReservations: active,
		TierLimit:          tierLimit,
		AvailableCredit:    tierLimit - outstanding,
	}
}

// GetAccountExposureForTier returns exposure with the correct tier limit applied.
func (s *Service) GetAccountExposureForTier(accountID, tier string) *Exposure {
	s.mu.RLock()
	tierLimit := s.tierLimits[tier]
	s.mu.RUnlock()

	reservations := s.pool.ListReservations(accountID)
	var outstanding float64
	var active int
	for _, r := range reservations {
		if r.Status == StatusPendingSettlement || r.Status == StatusMarginCalled {
			outstanding += r.Amount
			active++
		}
	}

	return &Exposure{
		AccountID:          accountID,
		TotalOutstanding:   outstanding,
		ActiveReservations: active,
		TierLimit:          tierLimit,
		AvailableCredit:    tierLimit - outstanding,
	}
}

// Stats returns aggregate settlement statistics across all reservations.
func (s *Service) Stats() *ServiceStats {
	s.pool.mu.RLock()
	defer s.pool.mu.RUnlock()

	stats := &ServiceStats{}
	for _, r := range s.pool.reservations {
		stats.TotalReservations++
		stats.TotalVolume += r.Amount
		switch r.Status {
		case StatusPendingSettlement:
			stats.PendingReservations++
			stats.PendingVolume += r.Amount
		case StatusSettled:
			stats.SettledReservations++
		case StatusFailed:
			stats.FailedReservations++
			stats.PendingVolume += r.Amount // still reserved
		case StatusLiquidated:
			stats.LiquidatedReservations++
		case StatusMarginCalled:
			stats.MarginCalledReservations++
			stats.PendingVolume += r.Amount // still reserved
		}
	}
	return stats
}
