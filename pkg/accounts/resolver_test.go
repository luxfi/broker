package accounts

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// mockProviderForResolver implements provider.Provider for testing AutoDiscover.
type mockProviderForResolver struct {
	name     string
	accounts []*types.Account
	listErr  error
}

func (m *mockProviderForResolver) Name() string { return m.name }
func (m *mockProviderForResolver) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetAccount(_ context.Context, _ string) (*types.Account, error) {
	return nil, nil
}
func (m *mockProviderForResolver) ListAccounts(_ context.Context) ([]*types.Account, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.accounts, nil
}
func (m *mockProviderForResolver) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error) {
	return nil, nil
}
func (m *mockProviderForResolver) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) {
	return nil, nil
}
func (m *mockProviderForResolver) ListOrders(_ context.Context, _ string) ([]*types.Order, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetOrder(_ context.Context, _, _ string) (*types.Order, error) {
	return nil, nil
}
func (m *mockProviderForResolver) CancelOrder(_ context.Context, _, _ string) error { return nil }
func (m *mockProviderForResolver) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, nil
}
func (m *mockProviderForResolver) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return nil, nil
}
func (m *mockProviderForResolver) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, nil
}
func (m *mockProviderForResolver) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return nil, nil
}
func (m *mockProviderForResolver) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetAsset(_ context.Context, _ string) (*types.Asset, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, nil
}
func (m *mockProviderForResolver) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, nil
}

func TestSetGetMapping(t *testing.T) {
	r := NewResolver()
	r.SetMapping("user1", "org1", "alpaca", "acct-abc")

	got, ok := r.ResolveAccount("user1", "alpaca")
	if !ok {
		t.Fatal("expected mapping to exist")
	}
	if got != "acct-abc" {
		t.Fatalf("expected acct-abc, got %s", got)
	}

	// Nonexistent lookup returns false.
	_, ok = r.ResolveAccount("user1", "ibkr")
	if ok {
		t.Fatal("expected no mapping for ibkr")
	}
}

func TestSetMappingOverwrite(t *testing.T) {
	r := NewResolver()
	r.SetMapping("user1", "org1", "alpaca", "acct-old")
	r.SetMapping("user1", "org1", "alpaca", "acct-new")

	got, ok := r.ResolveAccount("user1", "alpaca")
	if !ok {
		t.Fatal("expected mapping to exist after overwrite")
	}
	if got != "acct-new" {
		t.Fatalf("expected acct-new, got %s", got)
	}

	// Should have exactly one mapping, not two.
	mappings := r.ListMappings("user1")
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping after overwrite, got %d", len(mappings))
	}
}

func TestRemoveMapping(t *testing.T) {
	r := NewResolver()
	r.SetMapping("user1", "org1", "alpaca", "acct-abc")
	r.RemoveMapping("user1", "alpaca")

	_, ok := r.ResolveAccount("user1", "alpaca")
	if ok {
		t.Fatal("expected mapping to be removed")
	}

	mappings := r.ListMappings("user1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}

	// Removing nonexistent mapping is a no-op.
	r.RemoveMapping("user1", "alpaca")
}

func TestResolveAnyAccount(t *testing.T) {
	r := NewResolver()

	// No mappings returns false.
	_, _, ok := r.ResolveAnyAccount("user1")
	if ok {
		t.Fatal("expected no account for unknown user")
	}

	r.SetMapping("user1", "org1", "alpaca", "acct-abc")

	prov, acctID, ok := r.ResolveAnyAccount("user1")
	if !ok {
		t.Fatal("expected to resolve any account")
	}
	if prov != "alpaca" || acctID != "acct-abc" {
		t.Fatalf("unexpected result: provider=%s account=%s", prov, acctID)
	}
}

func TestListMappings(t *testing.T) {
	r := NewResolver()
	r.SetMapping("user1", "org1", "alpaca", "acct-a")
	r.SetMapping("user1", "org1", "ibkr", "acct-b")
	r.SetMapping("user1", "org1", "coinbase", "acct-c")

	mappings := r.ListMappings("user1")
	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mappings))
	}

	// Verify returned slice is a copy (modifying it doesn't affect internal state).
	mappings[0].AccountID = "tampered"
	original := r.ListMappings("user1")
	if original[0].AccountID == "tampered" {
		t.Fatal("ListMappings returned a reference, not a copy")
	}

	// Unknown user returns nil.
	if got := r.ListMappings("nobody"); got != nil {
		t.Fatalf("expected nil for unknown user, got %v", got)
	}
}

