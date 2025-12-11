package settlement

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- Pool Tests ---

func TestNewPool(t *testing.T) {
	cfg := PoolConfig{
		MaxPoolSize:           100_000,
		MaxPerUser:            10_000,
		MaxPerTransaction:     5_000,
		UtilizationWarningPct: 0.80,
	}
	pool := NewPool(cfg)
	if pool == nil {
		t.Fatal("NewPool returned nil")
	}

	status := pool.Status()
	if status.Total != 0 {
		t.Errorf("expected total=0, got %f", status.Total)
	}
	if status.Available != 0 {
		t.Errorf("expected available=0, got %f", status.Available)
	}
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0, got %f", status.Reserved)
	}
}

func TestPoolAddWithdrawCapital(t *testing.T) {
	pool := NewPool(PoolConfig{})
	pool.AddCapital(50_000)

	status := pool.Status()
	if status.Total != 50_000 {
		t.Errorf("expected total=50000, got %f", status.Total)
	}
	if status.Available != 50_000 {
		t.Errorf("expected available=50000, got %f", status.Available)
	}

	// Withdraw some
	if err := pool.WithdrawCapital(20_000); err != nil {
		t.Fatalf("withdraw failed: %v", err)
	}
	status = pool.Status()
	if status.Total != 30_000 {
		t.Errorf("expected total=30000, got %f", status.Total)
	}

	// Cannot withdraw more than available
	err := pool.WithdrawCapital(40_000)
	if err == nil {
		t.Fatal("expected error withdrawing more than available")
	}
}

func TestPoolWithdrawCapitalWithReservation(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(50_000)

	// Reserve 30k
	_, err := pool.Reserve("acct1", "AAPL", 30_000, 150.0)
	if err != nil {
		t.Fatalf("reserve failed: %v", err)
	}

	// Can only withdraw 20k (50k - 30k reserved)
	err = pool.WithdrawCapital(25_000)
	if err == nil {
		t.Fatal("expected error withdrawing into reserved capital")
	}

	if err := pool.WithdrawCapital(20_000); err != nil {
		t.Fatalf("withdraw of available capital failed: %v", err)
	}
}

func TestReserveSuccess(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:            10_000,
		MaxPerTransaction:     5_000,
		UtilizationWarningPct: 0.80,
	}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)

	r, err := pool.Reserve("acct1", "AAPL", 1000, 150.0)
	if err != nil {
		t.Fatalf("reserve failed: %v", err)
	}
	if r.AccountID != "acct1" {
		t.Errorf("expected accountID=acct1, got %s", r.AccountID)
	}
	if r.Amount != 1000 {
		t.Errorf("expected amount=1000, got %f", r.Amount)
	}
	if r.Status != StatusPendingSettlement {
		t.Errorf("expected status=pending_settlement, got %s", r.Status)
	}
	if r.ID == "" {
		t.Error("expected non-empty reservation ID")
	}
	if len(r.ID) < 10 {
		t.Error("reservation ID too short, expected crypto/rand ID")
	}

	status := pool.Status()
	if status.Reserved != 1000 {
		t.Errorf("expected reserved=1000, got %f", status.Reserved)
	}
	if status.Available != 99_000 {
		t.Errorf("expected available=99000, got %f", status.Available)
	}
}

func TestReservePerTransactionLimit(t *testing.T) {
	cfg := PoolConfig{MaxPerTransaction: 5_000}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)

	_, err := pool.Reserve("acct1", "AAPL", 6_000, 150.0)
	if err == nil {
		t.Fatal("expected per-transaction limit error")
	}
}

func TestReservePerUserLimit(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 5_000, MaxPerTransaction: 10_000}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)

	// First reservation: 3000
	_, err := pool.Reserve("acct1", "AAPL", 3_000, 150.0)
	if err != nil {
		t.Fatalf("first reserve failed: %v", err)
	}

	// Second reservation: 3000 would push to 6000, over 5000 limit
	_, err = pool.Reserve("acct1", "GOOG", 3_000, 2800.0)
	if err == nil {
		t.Fatal("expected per-user limit error")
	}

	// Different user should succeed
	_, err = pool.Reserve("acct2", "GOOG", 3_000, 2800.0)
	if err != nil {
		t.Fatalf("different user reserve failed: %v", err)
	}
}

func TestReserveUtilizationLimit(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:            100_000,
		MaxPerTransaction:     100_000,
		UtilizationWarningPct: 0.50, // only allow 50% utilization
	}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)

	// 40k should work (40% of 100k)
	_, err := pool.Reserve("acct1", "AAPL", 40_000, 150.0)
	if err != nil {
		t.Fatalf("first reserve failed: %v", err)
	}

	// 20k more would push to 60%, over 50% limit
	_, err = pool.Reserve("acct2", "GOOG", 20_000, 2800.0)
	if err == nil {
		t.Fatal("expected utilization limit error")
	}

	// 10k should work (50k total = exactly 50%)
	_, err = pool.Reserve("acct2", "GOOG", 10_000, 2800.0)
	if err != nil {
		t.Fatalf("reserve at exactly limit failed: %v", err)
	}
}

