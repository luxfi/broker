// Package custody manages asset custody across multiple custodians (Fireblocks,
// BitGo, etc.) and self-custody wallets. It maintains the position of record,
// tracks inter-custodian transfers, and runs reconciliation against provider
// balances. Required for BD regulatory compliance (SEC Rule 15c3-3).
package custody

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CustodianType categorizes the kind of custodial arrangement.
type CustodianType string

const (
	CustodianPrime   CustodianType = "prime"   // prime broker (e.g., Alpaca, IBKR)
	CustodianCrypto  CustodianType = "crypto"  // crypto custodian (e.g., Fireblocks, BitGo)
	CustodianSelf    CustodianType = "self"    // self-custody wallet
	CustodianBank    CustodianType = "bank"    // fiat bank custody
)

// Custodian represents a registered custody provider.
type Custodian struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`      // e.g., "fireblocks", "bitgo", "alpaca"
	Type     CustodianType `json:"type"`
	Status   string        `json:"status"`    // active, suspended, decommissioned
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Holding is the position of record for an asset at a specific custodian.
type Holding struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	CustodianID string    `json:"custodian_id"`
	Asset       string    `json:"asset"`      // symbol or currency
	Quantity    float64   `json:"quantity"`
	CostBasis   float64   `json:"cost_basis"` // USD cost basis for tax reporting
	UpdatedAt   time.Time `json:"updated_at"`
}

// TransferStatus tracks the state of an inter-custodian transfer.
type TransferStatus string

const (
	TransferPending   TransferStatus = "pending"
	TransferConfirmed TransferStatus = "confirmed"
	TransferFailed    TransferStatus = "failed"
	TransferCancelled TransferStatus = "cancelled"
)

// Transfer represents movement of assets between custodians.
type Transfer struct {
	ID              string         `json:"id"`
	AccountID       string         `json:"account_id"`
	FromCustodianID string         `json:"from_custodian_id"`
	ToCustodianID   string         `json:"to_custodian_id"`
	Asset           string         `json:"asset"`
	Quantity        float64        `json:"quantity"`
	Status          TransferStatus `json:"status"`
	ExternalRef     string         `json:"external_ref,omitempty"` // provider-side transfer ID
	CreatedAt       time.Time      `json:"created_at"`
	ConfirmedAt     *time.Time     `json:"confirmed_at,omitempty"`
}

// ReconciliationResult is the outcome of comparing our records against a custodian.
type ReconciliationResult struct {
	CustodianID string                 `json:"custodian_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Matched     int                    `json:"matched"`
	Mismatched  int                    `json:"mismatched"`
	Breaks      []ReconciliationBreak  `json:"breaks,omitempty"`
}

// ReconciliationBreak is a discrepancy between our records and the custodian.
type ReconciliationBreak struct {
	AccountID string  `json:"account_id"`
	Asset     string  `json:"asset"`
	OurQty    float64 `json:"our_qty"`
	TheirQty  float64 `json:"their_qty"`
	Delta     float64 `json:"delta"`
}

// BalanceFetcher retrieves current balances from a custody provider.
// Implementations wrap provider-specific APIs (Fireblocks, BitGo, etc.).
type BalanceFetcher func(ctx context.Context, custodianID string) (map[string]map[string]float64, error)
// Returns: accountID -> asset -> quantity

// Service manages custody state, inter-custodian transfers, and reconciliation.
type Service struct {
	mu         sync.RWMutex
	custodians map[string]*Custodian            // custodianID -> custodian
	holdings   map[holdingKey]*Holding           // (account, custodian, asset) -> holding
	transfers  map[string]*Transfer              // transferID -> transfer
	nextID     int
}

type holdingKey struct {
	accountID   string
	custodianID string
	asset       string
}

// NewService creates a custody management service.
func NewService() *Service {
	return &Service{
		custodians: make(map[string]*Custodian),
		holdings:   make(map[holdingKey]*Holding),
		transfers:  make(map[string]*Transfer),
	}
}

// RegisterCustodian adds a custodian to the registry.
func (s *Service) RegisterCustodian(c Custodian) error {
	if c.ID == "" || c.Name == "" {
		return fmt.Errorf("custodian id and name are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.custodians[c.ID]; exists {
		return fmt.Errorf("custodian %s already registered", c.ID)
	}
	s.custodians[c.ID] = &c
	return nil
}

// GetCustodian returns a registered custodian by ID.
func (s *Service) GetCustodian(id string) (*Custodian, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.custodians[id]
	if !ok {
		return nil, fmt.Errorf("custodian %s not found", id)
	}
	cp := *c
	return &cp, nil
}

// ListCustodians returns all registered custodians.
func (s *Service) ListCustodians() []*Custodian {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Custodian, 0, len(s.custodians))
	for _, c := range s.custodians {
		cp := *c
		result = append(result, &cp)
	}
	return result
}

