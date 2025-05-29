package marketdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// Feed aggregates real-time market data across all providers.
// It maintains a consolidated order book and best bid/offer (BBO)
// for smart order routing decisions.
type Feed struct {
	mu       sync.RWMutex
	books    map[string]*ConsolidatedBook // symbol -> consolidated book
	tickers  map[string]*Ticker           // symbol -> latest ticker
	subs     map[string][]chan *Ticker     // symbol -> subscriber channels
	subsMu   sync.RWMutex
}

// ConsolidatedBook aggregates order book depth across providers.
type ConsolidatedBook struct {
	Symbol    string        `json:"symbol"`
	Bids      []PriceLevel  `json:"bids"` // sorted best (highest) first
	Asks      []PriceLevel  `json:"asks"` // sorted best (lowest) first
	UpdatedAt time.Time     `json:"updated_at"`
}

// PriceLevel is a single price/quantity level with provider attribution.
type PriceLevel struct {
	Provider string  `json:"provider"`
	Price    float64 `json:"price"`
	Size     float64 `json:"size"`
}

// Ticker is a consolidated real-time price.
type Ticker struct {
	Symbol    string             `json:"symbol"`
	BestBid   float64            `json:"best_bid"`
	BestAsk   float64            `json:"best_ask"`
	BestBidProvider string       `json:"best_bid_provider"`
	BestAskProvider string       `json:"best_ask_provider"`
	Spread    float64            `json:"spread"`
	SpreadBps float64            `json:"spread_bps"` // basis points
	Last      float64            `json:"last"`
	Volume24h float64            `json:"volume_24h,omitempty"`
	Sources   map[string]*Quote  `json:"sources"` // per-provider quotes
	UpdatedAt time.Time          `json:"updated_at"`
}

// Quote is a single provider's quote.
type Quote struct {
	Provider  string  `json:"provider"`
	BidPrice  float64 `json:"bid_price"`
	BidSize   float64 `json:"bid_size"`
	AskPrice  float64 `json:"ask_price"`
	AskSize   float64 `json:"ask_size"`
	Last      float64 `json:"last"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewFeed() *Feed {
	return &Feed{
		books:   make(map[string]*ConsolidatedBook),
		tickers: make(map[string]*Ticker),
		subs:    make(map[string][]chan *Ticker),
	}
}

// UpdateQuote updates a provider's quote for a symbol and recalculates BBO.
func (f *Feed) UpdateQuote(symbol, provider string, bid, ask, bidSize, askSize, last float64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ticker, ok := f.tickers[symbol]
	if !ok {
		ticker = &Ticker{
			Symbol:  symbol,
			Sources: make(map[string]*Quote),
		}
		f.tickers[symbol] = ticker
	}

	ticker.Sources[provider] = &Quote{
		Provider:  provider,
		BidPrice:  bid,
		BidSize:   bidSize,
		AskPrice:  ask,
		AskSize:   askSize,
		Last:      last,
		UpdatedAt: time.Now(),
	}

	// Recalculate BBO across all providers
	var bestBid, bestAsk float64
	var bestBidProv, bestAskProv string
	first := true
	for prov, q := range ticker.Sources {
		if q.BidPrice > bestBid {
			bestBid = q.BidPrice
			bestBidProv = prov
		}
		if first || (q.AskPrice > 0 && q.AskPrice < bestAsk) {
			bestAsk = q.AskPrice
			bestAskProv = prov
			first = false
		}
	}

	ticker.BestBid = bestBid
	ticker.BestAsk = bestAsk
	ticker.BestBidProvider = bestBidProv
	ticker.BestAskProvider = bestAskProv
	if bestAsk > 0 {
		ticker.Spread = bestAsk - bestBid
		mid := (bestAsk + bestBid) / 2
		if mid > 0 {
			ticker.SpreadBps = (ticker.Spread / mid) * 10000
		}
	}
	if last > 0 {
		ticker.Last = last
	}
	ticker.UpdatedAt = time.Now()

	// Notify subscribers
	f.subsMu.RLock()
	for _, ch := range f.subs[symbol] {
		select {
		case ch <- ticker:
		default: // non-blocking, drop if slow
		}
	}
	f.subsMu.RUnlock()
}

// GetTicker returns the consolidated ticker for a symbol.
func (f *Feed) GetTicker(symbol string) (*Ticker, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	t, ok := f.tickers[symbol]
	if !ok {
		return nil, fmt.Errorf("no data for %s", symbol)
	}
	return t, nil
}

// GetBBO returns best bid/offer with provider attribution.
func (f *Feed) GetBBO(symbol string) (bestBid, bestAsk float64, bidProvider, askProvider string, err error) {
	t, err := f.GetTicker(symbol)
	if err != nil {
		return 0, 0, "", "", err
	}
	return t.BestBid, t.BestAsk, t.BestBidProvider, t.BestAskProvider, nil
}

// Subscribe returns a channel that receives ticker updates for a symbol.
func (f *Feed) Subscribe(symbol string) (<-chan *Ticker, func()) {
	ch := make(chan *Ticker, 16)
	f.subsMu.Lock()
	f.subs[symbol] = append(f.subs[symbol], ch)
	f.subsMu.Unlock()

	unsubscribe := func() {
		f.subsMu.Lock()
		defer f.subsMu.Unlock()
		subs := f.subs[symbol]
		for i, s := range subs {
			if s == ch {
				f.subs[symbol] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
	}
	return ch, unsubscribe
}

// PollSnapshots polls all providers for market data snapshots.
// This is used when WebSocket feeds aren't available.
func (f *Feed) PollSnapshots(ctx context.Context, symbols []string, getSnapshot func(ctx context.Context, symbol string) (map[string]*types.MarketSnapshot, error)) {
	for _, sym := range symbols {
		snaps, err := getSnapshot(ctx, sym)
		if err != nil {
			continue
		}
		for prov, snap := range snaps {
			var bid, ask, bidSz, askSz, last float64
			if snap.LatestQuote != nil {
				bid = snap.LatestQuote.BidPrice
				ask = snap.LatestQuote.AskPrice
				bidSz = snap.LatestQuote.BidSize
				askSz = snap.LatestQuote.AskSize
			}
			if snap.LatestTrade != nil {
				last = snap.LatestTrade.Price
			}
			f.UpdateQuote(sym, prov, bid, ask, bidSz, askSz, last)
		}
	}
}

// AllTickers returns all current tickers.
func (f *Feed) AllTickers() map[string]*Ticker {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[string]*Ticker, len(f.tickers))
	for k, v := range f.tickers {
		result[k] = v
	}
	return result
}