func TestReserveInsufficientCapital(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 100_000, MaxPerTransaction: 100_000}
	pool := NewPool(cfg)
	pool.AddCapital(1_000)

	_, err := pool.Reserve("acct1", "AAPL", 2_000, 150.0)
	if err == nil {
		t.Fatal("expected insufficient capital error")
	}
}

func TestSettleReservation(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	r, _ := pool.Reserve("acct1", "AAPL", 5_000, 150.0)

	if err := pool.Settle(r.ID); err != nil {
		t.Fatalf("settle failed: %v", err)
	}

	// After settlement, reserved funds should be released
	status := pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after settle, got %f", status.Reserved)
	}
	if status.Available != 100_000 {
		t.Errorf("expected available=100000 after settle, got %f", status.Available)
	}

	// Verify reservation status
	settled, _ := pool.GetReservation(r.ID)
	if settled.Status != StatusSettled {
		t.Errorf("expected status=settled, got %s", settled.Status)
	}
	if settled.SettledAt == nil {
		t.Error("expected settled_at to be set")
	}
}

func TestSettleInvalidStatus(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	r, _ := pool.Reserve("acct1", "AAPL", 5_000, 150.0)
	pool.Settle(r.ID)

	// Settling again should fail
	err := pool.Settle(r.ID)
	if err == nil {
		t.Fatal("expected error settling already-settled reservation")
	}
}

func TestSettleNotFound(t *testing.T) {
	pool := NewPool(PoolConfig{})
	err := pool.Settle("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent reservation")
	}
}

func TestFailAndLiquidate(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	r, _ := pool.Reserve("acct1", "AAPL", 5_000, 150.0)

	// ACH fails
	if err := pool.Fail(r.ID); err != nil {
		t.Fatalf("fail failed: %v", err)
	}

	// Funds should still be reserved (we hold the asset but need to liquidate)
	status := pool.Status()
	if status.Reserved != 5_000 {
		t.Errorf("expected reserved=5000 after fail, got %f", status.Reserved)
	}

	failed, _ := pool.GetReservation(r.ID)
	if failed.Status != StatusFailed {
		t.Errorf("expected status=failed, got %s", failed.Status)
	}

	// Liquidate: recover 4500 (10% loss)
	if err := pool.Liquidate(r.ID, 4_500); err != nil {
		t.Fatalf("liquidate failed: %v", err)
	}

	// Reserved should be released after liquidation
	status = pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after liquidation, got %f", status.Reserved)
	}

	liquidated, _ := pool.GetReservation(r.ID)
	if liquidated.Status != StatusLiquidated {
		t.Errorf("expected status=liquidated, got %s", liquidated.Status)
	}
}

func TestMarginCallAndSettle(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	r, _ := pool.Reserve("acct1", "AAPL", 5_000, 150.0)

	// Margin call
	if err := pool.MarginCall(r.ID); err != nil {
		t.Fatalf("margin call failed: %v", err)
	}

	mc, _ := pool.GetReservation(r.ID)
	if mc.Status != StatusMarginCalled {
		t.Errorf("expected status=margin_called, got %s", mc.Status)
	}

	// Should be able to settle from margin_called state (user deposited funds)
	if err := pool.Settle(r.ID); err != nil {
		t.Fatalf("settle from margin_called failed: %v", err)
	}
}

func TestMarginCallAndLiquidate(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	r, _ := pool.Reserve("acct1", "AAPL", 5_000, 150.0)

	pool.MarginCall(r.ID)

	// Liquidate from margin_called state
	if err := pool.Liquidate(r.ID, 3_500); err != nil {
		t.Fatalf("liquidate from margin_called failed: %v", err)
	}

	status := pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after liquidation, got %f", status.Reserved)
	}
}

func TestGetReservation(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	r, _ := pool.Reserve("acct1", "AAPL", 5_000, 150.0)

	got, err := pool.GetReservation(r.ID)
	if err != nil {
		t.Fatalf("get reservation failed: %v", err)
	}
	if got.ID != r.ID {
		t.Errorf("expected ID=%s, got %s", r.ID, got.ID)
	}

	// Verify it's a copy (mutation shouldn't affect internal state)
	got.Amount = 999
	original, _ := pool.GetReservation(r.ID)
	if original.Amount != 5_000 {
		t.Error("GetReservation did not return a copy — internal state was mutated")
	}
}

