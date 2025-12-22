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
