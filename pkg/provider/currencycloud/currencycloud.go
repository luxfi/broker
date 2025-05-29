package currencycloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// CurrencyCloud (Visa) — Institutional FX trading + cross-border payments.
// 35+ currencies, competitive spot FX, forward contracts.

const (
	ProdURL = "https://api.currencycloud.com"
	DemoURL = "https://devapi.currencycloud.com"
)

type Config struct{ BaseURL, LoginID, APIKey string }

type Provider struct {
	cfg       Config
	client    *http.Client
	authToken string
	tokenExp  time.Time
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" { cfg.BaseURL = ProdURL }
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "currencycloud" }

var errNotSupported = fmt.Errorf("not supported by currencycloud")

func (p *Provider) authenticate(ctx context.Context) error {
	if p.authToken != "" && time.Now().Before(p.tokenExp) { return nil }
	form := url.Values{"login_id": {p.cfg.LoginID}, "api_key": {p.cfg.APIKey}}
	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.BaseURL+"/v2/authenticate/api", strings.NewReader(form.Encode()))
	if err != nil { return err }
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 { return fmt.Errorf("currencycloud auth error %d: %s", resp.StatusCode, string(data)) }
	var result struct{ AuthToken string `json:"auth_token"` }
	json.Unmarshal(data, &result)
	p.authToken = result.AuthToken
	p.tokenExp = time.Now().Add(25 * time.Minute) // tokens last 30min
	return nil
}

