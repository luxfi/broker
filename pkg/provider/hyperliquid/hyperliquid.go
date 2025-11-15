package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

const DefaultBaseURL = "https://api.hyperliquid.xyz"

var ErrNotImplemented = fmt.Errorf("hyperliquid: not implemented (requires wallet signing)")

type Config struct {
	BaseURL     string `json:"base_url"`
	UserAddress string `json:"user_address"`
}

type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "hyperliquid" }

func (p *Provider) postInfo(ctx context.Context, payload any) ([]byte, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/info", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("hyperliquid %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (p *Provider) requireAddr() error {
	if p.cfg.UserAddress == "" {
		return fmt.Errorf("hyperliquid: user_address not configured")
	}
	return nil
}

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	data, err := p.postInfo(ctx, map[string]string{"type": "meta"})
	if err != nil {
		return nil, err
	}
	var meta struct{ Universe []struct{ Name string `json:"name"` } `json:"universe"` }
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("hyperliquid: parse meta: %w", err)
	}
	out := make([]*types.Asset, 0, len(meta.Universe))
	for _, u := range meta.Universe {
		out = append(out, &types.Asset{
			ID: u.Name, Provider: "hyperliquid", Symbol: u.Name + "/USD",
			Name: u.Name + " Perpetual", Class: "crypto", Exchange: "hyperliquid",
			Status: "active", Tradable: true,
		})
	}
	return out, nil
}

func (p *Provider) GetAsset(_ context.Context, symbolOrID string) (*types.Asset, error) {
	coin := normalizeCoin(symbolOrID)
	return &types.Asset{
		ID: coin, Provider: "hyperliquid", Symbol: coin + "/USD",
		Name: coin + " Perpetual", Class: "crypto", Exchange: "hyperliquid",
		Status: "active", Tradable: true,
	}, nil
}

func (p *Provider) fetchMids(ctx context.Context) (map[string]string, error) {
	data, err := p.postInfo(ctx, map[string]string{"type": "allMids"})
	if err != nil {
		return nil, err
	}
	var m map[string]string
	return m, json.Unmarshal(data, &m)
}

func (p *Provider) fetchQuote(ctx context.Context, coin string) *types.Quote {
	data, err := p.postInfo(ctx, map[string]any{"type": "l2Book", "coin": coin})
	if err != nil {
		return &types.Quote{}
	}
	var book struct {
		Levels [][]struct {
			Px string `json:"px"`
			Sz string `json:"sz"`
		} `json:"levels"`
	}
	if json.Unmarshal(data, &book) != nil || len(book.Levels) < 2 {
		return &types.Quote{}
	}
	q := &types.Quote{Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if b := book.Levels[0]; len(b) > 0 {
		q.BidPrice, q.BidSize = pf(b[0].Px), pf(b[0].Sz)
	}
	if a := book.Levels[1]; len(a) > 0 {
		q.AskPrice, q.AskSize = pf(a[0].Px), pf(a[0].Sz)
	}
	return q
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	coin, now := normalizeCoin(symbol), time.Now().UTC().Format(time.RFC3339)
	mids, err := p.fetchMids(ctx)
	if err != nil {
		return nil, err
	}
	midStr, ok := mids[coin]
	if !ok {
		return nil, fmt.Errorf("hyperliquid: no price for %s", coin)
	}
	q := p.fetchQuote(ctx, coin)
	return &types.MarketSnapshot{
		Symbol: symbol, LatestTrade: &types.Trade{Timestamp: now, Price: pf(midStr)}, LatestQuote: q,
	}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	mids, err := p.fetchMids(ctx)
	if err != nil {
		return nil, err
	}
	now, out := time.Now().UTC().Format(time.RFC3339), make(map[string]*types.MarketSnapshot, len(symbols))
	for _, sym := range symbols {
		if ms, ok := mids[normalizeCoin(sym)]; ok {
			out[sym] = &types.MarketSnapshot{Symbol: sym, LatestTrade: &types.Trade{Timestamp: now, Price: pf(ms)}}
		}
	}
	return out, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	coin, interval := normalizeCoin(symbol), mapTimeframe(timeframe)
	sMs, eMs := parseTimeRange(start, end)
	data, err := p.postInfo(ctx, map[string]any{
		"type": "candleSnapshot", "coin": coin, "interval": interval,
		"startTime": sMs, "endTime": eMs,
	})
	if err != nil {
		return nil, err
	}
	var candles []struct {
		T int64  `json:"t"`
		O string `json:"o"`
		H string `json:"h"`
		L string `json:"l"`
		C string `json:"c"`
		V string `json:"v"`
		N int    `json:"n"`
	}
	if json.Unmarshal(data, &candles) != nil {
		return nil, fmt.Errorf("hyperliquid: parse candles")
	}
	bars := make([]*types.Bar, 0, len(candles))
	for _, c := range candles {
		bars = append(bars, &types.Bar{
			Timestamp: time.UnixMilli(c.T).UTC().Format(time.RFC3339), TimeMs: c.T,
			Open: pf(c.O), High: pf(c.H), Low: pf(c.L), Close: pf(c.C),
			Volume: pf(c.V), TradeCount: c.N,
		})
	}
	if limit > 0 && len(bars) > limit {
		bars = bars[len(bars)-limit:]
	}
	return bars, nil
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	out := make(map[string]*types.Trade, len(symbols))
	for _, sym := range symbols {
		data, err := p.postInfo(ctx, map[string]any{"type": "recentTrades", "coin": normalizeCoin(sym)})
		if err != nil {
			continue
		}
		var tr []struct{ Px, Sz string; Time int64 `json:"time"` }
		if json.Unmarshal(data, &tr) != nil || len(tr) == 0 {
			continue
		}
		t := tr[len(tr)-1]
		out[sym] = &types.Trade{
			Timestamp: time.UnixMilli(t.Time).UTC().Format(time.RFC3339),
			Price: pf(t.Px), Size: pf(t.Sz), Exchange: "hyperliquid",
		}
	}
	return out, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	out := make(map[string]*types.Quote, len(symbols))
	for _, s := range symbols {
		out[s] = p.fetchQuote(ctx, normalizeCoin(s))
	}
	return out, nil
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return &types.MarketClock{Timestamp: time.Now().UTC().Format(time.RFC3339), IsOpen: true}, nil
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, nil // crypto perps trade 24/7
}

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("hyperliquid: accounts are ethereum wallets")
}

