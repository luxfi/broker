package uniswap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestName(t *testing.T) {
	p := New(Config{})
	if p.Name() != "uniswap" {
		t.Fatalf("expected uniswap, got %s", p.Name())
	}
}

func TestListAssets(t *testing.T) {
	p := New(Config{})
	assets, err := p.ListAssets(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 30 {
		t.Fatalf("expected 30 assets, got %d", len(assets))
	}
	// First asset should be ETH (WETH).
	if assets[0].Symbol != "ETH" {
		t.Fatalf("expected ETH first, got %s", assets[0].Symbol)
	}
	for _, a := range assets {
		if a.Provider != "uniswap" {
			t.Fatalf("expected provider uniswap, got %s", a.Provider)
		}
		if a.Class != "crypto" {
			t.Fatalf("expected class crypto, got %s for %s", a.Class, a.Symbol)
		}
		if a.Tradable {
			t.Fatalf("expected tradable=false for read-only provider, got true for %s", a.Symbol)
		}
	}
}

func TestGetAssetBySymbol(t *testing.T) {
	p := New(Config{})
	a, err := p.GetAsset(context.Background(), "USDC")
	if err != nil {
		t.Fatal(err)
	}
	if a.Symbol != "USDC" {
		t.Fatalf("expected USDC, got %s", a.Symbol)
	}
}

func TestGetAssetByAddress(t *testing.T) {
	p := New(Config{})
	a, err := p.GetAsset(context.Background(), "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	if err != nil {
		t.Fatal(err)
	}
	if a.Symbol != "USDC" {
		t.Fatalf("expected USDC, got %s", a.Symbol)
	}
}

func TestGetAssetNotFound(t *testing.T) {
	p := New(Config{})
	_, err := p.GetAsset(context.Background(), "DOESNOTEXIST")
	if err == nil {
		t.Fatal("expected error for unknown asset")
	}
}

func TestGetClock(t *testing.T) {
	p := New(Config{})
	clock, err := p.GetClock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !clock.IsOpen {
		t.Fatal("DEX should always be open")
	}
}

func TestGetCalendar(t *testing.T) {
	p := New(Config{})
	cal, err := p.GetCalendar(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cal != nil {
		t.Fatal("expected nil calendar for 24/7 DEX")
	}
}

func TestNotImplementedMethods(t *testing.T) {
	p := New(Config{})
	ctx := context.Background()

	if _, err := p.CreateAccount(ctx, nil); err != ErrNotImplemented {
		t.Fatalf("CreateAccount: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.GetAccount(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("GetAccount: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.ListAccounts(ctx); err != ErrNotImplemented {
		t.Fatalf("ListAccounts: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.GetPortfolio(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("GetPortfolio: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.CreateOrder(ctx, "x", nil); err != ErrNotImplemented {
		t.Fatalf("CreateOrder: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.ListOrders(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("ListOrders: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.GetOrder(ctx, "x", "y"); err != ErrNotImplemented {
		t.Fatalf("GetOrder: expected ErrNotImplemented, got %v", err)
	}
	if err := p.CancelOrder(ctx, "x", "y"); err != ErrNotImplemented {
		t.Fatalf("CancelOrder: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.CreateTransfer(ctx, "x", nil); err != ErrNotImplemented {
		t.Fatalf("CreateTransfer: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.ListTransfers(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("ListTransfers: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.CreateBankRelationship(ctx, "", "", "", "", ""); err != ErrNotImplemented {
		t.Fatalf("CreateBankRelationship: expected ErrNotImplemented, got %v", err)
	}
	if _, err := p.ListBankRelationships(ctx, ""); err != ErrNotImplemented {
		t.Fatalf("ListBankRelationships: expected ErrNotImplemented, got %v", err)
	}
}

func TestGetSnapshotWithMockSubgraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"token":  map[string]string{"derivedETH": "1.0"},
				"bundle": map[string]string{"ethPriceUSD": "3500.50"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{V3Subgraph: srv.URL})
	snap, err := p.GetSnapshot(context.Background(), "ETH")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Symbol != "ETH" {
		t.Fatalf("expected ETH, got %s", snap.Symbol)
	}
	if snap.LatestTrade.Price != 3500.50 {
		t.Fatalf("expected price 3500.50, got %f", snap.LatestTrade.Price)
	}
}

func TestGetSnapshotsWithMockSubgraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"token":  map[string]string{"derivedETH": "0.0003"},
				"bundle": map[string]string{"ethPriceUSD": "3000.00"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{V3Subgraph: srv.URL})
	snaps, err := p.GetSnapshots(context.Background(), []string{"USDC", "DAI"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}
}

func TestGetLatestQuotesWithMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"token":  map[string]string{"derivedETH": "0.5"},
				"bundle": map[string]string{"ethPriceUSD": "4000.00"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New(Config{V3Subgraph: srv.URL})
	quotes, err := p.GetLatestQuotes(context.Background(), []string{"UNI"})
	if err != nil {
		t.Fatal(err)
	}
	q, ok := quotes["UNI"]
	if !ok {
		t.Fatal("expected quote for UNI")
	}
	if q.BidPrice != 2000.0 {
		t.Fatalf("expected price 2000, got %f", q.BidPrice)
	}
}

func TestResolveTokenStripsUSD(t *testing.T) {
	p := New(Config{})
	tok := p.resolveToken("ETH/USD")
	if tok == nil {
		t.Fatal("expected to resolve ETH/USD")
	}
	if tok.symbol != "ETH" {
		t.Fatalf("expected ETH, got %s", tok.symbol)
	}
}

func TestGetBarsReturnsNil(t *testing.T) {
	p := New(Config{})
	bars, err := p.GetBars(context.Background(), "ETH", "1h", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if bars != nil {
		t.Fatal("expected nil bars")
	}
}

func TestGetLatestTradesReturnsEmpty(t *testing.T) {
	p := New(Config{})
	trades, err := p.GetLatestTrades(context.Background(), []string{"ETH"})
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) != 0 {
		t.Fatalf("expected empty trades, got %d", len(trades))
	}
}
