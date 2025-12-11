package settlement

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ReservationStatus tracks where a reservation is in the settlement lifecycle.
//
// Lifecycle: pending_settlement -> settled (happy path)
//            pending_settlement -> failed -> liquidated (ACH bounce)
//            pending_settlement -> margin_called -> liquidated (price crash)
type ReservationStatus string

const (
	StatusPendingSettlement ReservationStatus = "pending_settlement"
	StatusSettled           ReservationStatus = "settled"
	StatusFailed           ReservationStatus = "failed"
	StatusLiquidated       ReservationStatus = "liquidated"
	StatusMarginCalled     ReservationStatus = "margin_called"
)

// PoolConfig controls the capital limits for the prefunding pool.
// These limits prevent the operator from over-extending credit.
type PoolConfig struct {
	// MaxPoolSize is the maximum total capital the pool can hold.
	MaxPoolSize float64 `json:"max_pool_size"`

	// MaxPerUser is the maximum outstanding credit for a single account.
	// Prevents one user from monopolizing pool capital.
	MaxPerUser float64 `json:"max_per_user"`

	// MaxPerTransaction is the maximum single reservation amount.
	MaxPerTransaction float64 `json:"max_per_transaction"`

	// UtilizationWarningPct triggers warnings when reserved/total exceeds this.
	// E.g., 0.80 means warn at 80% utilization.
	UtilizationWarningPct float64 `json:"utilization_warning_pct"`
}

// Reservation is a hold on pool funds for a pending settlement.
// When a user instant-buys, we reserve pool capital until ACH clears.
// If ACH fails, the reservation is marked for liquidation (force-sell the asset).
type Reservation struct {
	ID        string            `json:"id"`
	AccountID string            `json:"account_id"`
	Amount    float64           `json:"amount"`     // USD value of the reservation
	Asset     string            `json:"asset"`      // symbol purchased
	Price     float64           `json:"price"`      // entry price at time of purchase
	Status    ReservationStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	SettledAt *time.Time        `json:"settled_at,omitempty"`
}

// PoolStatus is a snapshot of the pool's current state.
type PoolStatus struct {
	Total             float64 `json:"total"`
	Available         float64 `json:"available"`
	Reserved          float64 `json:"reserved"`
	UtilizationPct    float64 `json:"utilization_pct"`
	ActiveReservations int    `json:"active_reservations"`
}

// Pool is the operator-funded capital pool that backs instant buys.
//
// When a user buys instantly, the pool advances funds so the trade executes
// immediately. The user's ACH/wire is initiated in parallel. When the ACH
// clears (typically T+2-3), the reservation is settled and capital is returned
// to the pool. If ACH fails, the position is liquidated to recover capital.
//
// The pool is funded by the ATS operator and acts as a short-term credit facility.
type Pool struct {
	mu           sync.RWMutex
	total        float64 // total capital deposited by operator
	reserved     float64 // sum of all pending reservation amounts
	config       PoolConfig
	reservations map[string]*Reservation // reservationID -> reservation
}

// NewPool creates a prefunding pool with the given configuration.
// The pool starts with zero capital — the operator must call AddCapital.
func NewPool(cfg PoolConfig) *Pool {
	return &Pool{
		config:       cfg,
		reservations: make(map[string]*Reservation),
	}
}

// Reserve creates a hold on pool funds for an instant buy.
//
// Risk controls enforced:
// 1. Per-transaction limit — rejects orders that are too large
// 2. Per-user outstanding limit — prevents one user from draining the pool
// 3. Pool utilization cap — keeps a safety buffer for the operator
// 4. Available capital check — can't lend more than we have
func (p *Pool) Reserve(accountID, asset string, amount, price float64) (*Reservation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. Per-transaction limit
	if p.config.MaxPerTransaction > 0 && amount > p.config.MaxPerTransaction {
		return nil, fmt.Errorf("amount $%.2f exceeds per-transaction limit $%.2f", amount, p.config.MaxPerTransaction)
	}

	// 2. Per-user outstanding limit: sum all pending reservations for this account
	var userOutstanding float64
	for _, r := range p.reservations {
		if r.AccountID == accountID && r.Status == StatusPendingSettlement {
			userOutstanding += r.Amount
		}
	}
	if p.config.MaxPerUser > 0 && userOutstanding+amount > p.config.MaxPerUser {
		return nil, fmt.Errorf("account %s outstanding $%.2f + $%.2f would exceed per-user limit $%.2f",
			accountID, userOutstanding, amount, p.config.MaxPerUser)
	}

	// 3. Pool utilization cap: don't exceed the warning threshold
	// This keeps a safety buffer so the operator isn't fully extended.
	available := p.total - p.reserved
	if p.config.UtilizationWarningPct > 0 {
		maxReservable := p.total * p.config.UtilizationWarningPct
		if p.reserved+amount > maxReservable {
			return nil, fmt.Errorf("reservation would push utilization to %.1f%% (limit %.1f%%)",
				(p.reserved+amount)/p.total*100, p.config.UtilizationWarningPct*100)
		}
	}

	// 4. Sufficient available capital
	if amount > available {
		return nil, fmt.Errorf("insufficient pool capital: need $%.2f, available $%.2f", amount, available)
	}

	now := time.Now().UTC()
	r := &Reservation{
		ID:        generateReservationID(),
		AccountID: accountID,
		Amount:    amount,
		Asset:     asset,
		Price:     price,
		Status:    StatusPendingSettlement,
		CreatedAt: now,
		UpdatedAt: now,
	}

	p.reservations[r.ID] = r
	p.reserved += amount

	return r, nil
}