func TestGetReservationNotFound(t *testing.T) {
	pool := NewPool(PoolConfig{})
	_, err := pool.GetReservation("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent reservation")
	}
}

func TestListReservations(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(100_000)

	pool.Reserve("acct1", "AAPL", 1_000, 150.0)
	pool.Reserve("acct1", "GOOG", 2_000, 2800.0)
	pool.Reserve("acct2", "TSLA", 3_000, 250.0)

	acct1 := pool.ListReservations("acct1")
	if len(acct1) != 2 {
		t.Errorf("expected 2 reservations for acct1, got %d", len(acct1))
	}

	acct2 := pool.ListReservations("acct2")
	if len(acct2) != 1 {
		t.Errorf("expected 1 reservation for acct2, got %d", len(acct2))
	}

	none := pool.ListReservations("nonexistent")
	if len(none) != 0 {
		t.Errorf("expected 0 reservations for nonexistent, got %d", len(none))
	}
}

func TestPoolStatus(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)

	r1, _ := pool.Reserve("acct1", "AAPL", 10_000, 150.0)
	pool.Reserve("acct2", "GOOG", 20_000, 2800.0)

	status := pool.Status()
	if status.Total != 100_000 {
		t.Errorf("expected total=100000, got %f", status.Total)
	}
	if status.Reserved != 30_000 {
		t.Errorf("expected reserved=30000, got %f", status.Reserved)
	}
	if status.Available != 70_000 {
		t.Errorf("expected available=70000, got %f", status.Available)
	}
	if status.ActiveReservations != 2 {
		t.Errorf("expected 2 active reservations, got %d", status.ActiveReservations)
	}
	if status.UtilizationPct != 30.0 {
		t.Errorf("expected utilization=30%%, got %f%%", status.UtilizationPct)
	}

	// Settle one
	pool.Settle(r1.ID)
	status = pool.Status()
	if status.ActiveReservations != 1 {
		t.Errorf("expected 1 active after settle, got %d", status.ActiveReservations)
	}
}

// --- Margin Tests ---

func TestDefaultMarginPolicy(t *testing.T) {
	policy := DefaultMarginPolicy()
	if policy.WarningPct != 0.20 {
		t.Errorf("expected warning=0.20, got %f", policy.WarningPct)
	}
	if policy.MarginCallPct != 0.30 {
		t.Errorf("expected margin_call=0.30, got %f", policy.MarginCallPct)
	}
	if policy.LiquidationPct != 0.50 {
		t.Errorf("expected liquidation=0.50, got %f", policy.LiquidationPct)
	}
}

func TestCheckMarginNoAlert(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{
		ID:        "res_test",
		AccountID: "acct1",
		Asset:     "AAPL",
		Price:     100.0,
		Status:    StatusPendingSettlement,
	}

	// Price dropped 10% — below warning threshold
	alert := CheckMargin(r, 90.0, policy)
	if alert != nil {
		t.Errorf("expected no alert for 10%% drop, got %+v", alert)
	}

	// Price went up — no alert
	alert = CheckMargin(r, 110.0, policy)
	if alert != nil {
		t.Errorf("expected no alert for price increase, got %+v", alert)
	}
}

func TestCheckMarginWarning(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{
		ID:        "res_test",
		AccountID: "acct1",
		Asset:     "AAPL",
		Price:     100.0,
		Status:    StatusPendingSettlement,
	}

	// Exactly 20% drop — warning
	alert := CheckMargin(r, 80.0, policy)
	if alert == nil {
		t.Fatal("expected warning alert for 20% drop")
	}
	if alert.Type != AlertWarning {
		t.Errorf("expected alert type=warning, got %s", alert.Type)
	}
	if alert.DrawdownPct != 0.20 {
		t.Errorf("expected drawdown=0.20, got %f", alert.DrawdownPct)
	}
}

func TestCheckMarginCall(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{
		ID:        "res_test",
		AccountID: "acct1",
		Asset:     "AAPL",
		Price:     100.0,
		Status:    StatusPendingSettlement,
	}

	// 35% drop — margin call
	alert := CheckMargin(r, 65.0, policy)
	if alert == nil {
		t.Fatal("expected margin call alert for 35% drop")
	}
	if alert.Type != AlertMarginCall {
		t.Errorf("expected alert type=margin_call, got %s", alert.Type)
	}
}

func TestCheckMarginLiquidation(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{
		ID:        "res_test",
		AccountID: "acct1",
		Asset:     "AAPL",
		Price:     100.0,
		Status:    StatusPendingSettlement,
	}

	// 55% drop — liquidation
	alert := CheckMargin(r, 45.0, policy)
	if alert == nil {
		t.Fatal("expected liquidation alert for 55% drop")
	}
	if alert.Type != AlertLiquidation {
		t.Errorf("expected alert type=liquidation, got %s", alert.Type)
	}
}

