package binance

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

const (
	ProdURL    = "https://api.binance.us/api/v3"
	GlobalURL  = "https://api.binance.com/api/v3"
	SandboxURL = "https://testnet.binance.vision/api/v3"
)

// Config for Binance (US or global).
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = ProdURL
	}
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "binance" }

func (p *Provider) sign(params string) string {
	mac := hmac.New(sha256.New, []byte(p.cfg.APISecret))
	mac.Write([]byte(params))
	return hex.EncodeToString(mac.Sum(nil))
}

func (p *Provider) doPublic(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+path, nil)
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
		return nil, fmt.Errorf("binance %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (p *Provider) doSigned(ctx context.Context, method, path string, params string) ([]byte, error) {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	qstr := params
	if qstr != "" {
		qstr += "&"
	}
	qstr += "timestamp=" + ts
	qstr += "&signature=" + p.sign(qstr)

	var reqBody io.Reader
	url := p.cfg.BaseURL + path
	if method == http.MethodGet || method == http.MethodDelete {
		url += "?" + qstr
	} else {
		reqBody = bytes.NewReader([]byte(qstr))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", p.cfg.APIKey)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("binance %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// --- Accounts ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("binance: account creation via binance.us")
}

func (p *Provider) GetAccount(ctx context.Context, _ string) (*types.Account, error) {
	data, err := p.doSigned(ctx, http.MethodGet, "/account", "")
	if err != nil {
		return nil, err
	}
	var raw struct {
		AccountType string `json:"accountType"`
		CanTrade    bool   `json:"canTrade"`
	}
	json.Unmarshal(data, &raw)
	status := "inactive"
	if raw.CanTrade {
		status = "active"
	}
	return &types.Account{
		Provider: "binance", ProviderID: "default",
		Status: status, Currency: "USD", AccountType: raw.AccountType,
	}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	acct, err := p.GetAccount(ctx, "")
	if err != nil {
		return nil, err
	}
	return []*types.Account{acct}, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, _ string) (*types.Portfolio, error) {
	data, err := p.doSigned(ctx, http.MethodGet, "/account", "")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	json.Unmarshal(data, &raw)

	var positions []types.Position
	var cash string
	for _, b := range raw.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		total := free + locked
		if total <= 0 {
			continue
		}
		if strings.EqualFold(b.Asset, "USD") || strings.EqualFold(b.Asset, "USDT") || strings.EqualFold(b.Asset, "USDC") {
			cash = fmt.Sprintf("%.2f", total)
			continue
		}
		positions = append(positions, types.Position{
			Symbol: b.Asset, Qty: fmt.Sprintf("%.8f", total), AssetClass: "crypto",
		})
	}
	return &types.Portfolio{Cash: cash, Positions: positions}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	sym := normalizeSymbol(req.Symbol)
	params := fmt.Sprintf("symbol=%s&side=%s&type=%s&quantity=%s",
		sym, strings.ToUpper(req.Side), mapOrderType(req.Type), req.Qty)
	if req.LimitPrice != "" {
		params += "&price=" + req.LimitPrice + "&timeInForce=" + mapTIF(req.TimeInForce)
	}

	data, err := p.doSigned(ctx, http.MethodPost, "/order", params)
	if err != nil {
		return nil, err
	}
	var raw struct {
		OrderID       int    `json:"orderId"`
		Symbol        string `json:"symbol"`
		Status        string `json:"status"`
		ExecutedQty   string `json:"executedQty"`
		CumQuoteQty   string `json:"cummulativeQuoteQty"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider: "binance", ProviderID: strconv.Itoa(raw.OrderID),
		Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Type: req.Type,
		Status: mapBinanceStatus(raw.Status), FilledQty: raw.ExecutedQty,
		CreatedAt: time.Now(),
	}, nil
}

func (p *Provider) ListOrders(ctx context.Context, _ string) ([]*types.Order, error) {
	data, err := p.doSigned(ctx, http.MethodGet, "/openOrders", "")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		OrderID int    `json:"orderId"`
		Symbol  string `json:"symbol"`
		Side    string `json:"side"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Qty     string `json:"origQty"`
		Filled  string `json:"executedQty"`
		Price   string `json:"price"`
	}
	json.Unmarshal(data, &raw)
	orders := make([]*types.Order, len(raw))
	for i, o := range raw {
		orders[i] = &types.Order{
			Provider: "binance", ProviderID: strconv.Itoa(o.OrderID),
			Symbol: o.Symbol, Side: strings.ToLower(o.Side), Type: strings.ToLower(o.Type),
			Status: mapBinanceStatus(o.Status), Qty: o.Qty, FilledQty: o.Filled,
		}
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, _ string, orderID string) (*types.Order, error) {
	// Binance requires symbol — use allOrders with orderId
	data, err := p.doSigned(ctx, http.MethodGet, "/order", "orderId="+orderID)
	if err != nil {
		return nil, err
	}
	var raw struct {
		OrderID int    `json:"orderId"`
		Symbol  string `json:"symbol"`
		Side    string `json:"side"`
		Status  string `json:"status"`
		Qty     string `json:"origQty"`
		Filled  string `json:"executedQty"`
	}
	json.Unmarshal(data, &raw)
	return &types.Order{
		Provider: "binance", ProviderID: strconv.Itoa(raw.OrderID),
		Symbol: raw.Symbol, Side: strings.ToLower(raw.Side),
		Status: mapBinanceStatus(raw.Status), Qty: raw.Qty, FilledQty: raw.Filled,
	}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, _ string, orderID string) error {
	_, err := p.doSigned(ctx, http.MethodDelete, "/order", "orderId="+orderID)
	return err
}

// --- Stubs ---

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("binance: transfers via binance.us")
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return []*types.Transfer{}, nil
}
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("binance: bank linking via binance.us")
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return []*types.BankRelationship{}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, _ string) ([]*types.Asset, error) {
	data, err := p.doPublic(ctx, "/exchangeInfo")
	if err != nil {
		return defaultAssets(), nil
	}
	var info struct {
		Symbols []struct {
			Symbol     string `json:"symbol"`
			BaseAsset  string `json:"baseAsset"`
			QuoteAsset string `json:"quoteAsset"`
			Status     string `json:"status"`
		} `json:"symbols"`
	}
	json.Unmarshal(data, &info)
	assets := make([]*types.Asset, 0)
	for _, s := range info.Symbols {
		if s.QuoteAsset != "USD" && s.QuoteAsset != "USDT" {
			continue
		}
		tradable := s.Status == "TRADING"
		assets = append(assets, &types.Asset{
			ID: s.Symbol, Provider: "binance",
			Symbol: s.BaseAsset + "/" + s.QuoteAsset, Name: s.Symbol,
			Class: "crypto", Status: "active", Tradable: tradable,
		})
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{ID: sym, Provider: "binance", Symbol: sym, Class: "crypto", Status: "active", Tradable: true}, nil
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	sym := normalizeSymbol(symbol)
	data, err := p.doPublic(ctx, "/ticker/24hr?symbol="+sym)
	if err != nil {
		return nil, err
	}
	var raw struct {
		BidPrice  string  `json:"bidPrice"`
		AskPrice  string  `json:"askPrice"`
		LastPrice string  `json:"lastPrice"`
		Volume    string  `json:"volume"`
		High      string  `json:"highPrice"`
		Low       string  `json:"lowPrice"`
		Open      string  `json:"openPrice"`
	}
	json.Unmarshal(data, &raw)
	bid, _ := strconv.ParseFloat(raw.BidPrice, 64)
	ask, _ := strconv.ParseFloat(raw.AskPrice, 64)
	last, _ := strconv.ParseFloat(raw.LastPrice, 64)
	vol, _ := strconv.ParseFloat(raw.Volume, 64)
	hi, _ := strconv.ParseFloat(raw.High, 64)
	lo, _ := strconv.ParseFloat(raw.Low, 64)
	op, _ := strconv.ParseFloat(raw.Open, 64)

	return &types.MarketSnapshot{
		Symbol: symbol,
		LatestQuote: &types.Quote{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			BidPrice: bid, AskPrice: ask,
		},
		LatestTrade: &types.Trade{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Price: last,
		},
		DailyBar: &types.Bar{Open: op, High: hi, Low: lo, Close: last, Volume: vol},
	}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, s := range symbols {
		snap, err := p.GetSnapshot(ctx, s)
		if err == nil {
			result[s] = snap
		}
	}
	return result, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("binance: bars not yet implemented")
}
func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("binance: not yet implemented")
}
func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("binance: not yet implemented")
}
func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return &types.MarketClock{Timestamp: time.Now().UTC().Format(time.RFC3339), IsOpen: true}, nil
}
func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("binance: crypto 24/7")
}

