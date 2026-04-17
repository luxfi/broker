// Package alpaca_omnisub implements the broker Provider interface for
// Alpaca's OmniSub (Omnibus + Sub-accounts) model. Sub-accounts are
// created under a single omnibus master; orders and positions are
// scoped per-sub, while cash flows through the omnibus aggregate.
package alpaca_omnisub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

const (
	SandboxURL     = "https://broker-api.sandbox.alpaca.markets"
	ProductionURL  = "https://broker-api.alpaca.markets"
	DataURL        = "https://data.alpaca.markets"
	DataSandboxURL = "https://data.sandbox.alpaca.markets"
)

// Config for the Alpaca OmniSub provider.
//
// Auth: supply EITHER (APIKey + APISecret) for legacy HTTP-Basic auth,
// OR (ClientID + PrivateKeyPEM) for JWT-P-256 client-credentials. If both
// are set, JWT takes precedence. JWT is the canonical post-2025 auth mode
// and is required for newly-issued Alpaca partner credentials.
type Config struct {
	BaseURL          string `json:"base_url"`
	APIKey           string `json:"-"`
	APISecret        string `json:"-"`
	OmnibusAccountID string `json:"omnibus_account_id"`

	// JWT-P-256 client credentials (Alpaca's canonical auth).
	ClientID      string `json:"-"`
	PrivateKeyPEM string `json:"-"`
}

// Provider implements the broker Provider interface for Alpaca OmniSub.
type Provider struct {
	cfg     Config
	client  *http.Client
	dataURL string
	jwt     *jwtSigner // nil when using legacy Basic auth
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	dataURL := DataURL
	if strings.Contains(cfg.BaseURL, "sandbox") {
		dataURL = DataSandboxURL
	}
	p := &Provider{
		cfg:     cfg,
		dataURL: dataURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	if cfg.ClientID != "" && cfg.PrivateKeyPEM != "" {
		signer, err := newJWTSigner(cfg.ClientID, cfg.PrivateKeyPEM, cfg.BaseURL, &http.Client{Timeout: 10 * time.Second})
		if err != nil {
			// Constructor can't return error per interface; log via panic in
			// tests / early-boot. Callers should catch this at bootstrap.
			panic(fmt.Sprintf("alpaca_omnisub: jwt signer init: %v", err))
		}
		p.jwt = signer
	}
	return p
}

func (p *Provider) Name() string { return "alpaca_omnisub" }

// --- HTTP helpers ---

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	fullURL := p.cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, 0, err
	}
	// Unified auth: JWT Bearer when configured, else legacy HTTP-Basic.
	// Both terminate at the same Broker API; only the auth header differs.
	if p.jwt != nil {
		tok, terr := p.jwt.Token(ctx)
		if terr != nil {
			return nil, 0, fmt.Errorf("alpaca_omnisub: get access token: %w", terr)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	} else {
		req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return nil, resp.StatusCode, fmt.Errorf("alpaca_omnisub %d: %s", apiErr.Code, apiErr.Message)
		}
		return nil, resp.StatusCode, fmt.Errorf("alpaca_omnisub %d: %s", resp.StatusCode, string(data))
	}

	return data, resp.StatusCode, nil
}

