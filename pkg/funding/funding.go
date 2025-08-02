package funding

import (
	"context"
	"fmt"
	"time"

	"github.com/hanzoai/commerce/models/types/currency"
	"github.com/hanzoai/commerce/payment/processor"
)

// Service handles deposit and withdrawal operations via payment processors.
type Service struct {
	registry *processor.Registry
}

// New creates a funding service using the global processor registry.
func New() *Service {
	return &Service{registry: processor.Global()}
}

// NewWithRegistry creates a funding service with a custom registry.
func NewWithRegistry(r *processor.Registry) *Service {
	return &Service{registry: r}
}

// DepositRequest represents a request to deposit funds into a trading account.
type DepositRequest struct {
	AccountID     string `json:"account_id"`
	Provider      string `json:"provider"`
	Amount        int64  `json:"amount"`          // cents
	Currency      string `json:"currency"`         // e.g. "usd", "btc"
	PaymentMethod string `json:"payment_method"`   // "card", "bank_transfer", "crypto"
	Token         string `json:"token,omitempty"`   // payment method nonce/token (card)
	TxHash        string `json:"tx_hash,omitempty"` // on-chain tx hash (crypto)
	Chain         string `json:"chain,omitempty"`   // blockchain network (crypto)
	Address       string `json:"address,omitempty"` // deposit address (crypto)
}

// WithdrawRequest represents a request to withdraw funds from a trading account.
type WithdrawRequest struct {
	AccountID      string `json:"account_id"`
	Provider       string `json:"provider"`
	Amount         int64  `json:"amount"`   // cents
	Currency       string `json:"currency"`
	PaymentMethod  string `json:"payment_method"` // "bank_transfer", "crypto"
	RelationshipID string `json:"relationship_id,omitempty"`
	DestAddress    string `json:"dest_address,omitempty"`
	Chain          string `json:"chain,omitempty"`
}

// Result is returned from deposit/withdraw operations.
type Result struct {
	ID            string `json:"id"`
	Status        string `json:"status"` // "pending", "completed", "failed", "authorized"
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	PaymentMethod string `json:"payment_method"`
	ProcessorRef  string `json:"processor_ref,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// Deposit processes a deposit using the appropriate payment processor.
func (s *Service) Deposit(ctx context.Context, req *DepositRequest) (*Result, error) {
	cur := currency.Type(req.Currency)
	isCrypto := processor.IsCryptoCurrency(cur)

	payReq := processor.PaymentRequest{
		Amount:   currency.Cents(req.Amount),
		Currency: cur,
		Metadata: map[string]interface{}{
			"account_id": req.AccountID,
			"provider":   req.Provider,
			"type":       "deposit",
		},
	}

	if req.Token != "" {
		payReq.Token = req.Token
	}
	if isCrypto {
		payReq.Address = req.Address
		payReq.Chain = req.Chain
	}

	proc, err := s.registry.SelectProcessor(ctx, payReq)
	if err != nil {
		return nil, fmt.Errorf("no processor available for %s: %w", req.Currency, err)
	}

	result, err := proc.Charge(ctx, payReq)
	if err != nil {
		return nil, fmt.Errorf("deposit charge failed: %w", err)
	}

	return &Result{
		ID:            result.TransactionID,
		Status:        result.Status,
		Amount:        int64(payReq.Amount),
		Currency:      req.Currency,
		PaymentMethod: req.PaymentMethod,
		ProcessorRef:  result.ProcessorRef,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Withdraw processes a withdrawal using the appropriate payment processor.
func (s *Service) Withdraw(ctx context.Context, req *WithdrawRequest) (*Result, error) {
	cur := currency.Type(req.Currency)

	payReq := processor.PaymentRequest{
		Amount:   currency.Cents(req.Amount),
		Currency: cur,
		Metadata: map[string]interface{}{
			"account_id":      req.AccountID,
			"provider":        req.Provider,
			"type":            "withdrawal",
			"relationship_id": req.RelationshipID,
			"dest_address":    req.DestAddress,
			"chain":           req.Chain,
		},
	}

	if req.DestAddress != "" {
		payReq.Address = req.DestAddress
		payReq.Chain = req.Chain
	}

	proc, err := s.registry.SelectProcessor(ctx, payReq)
	if err != nil {
		return nil, fmt.Errorf("no processor available for %s: %w", req.Currency, err)
	}

	result, err := proc.Charge(ctx, payReq)
	if err != nil {
		return nil, fmt.Errorf("withdrawal failed: %w", err)
	}

	return &Result{
		ID:            result.TransactionID,
		Status:        result.Status,
		Amount:        int64(payReq.Amount),
		Currency:      req.Currency,
		PaymentMethod: req.PaymentMethod,
		ProcessorRef:  result.ProcessorRef,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ValidateWebhook validates and parses an incoming payment webhook.
func (s *Service) ValidateWebhook(ctx context.Context, processorName string, payload []byte, signature string) (*processor.WebhookEvent, error) {
	pt := processor.ProcessorType(processorName)
	proc, err := s.registry.Get(pt)
	if err != nil {
		return nil, fmt.Errorf("unknown processor %q: %w", processorName, err)
	}
	return proc.ValidateWebhook(ctx, payload, signature)
}

// ListProcessors returns available processor type names.
func (s *Service) ListProcessors(ctx context.Context) []string {
	procs := s.registry.Available(ctx)
	names := make([]string, len(procs))
	for i, p := range procs {
		names[i] = string(p.Type())
	}
	return names
}
