package polygon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// Polygon.io — Market data aggregator for stocks, options, forex, crypto.
// Read-only provider: no account/order support, market data only.

const ProdURL = "https://api.polygon.io"

type Config struct{ BaseURL, APIKey string }

type Provider struct{ cfg Config; client *http.Client }

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" { cfg.BaseURL = ProdURL }
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "polygon" }

var errNotSupported = fmt.Errorf("not supported by polygon (market data only)")

func (p *Provider) doRequest(ctx context.Context, path string) ([]byte, error) {
	sep := "?"
	if strings.Contains(path, "?") { sep = "&" }
	url := p.cfg.BaseURL + path + sep + "apiKey=" + p.cfg.APIKey
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil { return nil, err }
	resp, err := p.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 { return nil, fmt.Errorf("polygon error %d: %s", resp.StatusCode, string(data)) }
	return data, nil
}

// Account/Order — not supported (market data only)
func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) { return nil, errNotSupported }
func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error) { return nil, errNotSupported }
func (p *Provider) ListAccounts(_ context.Context) ([]*types.Account, error) { return nil, errNotSupported }
func (p *Provider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error) { return nil, errNotSupported }
func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) { return nil, errNotSupported }
func (p *Provider) ListOrders(_ context.Context, _ string) ([]*types.Order, error) { return nil, errNotSupported }
func (p *Provider) GetOrder(_ context.Context, _, _ string) (*types.Order, error) { return nil, errNotSupported }
func (p *Provider) CancelOrder(_ context.Context, _, _ string) error { return errNotSupported }
func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, errNotSupported }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, errNotSupported }

// Assets
func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	market := "stocks"
	if class == "crypto" { market = "crypto" }
	if class == "forex" || class == "fx" { market = "fx" }
	data, err := p.doRequest(ctx, "/v3/reference/tickers?market="+market+"&active=true&limit=100")
	if err != nil { return nil, err }
	var resp struct{ Results []struct{ Ticker string `json:"ticker"`; Name string `json:"name"`; Market string `json:"market"` } `json:"results"` }
	json.Unmarshal(data, &resp)
	assets := make([]*types.Asset, 0, len(resp.Results))
	for _, r := range resp.Results {
		c := "us_equity"
		if r.Market == "crypto" { c = "crypto" } else if r.Market == "fx" { c = "forex" }
		assets = append(assets, &types.Asset{ID: r.Ticker, Provider: "polygon", Symbol: r.Ticker, Name: r.Name, Class: c, Status: "active", Tradable: false})
	}
	return assets, nil
}

func (p *Provider) GetAsset(ctx context.Context, sym string) (*types.Asset, error) {
	data, err := p.doRequest(ctx, "/v3/reference/tickers/"+sym)
	if err != nil { return nil, err }
	var resp struct{ Results struct{ Ticker string `json:"ticker"`; Name string `json:"name"`; Market string `json:"market"` } `json:"results"` }
	json.Unmarshal(data, &resp)
	c := "us_equity"
	if resp.Results.Market == "crypto" { c = "crypto" } else if resp.Results.Market == "fx" { c = "forex" }
	return &types.Asset{ID: resp.Results.Ticker, Provider: "polygon", Symbol: resp.Results.Ticker, Name: resp.Results.Name, Class: c, Status: "active", Tradable: false}, nil
}

