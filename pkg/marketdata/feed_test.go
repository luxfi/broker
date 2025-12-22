package marketdata

import (
	"sync"
	"testing"
	"time"
)

func TestNewFeed(t *testing.T) {
	f := NewFeed()
	if f == nil {
		t.Fatal("NewFeed returned nil")
	}
	if len(f.AllTickers()) != 0 {
		t.Fatalf("expected 0 tickers, got %d", len(f.AllTickers()))
	}
}

func TestUpdateQuoteAndGetTicker(t *testing.T) {
	f := NewFeed()
	f.UpdateQuote("BTC/USD", "coinbase", 50000, 50010, 1.5, 2.0, 50005)

	ticker, err := f.GetTicker("BTC/USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticker.Symbol != "BTC/USD" {
		t.Fatalf("expected BTC/USD, got %s", ticker.Symbol)
	}
	if ticker.BestBid != 50000 {
		t.Fatalf("expected best bid 50000, got %f", ticker.BestBid)
	}
	if ticker.BestAsk != 50010 {
		t.Fatalf("expected best ask 50010, got %f", ticker.BestAsk)
	}
	if ticker.Last != 50005 {
		t.Fatalf("expected last 50005, got %f", ticker.Last)
	}
}

func TestGetTickerNotFound(t *testing.T) {
	f := NewFeed()
	_, err := f.GetTicker("DOESNOTEXIST")
	if err == nil {
		t.Fatal("expected error for missing ticker")
	}
}

func TestMultiProviderBBO(t *testing.T) {
	f := NewFeed()
	f.UpdateQuote("BTC/USD", "coinbase", 50000, 50020, 1.0, 1.0, 50010)
	f.UpdateQuote("BTC/USD", "kraken", 50010, 50015, 0.5, 0.5, 50012)

	ticker, _ := f.GetTicker("BTC/USD")
	if ticker.BestBid != 50010 {
		t.Fatalf("expected BBO bid 50010, got %f", ticker.BestBid)
	}
	if ticker.BestBidProvider != "kraken" {
		t.Fatalf("expected bid provider kraken, got %s", ticker.BestBidProvider)
	}
	if ticker.BestAsk != 50015 {
		t.Fatalf("expected BBO ask 50015, got %f", ticker.BestAsk)
	}
}

func TestGetBBO(t *testing.T) {
	f := NewFeed()
	f.UpdateQuote("ETH/USD", "binance", 3000, 3005, 10, 10, 3002)

	bid, ask, bidProv, askProv, err := f.GetBBO("ETH/USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bid != 3000 || ask != 3005 {
		t.Fatalf("expected 3000/3005, got %f/%f", bid, ask)
	}
	if bidProv != "binance" || askProv != "binance" {
		t.Fatalf("expected binance, got %s/%s", bidProv, askProv)
	}
}

func TestAllTickers(t *testing.T) {
	f := NewFeed()
	f.UpdateQuote("BTC/USD", "coinbase", 50000, 50010, 1, 1, 50005)
	f.UpdateQuote("ETH/USD", "binance", 3000, 3005, 10, 10, 3002)

	tickers := f.AllTickers()
	if len(tickers) != 2 {
		t.Fatalf("expected 2 tickers, got %d", len(tickers))
	}
}

func TestSubscribe(t *testing.T) {
	f := NewFeed()
	ch, unsub := f.Subscribe("BTC/USD")
	defer unsub()

	f.UpdateQuote("BTC/USD", "coinbase", 50000, 50010, 1, 1, 50005)

	select {
	case ticker := <-ch:
		if ticker.Symbol != "BTC/USD" {
			t.Fatalf("expected BTC/USD, got %s", ticker.Symbol)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ticker")
	}
}

func TestSpreadBps(t *testing.T) {
	f := NewFeed()
	f.UpdateQuote("TEST", "prov", 100, 101, 1, 1, 100.5)
	ticker, _ := f.GetTicker("TEST")
	expectedBps := (1.0 / 100.5) * 10000
	if ticker.SpreadBps < expectedBps-0.1 || ticker.SpreadBps > expectedBps+0.1 {
		t.Fatalf("expected ~%.1f bps, got %f", expectedBps, ticker.SpreadBps)
	}
}

func TestConcurrentAccess(t *testing.T) {
	f := NewFeed()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f.UpdateQuote("BTC/USD", "prov", float64(50000+i), float64(50010+i), 1, 1, float64(50005+i))
		}(i)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.GetTicker("BTC/USD")
			f.AllTickers()
		}()
	}
	wg.Wait()
}