func TestCheckMarginSkipsSettledReservations(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{
		ID:     "res_test",
		Price:  100.0,
		Status: StatusSettled,
	}

	alert := CheckMargin(r, 10.0, policy) // 90% drop on settled — should be ignored
	if alert != nil {
		t.Error("expected no alert for settled reservation")
	}
}

func TestCheckMarginMonitorsMarginCalledStatus(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{
		ID:        "res_test",
		AccountID: "acct1",
		Asset:     "AAPL",
		Price:     100.0,
		Status:    StatusMarginCalled,
	}

	// Already margin-called but price dropped further to liquidation level
	alert := CheckMargin(r, 45.0, policy)
	if alert == nil {
		t.Fatal("expected liquidation alert for margin-called reservation")
	}
	if alert.Type != AlertLiquidation {
		t.Errorf("expected liquidation, got %s", alert.Type)
	}
}

func TestCheckMarginZeroPrice(t *testing.T) {
	policy := DefaultMarginPolicy()
	r := &Reservation{Price: 0, Status: StatusPendingSettlement}
	if CheckMargin(r, 100.0, policy) != nil {
		t.Error("expected nil for zero entry price")
	}

	r = &Reservation{Price: 100.0, Status: StatusPendingSettlement}
	if CheckMargin(r, 0, policy) != nil {
		t.Error("expected nil for zero current price")
	}
}

// --- Settlement Service Tests ---

func TestNewService(t *testing.T) {
	pool := NewPool(PoolConfig{})
	svc := NewService(pool)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestSetTierLimit(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 500_000, MaxPerTransaction: 500_000})
	pool.AddCapital(1_000_000)
	svc := NewService(pool)

	svc.SetTierLimit("custom", 999.99)

	// Verify the tier works
	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       500,
		Side:      "buy",
		KYCTier:   "custom",
	}
	_, err := svc.InstantBuy(context.Background(), req)
	if err != nil {
		t.Fatalf("instant buy with custom tier failed: %v", err)
	}
}

func TestInstantBuySuccess(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:            50_000,
		MaxPerTransaction:     50_000,
		UtilizationWarningPct: 0.90,
	}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       1000, // $1000
		Side:      "buy",
		KYCTier:   TierStandard, // $5000 limit
	}

	result, err := svc.InstantBuy(context.Background(), req)
	if err != nil {
		t.Fatalf("instant buy failed: %v", err)
	}
	if result.ReservationID == "" {
		t.Error("expected non-empty reservation ID")
	}
	if result.OrderID == "" {
		t.Error("expected non-empty order ID")
	}
	if result.Status != string(StatusPendingSettlement) {
		t.Errorf("expected status=pending_settlement, got %s", result.Status)
	}
	if result.EstimatedSettlement.Before(time.Now()) {
		t.Error("estimated settlement should be in the future")
	}

	// Verify the pool was debited
	status := pool.Status()
	if status.Reserved != 1000 {
		t.Errorf("expected reserved=1000, got %f", status.Reserved)
	}
}

func TestInstantBuyKYCTierLimit(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:        50_000,
		MaxPerTransaction: 50_000,
	}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	// Basic tier: $250 limit
	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       300, // $300 > $250 basic limit
		Side:      "buy",
		KYCTier:   TierBasic,
	}

	_, err := svc.InstantBuy(context.Background(), req)
	if err == nil {
		t.Fatal("expected KYC tier limit error for basic tier")
	}

	// Reduce to within limit
	req.Qty = 200
	result, err := svc.InstantBuy(context.Background(), req)
	if err != nil {
		t.Fatalf("instant buy within basic limit failed: %v", err)
	}
	if result.ReservationID == "" {
		t.Error("expected non-empty reservation ID")
	}
}

func TestInstantBuyKYCTierLimitCumulative(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:        50_000,
		MaxPerTransaction: 50_000,
	}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	// Standard tier: $5000 limit
	// First buy: $3000
	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       3000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	_, err := svc.InstantBuy(context.Background(), req)
	if err != nil {
		t.Fatalf("first buy failed: %v", err)
	}

	// Second buy: $3000 would push total to $6000 > $5000
	_, err = svc.InstantBuy(context.Background(), req)
	if err == nil {
		t.Fatal("expected KYC tier cumulative limit error")
	}
}