// Market Data — primary value of Polygon
func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	data, err := p.doRequest(ctx, "/v2/snapshot/locale/us/markets/stocks/tickers/"+symbol)
	if err != nil {
		// Try crypto
		data, err = p.doRequest(ctx, "/v2/snapshot/locale/global/markets/crypto/tickers/X:"+symbol+"USD")
		if err != nil { return nil, err }
	}
	var resp struct{ Ticker struct {
		LastTrade struct{ P float64 `json:"p"`; S float64 `json:"s"` } `json:"lastTrade"`
		LastQuote struct{ P float64 `json:"P"`; S float64 `json:"S"`; Bp float64 `json:"p"`; Bs float64 `json:"s"` } `json:"lastQuote"`
		Day struct{ O float64 `json:"o"`; H float64 `json:"h"`; L float64 `json:"l"`; C float64 `json:"c"`; V float64 `json:"v"`; VW float64 `json:"vw"` } `json:"day"`
		PrevDay struct{ O float64 `json:"o"`; H float64 `json:"h"`; L float64 `json:"l"`; C float64 `json:"c"`; V float64 `json:"v"` } `json:"prevDay"`
	} `json:"ticker"` }
	json.Unmarshal(data, &resp)
	t := resp.Ticker
	snap := &types.MarketSnapshot{
		Symbol:      symbol,
		LatestTrade: &types.Trade{Price: t.LastTrade.P, Size: t.LastTrade.S},
		LatestQuote: &types.Quote{BidPrice: t.LastQuote.Bp, BidSize: t.LastQuote.Bs, AskPrice: t.LastQuote.P, AskSize: t.LastQuote.S},
		DailyBar:    &types.Bar{Open: t.Day.O, High: t.Day.H, Low: t.Day.L, Close: t.Day.C, Volume: t.Day.V, VWAP: t.Day.VW},
		PrevDailyBar: &types.Bar{Open: t.PrevDay.O, High: t.PrevDay.H, Low: t.PrevDay.L, Close: t.PrevDay.C, Volume: t.PrevDay.V},
	}
	return snap, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err == nil { result[sym] = snap }
	}
	return result, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	multiplier := "1"
	span := "day"
	switch timeframe {
	case "1Min", "1m": multiplier, span = "1", "minute"
	case "5Min", "5m": multiplier, span = "5", "minute"
	case "15Min", "15m": multiplier, span = "15", "minute"
	case "1Hour", "1h": multiplier, span = "1", "hour"
	case "1Day", "1d": multiplier, span = "1", "day"
	case "1Week", "1w": multiplier, span = "1", "week"
	}
	path := fmt.Sprintf("/v2/aggs/ticker/%s/range/%s/%s/%s/%s?limit=%d", symbol, multiplier, span, start, end, limit)
	data, err := p.doRequest(ctx, path)
	if err != nil { return nil, err }
	var resp struct{ Results []struct{ T int64 `json:"t"`; O float64 `json:"o"`; H float64 `json:"h"`; L float64 `json:"l"`; C float64 `json:"c"`; V float64 `json:"v"`; VW float64 `json:"vw"`; N int `json:"n"` } `json:"results"` }
	json.Unmarshal(data, &resp)
	bars := make([]*types.Bar, 0, len(resp.Results))
	for _, r := range resp.Results {
		bars = append(bars, &types.Bar{
			Timestamp: time.UnixMilli(r.T).Format(time.RFC3339),
			Open: r.O, High: r.H, Low: r.L, Close: r.C, Volume: r.V, VWAP: r.VW, TradeCount: r.N,
		})
	}
	return bars, nil
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	result := make(map[string]*types.Trade)
	for _, sym := range symbols {
		data, err := p.doRequest(ctx, "/v2/last/trade/"+sym)
		if err != nil { continue }
		var resp struct{ Results struct{ P float64 `json:"p"`; S float64 `json:"s"`; T int64 `json:"t"` } `json:"results"` }
		json.Unmarshal(data, &resp)
		result[sym] = &types.Trade{Price: resp.Results.P, Size: resp.Results.S, Timestamp: time.UnixMilli(resp.Results.T).Format(time.RFC3339)}
	}
	return result, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	result := make(map[string]*types.Quote)
	for _, sym := range symbols {
		data, err := p.doRequest(ctx, "/v3/quotes/"+sym+"?limit=1&order=desc")
		if err != nil { continue }
		var resp struct{ Results []struct{ BP float64 `json:"bid_price"`; BS float64 `json:"bid_size"`; AP float64 `json:"ask_price"`; AS float64 `json:"ask_size"` } `json:"results"` }
		json.Unmarshal(data, &resp)
		if len(resp.Results) > 0 {
			r := resp.Results[0]
			result[sym] = &types.Quote{BidPrice: r.BP, BidSize: r.BS, AskPrice: r.AP, AskSize: r.AS}
		}
	}
	return result, nil
}

func (p *Provider) GetClock(ctx context.Context) (*types.MarketClock, error) {
	data, err := p.doRequest(ctx, "/v1/marketstatus/now")
	if err != nil { return nil, err }
	var resp struct{ Market string `json:"market"`; ServerTime string `json:"serverTime"` }
	json.Unmarshal(data, &resp)
	return &types.MarketClock{IsOpen: resp.Market == "open", Timestamp: resp.ServerTime}, nil
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, errNotSupported
}