func normalizeSymbol(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}
func mapOrderType(t string) string {
	switch t {
	case "market":
		return "MARKET"
	case "limit":
		return "LIMIT"
	case "stop":
		return "STOP_LOSS"
	case "stop_limit":
		return "STOP_LOSS_LIMIT"
	default:
		return "MARKET"
	}
}
func mapTIF(t string) string {
	switch t {
	case "gtc":
		return "GTC"
	case "ioc":
		return "IOC"
	case "fok":
		return "FOK"
	default:
		return "GTC"
	}
}
func mapBinanceStatus(s string) string {
	switch s {
	case "NEW":
		return "new"
	case "PARTIALLY_FILLED":
		return "partially_filled"
	case "FILLED":
		return "filled"
	case "CANCELED":
		return "cancelled"
	case "REJECTED":
		return "rejected"
	case "EXPIRED":
		return "expired"
	default:
		return strings.ToLower(s)
	}
}

func defaultAssets() []*types.Asset {
	syms := []string{"BTC", "ETH", "SOL", "AVAX", "DOT", "LINK", "UNI", "LTC", "XRP", "ADA", "DOGE", "MATIC", "ATOM"}
	out := make([]*types.Asset, len(syms))
	for i, s := range syms {
		out[i] = &types.Asset{ID: s + "USD", Provider: "binance", Symbol: s + "/USD", Class: "crypto", Status: "active", Tradable: true}
	}
	return out
}