func TestInstantBuyValidation(t *testing.T) {
	pool := NewPool(PoolConfig{})
	pool.AddCapital(100_000)
	svc := NewService(pool)

	tests := []struct {
		name string
		req  *InstantBuyRequest
	}{
		{"empty account", &InstantBuyRequest{Symbol: "AAPL", Qty: 100, Side: "buy", KYCTier: TierBasic}},
		{"empty symbol", &InstantBuyRequest{AccountID: "acct1", Qty: 100, Side: "buy", KYCTier: TierBasic}},
		{"zero qty", &InstantBuyRequest{AccountID: "acct1", Symbol: "AAPL", Qty: 0, Side: "buy", KYCTier: TierBasic}},
		{"sell side", &InstantBuyRequest{AccountID: "acct1", Symbol: "AAPL", Qty: 100, Side: "sell", KYCTier: TierBasic}},
		{"unknown tier", &InstantBuyRequest{AccountID: "acct1", Symbol: "AAPL", Qty: 100, Side: "buy", KYCTier: "platinum"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.InstantBuy(context.Background(), tt.req)
			if err == nil {
				t.Errorf("expected validation error for %s", tt.name)
			}
		})
	}
}

func TestInstantBuyAllTiers(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:        500_000,
		MaxPerTransaction: 500_000,
	}
	pool := NewPool(cfg)
	pool.AddCapital(1_000_000)
	svc := NewService(pool)

	tiers := map[string]float64{
		TierBasic:         250,
		TierStandard:      5_000,
		TierEnhanced:      25_000,
		TierInstitutional: 250_000,
	}

	for tier, limit := range tiers {
		t.Run(tier, func(t *testing.T) {
			// Buy exactly at the limit
			req := &InstantBuyRequest{
				AccountID: "acct_" + tier,
				Symbol:    "AAPL",
				Qty:       limit,
				Side:      "buy",
				KYCTier:   tier,
			}
			_, err := svc.InstantBuy(context.Background(), req)
			if err != nil {
				t.Fatalf("buy at limit for %s failed: %v", tier, err)
			}

			// Buy $1 more should fail
			req2 := &InstantBuyRequest{
				AccountID: "acct_" + tier,
				Symbol:    "GOOG",
				Qty:       1,
				Side:      "buy",
				KYCTier:   tier,
			}
			_, err = svc.InstantBuy(context.Background(), req2)
			if err == nil {
				t.Fatalf("expected tier limit error for %s at $%.0f + $1", tier, limit)
			}
		})
	}
}

func TestProcessSettlementACHCleared(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)
	svc := NewService(pool)

	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       1000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	result, _ := svc.InstantBuy(context.Background(), req)

	// ACH initiated — no state change
	err := svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventACHInitiated})
	if err != nil {
		t.Fatalf("ach_initiated failed: %v", err)
	}

	// ACH pending — no state change
	err = svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventACHPending})
	if err != nil {
		t.Fatalf("ach_pending failed: %v", err)
	}

	// ACH cleared — settle
	err = svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventACHCleared})
	if err != nil {
		t.Fatalf("ach_cleared failed: %v", err)
	}

	// Verify settled
	r, _ := pool.GetReservation(result.ReservationID)
	if r.Status != StatusSettled {
		t.Errorf("expected settled, got %s", r.Status)
	}

	status := pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after settlement, got %f", status.Reserved)
	}
}

func TestProcessSettlementACHFailed(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)
	svc := NewService(pool)

	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       1000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	result, _ := svc.InstantBuy(context.Background(), req)

	// ACH failed
	err := svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventACHFailed})
	if err != nil {
		t.Fatalf("ach_failed failed: %v", err)
	}

	r, _ := pool.GetReservation(result.ReservationID)
	if r.Status != StatusFailed {
		t.Errorf("expected failed, got %s", r.Status)
	}

	// Liquidate
	err = svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventLiquidated})
	if err != nil {
		t.Fatalf("liquidation failed: %v", err)
	}

	r, _ = pool.GetReservation(result.ReservationID)
	if r.Status != StatusLiquidated {
		t.Errorf("expected liquidated, got %s", r.Status)
	}
}

func TestProcessSettlementMarginCall(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)
	svc := NewService(pool)

	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       1000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	result, _ := svc.InstantBuy(context.Background(), req)

	err := svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventMarginCall})
	if err != nil {
		t.Fatalf("margin call failed: %v", err)
	}

	r, _ := pool.GetReservation(result.ReservationID)
	if r.Status != StatusMarginCalled {
		t.Errorf("expected margin_called, got %s", r.Status)
	}
}

