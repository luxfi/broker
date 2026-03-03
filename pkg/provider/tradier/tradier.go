package tradier

import (
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

const (
	ProdURL    = "https://api.tradier.com"
	SandboxURL = "https://sandbox.tradier.com"
)

type Config struct{ BaseURL, AccessToken, AccountID string }

type Provider struct{ cfg Config; client *http.Client }

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" { cfg.BaseURL = ProdURL }
	return &Provider{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *Provider) Name() string { return "tradier" }

var errNotSupported = fmt.Errorf("not supported by tradier")

func (p *Provider) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, nil)
	if err != nil { return nil, err }
	req.Header.Set("Authorization", "Bearer "+p.cfg.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 { return nil, fmt.Errorf("tradier error %d: %s", resp.StatusCode, string(data)) }
	return data, nil
}

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) { return nil, errNotSupported }

func (p *Provider) GetAccount(ctx context.Context, id string) (*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/accounts/"+id)
	if err != nil { return nil, err }
	var resp struct{ Account struct{ AccountNumber string `json:"account_number"`; Status string `json:"status"` } `json:"account"` }
	json.Unmarshal(data, &resp)
	return &types.Account{ID: id, Provider: "tradier", ProviderID: id, AccountNumber: resp.Account.AccountNumber, Status: resp.Account.Status}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/user/profile")
	if err != nil { return nil, err }
	var resp struct{ Profile struct{ Account []struct{ AccountNumber string `json:"account_number"` } `json:"account"` } `json:"profile"` }
	json.Unmarshal(data, &resp)
	accts := make([]*types.Account, 0, len(resp.Profile.Account))
	for _, a := range resp.Profile.Account {
		accts = append(accts, &types.Account{ID: a.AccountNumber, Provider: "tradier", ProviderID: a.AccountNumber, AccountNumber: a.AccountNumber, Status: "active"})
	}
	return accts, nil
}

func (p *Provider) GetPortfolio(ctx context.Context, id string) (*types.Portfolio, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/accounts/"+id+"/balances")
	if err != nil { return nil, err }
	var resp struct{ Balances struct{ TotalEquity float64 `json:"total_equity"`; TotalCash float64 `json:"total_cash"`; MarketValue float64 `json:"market_value"` } `json:"balances"` }
	json.Unmarshal(data, &resp)
	return &types.Portfolio{AccountID: id, Cash: fmt.Sprintf("%.2f", resp.Balances.TotalCash), Equity: fmt.Sprintf("%.2f", resp.Balances.TotalEquity), PortfolioValue: fmt.Sprintf("%.2f", resp.Balances.MarketValue)}, nil
}