// RecordHolding updates the position of record for an asset at a custodian.
// This is the BD's authoritative ledger of what's held where.
func (s *Service) RecordHolding(accountID, custodianID, asset string, quantity, costBasis float64) error {
	if accountID == "" || custodianID == "" || asset == "" {
		return fmt.Errorf("account_id, custodian_id, and asset are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.custodians[custodianID]; !ok {
		return fmt.Errorf("custodian %s not registered", custodianID)
	}

	key := holdingKey{accountID: accountID, custodianID: custodianID, asset: asset}
	s.nextID++
	s.holdings[key] = &Holding{
		ID:          fmt.Sprintf("hold_%d", s.nextID),
		AccountID:   accountID,
		CustodianID: custodianID,
		Asset:       asset,
		Quantity:    quantity,
		CostBasis:   costBasis,
		UpdatedAt:   time.Now().UTC(),
	}
	return nil
}

// GetHolding returns the position of record for a specific account/custodian/asset.
func (s *Service) GetHolding(accountID, custodianID, asset string) (*Holding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := holdingKey{accountID: accountID, custodianID: custodianID, asset: asset}
	h, ok := s.holdings[key]
	if !ok {
		return nil, fmt.Errorf("no holding for account %s, custodian %s, asset %s", accountID, custodianID, asset)
	}
	cp := *h
	return &cp, nil
}

// ListHoldings returns all holdings for an account across all custodians.
func (s *Service) ListHoldings(accountID string) []*Holding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Holding
	for _, h := range s.holdings {
		if h.AccountID == accountID {
			cp := *h
			result = append(result, &cp)
		}
	}
	return result
}

// ListHoldingsByCustodian returns all holdings at a specific custodian.
func (s *Service) ListHoldingsByCustodian(custodianID string) []*Holding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Holding
	for _, h := range s.holdings {
		if h.CustodianID == custodianID {
			cp := *h
			result = append(result, &cp)
		}
	}
	return result
}

// InitiateTransfer creates a pending inter-custodian transfer.
// The caller is responsible for executing the actual transfer at each provider.
func (s *Service) InitiateTransfer(accountID, fromCustodianID, toCustodianID, asset string, quantity float64) (*Transfer, error) {
	if accountID == "" || fromCustodianID == "" || toCustodianID == "" || asset == "" {
		return nil, fmt.Errorf("account_id, from_custodian_id, to_custodian_id, and asset are required")
	}
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be positive")
	}
	if fromCustodianID == toCustodianID {
		return nil, fmt.Errorf("from and to custodian must differ")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.custodians[fromCustodianID]; !ok {
		return nil, fmt.Errorf("source custodian %s not registered", fromCustodianID)
	}
	if _, ok := s.custodians[toCustodianID]; !ok {
		return nil, fmt.Errorf("destination custodian %s not registered", toCustodianID)
	}

	// Verify sufficient holdings at source.
	key := holdingKey{accountID: accountID, custodianID: fromCustodianID, asset: asset}
	h, ok := s.holdings[key]
	if !ok || h.Quantity < quantity {
		avail := 0.0
		if ok {
			avail = h.Quantity
		}
		return nil, fmt.Errorf("insufficient holdings: need %.8f %s, have %.8f at %s",
			quantity, asset, avail, fromCustodianID)
	}

	s.nextID++
	t := &Transfer{
		ID:              fmt.Sprintf("xfr_%d", s.nextID),
		AccountID:       accountID,
		FromCustodianID: fromCustodianID,
		ToCustodianID:   toCustodianID,
		Asset:           asset,
		Quantity:        quantity,
		Status:          TransferPending,
		CreatedAt:       time.Now().UTC(),
	}
	s.transfers[t.ID] = t
	return t, nil
}