func TestProcessSettlementUnknownEvent(t *testing.T) {
	pool := NewPool(PoolConfig{})
	svc := NewService(pool)

	err := svc.ProcessSettlement("res_test", SettlementEvent{Type: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
}

func TestCheckMarginHealth(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	// Create several reservations at different "prices"
	pool.Reserve("acct1", "AAPL", 5_000, 100.0) // AAPL at $100
	pool.Reserve("acct2", "GOOG", 5_000, 200.0)  // GOOG at $200
	pool.Reserve("acct3", "TSLA", 5_000, 300.0)  // TSLA at $300

	// Price function: AAPL dropped 35% (margin call), GOOG flat, TSLA dropped 55% (liquidation)
	priceFunc := func(asset string) float64 {
		switch asset {
		case "AAPL":
			return 65.0 // 35% drop
		case "GOOG":
			return 200.0 // flat
		case "TSLA":
			return 135.0 // 55% drop
		default:
			return 0
		}
	}

	alerts := svc.CheckMarginHealth(priceFunc)
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}

	// Find alerts by asset
	alertMap := make(map[string]MarginAlert)
	for _, a := range alerts {
		alertMap[a.Asset] = a
	}

	aaplAlert, ok := alertMap["AAPL"]
	if !ok {
		t.Fatal("expected alert for AAPL")
	}
	if aaplAlert.Type != AlertMarginCall {
		t.Errorf("expected AAPL alert=margin_call, got %s", aaplAlert.Type)
	}

	tslaAlert, ok := alertMap["TSLA"]
	if !ok {
		t.Fatal("expected alert for TSLA")
	}
	if tslaAlert.Type != AlertLiquidation {
		t.Errorf("expected TSLA alert=liquidation, got %s", tslaAlert.Type)
	}
}

func TestGetAccountExposure(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	req1 := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       2_000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	svc.InstantBuy(context.Background(), req1)

	req2 := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "GOOG",
		Qty:       1_000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	svc.InstantBuy(context.Background(), req2)

	exposure := svc.GetAccountExposure("acct1")
	if exposure.TotalOutstanding != 3_000 {
		t.Errorf("expected outstanding=3000, got %f", exposure.TotalOutstanding)
	}
	if exposure.ActiveReservations != 2 {
		t.Errorf("expected 2 active, got %d", exposure.ActiveReservations)
	}
}

func TestGetAccountExposureForTier(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	req := &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       2_000,
		Side:      "buy",
		KYCTier:   TierStandard,
	}
	svc.InstantBuy(context.Background(), req)

	exposure := svc.GetAccountExposureForTier("acct1", TierStandard)
	if exposure.TierLimit != 5_000 {
		t.Errorf("expected tier limit=5000, got %f", exposure.TierLimit)
	}
	if exposure.AvailableCredit != 3_000 {
		t.Errorf("expected available credit=3000, got %f", exposure.AvailableCredit)
	}
}

func TestStats(t *testing.T) {
	cfg := PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	// Create several reservations
	r1req := &InstantBuyRequest{AccountID: "acct1", Symbol: "AAPL", Qty: 1_000, Side: "buy", KYCTier: TierStandard}
	r1, _ := svc.InstantBuy(context.Background(), r1req)

	r2req := &InstantBuyRequest{AccountID: "acct2", Symbol: "GOOG", Qty: 2_000, Side: "buy", KYCTier: TierStandard}
	r2, _ := svc.InstantBuy(context.Background(), r2req)

	r3req := &InstantBuyRequest{AccountID: "acct3", Symbol: "TSLA", Qty: 3_000, Side: "buy", KYCTier: TierStandard}
	svc.InstantBuy(context.Background(), r3req)

	// Settle one
	svc.ProcessSettlement(r1.ReservationID, SettlementEvent{Type: EventACHCleared})

	// Fail one
	svc.ProcessSettlement(r2.ReservationID, SettlementEvent{Type: EventACHFailed})

	stats := svc.Stats()
	if stats.TotalReservations != 3 {
		t.Errorf("expected total=3, got %d", stats.TotalReservations)
	}
	if stats.PendingReservations != 1 {
		t.Errorf("expected pending=1, got %d", stats.PendingReservations)
	}
	if stats.SettledReservations != 1 {
		t.Errorf("expected settled=1, got %d", stats.SettledReservations)
	}
	if stats.FailedReservations != 1 {
		t.Errorf("expected failed=1, got %d", stats.FailedReservations)
	}
	if stats.TotalVolume != 6_000 {
		t.Errorf("expected total volume=6000, got %f", stats.TotalVolume)
	}
	// Pending volume: 3000 (pending) + 2000 (failed, still reserved) = 5000
	if stats.PendingVolume != 5_000 {
		t.Errorf("expected pending volume=5000, got %f", stats.PendingVolume)
	}
}

// --- Concurrency Tests ---

