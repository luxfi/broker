package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/luxfi/broker/pkg/types"
)

// --- CryptoDataProvider implementation ---

func (p *Provider) GetCryptoBars(ctx context.Context, req *types.CryptoBarsRequest) (*types.BarsResponse, error) {
	path := "/v1beta3/crypto/us/bars?symbols=" + url.QueryEscape(strings.Join(req.Symbols, ","))
	if req.Timeframe != "" {
		path += "&timeframe=" + req.Timeframe
	}
	if req.Start != "" {
		path += "&start=" + req.Start
	}
	if req.End != "" {
		path += "&end=" + req.End
	}
	if req.Limit > 0 {
		path += "&limit=" + strconv.Itoa(req.Limit)
	}
	if req.PageToken != "" {
		path += "&page_token=" + req.PageToken
	}

	data, _, err := p.doData(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Bars map[string][]struct {
			T  string  `json:"t"`
			O  float64 `json:"o"`
			H  float64 `json:"h"`
			L  float64 `json:"l"`
			C  float64 `json:"c"`
			V  float64 `json:"v"`
			VW float64 `json:"vw"`
			N  int     `json:"n"`
		} `json:"bars"`
		NextPageToken string `json:"next_page_token"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	resp := &types.BarsResponse{
		Bars:          make(map[string][]*types.Bar),
		NextPageToken: raw.NextPageToken,
	}
	for sym, bars := range raw.Bars {
		for _, b := range bars {
			resp.Bars[sym] = append(resp.Bars[sym], &types.Bar{
				Timestamp:  b.T,
				Open:       b.O,
				High:       b.H,
				Low:        b.L,
				Close:      b.C,
				Volume:     b.V,
				VWAP:       b.VW,
				TradeCount: b.N,
			})
		}
	}
	return resp, nil
}

func (p *Provider) GetCryptoQuotes(ctx context.Context, req *types.CryptoQuotesRequest) (*types.QuotesResponse, error) {
	path := "/v1beta3/crypto/us/quotes?symbols=" + url.QueryEscape(strings.Join(req.Symbols, ","))
	if req.Start != "" {
		path += "&start=" + req.Start
	}
	if req.End != "" {
		path += "&end=" + req.End
	}
	if req.Limit > 0 {
		path += "&limit=" + strconv.Itoa(req.Limit)
	}
	if req.PageToken != "" {
		path += "&page_token=" + req.PageToken
	}

	data, _, err := p.doData(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Quotes map[string][]struct {
			T  string  `json:"t"`
			BP float64 `json:"bp"`
			BS float64 `json:"bs"`
			AP float64 `json:"ap"`
			AS float64 `json:"as"`
		} `json:"quotes"`
		NextPageToken string `json:"next_page_token"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	resp := &types.QuotesResponse{
		Quotes:        make(map[string][]*types.Quote),
		NextPageToken: raw.NextPageToken,
	}
	for sym, quotes := range raw.Quotes {
		for _, q := range quotes {
			resp.Quotes[sym] = append(resp.Quotes[sym], &types.Quote{
				Timestamp: q.T,
				BidPrice:  q.BP,
				BidSize:   q.BS,
				AskPrice:  q.AP,
				AskSize:   q.AS,
			})
		}
	}
	return resp, nil
}

func (p *Provider) GetCryptoTrades(ctx context.Context, req *types.CryptoTradesRequest) (*types.TradesResponse, error) {
	path := "/v1beta3/crypto/us/trades?symbols=" + url.QueryEscape(strings.Join(req.Symbols, ","))
	if req.Start != "" {
		path += "&start=" + req.Start
	}
	if req.End != "" {
		path += "&end=" + req.End
	}
	if req.Limit > 0 {
		path += "&limit=" + strconv.Itoa(req.Limit)
	}
	if req.PageToken != "" {
		path += "&page_token=" + req.PageToken
	}

	data, _, err := p.doData(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Trades map[string][]struct {
			T string  `json:"t"`
			P float64 `json:"p"`
			S float64 `json:"s"`
		} `json:"trades"`
		NextPageToken string `json:"next_page_token"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	resp := &types.TradesResponse{
		Trades:        make(map[string][]*types.Trade),
		NextPageToken: raw.NextPageToken,
	}
	for sym, trades := range raw.Trades {
		for _, t := range trades {
			resp.Trades[sym] = append(resp.Trades[sym], &types.Trade{
				Timestamp: t.T,
				Price:     t.P,
				Size:      t.S,
			})
		}
	}
	return resp, nil
}

// ListCryptoAssets returns all active crypto assets with their specific fields.
func (p *Provider) ListCryptoAssets(ctx context.Context) ([]*types.Asset, error) {
	return p.ListAssets(ctx, "crypto")
}

// ListCryptoWallets lists crypto wallets for a given account.
func (p *Provider) ListCryptoWallets(ctx context.Context, accountID string) ([]types.CryptoWallet, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+accountID+"/wallets/crypto", nil)
	if err != nil {
		return nil, err
	}
	var wallets []types.CryptoWallet
	if err := json.Unmarshal(data, &wallets); err != nil {
		return nil, err
	}
	return wallets, nil
}

// GetCryptoWallet returns a single crypto wallet by ID.
func (p *Provider) GetCryptoWallet(ctx context.Context, accountID, walletID string) (*types.CryptoWallet, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+accountID+"/wallets/crypto/"+walletID, nil)
	if err != nil {
		return nil, err
	}
	var w types.CryptoWallet
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (p *Provider) GetCryptoSnapshots(ctx context.Context, symbols []string) (map[string]*types.CryptoSnapshot, error) {
	path := "/v1beta3/crypto/us/snapshots?symbols=" + strings.Join(symbols, ",")
	data, _, err := p.doData(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Snapshots map[string]struct {
			LatestTrade *struct {
				T string  `json:"t"`
				P float64 `json:"p"`
				S float64 `json:"s"`
			} `json:"latestTrade"`
			LatestQuote *struct {
				T  string  `json:"t"`
				BP float64 `json:"bp"`
				BS float64 `json:"bs"`
				AP float64 `json:"ap"`
				AS float64 `json:"as"`
			} `json:"latestQuote"`
			MinuteBar *struct {
				T  string  `json:"t"`
				O  float64 `json:"o"`
				H  float64 `json:"h"`
				L  float64 `json:"l"`
				C  float64 `json:"c"`
				V  float64 `json:"v"`
				VW float64 `json:"vw"`
				N  int     `json:"n"`
			} `json:"minuteBar"`
			DailyBar *struct {
				T  string  `json:"t"`
				O  float64 `json:"o"`
				H  float64 `json:"h"`
				L  float64 `json:"l"`
				C  float64 `json:"c"`
				V  float64 `json:"v"`
				VW float64 `json:"vw"`
				N  int     `json:"n"`
			} `json:"dailyBar"`
			PrevDailyBar *struct {
				T  string  `json:"t"`
				O  float64 `json:"o"`
				H  float64 `json:"h"`
				L  float64 `json:"l"`
				C  float64 `json:"c"`
				V  float64 `json:"v"`
				VW float64 `json:"vw"`
				N  int     `json:"n"`
			} `json:"prevDailyBar"`
		} `json:"snapshots"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	result := make(map[string]*types.CryptoSnapshot, len(raw.Snapshots))
	for sym, s := range raw.Snapshots {
		snap := &types.CryptoSnapshot{}
		if s.LatestTrade != nil {
			snap.LatestTrade = &types.Trade{Timestamp: s.LatestTrade.T, Price: s.LatestTrade.P, Size: s.LatestTrade.S}
		}
		if s.LatestQuote != nil {
			snap.LatestQuote = &types.Quote{Timestamp: s.LatestQuote.T, BidPrice: s.LatestQuote.BP, BidSize: s.LatestQuote.BS, AskPrice: s.LatestQuote.AP, AskSize: s.LatestQuote.AS}
		}
		if s.MinuteBar != nil {
			snap.MinuteBar = &types.Bar{Timestamp: s.MinuteBar.T, Open: s.MinuteBar.O, High: s.MinuteBar.H, Low: s.MinuteBar.L, Close: s.MinuteBar.C, Volume: s.MinuteBar.V, VWAP: s.MinuteBar.VW, TradeCount: s.MinuteBar.N}
		}
		if s.DailyBar != nil {
			snap.DailyBar = &types.Bar{Timestamp: s.DailyBar.T, Open: s.DailyBar.O, High: s.DailyBar.H, Low: s.DailyBar.L, Close: s.DailyBar.C, Volume: s.DailyBar.V, VWAP: s.DailyBar.VW, TradeCount: s.DailyBar.N}
		}
		if s.PrevDailyBar != nil {
			snap.PrevDailyBar = &types.Bar{Timestamp: s.PrevDailyBar.T, Open: s.PrevDailyBar.O, High: s.PrevDailyBar.H, Low: s.PrevDailyBar.L, Close: s.PrevDailyBar.C, Volume: s.PrevDailyBar.V, VWAP: s.PrevDailyBar.VW, TradeCount: s.PrevDailyBar.N}
		}
		result[sym] = snap
	}
	return result, nil
}