func (p *Provider) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	url := fmt.Sprintf("/v1/accounts/%s/orders?class=equity&symbol=%s&side=%s&quantity=%s&type=%s&duration=%s", accountID, req.Symbol, req.Side, req.Qty, req.Type, req.TimeInForce)
	data, err := p.doRequest(ctx, "POST", url)
	if err != nil { return nil, err }
	var resp struct{ Order struct{ ID int `json:"id"`; Status string `json:"status"` } `json:"order"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: strconv.Itoa(resp.Order.ID), Provider: "tradier", ProviderID: strconv.Itoa(resp.Order.ID), AccountID: accountID, Symbol: req.Symbol, Qty: req.Qty, Side: req.Side, Type: req.Type, Status: resp.Order.Status, CreatedAt: time.Now()}, nil
}

func (p *Provider) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/accounts/"+accountID+"/orders")
	if err != nil { return nil, err }
	var resp struct{ Orders struct{ Order []struct{ ID int `json:"id"`; Symbol string `json:"symbol"`; Side string `json:"side"`; Status string `json:"status"` } `json:"order"` } `json:"orders"` }
	json.Unmarshal(data, &resp)
	orders := make([]*types.Order, 0, len(resp.Orders.Order))
	for _, o := range resp.Orders.Order { orders = append(orders, &types.Order{ID: strconv.Itoa(o.ID), Provider: "tradier", ProviderID: strconv.Itoa(o.ID), Symbol: o.Symbol, Side: o.Side, Status: o.Status}) }
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/accounts/"+accountID+"/orders/"+orderID)
	if err != nil { return nil, err }
	var resp struct{ Order struct{ ID int `json:"id"`; Status string `json:"status"` } `json:"order"` }
	json.Unmarshal(data, &resp)
	return &types.Order{ID: strconv.Itoa(resp.Order.ID), Provider: "tradier", ProviderID: strconv.Itoa(resp.Order.ID), Status: resp.Order.Status}, nil
}

func (p *Provider) CancelOrder(ctx context.Context, accountID, orderID string) error {
	_, err := p.doRequest(ctx, "DELETE", "/v1/accounts/"+accountID+"/orders/"+orderID)
	return err
}

func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) { return nil, errNotSupported }
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) { return nil, errNotSupported }
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) { return nil, errNotSupported }

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	return []*types.Asset{}, nil // Tradier requires search query, return empty for list
}

func (p *Provider) GetAsset(ctx context.Context, sym string) (*types.Asset, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/markets/quotes?symbols="+sym)
	if err != nil { return nil, err }
	var resp struct{ Quotes struct{ Quote struct{ Symbol string `json:"symbol"`; Description string `json:"description"` } `json:"quote"` } `json:"quotes"` }
	json.Unmarshal(data, &resp)
	return &types.Asset{ID: resp.Quotes.Quote.Symbol, Provider: "tradier", Symbol: resp.Quotes.Quote.Symbol, Name: resp.Quotes.Quote.Description, Class: "us_equity", Status: "active", Tradable: true}, nil
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/markets/quotes?symbols="+symbol)
	if err != nil { return nil, err }
	var resp struct{ Quotes struct{ Quote struct{ Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Last float64 `json:"last"`; Volume int `json:"volume"` } `json:"quote"` } `json:"quotes"` }
	json.Unmarshal(data, &resp)
	q := resp.Quotes.Quote
	return &types.MarketSnapshot{Symbol: symbol, LatestQuote: &types.Quote{BidPrice: q.Bid, AskPrice: q.Ask}, LatestTrade: &types.Trade{Price: q.Last, Size: float64(q.Volume)}}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/markets/quotes?symbols="+strings.Join(symbols, ","))
	if err != nil { return nil, err }
	result := make(map[string]*types.MarketSnapshot)
	var resp struct{ Quotes struct{ Quote json.RawMessage `json:"quote"` } `json:"quotes"` }
	json.Unmarshal(data, &resp)
	// Handle single vs array
	var quotes []struct{ Symbol string `json:"symbol"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Last float64 `json:"last"` }
	if err := json.Unmarshal(resp.Quotes.Quote, &quotes); err != nil {
		var single struct{ Symbol string `json:"symbol"`; Bid float64 `json:"bid"`; Ask float64 `json:"ask"`; Last float64 `json:"last"` }
		json.Unmarshal(resp.Quotes.Quote, &single)
		quotes = append(quotes, single)
	}
	for _, q := range quotes {
		result[q.Symbol] = &types.MarketSnapshot{Symbol: q.Symbol, LatestQuote: &types.Quote{BidPrice: q.Bid, AskPrice: q.Ask}, LatestTrade: &types.Trade{Price: q.Last}}
	}
	return result, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	url := fmt.Sprintf("/v1/markets/history?symbol=%s&interval=daily", symbol)
	if start != "" { url += "&start=" + start }
	if end != "" { url += "&end=" + end }
	data, err := p.doRequest(ctx, "GET", url)
	if err != nil { return nil, err }
	var resp struct{ History struct{ Day []struct{ Date string `json:"date"`; Open float64 `json:"open"`; High float64 `json:"high"`; Low float64 `json:"low"`; Close float64 `json:"close"`; Volume float64 `json:"volume"` } `json:"day"` } `json:"history"` }
	json.Unmarshal(data, &resp)
	bars := make([]*types.Bar, 0, len(resp.History.Day))
	for _, d := range resp.History.Day { bars = append(bars, &types.Bar{Timestamp: d.Date, Open: d.Open, High: d.High, Low: d.Low, Close: d.Close, Volume: d.Volume}) }
	return bars, nil
}

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

func (p *Provider) GetClock(ctx context.Context) (*types.MarketClock, error) {
	data, err := p.doRequest(ctx, "GET", "/v1/markets/clock")
	if err != nil { return nil, err }
	var resp struct{ Clock struct{ State string `json:"state"` } `json:"clock"` }
	json.Unmarshal(data, &resp)
	return &types.MarketClock{IsOpen: resp.Clock.State == "open", Timestamp: time.Now().Format(time.RFC3339)}, nil
}

func (p *Provider) GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error) {
	url := "/v1/markets/calendar"
	if start != "" { url += "?start=" + start }
	data, err := p.doRequest(ctx, "GET", url)
	if err != nil { return nil, err }
	var resp struct{ Calendar struct{ Days struct{ Day []struct{ Date string `json:"date"`; Open string `json:"open"`; Close string `json:"close"` } `json:"day"` } `json:"days"` } `json:"calendar"` }
	json.Unmarshal(data, &resp)
	days := make([]*types.MarketCalendarDay, 0, len(resp.Calendar.Days.Day))
	for _, d := range resp.Calendar.Days.Day { days = append(days, &types.MarketCalendarDay{Date: d.Date, Open: d.Open, Close: d.Close}) }
	return days, nil
}