func TestMultipleProvidersPerUser(t *testing.T) {
	r := NewResolver()
	r.SetMapping("user1", "org1", "alpaca", "acct-a")
	r.SetMapping("user1", "org1", "ibkr", "acct-b")

	a, ok := r.ResolveAccount("user1", "alpaca")
	if !ok || a != "acct-a" {
		t.Fatalf("alpaca: expected acct-a, got %s (ok=%v)", a, ok)
	}

	b, ok := r.ResolveAccount("user1", "ibkr")
	if !ok || b != "acct-b" {
		t.Fatalf("ibkr: expected acct-b, got %s (ok=%v)", b, ok)
	}

	// Remove one, the other remains.
	r.RemoveMapping("user1", "alpaca")
	_, ok = r.ResolveAccount("user1", "alpaca")
	if ok {
		t.Fatal("alpaca mapping should be gone")
	}
	b, ok = r.ResolveAccount("user1", "ibkr")
	if !ok || b != "acct-b" {
		t.Fatal("ibkr mapping should still exist")
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewResolver()
	var wg sync.WaitGroup

	// Concurrent writes.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			userID := "user1"
			provider := "alpaca"
			if n%2 == 0 {
				provider = "ibkr"
			}
			r.SetMapping(userID, "org1", provider, "acct")
		}(i)
	}

	// Concurrent reads interleaved with writes.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.ResolveAccount("user1", "alpaca")
			r.ResolveAnyAccount("user1")
			r.ListMappings("user1")
		}()
	}

	// Concurrent removes.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			provider := "alpaca"
			if n%2 == 0 {
				provider = "ibkr"
			}
			r.RemoveMapping("user1", provider)
		}(i)
	}

	wg.Wait()

	// No race conditions — if we got here without -race failing, it's correct.
}

func TestAutoDiscover(t *testing.T) {
	mp := &mockProviderForResolver{
		name: "alpaca",
		accounts: []*types.Account{
			{ProviderID: "acct-1", UserID: "user1", OrgID: "org1"},
			{ProviderID: "acct-2", UserID: "user2", OrgID: "org2"},
		},
	}
	mp2 := &mockProviderForResolver{
		name: "ibkr",
		accounts: []*types.Account{
			{ProviderID: "ibkr-1", UserID: "user1", OrgID: "org1"},
		},
	}

	registry := provider.NewRegistry()
	registry.Register(mp)
	registry.Register(mp2)

	r := NewResolver()
	err := r.AutoDiscover(context.Background(), registry, "user1", "org1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// user1 should have accounts from both providers
	mappings := r.ListMappings("user1")
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings for user1, got %d", len(mappings))
	}

	acctA, ok := r.ResolveAccount("user1", "alpaca")
	if !ok || acctA != "acct-1" {
		t.Fatalf("expected alpaca acct-1, got %s (ok=%v)", acctA, ok)
	}

	acctB, ok := r.ResolveAccount("user1", "ibkr")
	if !ok || acctB != "ibkr-1" {
		t.Fatalf("expected ibkr ibkr-1, got %s (ok=%v)", acctB, ok)
	}
}

