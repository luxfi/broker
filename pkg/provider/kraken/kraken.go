package kraken

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
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
	ProdURL = "https://api.kraken.com"
)

// Config for Kraken exchange.
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"` // base64-encoded
}

type Provider struct {
	cfg    Config
	client *http.Client
	secret []byte
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = ProdURL
	}
	secret, _ := base64.StdEncoding.DecodeString(cfg.APISecret)
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}, secret: secret}
}

func (p *Provider) Name() string { return "kraken" }

func (p *Provider) doPublic(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/0/public"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kraken %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (p *Provider) doPrivate(ctx context.Context, path string, params url.Values) ([]byte, error) {
	fullPath := "/0/private" + path
	if params == nil {
		params = url.Values{}
	}
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)
	params.Set("nonce", nonce)
	body := params.Encode()

	// Kraken signature: HMAC-SHA512(path + SHA256(nonce + body), base64decode(secret))
	sha := sha256.Sum256([]byte(nonce + body))
	mac := hmac.New(sha512.New, p.secret)
	mac.Write(append([]byte(fullPath), sha[:]...))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+fullPath, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("API-Key", p.cfg.APIKey)
	req.Header.Set("API-Sign", sig)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kraken %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Error  []string        `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	json.Unmarshal(data, &result)
	if len(result.Error) > 0 {
		return nil, fmt.Errorf("kraken: %s", strings.Join(result.Error, "; "))
	}
	return result.Result, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("kraken: account creation via kraken.com")
}

func (p *Provider) GetAccount(ctx context.Context, _ string) (*types.Account, error) {
	return &types.Account{Provider: "kraken", ProviderID: "default", Status: "active", Currency: "USD"}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	acct, _ := p.GetAccount(ctx, "")
	return []*types.Account{acct}, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	data, err := p.doPrivate(ctx, "/Balance", nil)
	if err != nil {
		return nil, err
	}
	var balances map[string]string
	json.Unmarshal(data, &balances)

	var positions []types.Position
	var cash string
	for asset, bal := range balances {
		f, _ := strconv.ParseFloat(bal, 64)
		if f <= 0 {
			continue
		}
		if asset == "ZUSD" || asset == "USD" {
			cash = fmt.Sprintf("%.2f", f)
			continue
		}
		positions = append(positions, types.Position{
			Symbol: normalizeKrakenAsset(asset), Qty: bal, AssetClass: "crypto",
		})
	}
	return &types.Portfolio{Cash: cash, Positions: positions}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	params := url.Values{}
	params.Set("pair", krakenPair(req.Symbol))
	params.Set("type", req.Side)
	params.Set("ordertype", req.Type)
	params.Set("volume", req.Qty)
	if req.LimitPrice != "" {
		params.Set("price", req.LimitPrice)
	}

	data, err := p.doPrivate(ctx, "/AddOrder", params)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Descr struct{ Order string `json:"order"` } `json:"descr"`
		TxID  []string                               `json:"txid"`
	}
	json.Unmarshal(data, &raw)
	orderID := ""
	if len(raw.TxID) > 0 {
		orderID = raw.TxID[0]
	}
	return &types.Order{
		Provider: "kraken", ProviderID: orderID,
		Symbol: req.Symbol, Qty: req.Qty, Side: req.Side,
		Type: req.Type, Status: "new", CreatedAt: time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, err := p.doPrivate(ctx, "/OpenOrders", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Open map[string]struct {
			Descr struct {
				Pair  string `json:"pair"`
				Type  string `json:"type"`
				Order string `json:"ordertype"`
			} `json:"descr"`
			Vol       string `json:"vol"`
			VolExec   string `json:"vol_exec"`
			Status    string `json:"status"`
		} `json:"open"`
	}
	json.Unmarshal(data, &raw)
	orders := make([]*types.Order, 0, len(raw.Open))
	for id, o := range raw.Open {
		orders = append(orders, &types.Order{
			Provider: "kraken", ProviderID: id,
			Symbol: o.Descr.Pair, Side: o.Descr.Type, Type: o.Descr.Order,
			Qty: o.Vol, FilledQty: o.VolExec, Status: o.Status,
		})
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	params := url.Values{}
	params.Set("txid", orderID)
	data, err := p.doPrivate(ctx, "/QueryOrders", params)
	if err != nil {
		return nil, err
	}
	var raw map[string]struct {
		Descr struct {
			Pair  string `json:"pair"`
			Type  string `json:"type"`
		} `json:"descr"`
		Vol     string `json:"vol"`
		VolExec string `json:"vol_exec"`
		Status  string `json:"status"`
	}
	json.Unmarshal(data, &raw)
	if o, ok := raw[orderID]; ok {
		return &types.Order{
			Provider: "kraken", ProviderID: orderID,
			Symbol: o.Descr.Pair, Side: o.Descr.Type,
			Qty: o.Vol, FilledQty: o.VolExec, Status: o.Status,
		}, nil
	}
	return nil, fmt.Errorf("kraken: order %s not found", orderID)
}

func (p *Provider) CancelOrder(ctx context.Context, _ string, orderID string) error {
	params := url.Values{}
	params.Set("txid", orderID)
	_, err := p.doPrivate(ctx, "/CancelOrder", params)
	return err
}

// --- Stubs ---

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("kraken: transfers via kraken.com")
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return []*types.Transfer{}, nil
}
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("kraken: bank linking via kraken.com")
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	data, err := p.doPublic(ctx, "/AssetPairs")
	if err != nil {
		return defaultAssets(), nil
	}
	var resp struct {
		Error  []string                          `json:"error"`
		Result map[string]struct {
			Base  string `json:"base"`
			Quote string `json:"quote"`
		} `json:"result"`
	}
	json.Unmarshal(data, &resp)
	assets := make([]*types.Asset, 0)
	for pair, info := range resp.Result {
		q := normalizeKrakenAsset(info.Quote)
		if q != "USD" && q != "USDT" {
			continue
		}
		b := normalizeKrakenAsset(info.Base)
		assets = append(assets, &types.Asset{
			ID: pair, Provider: "kraken", Symbol: b + "/" + q,
			Class: "crypto", Status: "active", Tradable: true,
		})
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{ID: sym, Provider: "kraken", Symbol: sym, Class: "crypto", Status: "active", Tradable: true}, nil
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	pair := krakenPair(symbol)
	data, err := p.doPublic(ctx, "/Ticker?pair="+pair)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Error  []string                   `json:"error"`
		Result map[string]struct {
			A []string `json:"a"` // ask [price, whole_lot_volume, lot_volume]
			B []string `json:"b"` // bid
			C []string `json:"c"` // last trade closed [price, lot_volume]
			V []string `json:"v"` // volume [today, 24h]
			H []string `json:"h"` // high
			L []string `json:"l"` // low
			O string   `json:"o"` // open
		} `json:"result"`
	}
	json.Unmarshal(data, &resp)
	for _, t := range resp.Result {
		bid, _ := strconv.ParseFloat(t.B[0], 64)
		ask, _ := strconv.ParseFloat(t.A[0], 64)
		last, _ := strconv.ParseFloat(t.C[0], 64)
		vol, _ := strconv.ParseFloat(t.V[1], 64)
		hi, _ := strconv.ParseFloat(t.H[1], 64)
		lo, _ := strconv.ParseFloat(t.L[1], 64)
		op, _ := strconv.ParseFloat(t.O, 64)
		return &types.MarketSnapshot{
			Symbol: symbol,
			LatestQuote: &types.Quote{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				BidPrice: bid, AskPrice: ask,
			},
			LatestTrade: &types.Trade{Timestamp: time.Now().UTC().Format(time.RFC3339), Price: last},
			DailyBar:    &types.Bar{Open: op, High: hi, Low: lo, Close: last, Volume: vol},
		}, nil
	}
	return nil, fmt.Errorf("kraken: no data for %s", symbol)
}

func (p *Provider) GetSnapshots(ctx context.Context, syms []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, s := range syms {
		if snap, err := p.GetSnapshot(ctx, s); err == nil {
			result[s] = snap
		}
	}
	return result, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("kraken: bars not yet implemented")
}
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("kraken: not yet implemented")
}
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("kraken: not yet implemented")
}
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return &types.MarketClock{Timestamp: time.Now().UTC().Format(time.RFC3339), IsOpen: true}, nil
}
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("kraken: crypto 24/7")
}

// --- Helpers ---

func krakenPair(sym string) string {
	s := strings.ToUpper(sym)
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

func normalizeKrakenAsset(a string) string {
	a = strings.TrimPrefix(a, "X")
	a = strings.TrimPrefix(a, "Z")
	return strings.ToUpper(a)
}

func defaultAssets() []*types.Asset {
	syms := []string{"BTC", "ETH", "SOL", "AVAX", "DOT", "LINK", "UNI", "LTC", "XRP", "ADA", "DOGE", "ATOM"}
	out := make([]*types.Asset, len(syms))
	for i, s := range syms {
		out[i] = &types.Asset{ID: s + "USD", Provider: "kraken", Symbol: s + "/USD", Class: "crypto", Status: "active", Tradable: true}
	}
	return out
}