func TestConcurrentReservations(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:            1_000_000,
		MaxPerTransaction:     1_000,
		UtilizationWarningPct: 0.95,
	}
	pool := NewPool(cfg)
	pool.AddCapital(1_000_000)

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	reservations := make(chan *Reservation, 100)

	// 100 concurrent reservations of $1000 each
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			accountID := fmt.Sprintf("acct_%d", i%10)
			r, err := pool.Reserve(accountID, "AAPL", 1_000, 150.0)
			if err != nil {
				errs <- err
				return
			}
			reservations <- r
		}(i)
	}

	wg.Wait()
	close(errs)
	close(reservations)

	var errCount int
	for range errs {
		errCount++
	}

	var resCount int
	for range reservations {
		resCount++
	}

	// Should have exactly 100 total (100k * 0.95 = 95k max, but per-user limit of 1M allows all)
	// With 95% utilization cap on 1M pool, max reserved = 950k. 100 * 1000 = 100k << 950k.
	if resCount+errCount != 100 {
		t.Errorf("expected 100 total operations, got %d success + %d errors", resCount, errCount)
	}

	status := pool.Status()
	expectedReserved := float64(resCount) * 1_000
	if status.Reserved != expectedReserved {
		t.Errorf("expected reserved=%f, got %f", expectedReserved, status.Reserved)
	}
}

func TestConcurrentReserveAndSettle(t *testing.T) {
	cfg := PoolConfig{
		MaxPerUser:        1_000_000,
		MaxPerTransaction: 10_000,
	}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)

	var wg sync.WaitGroup

	// Create 10 reservations
	ids := make([]string, 10)
	for i := 0; i < 10; i++ {
		r, err := pool.Reserve(fmt.Sprintf("acct_%d", i), "AAPL", 5_000, 150.0)
		if err != nil {
			t.Fatalf("setup reserve %d failed: %v", i, err)
		}
		ids[i] = r.ID
	}

	// Concurrently settle all of them
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			pool.Settle(id)
		}(id)
	}

	wg.Wait()

	status := pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after settling all, got %f", status.Reserved)
	}
	if status.Available != 100_000 {
		t.Errorf("expected available=100000 after settling all, got %f", status.Available)
	}
}

func TestConcurrentCapitalOperations(t *testing.T) {
	pool := NewPool(PoolConfig{})

	var wg sync.WaitGroup

	// 50 goroutines adding capital
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.AddCapital(1_000)
		}()
	}

	wg.Wait()

	status := pool.Status()
	if status.Total != 50_000 {
		t.Errorf("expected total=50000 after concurrent adds, got %f", status.Total)
	}
}

func TestFullLifecycleEndToEnd(t *testing.T) {
	// Simulate the complete instant-buy lifecycle:
	// 1. Operator funds pool
	// 2. User instant-buys
	// 3. ACH is initiated
	// 4. ACH clears
	// 5. Pool capital is returned

	cfg := PoolConfig{
		MaxPerUser:            25_000,
		MaxPerTransaction:     10_000,
		UtilizationWarningPct: 0.80,
	}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)
	svc := NewService(pool)

	// Step 1: User buys $2000 of AAPL
	result, err := svc.InstantBuy(context.Background(), &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       2_000,
		Side:      "buy",
		KYCTier:   TierStandard,
	})
	if err != nil {
		t.Fatalf("instant buy failed: %v", err)
	}

	// Verify pool state
	status := pool.Status()
	if status.Reserved != 2_000 {
		t.Errorf("step 1: expected reserved=2000, got %f", status.Reserved)
	}

	// Step 2: ACH initiated
	svc.ProcessSettlement(result.ReservationID, SettlementEvent{
		Type:      EventACHInitiated,
		Timestamp: time.Now(),
	})

	// Step 3: Check margin — price is stable
	alerts := svc.CheckMarginHealth(func(asset string) float64 { return 150.0 })
	if len(alerts) != 0 {
		t.Errorf("step 3: expected no margin alerts, got %d", len(alerts))
	}

	// Step 4: ACH clears
	err = svc.ProcessSettlement(result.ReservationID, SettlementEvent{
		Type:      EventACHCleared,
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("step 4: settlement failed: %v", err)
	}

	// Step 5: Pool is whole again
	status = pool.Status()
	if status.Reserved != 0 {
		t.Errorf("step 5: expected reserved=0, got %f", status.Reserved)
	}
	if status.Available != 100_000 {
		t.Errorf("step 5: expected available=100000, got %f", status.Available)
	}

	// Verify stats
	stats := svc.Stats()
	if stats.TotalReservations != 1 {
		t.Errorf("expected 1 total reservation, got %d", stats.TotalReservations)
	}
	if stats.SettledReservations != 1 {
		t.Errorf("expected 1 settled, got %d", stats.SettledReservations)
	}
}

