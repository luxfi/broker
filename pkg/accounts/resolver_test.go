package accounts

import (
	"sync"
	"testing"
)

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
