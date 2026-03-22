package funding

import (
	"context"
	"strings"
	"testing"

	"github.com/hanzoai/commerce/payment/processor"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.registry == nil {
		t.Fatal("registry is nil")
	}
}

func TestNewWithRegistry(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)
	if s == nil {
		t.Fatal("NewWithRegistry returned nil")
	}
	if s.registry != r {
		t.Fatal("registry not set correctly")
	}
}

func TestDepositNoProcessors(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.Deposit(context.Background(), &DepositRequest{
		AccountID: "acct1",
		Provider:  "alpaca",
		Amount:    10000,
		Currency:  "usd",
	})
	if err == nil {
		t.Fatal("expected error when no processors available")
	}
	if !strings.Contains(err.Error(), "no processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithdrawNoProcessors(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.Withdraw(context.Background(), &WithdrawRequest{
		AccountID: "acct1",
		Provider:  "alpaca",
		Amount:    5000,
		Currency:  "usd",
	})
	if err == nil {
		t.Fatal("expected error when no processors available")
	}
	if !strings.Contains(err.Error(), "no processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWebhookUnknownProcessor(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.ValidateWebhook(context.Background(), "nonexistent", []byte("{}"), "sig")
	if err == nil {
		t.Fatal("expected error for unknown processor")
	}
	if !strings.Contains(err.Error(), "unknown processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListProcessorsEmpty(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	names := s.ListProcessors(context.Background())
	if len(names) != 0 {
		t.Fatalf("processors len = %d, want 0", len(names))
	}
}

func TestDepositCryptoNoProcessors(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.Deposit(context.Background(), &DepositRequest{
		AccountID:     "acct1",
		Provider:      "alpaca",
		Amount:        100000,
		Currency:      "btc",
		PaymentMethod: "crypto",
		TxHash:        "0xabc123",
		Chain:         "bitcoin",
		Address:       "bc1qexample",
	})
	if err == nil {
		t.Fatal("expected error when no processors available for crypto")
	}
	if !strings.Contains(err.Error(), "no processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithdrawCryptoNoProcessors(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.Withdraw(context.Background(), &WithdrawRequest{
		AccountID:   "acct1",
		Provider:    "alpaca",
		Amount:      50000,
		Currency:    "eth",
		DestAddress: "0xdef456",
		Chain:       "ethereum",
	})
	if err == nil {
		t.Fatal("expected error for crypto withdrawal with no processors")
	}
	if !strings.Contains(err.Error(), "no processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDepositWithToken(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.Deposit(context.Background(), &DepositRequest{
		AccountID:     "acct1",
		Provider:      "alpaca",
		Amount:        10000,
		Currency:      "usd",
		PaymentMethod: "card",
		Token:         "tok_test_123",
	})
	if err == nil {
		t.Fatal("expected error (no processor), but validating token path is exercised")
	}
	if !strings.Contains(err.Error(), "no processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWithdrawWithRelationship(t *testing.T) {
	r := processor.NewRegistry(nil)
	s := NewWithRegistry(r)

	_, err := s.Withdraw(context.Background(), &WithdrawRequest{
		AccountID:      "acct1",
		Provider:       "alpaca",
		Amount:         25000,
		Currency:       "usd",
		PaymentMethod:  "bank_transfer",
		RelationshipID: "rel-123",
	})
	if err == nil {
		t.Fatal("expected error (no processor)")
	}
	if !strings.Contains(err.Error(), "no processor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewUsesGlobalRegistry(t *testing.T) {
	s := New()
	if s.registry == nil {
		t.Fatal("expected non-nil registry from New()")
	}
	// ListProcessors should work even with global registry
	names := s.ListProcessors(context.Background())
	// May have processors or not depending on global state, but should not panic
	_ = names
}