func TestAutoDiscoverByOrgID(t *testing.T) {
	// Account belongs to org but has a different userID
	mp := &mockProviderForResolver{
		name: "alpaca",
		accounts: []*types.Account{
			{ProviderID: "acct-org", UserID: "other-user", OrgID: "shared-org"},
		},
	}

	registry := provider.NewRegistry()
	registry.Register(mp)

	r := NewResolver()
	err := r.AutoDiscover(context.Background(), registry, "user1", "shared-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should match by orgID
	acct, ok := r.ResolveAccount("user1", "alpaca")
	if !ok || acct != "acct-org" {
		t.Fatalf("expected acct-org via orgID match, got %s (ok=%v)", acct, ok)
	}
}

func TestAutoDiscoverNoMatchingAccounts(t *testing.T) {
	mp := &mockProviderForResolver{
		name: "alpaca",
		accounts: []*types.Account{
			{ProviderID: "acct-1", UserID: "other-user", OrgID: "other-org"},
		},
	}

	registry := provider.NewRegistry()
	registry.Register(mp)

	r := NewResolver()
	err := r.AutoDiscover(context.Background(), registry, "user1", "org1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mappings := r.ListMappings("user1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}
}

func TestAutoDiscoverProviderError(t *testing.T) {
	mp := &mockProviderForResolver{
		name:     "broken",
		listErr:  fmt.Errorf("connection refused"),
	}

	registry := provider.NewRegistry()
	registry.Register(mp)

	r := NewResolver()
	err := r.AutoDiscover(context.Background(), registry, "user1", "org1")
	if err == nil {
		t.Fatal("expected error when provider fails")
	}
	if !strings.Contains(err.Error(), "1 errors") {
		t.Fatalf("expected error count in message, got: %v", err)
	}
}

func TestAutoDiscoverEmptyRegistry(t *testing.T) {
	registry := provider.NewRegistry()
	r := NewResolver()
	err := r.AutoDiscover(context.Background(), registry, "user1", "org1")
	if err != nil {
		t.Fatalf("expected no error for empty registry, got: %v", err)
	}
}

func TestImportFromJWT(t *testing.T) {
	r := NewResolver()
	payload := map[string]interface{}{
		"sub":   "user1",
		"owner": "org1",
		"accounts": map[string]interface{}{
			"alpaca": "acct-abc",
			"ibkr":   "acct-xyz",
		},
	}

	err := r.ImportFromJWT(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	acctA, ok := r.ResolveAccount("user1", "alpaca")
	if !ok || acctA != "acct-abc" {
		t.Fatalf("expected alpaca acct-abc, got %s (ok=%v)", acctA, ok)
	}
	acctB, ok := r.ResolveAccount("user1", "ibkr")
	if !ok || acctB != "acct-xyz" {
		t.Fatalf("expected ibkr acct-xyz, got %s (ok=%v)", acctB, ok)
	}

	// Verify org was set
	mappings := r.ListMappings("user1")
	for _, m := range mappings {
		if m.OrgID != "org1" {
			t.Fatalf("expected orgID org1, got %s", m.OrgID)
		}
	}
}

func TestImportFromJWTMissingSub(t *testing.T) {
	r := NewResolver()
	err := r.ImportFromJWT(map[string]interface{}{
		"owner": "org1",
	})
	if err == nil {
		t.Fatal("expected error for missing sub claim")
	}
	if !strings.Contains(err.Error(), "sub") {
		t.Fatalf("expected error about 'sub', got: %v", err)
	}
}

func TestImportFromJWTNoAccounts(t *testing.T) {
	r := NewResolver()
	err := r.ImportFromJWT(map[string]interface{}{
		"sub":   "user1",
		"owner": "org1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No accounts key means no mappings created, but no error
	mappings := r.ListMappings("user1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 mappings, got %d", len(mappings))
	}
}

func TestImportFromJWTNoOwner(t *testing.T) {
	r := NewResolver()
	err := r.ImportFromJWT(map[string]interface{}{
		"sub": "user1",
		"accounts": map[string]interface{}{
			"alpaca": "acct-1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	acct, ok := r.ResolveAccount("user1", "alpaca")
	if !ok || acct != "acct-1" {
		t.Fatalf("expected acct-1, got %s (ok=%v)", acct, ok)
	}
	// OrgID should be empty
	mappings := r.ListMappings("user1")
	if mappings[0].OrgID != "" {
		t.Fatalf("expected empty orgID, got %s", mappings[0].OrgID)
	}
}

func TestImportFromJWTNonStringAccountValue(t *testing.T) {
	r := NewResolver()
	err := r.ImportFromJWT(map[string]interface{}{
		"sub": "user1",
		"accounts": map[string]interface{}{
			"alpaca": "acct-good",
			"broken": 12345, // non-string value, should be skipped
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := r.ResolveAccount("user1", "broken")
	if ok {
		t.Fatal("expected non-string account value to be skipped")
	}
	acct, ok := r.ResolveAccount("user1", "alpaca")
	if !ok || acct != "acct-good" {
		t.Fatalf("expected acct-good, got %s", acct)
	}
}

func TestImportFromJWTEmptySub(t *testing.T) {
	r := NewResolver()
	err := r.ImportFromJWT(map[string]interface{}{
		"sub": "",
	})
	if err == nil {
		t.Fatal("expected error for empty sub")
	}
}

func TestMappingCreatedAtIsSet(t *testing.T) {
	r := NewResolver()
	before := time.Now()
	r.SetMapping("user1", "org1", "alpaca", "acct-1")
	after := time.Now()

	mappings := r.ListMappings("user1")
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}
	if mappings[0].CreatedAt.Before(before) || mappings[0].CreatedAt.After(after) {
		t.Fatalf("CreatedAt %v not between %v and %v", mappings[0].CreatedAt, before, after)
	}
}