func (p *Provider) doData(ctx context.Context, method, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.dataURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("alpaca_omnisub data %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Market Data (shared Alpaca data API, identical to standalone) ---

func isCryptoSymbol(symbol string) bool {
	return strings.Contains(symbol, "/")
}

func stocksOrCryptoPath(symbol string) string {
	if isCryptoSymbol(symbol) {
		return "/v1beta3/crypto/us"
	}
	return "/v2/stocks"
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	if isCryptoSymbol(symbol) {
		path := "/v1beta3/crypto/us/snapshots?symbols=" + url.QueryEscape(symbol)
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var wrapper struct {
			Snapshots map[string]json.RawMessage `json:"snapshots"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("parse crypto snapshots: %w", err)
		}
		raw, ok := wrapper.Snapshots[symbol]
		if !ok {
			return nil, fmt.Errorf("crypto snapshot not found for %s", symbol)
		}
		return p.parseSnapshot(symbol, raw)
	}
	data, _, err := p.doData(ctx, http.MethodGet, "/v2/stocks/"+symbol+"/snapshot")
	if err != nil {
		return nil, err
	}
	return p.parseSnapshot(symbol, data)
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	var stocks, cryptos []string
	for _, s := range symbols {
		if isCryptoSymbol(s) {
			cryptos = append(cryptos, s)
		} else {
			stocks = append(stocks, s)
		}
	}

	result := make(map[string]*types.MarketSnapshot)

	if len(stocks) > 0 {
		path := "/v2/stocks/snapshots?symbols=" + url.QueryEscape(strings.Join(stocks, ","))
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, d := range raw {
			snap, err := p.parseSnapshot(sym, d)
			if err != nil {
				continue
			}
			result[sym] = snap
		}
	}

	if len(cryptos) > 0 {
		path := "/v1beta3/crypto/us/snapshots?symbols=" + url.QueryEscape(strings.Join(cryptos, ","))
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var wrapper struct {
			Snapshots map[string]json.RawMessage `json:"snapshots"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, err
		}
		for sym, d := range wrapper.Snapshots {
			snap, err := p.parseSnapshot(sym, d)
			if err != nil {
				continue
			}
			result[sym] = snap
		}
	}

	return result, nil
}

func (p *Provider) parseSnapshot(symbol string, data []byte) (*types.MarketSnapshot, error) {
	var raw struct {
		LatestTrade *struct {
			T string  `json:"t"`
			P float64 `json:"p"`
			S float64 `json:"s"`
			X string  `json:"x"`
		} `json:"latestTrade"`
		LatestQuote *struct {
			T  string  `json:"t"`
			BP float64 `json:"bp"`
			BS float64 `json:"bs"`
			AP float64 `json:"ap"`
			AS float64 `json:"as"`
		} `json:"latestQuote"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	snap := &types.MarketSnapshot{Symbol: symbol}
	if raw.LatestTrade != nil {
		snap.LatestTrade = &types.Trade{Timestamp: raw.LatestTrade.T, Price: raw.LatestTrade.P, Size: raw.LatestTrade.S, Exchange: raw.LatestTrade.X}
	}
	if raw.LatestQuote != nil {
		snap.LatestQuote = &types.Quote{Timestamp: raw.LatestQuote.T, BidPrice: raw.LatestQuote.BP, BidSize: raw.LatestQuote.BS, AskPrice: raw.LatestQuote.AP, AskSize: raw.LatestQuote.AS}
	}
	return snap, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	var allBars []*types.Bar
	nextPageToken := ""

	crypto := isCryptoSymbol(symbol)

	for page := 0; page < 100; page++ {
		var path string
		if crypto {
			path = "/v1beta3/crypto/us/bars?symbols=" + symbol + "&timeframe=" + timeframe
		} else {
			path = stocksOrCryptoPath(symbol) + "/" + symbol + "/bars?timeframe=" + timeframe
		}
		if start != "" {
			path += "&start=" + start
		}
		if end != "" {
			path += "&end=" + end
		}
		if limit > 0 {
			path += "&limit=" + strconv.Itoa(limit)
		}
		if nextPageToken != "" {
			path += "&page_token=" + nextPageToken
		}

		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}

		type barEntry struct {
			T  string  `json:"t"`
			O  float64 `json:"o"`
			H  float64 `json:"h"`
			L  float64 `json:"l"`
			C  float64 `json:"c"`
			V  float64 `json:"v"`
			VW float64 `json:"vw"`
			N  int     `json:"n"`
		}

		var bars []barEntry
		var nextToken string

		if crypto {
			var raw struct {
				Bars          map[string][]barEntry `json:"bars"`
				NextPageToken string                `json:"next_page_token"`
			}
			if err := json.Unmarshal(data, &raw); err != nil {
				return nil, err
			}
			bars = raw.Bars[symbol]
			nextToken = raw.NextPageToken
		} else {
			var raw struct {
				Bars          []barEntry `json:"bars"`
				NextPageToken string     `json:"next_page_token"`
			}
			if err := json.Unmarshal(data, &raw); err != nil {
				return nil, err
			}
			bars = raw.Bars
			nextToken = raw.NextPageToken
		}

		for _, b := range bars {
			var timeMs int64
			if t, err := time.Parse(time.RFC3339, b.T); err == nil {
				timeMs = t.UnixMilli()
			} else if t, err := time.Parse(time.RFC3339Nano, b.T); err == nil {
				timeMs = t.UnixMilli()
			}
			allBars = append(allBars, &types.Bar{
				Timestamp:  b.T,
				TimeMs:     timeMs,
				Open:       b.O,
				High:       b.H,
				Low:        b.L,
				Close:      b.C,
				Volume:     b.V,
				VWAP:       b.VW,
				TradeCount: b.N,
			})
		}

		if nextToken == "" {
			break
		}
		if limit > 0 && len(allBars) >= limit {
			allBars = allBars[:limit]
			break
		}
		nextPageToken = nextToken
	}

	return allBars, nil
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	result := make(map[string]*types.Trade)
	var stocks, cryptos []string
	for _, s := range symbols {
		if isCryptoSymbol(s) {
			cryptos = append(cryptos, s)
		} else {
			stocks = append(stocks, s)
		}
	}
	if len(stocks) > 0 {
		path := "/v2/stocks/trades/latest?symbols=" + strings.Join(stocks, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Trades map[string]struct {
				T string  `json:"t"`
				P float64 `json:"p"`
				S float64 `json:"s"`
				X string  `json:"x"`
			} `json:"trades"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, t := range raw.Trades {
			result[sym] = &types.Trade{Timestamp: t.T, Price: t.P, Size: t.S, Exchange: t.X}
		}
	}
	if len(cryptos) > 0 {
		path := "/v1beta3/crypto/us/latest/trades?symbols=" + strings.Join(cryptos, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Trades map[string]struct {
				T string  `json:"t"`
				P float64 `json:"p"`
				S float64 `json:"s"`
			} `json:"trades"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, t := range raw.Trades {
			result[sym] = &types.Trade{Timestamp: t.T, Price: t.P, Size: t.S}
		}
	}
	return result, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	result := make(map[string]*types.Quote)
	var stocks, cryptos []string
	for _, s := range symbols {
		if isCryptoSymbol(s) {
			cryptos = append(cryptos, s)
		} else {
			stocks = append(stocks, s)
		}
	}
	if len(stocks) > 0 {
		path := "/v2/stocks/quotes/latest?symbols=" + strings.Join(stocks, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Quotes map[string]struct {
				T  string  `json:"t"`
				BP float64 `json:"bp"`
				BS float64 `json:"bs"`
				AP float64 `json:"ap"`
				AS float64 `json:"as"`
			} `json:"quotes"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, q := range raw.Quotes {
			result[sym] = &types.Quote{Timestamp: q.T, BidPrice: q.BP, BidSize: q.BS, AskPrice: q.AP, AskSize: q.AS}
		}
	}
	if len(cryptos) > 0 {
		path := "/v1beta3/crypto/us/latest/quotes?symbols=" + strings.Join(cryptos, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Quotes map[string]struct {
				T  string  `json:"t"`
				BP float64 `json:"bp"`
				BS float64 `json:"bs"`
				AP float64 `json:"ap"`
				AS float64 `json:"as"`
			} `json:"quotes"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, q := range raw.Quotes {
			result[sym] = &types.Quote{Timestamp: q.T, BidPrice: q.BP, BidSize: q.BS, AskPrice: q.AP, AskSize: q.AS}
		}
	}
	return result, nil
}

func (p *Provider) GetClock(ctx context.Context) (*types.MarketClock, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/clock", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Timestamp string `json:"timestamp"`
		IsOpen    bool   `json:"is_open"`
		NextOpen  string `json:"next_open"`
		NextClose string `json:"next_close"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.MarketClock{
		Timestamp: raw.Timestamp,
		IsOpen:    raw.IsOpen,
		NextOpen:  raw.NextOpen,
		NextClose: raw.NextClose,
	}, nil
}

func (p *Provider) GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error) {
	path := "/v1/calendar"
	sep := "?"
	if start != "" {
		path += sep + "start=" + start
		sep = "&"
	}
	if end != "" {
		path += sep + "end=" + end
	}
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Date  string `json:"date"`
		Open  string `json:"open"`
		Close string `json:"close"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	days := make([]*types.MarketCalendarDay, len(raw))
	for i, d := range raw {
		days[i] = &types.MarketCalendarDay{Date: d.Date, Open: d.Open, Close: d.Close}
	}
	return days, nil
}

// --- Assets (same Alpaca API, different provider tag) ---

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	var assets []*types.Asset

	// Standard asset listing (equities, crypto, etc.)
	if class != "fixed_income" {
		path := "/v1/assets?status=active"
		if class != "" {
			path += "&asset_class=" + class
		}
		data, _, err := p.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var raw []struct {
			ID                string `json:"id"`
			Symbol            string `json:"symbol"`
			Name              string `json:"name"`
			Class             string `json:"class"`
			Exchange          string `json:"exchange"`
			Status            string `json:"status"`
			Tradable          bool   `json:"tradable"`
			Fractionable      bool   `json:"fractionable"`
			MinOrderSize      string `json:"min_order_size"`
			PriceIncrement    string `json:"price_increment"`
			MinTradeIncrement string `json:"min_trade_increment"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for _, r := range raw {
			assets = append(assets, &types.Asset{
				ID:                r.ID,
				Provider:          "alpaca_omnisub",
				Symbol:            r.Symbol,
				Name:              r.Name,
				Class:             r.Class,
				Exchange:          r.Exchange,
				Status:            r.Status,
				Tradable:          r.Tradable,
				Fractionable:      r.Fractionable,
				MinOrderSize:      r.MinOrderSize,
				PriceIncrement:    r.PriceIncrement,
				MinTradeIncrement: r.MinTradeIncrement,
			})
		}
	}

	// Fixed income assets: Alpaca serves these from separate endpoints.
	if class == "" || class == "fixed_income" {
		assets = append(assets, p.listFIAssets(ctx, "/v1/assets/fixed_income/us_treasuries", "us_treasuries")...)
		assets = append(assets, p.listFIAssets(ctx, "/v1/assets/fixed_income/us_corporates", "us_corporates")...)
	}

	return assets, nil
}

// listFIAssets fetches fixed income assets from one of Alpaca's FI endpoints.
// Returns nil on error (403 if not subscribed, network issues, etc.) — callers
// treat FI as best-effort since corporate bonds require a separate subscription.
func (p *Provider) listFIAssets(ctx context.Context, path, key string) []*types.Asset {
	data, status, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil || status >= 400 {
		return nil
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil
	}
	raw, ok := wrapper[key]
	if !ok {
		return nil
	}
	var bonds []struct {
		CUSIP        string `json:"cusip"`
		ISIN         string `json:"isin"`
		Tradable     bool   `json:"tradable"`
		BondStatus   string `json:"bond_status"`
		Subtype      string `json:"subtype"`
		CouponRate   string `json:"coupon_rate"`
		MaturityDate string `json:"maturity_date"`
	}
	if err := json.Unmarshal(raw, &bonds); err != nil {
		return nil
	}
	var assets []*types.Asset
	for _, b := range bonds {
		name := "US Treasury " + b.Subtype
		if key == "us_corporates" {
			name = "US Corporate " + b.Subtype
		}
		assets = append(assets, &types.Asset{
			ID:       b.CUSIP,
			Provider: "alpaca_omnisub",
			Symbol:   b.CUSIP,
			Name:     name,
			Class:    "fixed_income",
			Status:   b.BondStatus,
			Tradable: b.BondStatus == "outstanding" && b.Tradable,
		})
	}
	return assets
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	apiSymbol := strings.ReplaceAll(symbolOrID, "/", "")
	data, _, err := p.do(ctx, http.MethodGet, "/v1/assets/"+apiSymbol, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID           string `json:"id"`
		Symbol       string `json:"symbol"`
		Name         string `json:"name"`
		Class        string `json:"class"`
		Exchange     string `json:"exchange"`
		Status       string `json:"status"`
		Tradable     bool   `json:"tradable"`
		Fractionable bool   `json:"fractionable"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.Asset{
		ID:           raw.ID,
		Provider:     "alpaca_omnisub",
		Symbol:       raw.Symbol,
		Name:         raw.Name,
		Class:        raw.Class,
		Exchange:     raw.Exchange,
		Status:       raw.Status,
		Tradable:     raw.Tradable,
		Fractionable: raw.Fractionable,
	}, nil
}