func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error) {
	if err := p.requireAddr(); err != nil {
		return nil, err
	}
	return &types.Account{Provider: "hyperliquid", ProviderID: p.cfg.UserAddress, Status: "active", Currency: "USD"}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	a, err := p.GetAccount(ctx, "")
	if err != nil {
		return nil, err
	}
	return []*types.Account{a}, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	if err := p.requireAddr(); err != nil {
		return nil, err
	}
	data, err := p.postInfo(ctx, map[string]string{"type": "clearinghouseState", "user": p.cfg.UserAddress})
	if err != nil {
		return nil, err
	}
	var state struct {
		MarginSummary struct {
			AccountValue string `json:"accountValue"`
			TotalNtlPos  string `json:"totalNtlPos"`
		} `json:"marginSummary"`
		AssetPositions []struct {
			Position struct {
				Coin          string `json:"coin"`
				Szi           string `json:"szi"`
				EntryPx       string `json:"entryPx"`
				PositionValue string `json:"positionValue"`
				UnrealizedPnl string `json:"unrealizedPnl"`
			} `json:"position"`
		} `json:"assetPositions"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("hyperliquid: parse state: %w", err)
	}
	positions := make([]types.Position, 0, len(state.AssetPositions))
	for _, ap := range state.AssetPositions {
		pos := ap.Position
		if pf(pos.Szi) == 0 {
			continue
		}
		side := "long"
		if pf(pos.Szi) < 0 {
			side = "short"
		}
		positions = append(positions, types.Position{
			Symbol: pos.Coin + "/USD", Qty: pos.Szi, AvgEntryPrice: pos.EntryPx,
			MarketValue: pos.PositionValue, UnrealizedPL: pos.UnrealizedPnl,
			Side: side, AssetClass: "crypto",
		})
	}
	return &types.Portfolio{
		AccountID: p.cfg.UserAddress, Equity: state.MarginSummary.AccountValue,
		PortfolioValue: state.MarginSummary.TotalNtlPos, Positions: positions,
	}, nil
}

func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	if err := p.requireAddr(); err != nil {
		return nil, err
	}
	data, err := p.postInfo(ctx, map[string]string{"type": "openOrders", "user": p.cfg.UserAddress})
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Coin      string `json:"coin"`
		Side      string `json:"side"`
		LimitPx   string `json:"limitPx"`
		Sz        string `json:"sz"`
		Oid       int64  `json:"oid"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("hyperliquid: parse orders: %w", err)
	}
	orders := make([]*types.Order, 0, len(raw))
	for _, o := range raw {
		side := strings.ToLower(o.Side)
		if side == "a" {
			side = "sell"
		} else if side == "b" {
			side = "buy"
		}
		orders = append(orders, &types.Order{
			Provider: "hyperliquid", ProviderID: strconv.FormatInt(o.Oid, 10),
			Symbol: o.Coin + "/USD", Qty: o.Sz, Side: side, Type: "limit",
			LimitPrice: o.LimitPx, Status: "open", CreatedAt: time.UnixMilli(o.Timestamp),
		})
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	orders, err := p.ListOrders(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		if o.ProviderID == orderID {
			return o, nil
		}
	}
	return nil, fmt.Errorf("hyperliquid: order %s not found", orderID)
}
func (p *Provider) CancelOrder(_ context.Context, _, _ string) error { return ErrNotImplemented }
func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) { return nil, nil }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return nil, nil
}

func pf(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }

func normalizeCoin(s string) string {
	s = strings.ToUpper(strings.TrimSuffix(strings.ToUpper(s), "-PERP"))
	if i := strings.IndexAny(s, "/-"); i > 0 {
		s = s[:i]
	}
	return s
}

var tfMap = map[string]string{
	"1min": "1m", "1m": "1m", "5min": "5m", "5m": "5m", "15min": "15m", "15m": "15m",
	"30min": "30m", "30m": "30m", "1h": "1h", "1hour": "1h", "4h": "4h", "4hour": "4h",
	"1d": "1d", "1day": "1d",
}

func mapTimeframe(tf string) string {
	if v, ok := tfMap[strings.ToLower(tf)]; ok {
		return v
	}
	return "1h"
}

func parseTimeRange(start, end string) (int64, int64) {
	now := time.Now().UTC()
	s, e := now.Add(-24*time.Hour), now
	for _, f := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(f, start); err == nil {
			s = t
			break
		}
	}
	for _, f := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(f, end); err == nil {
			e = t
			break
		}
	}
	return s.UnixMilli(), e.UnixMilli()
}
