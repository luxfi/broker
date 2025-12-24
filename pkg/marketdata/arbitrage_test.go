package marketdata

import (
	"testing"
)

func TestArbitrageDetectorNoOpportunity(t *testing.T) {
	feed := NewFeed()
	// Both venues have overlapping spread (no arb)
	feed.UpdateQuote("BTC/USD", "coinbase", 50000, 50010, 1, 1, 50005)
	feed.UpdateQuote("BTC/USD", "kraken", 49990, 50020, 1, 1, 50005)

	det := NewArbitrageDetector(feed, 0)
	opps := det.Scan()
	if len(opps) != 0 {
		t.Errorf("expected 0 opportunities, got %d", len(opps))
	}
}

func TestArbitrageDetectorFindsOpportunity(t *testing.T) {
	feed := NewFeed()
	// Coinbase bid 50100 > Kraken ask 50000 => buy Kraken, sell Coinbase
	feed.UpdateQuote("BTC/USD", "coinbase", 50100, 50200, 2, 2, 50150)
	feed.UpdateQuote("BTC/USD", "kraken", 49900, 50000, 3, 3, 49950)

	det := NewArbitrageDetector(feed, 0)
	opps := det.Scan()
	if len(opps) != 1 {
		t.Fatalf("expected 1 opportunity, got %d", len(opps))
	}

	opp := opps[0]
	if opp.BuyVenue != "kraken" {
		t.Errorf("BuyVenue = %q, want 'kraken'", opp.BuyVenue)
	}
	if opp.SellVenue != "coinbase" {
		t.Errorf("SellVenue = %q, want 'coinbase'", opp.SellVenue)
	}
	if opp.BuyPrice != 50000 {
		t.Errorf("BuyPrice = %f, want 50000", opp.BuyPrice)
	}
	if opp.SellPrice != 50100 {
		t.Errorf("SellPrice = %f, want 50100", opp.SellPrice)
	}
	if opp.SpreadAbs != 100 {
		t.Errorf("SpreadAbs = %f, want 100", opp.SpreadAbs)
	}
	if opp.SpreadBps <= 0 {
		t.Errorf("SpreadBps = %f, want > 0", opp.SpreadBps)
	}
}

func TestArbitrageDetectorThreshold(t *testing.T) {
	feed := NewFeed()
	// Small arb: bid 50010 > ask 50000 => 10 spread, ~2 bps
	feed.UpdateQuote("ETH/USD", "gemini", 50010, 50100, 1, 1, 50050)
	feed.UpdateQuote("ETH/USD", "binance", 49900, 50000, 1, 1, 49950)

	// With a high threshold, should not trigger
	det := NewArbitrageDetector(feed, 100) // 100 bps = 1%
	opps := det.Scan()
	if len(opps) != 0 {
		t.Errorf("expected 0 opportunities at 100bps threshold, got %d", len(opps))
	}

	// With low threshold, should trigger
	det2 := NewArbitrageDetector(feed, 1) // 1 bps
	opps2 := det2.Scan()
	if len(opps2) != 1 {
		t.Errorf("expected 1 opportunity at 1bps threshold, got %d", len(opps2))
	}
}

func TestArbitrageDetectorSingleVenue(t *testing.T) {
	feed := NewFeed()
	feed.UpdateQuote("SOL/USD", "coinbase", 100, 101, 10, 10, 100.5)

	det := NewArbitrageDetector(feed, 0)
	opps := det.CheckSymbol("SOL/USD")
	if len(opps) != 0 {
		t.Errorf("expected 0 opportunities with single venue, got %d", len(opps))
	}
}

func TestArbitrageDetectorBidirectional(t *testing.T) {
	feed := NewFeed()
	// Both directions have arb (crossed market)
	feed.UpdateQuote("DOGE/USD", "exchange_a", 0.15, 0.14, 100, 100, 0.145)
	feed.UpdateQuote("DOGE/USD", "exchange_b", 0.16, 0.13, 100, 100, 0.145)

	det := NewArbitrageDetector(feed, 0)
	opps := det.CheckSymbol("DOGE/USD")
	// a.bid(0.15) > b.ask(0.13) => buy b sell a
	// b.bid(0.16) > a.ask(0.14) => buy a sell b
	if len(opps) != 2 {
		t.Errorf("expected 2 bidirectional opportunities, got %d", len(opps))
	}
}
