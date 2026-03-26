package marketdata

import (
	"time"
)

// ArbitrageOpportunity represents a detected cross-venue price discrepancy
// where the best bid on one venue exceeds the best ask on another.
type ArbitrageOpportunity struct {
	Symbol      string  `json:"symbol"`
	BuyVenue    string  `json:"buy_venue"`    // venue with the lower ask
	SellVenue   string  `json:"sell_venue"`   // venue with the higher bid
	BuyPrice    float64 `json:"buy_price"`    // ask on buy venue
	SellPrice   float64 `json:"sell_price"`   // bid on sell venue
	SpreadAbs   float64 `json:"spread_abs"`   // sell - buy
	SpreadBps   float64 `json:"spread_bps"`   // in basis points
	BuySize     float64 `json:"buy_size"`     // available size at ask
	SellSize    float64 `json:"sell_size"`    // available size at bid
	DetectedAt  time.Time `json:"detected_at"`
}

// ArbitrageDetector scans the consolidated feed for cross-venue arbitrage.
type ArbitrageDetector struct {
	feed         *Feed
	thresholdBps float64 // minimum spread in bps to flag
}

// NewArbitrageDetector creates a detector with the given threshold.
// Opportunities where the cross-venue spread exceeds thresholdBps are reported.
func NewArbitrageDetector(feed *Feed, thresholdBps float64) *ArbitrageDetector {
	return &ArbitrageDetector{
		feed:         feed,
		thresholdBps: thresholdBps,
	}
}

// Scan checks all symbols in the feed for arbitrage opportunities.
func (d *ArbitrageDetector) Scan() []*ArbitrageOpportunity {
	tickers := d.feed.AllTickers()
	var opps []*ArbitrageOpportunity

	for _, ticker := range tickers {
		opp := d.checkTicker(ticker)
		if opp != nil {
			opps = append(opps, opp...)
		}
	}
	return opps
}

// CheckSymbol checks a single symbol for arbitrage opportunities.
func (d *ArbitrageDetector) CheckSymbol(symbol string) []*ArbitrageOpportunity {
	ticker, err := d.feed.GetTicker(symbol)
	if err != nil {
		return nil
	}
	return d.checkTicker(ticker)
}

// checkTicker examines all pairs of venue quotes for a single ticker.
// An arbitrage exists when venue A's bid > venue B's ask (or vice versa).
func (d *ArbitrageDetector) checkTicker(ticker *Ticker) []*ArbitrageOpportunity {
	if len(ticker.Sources) < 2 {
		return nil
	}

	type venueQuote struct {
		venue string
		q     *Quote
	}

	quotes := make([]venueQuote, 0, len(ticker.Sources))
	for venue, q := range ticker.Sources {
		if q.BidPrice > 0 && q.AskPrice > 0 {
			quotes = append(quotes, venueQuote{venue: venue, q: q})
		}
	}

	var opps []*ArbitrageOpportunity
	now := time.Now()

	for i := 0; i < len(quotes); i++ {
		for j := i + 1; j < len(quotes); j++ {
			a := quotes[i]
			b := quotes[j]

			// Check if a.bid > b.ask (buy at b, sell at a)
			if a.q.BidPrice > b.q.AskPrice {
				spread := a.q.BidPrice - b.q.AskPrice
				mid := (a.q.BidPrice + b.q.AskPrice) / 2
				spreadBps := (spread / mid) * 10000
				if spreadBps >= d.thresholdBps {
					opps = append(opps, &ArbitrageOpportunity{
						Symbol:     ticker.Symbol,
						BuyVenue:   b.venue,
						SellVenue:  a.venue,
						BuyPrice:   b.q.AskPrice,
						SellPrice:  a.q.BidPrice,
						SpreadAbs:  spread,
						SpreadBps:  spreadBps,
						BuySize:    b.q.AskSize,
						SellSize:   a.q.BidSize,
						DetectedAt: now,
					})
				}
			}

			// Check if b.bid > a.ask (buy at a, sell at b)
			if b.q.BidPrice > a.q.AskPrice {
				spread := b.q.BidPrice - a.q.AskPrice
				mid := (b.q.BidPrice + a.q.AskPrice) / 2
				spreadBps := (spread / mid) * 10000
				if spreadBps >= d.thresholdBps {
					opps = append(opps, &ArbitrageOpportunity{
						Symbol:     ticker.Symbol,
						BuyVenue:   a.venue,
						SellVenue:  b.venue,
						BuyPrice:   a.q.AskPrice,
						SellPrice:  b.q.BidPrice,
						SpreadAbs:  spread,
						SpreadBps:  spreadBps,
						BuySize:    a.q.AskSize,
						SellSize:   b.q.BidSize,
						DetectedAt: now,
					})
				}
			}
		}
	}

	return opps
}