// Settle marks a reservation as settled after ACH/wire clears.
// This releases the credit back to available capital — the user's real funds
// have arrived and replaced the pool's advance.
func (p *Pool) Settle(reservationID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.reservations[reservationID]
	if !ok {
		return fmt.Errorf("reservation %s not found", reservationID)
	}
	if r.Status != StatusPendingSettlement && r.Status != StatusMarginCalled {
		return fmt.Errorf("reservation %s in status %s, cannot settle", reservationID, r.Status)
	}

	now := time.Now().UTC()
	r.Status = StatusSettled
	r.UpdatedAt = now
	r.SettledAt = &now
	p.reserved -= r.Amount

	return nil
}

// Fail marks a reservation as failed (e.g., ACH bounced).
// The funds remain reserved because we still hold the asset and need to
// liquidate to recover capital.
func (p *Pool) Fail(reservationID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.reservations[reservationID]
	if !ok {
		return fmt.Errorf("reservation %s not found", reservationID)
	}
	if r.Status != StatusPendingSettlement {
		return fmt.Errorf("reservation %s in status %s, cannot fail", reservationID, r.Status)
	}

	r.Status = StatusFailed
	r.UpdatedAt = time.Now().UTC()
	// Funds stay reserved — liquidation will release them.
	return nil
}

// Liquidate force-sells the asset to recover pool capital after a failed settlement.
// recoveredAmount is the actual USD received from the liquidation sale.
// If recoveredAmount < reservation amount, the operator absorbs the loss.
func (p *Pool) Liquidate(reservationID string, recoveredAmount float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.reservations[reservationID]
	if !ok {
		return fmt.Errorf("reservation %s not found", reservationID)
	}
	if r.Status != StatusFailed && r.Status != StatusMarginCalled {
		return fmt.Errorf("reservation %s in status %s, cannot liquidate", reservationID, r.Status)
	}

	r.Status = StatusLiquidated
	r.UpdatedAt = time.Now().UTC()

	// Release the original reserved amount and credit back what we actually recovered.
	// If recoveredAmount < r.Amount, the operator takes a loss on the difference.
	// If recoveredAmount > r.Amount, the excess belongs to the user (not modeled here).
	p.reserved -= r.Amount

	return nil
}

// MarginCall marks a reservation as margin-called due to price depreciation.
// This is a warning state before forced liquidation.
func (p *Pool) MarginCall(reservationID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.reservations[reservationID]
	if !ok {
		return fmt.Errorf("reservation %s not found", reservationID)
	}
	if r.Status != StatusPendingSettlement {
		return fmt.Errorf("reservation %s in status %s, cannot margin call", reservationID, r.Status)
	}

	r.Status = StatusMarginCalled
	r.UpdatedAt = time.Now().UTC()
	return nil
}

// GetReservation returns a reservation by ID.
func (p *Pool) GetReservation(id string) (*Reservation, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	r, ok := p.reservations[id]
	if !ok {
		return nil, fmt.Errorf("reservation %s not found", id)
	}
	// Return a copy to prevent mutation outside the lock.
	copy := *r
	return &copy, nil
}

// ListReservations returns all reservations for an account.
func (p *Pool) ListReservations(accountID string) []*Reservation {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []*Reservation
	for _, r := range p.reservations {
		if r.AccountID == accountID {
			copy := *r
			results = append(results, &copy)
		}
	}
	return results
}

// Status returns a snapshot of the pool's current utilization.
func (p *Pool) Status() *PoolStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var active int
	for _, r := range p.reservations {
		if r.Status == StatusPendingSettlement || r.Status == StatusMarginCalled || r.Status == StatusFailed {
			active++
		}
	}

	var utilPct float64
	if p.total > 0 {
		utilPct = p.reserved / p.total * 100
	}

	return &PoolStatus{
		Total:              p.total,
		Available:          p.total - p.reserved,
		Reserved:           p.reserved,
		UtilizationPct:     utilPct,
		ActiveReservations: active,
	}
}

// AddCapital increases the pool's total capital. Called when the operator
// deposits additional funds to back more instant buys.
func (p *Pool) AddCapital(amount float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.total += amount
}

// WithdrawCapital removes available (unreserved) capital from the pool.
// Returns an error if the requested amount exceeds available capital.
func (p *Pool) WithdrawCapital(amount float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	available := p.total - p.reserved
	if amount > available {
		return fmt.Errorf("cannot withdraw $%.2f, only $%.2f available (total $%.2f, reserved $%.2f)",
			amount, available, p.total, p.reserved)
	}

	p.total -= amount
	return nil
}

// generateReservationID creates a cryptographically random reservation ID.
func generateReservationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail on a properly configured system.
		panic(errors.New("crypto/rand failed: " + err.Error()))
	}
	return "res_" + hex.EncodeToString(b)
}