// ConfirmTransfer marks a transfer as confirmed and updates holdings.
// Called when the actual custodian-level transfer completes.
func (s *Service) ConfirmTransfer(transferID, externalRef string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.transfers[transferID]
	if !ok {
		return fmt.Errorf("transfer %s not found", transferID)
	}
	if t.Status != TransferPending {
		return fmt.Errorf("transfer %s in status %s, cannot confirm", transferID, t.Status)
	}

	now := time.Now().UTC()
	t.Status = TransferConfirmed
	t.ExternalRef = externalRef
	t.ConfirmedAt = &now

	// Debit source holding.
	fromKey := holdingKey{accountID: t.AccountID, custodianID: t.FromCustodianID, asset: t.Asset}
	if h, ok := s.holdings[fromKey]; ok {
		h.Quantity -= t.Quantity
		h.UpdatedAt = now
	}

	// Credit destination holding (create if needed).
	toKey := holdingKey{accountID: t.AccountID, custodianID: t.ToCustodianID, asset: t.Asset}
	if h, ok := s.holdings[toKey]; ok {
		h.Quantity += t.Quantity
		h.UpdatedAt = now
	} else {
		s.nextID++
		s.holdings[toKey] = &Holding{
			ID:          fmt.Sprintf("hold_%d", s.nextID),
			AccountID:   t.AccountID,
			CustodianID: t.ToCustodianID,
			Asset:       t.Asset,
			Quantity:    t.Quantity,
			UpdatedAt:   now,
		}
	}

	return nil
}

// FailTransfer marks a transfer as failed. No holding changes are made.
func (s *Service) FailTransfer(transferID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.transfers[transferID]
	if !ok {
		return fmt.Errorf("transfer %s not found", transferID)
	}
	if t.Status != TransferPending {
		return fmt.Errorf("transfer %s in status %s, cannot fail", transferID, t.Status)
	}

	t.Status = TransferFailed
	return nil
}

// GetTransfer returns a transfer by ID.
func (s *Service) GetTransfer(id string) (*Transfer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.transfers[id]
	if !ok {
		return nil, fmt.Errorf("transfer %s not found", id)
	}
	cp := *t
	return &cp, nil
}

// ListTransfers returns all transfers for an account.
func (s *Service) ListTransfers(accountID string) []*Transfer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Transfer
	for _, t := range s.transfers {
		if t.AccountID == accountID {
			cp := *t
			result = append(result, &cp)
		}
	}
	return result
}

// Reconcile compares our holdings against actual balances from a custodian.
// The fetchFn retrieves current balances from the custodian API.
func (s *Service) Reconcile(ctx context.Context, custodianID string, fetchFn BalanceFetcher) (*ReconciliationResult, error) {
	actual, err := fetchFn(ctx, custodianID)
	if err != nil {
		return nil, fmt.Errorf("fetch balances from %s: %w", custodianID, err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := &ReconciliationResult{
		CustodianID: custodianID,
		Timestamp:   time.Now().UTC(),
	}

	// Build map of our holdings at this custodian.
	ourHoldings := make(map[string]map[string]float64) // accountID -> asset -> qty
	for key, h := range s.holdings {
		if key.custodianID != custodianID {
			continue
		}
		if ourHoldings[h.AccountID] == nil {
			ourHoldings[h.AccountID] = make(map[string]float64)
		}
		ourHoldings[h.AccountID][h.Asset] = h.Quantity
	}

	// Compare against actual.
	seen := make(map[string]map[string]bool)
	for acct, assets := range actual {
		if seen[acct] == nil {
			seen[acct] = make(map[string]bool)
		}
		for asset, theirQty := range assets {
			seen[acct][asset] = true
			ourQty := 0.0
			if ours, ok := ourHoldings[acct]; ok {
				ourQty = ours[asset]
			}
			if ourQty == theirQty {
				result.Matched++
			} else {
				result.Mismatched++
				result.Breaks = append(result.Breaks, ReconciliationBreak{
					AccountID: acct,
					Asset:     asset,
					OurQty:    ourQty,
					TheirQty:  theirQty,
					Delta:     ourQty - theirQty,
				})
			}
		}
	}

	// Check for holdings we have that the custodian doesn't.
	for acct, assets := range ourHoldings {
		for asset, ourQty := range assets {
			if ourQty == 0 {
				continue
			}
			if seen[acct] == nil || !seen[acct][asset] {
				result.Mismatched++
				result.Breaks = append(result.Breaks, ReconciliationBreak{
					AccountID: acct,
					Asset:     asset,
					OurQty:    ourQty,
					TheirQty:  0,
					Delta:     ourQty,
				})
			}
		}
	}

	return result, nil
}