func (p *Provider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	if err := p.authenticate(ctx); err != nil { return nil, err }
	var reqBody io.Reader
	if body != nil {
		if form, ok := body.(url.Values); ok {
			reqBody = strings.NewReader(form.Encode())
		} else {
			b, _ := json.Marshal(body)
			reqBody = bytes.NewReader(b)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil { return nil, err }
	req.Header.Set("X-Auth-Token", p.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 { return nil, fmt.Errorf("currencycloud error %d: %s", resp.StatusCode, string(data)) }
	return data, nil
}

// CurrencyCloud uses sub-accounts, not brokerage accounts
func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) { return nil, errNotSupported }

func (p *Provider) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/v2/accounts/"+id, nil)
	if err != nil { return nil, err }
	var resp struct{ ID string `json:"id"`; AccountName string `json:"account_name"`; Status string `json:"status"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: resp.ID, Provider: "currencycloud", ProviderID: resp.ID, AccountNumber: resp.AccountName, Status: resp.Status}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/v2/accounts/find", nil)
	if err != nil { return nil, err }
	var resp struct{ Accounts []struct{ ID string `json:"id"`; AccountName string `json:"account_name"`; Status string `json:"status"` } `json:"accounts"` }
	json.Unmarshal(data, &resp)
	accts := make([]*types.Account, 0, len(resp.Accounts))
	for _, a := range resp.Accounts {
		accts = append(accts, &types.Account{ID: a.ID, Provider: "currencycloud", ProviderID: a.ID, AccountNumber: a.AccountName, Status: a.Status})
	}
	return accts, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, id string) (*types.Portfolio, error) {
	data, err := p.doRequest(ctx, "GET", "/v2/balances/find", nil)
	if err != nil { return nil, err }
	var resp struct{ Balances []struct{ Currency string `json:"currency"`; Amount string `json:"amount"` } `json:"balances"` }
	json.Unmarshal(data, &resp)
	positions := make([]types.Position, 0, len(resp.Balances))
	for _, b := range resp.Balances {
		positions = append(positions, types.Position{Symbol: b.Currency, Qty: b.Amount, AssetClass: "forex"})
	}
	return &types.Portfolio{AccountID: id, Positions: positions}, nil
}

// FX "orders" are conversions in CurrencyCloud
func (p *Provider) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	// Parse symbol as currency pair (e.g. GBPUSD -> buy GBP, sell USD)
	if len(req.Symbol) != 6 { return nil, fmt.Errorf("symbol must be 6-char FX pair (e.g. GBPUSD)") }
	buyCcy := req.Symbol[:3]
	sellCcy := req.Symbol[3:]
	form := url.Values{
		"buy_currency":  {buyCcy},
		"sell_currency": {sellCcy},
		"amount":        {req.Qty},
		"fixed_side":    {"buy"},
		"term_agreement": {"true"},
	}
	data, err := p.doRequest(ctx, "POST", "/v2/conversions/create", form)
	if err != nil { return nil, err }
	var resp struct{ ID string `json:"id"`; Status string `json:"status"`; ClientRate string `json:"client_rate"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.ID, Provider: "currencycloud", ProviderID: resp.ID, AccountID: accountID, Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Type: "fx_spot", Status: resp.Status, FilledAvgPrice: resp.ClientRate, CreatedAt: time.Now()}, nil
}

func (p *Provider) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/v2/conversions/find", nil)
	if err != nil { return nil, err }
	var resp struct{ Conversions []struct{ ID string `json:"id"`; BuyCurrency string `json:"buy_currency"`; SellCurrency string `json:"sell_currency"`; Status string `json:"status"` } `json:"conversions"` }
	json.Unmarshal(data, &resp)
	orders := make([]*types.Order, 0, len(resp.Conversions))
	for _, c := range resp.Conversions {
		orders = append(orders, &types.Order{ID: c.ID, Provider: "currencycloud", ProviderID: c.ID, Symbol: c.BuyCurrency + c.SellCurrency, Status: c.Status})
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/v2/conversions/"+orderID, nil)
	if err != nil { return nil, err }
	var resp struct{ ID string `json:"id"`; Status string `json:"status"`; ClientRate string `json:"client_rate"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: resp.ID, Provider: "currencycloud", ProviderID: resp.ID, Status: resp.Status, FilledAvgPrice: resp.ClientRate}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, accountID, orderID string) error {
	_, err := p.doRequest(ctx, "POST", "/v2/conversions/"+orderID+"/cancel", nil)
	return err
}

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, errNotSupported }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, errNotSupported }

// FX pairs as assets
func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	pairs := []string{"EURUSD", "GBPUSD", "USDJPY", "USDCHF", "AUDUSD", "USDCAD", "NZDUSD", "EURGBP", "EURJPY", "GBPJPY",
		"EURCAD", "EURAUD", "EURNZD", "GBPAUD", "GBPCAD", "AUDCAD", "AUDNZD", "AUDJPY", "CADJPY", "CHFJPY",
		"USDSGD", "USDHKD", "USDMXN", "USDZAR", "USDTRY", "USDPLN", "USDSEK", "USDNOK", "USDDKK", "USDCZK",
		"USDHUF", "USDILS", "USDTHB", "USDINR", "USDCNH"}
	assets := make([]*types.Asset, 0, len(pairs))
	for _, pair := range pairs {
		assets = append(assets, &types.Asset{ID: pair, Provider: "currencycloud", Symbol: pair, Name: pair[:3] + "/" + pair[3:], Class: "forex", Status: "active", Tradable: true})
	}
	return assets, nil
}

func (p *Provider) GetAsset(_ context.Context, sym string) (*types.Asset, error) {
	return &types.Asset{ID: sym, Provider: "currencycloud", Symbol: sym, Name: sym[:3] + "/" + sym[3:], Class: "forex", Status: "active", Tradable: true}, nil
}

// FX rates as market snapshots
func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	if len(symbol) != 6 { return nil, fmt.Errorf("symbol must be 6-char FX pair") }
	pair := symbol[:3] + symbol[3:]
	form := url.Values{"currency_pair": {pair}}
	data, err := p.doRequest(ctx, "GET", "/v2/rates/detailed?currency_pair="+pair, form)
	if err != nil { return nil, err }
	var resp struct{ BidPrice float64 `json:"client_buy_price,string"`; OfferPrice float64 `json:"client_sell_price,string"` }
	json.Unmarshal(data, &resp)
	mid := (resp.BidPrice + resp.OfferPrice) / 2
	return &types.MarketSnapshot{Symbol: symbol, LatestQuote: &types.Quote{BidPrice: resp.BidPrice, AskPrice: resp.OfferPrice}, LatestTrade: &types.Trade{Price: mid}}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err == nil { result[sym] = snap }
	}
	return result, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) { return nil, errNotSupported }

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	snaps, err := p.GetSnapshots(ctx, symbols)
	if err != nil { return nil, err }
	result := make(map[string]*types.Trade)
	for sym, snap := range snaps { if snap.LatestTrade != nil { result[sym] = snap.LatestTrade } }
	return result, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	snaps, err := p.GetSnapshots(ctx, symbols)
	if err != nil { return nil, err }
	result := make(map[string]*types.Quote)
	for sym, snap := range snaps { if snap.LatestQuote != nil { result[sym] = snap.LatestQuote } }
	return result, nil
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	// FX market is open 24/5
	now := time.Now().UTC()
	wd := now.Weekday()
	isOpen := wd >= time.Monday && wd <= time.Friday
	return &types.MarketClock{IsOpen: isOpen, Timestamp: now.Format(time.RFC3339)}, nil
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) { return nil, errNotSupported }