func TestFailedACHLifecycle(t *testing.T) {
	// Simulate: instant buy -> ACH fails -> liquidation

	cfg := PoolConfig{
		MaxPerUser:        25_000,
		MaxPerTransaction: 10_000,
	}
	pool := NewPool(cfg)
	pool.AddCapital(100_000)
	svc := NewService(pool)

	result, _ := svc.InstantBuy(context.Background(), &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "AAPL",
		Qty:       5_000,
		Side:      "buy",
		KYCTier:   TierEnhanced,
	})

	// ACH fails
	svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventACHFailed})

	// Pool capital is still locked (we hold the asset)
	status := pool.Status()
	if status.Reserved != 5_000 {
		t.Errorf("expected reserved=5000 after ACH fail, got %f", status.Reserved)
	}

	// Liquidate (force sell)
	svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventLiquidated})

	// Pool capital released
	status = pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after liquidation, got %f", status.Reserved)
	}
}

func TestMarginCallLifecycle(t *testing.T) {
	// Simulate: instant buy -> price drops -> margin call -> liquidation

	cfg := PoolConfig{
		MaxPerUser:        50_000,
		MaxPerTransaction: 50_000,
	}
	pool := NewPool(cfg)
	pool.AddCapital(500_000)
	svc := NewService(pool)

	result, _ := svc.InstantBuy(context.Background(), &InstantBuyRequest{
		AccountID: "acct1",
		Symbol:    "TSLA",
		Qty:       10_000,
		Side:      "buy",
		KYCTier:   TierEnhanced,
	})

	// Price crashes 35% — margin call territory
	alerts := svc.CheckMarginHealth(func(asset string) float64 {
		return 0.65 // entry price was 1.0 (10000/10000), now 0.65 = 35% drop
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 margin alert, got %d", len(alerts))
	}
	if alerts[0].Type != AlertMarginCall {
		t.Errorf("expected margin_call alert, got %s", alerts[0].Type)
	}

	// Process margin call
	svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventMarginCall})

	r, _ := pool.GetReservation(result.ReservationID)
	if r.Status != StatusMarginCalled {
		t.Errorf("expected margin_called, got %s", r.Status)
	}

	// Price drops further to 50% — auto-liquidate
	alerts = svc.CheckMarginHealth(func(asset string) float64 {
		return 0.45 // 55% drop
	})
	if len(alerts) != 1 {
		t.Fatalf("expected 1 liquidation alert, got %d", len(alerts))
	}
	if alerts[0].Type != AlertLiquidation {
		t.Errorf("expected liquidation alert, got %s", alerts[0].Type)
	}

	// Execute liquidation
	svc.ProcessSettlement(result.ReservationID, SettlementEvent{Type: EventLiquidated})

	r, _ = pool.GetReservation(result.ReservationID)
	if r.Status != StatusLiquidated {
		t.Errorf("expected liquidated, got %s", r.Status)
	}

	status := pool.Status()
	if status.Reserved != 0 {
		t.Errorf("expected reserved=0 after liquidation, got %f", status.Reserved)
	}
}

func TestReservationIDsAreUnique(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 1_000_000, MaxPerTransaction: 1_000_000})
	pool.AddCapital(1_000_000)

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		r, err := pool.Reserve("acct1", "AAPL", 1, 1.0)
		if err != nil {
			t.Fatalf("reserve %d failed: %v", i, err)
		}
		if ids[r.ID] {
			t.Fatalf("duplicate reservation ID: %s", r.ID)
		}
		ids[r.ID] = true
	}
}

func TestReservationIDPrefix(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 1_000, MaxPerTransaction: 1_000})
	pool.AddCapital(1_000)

	r, _ := pool.Reserve("acct1", "AAPL", 100, 150.0)
	if len(r.ID) < 4 || r.ID[:4] != "res_" {
		t.Errorf("expected ID to start with 'res_', got %s", r.ID)
	}
}

func TestSetMarginPolicy(t *testing.T) {
	pool := NewPool(PoolConfig{MaxPerUser: 50_000, MaxPerTransaction: 50_000})
	pool.AddCapital(500_000)
	svc := NewService(pool)

	// Set aggressive policy: 5% warning, 10% margin call, 15% liquidation
	svc.SetMarginPolicy(&MarginPolicy{
		WarningPct:     0.05,
		MarginCallPct:  0.10,
		LiquidationPct: 0.15,
		GracePeriod:    "1h",
	})

	pool.Reserve("acct1", "AAPL", 5_000, 100.0)

	// 12% drop — should trigger margin call with aggressive policy
	alerts := svc.CheckMarginHealth(func(asset string) float64 { return 88.0 })
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert with aggressive policy, got %d", len(alerts))
	}
	if alerts[0].Type != AlertMarginCall {
		t.Errorf("expected margin_call with aggressive policy, got %s", alerts[0].Type)
	}
}
