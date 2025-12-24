// Package crypto provides a generic crypto exchange adapter template.
// Concrete exchanges can embed BaseExchange and override only the methods
// that differ from the common REST/HMAC pattern shared by most CEXes.
package crypto

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

// Config holds the common credentials and endpoints for a crypto exchange.
type Config struct {
	Name       string `json:"name"`        // exchange identifier (e.g. "myexchange")
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	APISecret  string `json:"api_secret"`
	Passphrase string `json:"passphrase,omitempty"` // some exchanges require a passphrase
	Sandbox    bool   `json:"sandbox,omitempty"`
}

// Endpoints maps logical operations to concrete REST paths.
// Override these for each exchange.
type Endpoints struct {
	CreateAccount       string // POST
	GetAccount          string // GET, {account_id}
	ListAccounts        string // GET
	GetPortfolio        string // GET, {account_id}
	CreateOrder         string // POST
	ListOrders          string // GET, {account_id}
	GetOrder            string // GET, {account_id}, {order_id}
	CancelOrder         string // DELETE, {account_id}, {order_id}
	ListAssets          string // GET
	GetAsset            string // GET, {symbol}
	GetSnapshot         string // GET, {symbol}
	GetSnapshots        string // GET, ?symbols=
	GetBars             string // GET, {symbol}
	GetLatestTrades     string // GET
	GetLatestQuotes     string // GET
	GetClock            string // GET
	GetCalendar         string // GET
	CreateTransfer      string // POST
	ListTransfers       string // GET
	CreateBankRelation  string // POST
	ListBankRelations   string // GET
}

// DefaultEndpoints returns sensible defaults that match the most common
// crypto exchange REST API patterns.
func DefaultEndpoints() Endpoints {
	return Endpoints{
		ListAccounts:   "/api/v1/accounts",
		GetAccount:     "/api/v1/accounts/{account_id}",
		GetPortfolio:   "/api/v1/accounts/{account_id}/portfolio",
		CreateOrder:    "/api/v1/orders",
		ListOrders:     "/api/v1/orders",
		GetOrder:       "/api/v1/orders/{order_id}",
		CancelOrder:    "/api/v1/orders/{order_id}",
		ListAssets:     "/api/v1/assets",
		GetAsset:       "/api/v1/assets/{symbol}",
		GetSnapshot:    "/api/v1/ticker/{symbol}",
		GetSnapshots:   "/api/v1/tickers",
		GetBars:        "/api/v1/candles/{symbol}",
		GetLatestTrades: "/api/v1/trades",
		GetLatestQuotes: "/api/v1/quotes",
		CreateTransfer: "/api/v1/transfers",
		ListTransfers:  "/api/v1/transfers",
	}
}

// BaseExchange implements provider.Provider with common crypto exchange patterns.
// Embed this in a concrete exchange provider and override methods as needed.
type BaseExchange struct {
	Cfg       Config
	Eps       Endpoints
	Client    *http.Client
	SignFn    func(method, path, body string, timestamp int64) string // custom signer
}

// New creates a BaseExchange with default settings.
func New(cfg Config) *BaseExchange {
	if cfg.Name == "" {
		cfg.Name = "crypto"
	}
	return &BaseExchange{
		Cfg:    cfg,
		Eps:    DefaultEndpoints(),
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *BaseExchange) Name() string { return e.Cfg.Name }

// Do executes an authenticated HTTP request against the exchange.
func (e *BaseExchange) Do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	}

	url := e.Cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	ts := time.Now().Unix()
	sig := e.sign(method, path, string(bodyBytes), ts)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", e.Cfg.APIKey)
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Signature", sig)
	if e.Cfg.Passphrase != "" {
		req.Header.Set("X-Passphrase", e.Cfg.Passphrase)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return data, resp.StatusCode, fmt.Errorf("api error %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

func (e *BaseExchange) sign(method, path, body string, timestamp int64) string {
	if e.SignFn != nil {
		return e.SignFn(method, path, body, timestamp)
	}
	// Default: HMAC-SHA256(timestamp + method + path + body)
	msg := strconv.FormatInt(timestamp, 10) + method + path + body
	mac := hmac.New(sha256.New, []byte(e.Cfg.APISecret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

func (e *BaseExchange) resolve(pattern string, params map[string]string) string {
	result := pattern
	for k, v := range params {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// --- Provider Interface Implementation ---

func (e *BaseExchange) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	data, _, err := e.Do(ctx, "POST", e.Eps.CreateAccount, req)
	if err != nil {
		return nil, err
	}
	var acct types.Account
	if err := json.Unmarshal(data, &acct); err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}
	acct.Provider = e.Cfg.Name
	return &acct, nil
}

func (e *BaseExchange) GetAccount(ctx context.Context, accountID string) (*types.Account, error) {
	path := e.resolve(e.Eps.GetAccount, map[string]string{"account_id": accountID})
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var acct types.Account
	if err := json.Unmarshal(data, &acct); err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}
	acct.Provider = e.Cfg.Name
	return &acct, nil
}

func (e *BaseExchange) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, _, err := e.Do(ctx, "GET", e.Eps.ListAccounts, nil)
	if err != nil {
		return nil, err
	}
	var accts []*types.Account
	if err := json.Unmarshal(data, &accts); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}
	for _, a := range accts {
		a.Provider = e.Cfg.Name
	}
	return accts, nil
}

func (e *BaseExchange) GetPortfolio(ctx context.Context, accountID string) (*types.Portfolio, error) {
	path := e.resolve(e.Eps.GetPortfolio, map[string]string{"account_id": accountID})
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var port types.Portfolio
	if err := json.Unmarshal(data, &port); err != nil {
		return nil, fmt.Errorf("decode portfolio: %w", err)
	}
	return &port, nil
}

func (e *BaseExchange) CreateOrder(ctx context.Context, accountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	payload := map[string]interface{}{
		"account_id":    accountID,
		"symbol":        req.Symbol,
		"side":          req.Side,
		"type":          req.Type,
		"time_in_force": req.TimeInForce,
	}
	if req.Qty != "" {
		payload["qty"] = req.Qty
	}
	if req.Notional != "" {
		payload["notional"] = req.Notional
	}
	if req.LimitPrice != "" {
		payload["limit_price"] = req.LimitPrice
	}
	if req.StopPrice != "" {
		payload["stop_price"] = req.StopPrice
	}

	data, _, err := e.Do(ctx, "POST", e.Eps.CreateOrder, payload)
	if err != nil {
		return nil, err
	}
	var order types.Order
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("decode order: %w", err)
	}
	order.Provider = e.Cfg.Name
	return &order, nil
}

func (e *BaseExchange) ListOrders(ctx context.Context, accountID string) ([]*types.Order, error) {
	path := e.Eps.ListOrders + "?account_id=" + accountID
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var orders []*types.Order
	if err := json.Unmarshal(data, &orders); err != nil {
		return nil, fmt.Errorf("decode orders: %w", err)
	}
	for _, o := range orders {
		o.Provider = e.Cfg.Name
	}
	return orders, nil
}

func (e *BaseExchange) GetOrder(ctx context.Context, accountID, orderID string) (*types.Order, error) {
	path := e.resolve(e.Eps.GetOrder, map[string]string{"order_id": orderID})
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var order types.Order
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("decode order: %w", err)
	}
	order.Provider = e.Cfg.Name
	return &order, nil
}

func (e *BaseExchange) CancelOrder(ctx context.Context, accountID, orderID string) error {
	path := e.resolve(e.Eps.CancelOrder, map[string]string{"order_id": orderID})
	_, _, err := e.Do(ctx, "DELETE", path, nil)
	return err
}

func (e *BaseExchange) CreateTransfer(ctx context.Context, accountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	data, _, err := e.Do(ctx, "POST", e.Eps.CreateTransfer, req)
	if err != nil {
		return nil, err
	}
	var xfer types.Transfer
	if err := json.Unmarshal(data, &xfer); err != nil {
		return nil, fmt.Errorf("decode transfer: %w", err)
	}
	xfer.Provider = e.Cfg.Name
	return &xfer, nil
}

func (e *BaseExchange) ListTransfers(ctx context.Context, accountID string) ([]*types.Transfer, error) {
	path := e.Eps.ListTransfers + "?account_id=" + accountID
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var xfers []*types.Transfer
	if err := json.Unmarshal(data, &xfers); err != nil {
		return nil, fmt.Errorf("decode transfers: %w", err)
	}
	for _, x := range xfers {
		x.Provider = e.Cfg.Name
	}
	return xfers, nil
}

func (e *BaseExchange) CreateBankRelationship(ctx context.Context, accountID, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error) {
	payload := map[string]string{
		"account_id":     accountID,
		"owner_name":     ownerName,
		"account_type":   accountType,
		"account_number": accountNumber,
		"routing_number": routingNumber,
	}
	data, _, err := e.Do(ctx, "POST", e.Eps.CreateBankRelation, payload)
	if err != nil {
		return nil, err
	}
	var rel types.BankRelationship
	if err := json.Unmarshal(data, &rel); err != nil {
		return nil, fmt.Errorf("decode bank relationship: %w", err)
	}
	rel.Provider = e.Cfg.Name
	return &rel, nil
}

func (e *BaseExchange) ListBankRelationships(ctx context.Context, accountID string) ([]*types.BankRelationship, error) {
	path := e.Eps.ListBankRelations + "?account_id=" + accountID
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var rels []*types.BankRelationship
	if err := json.Unmarshal(data, &rels); err != nil {
		return nil, fmt.Errorf("decode bank relationships: %w", err)
	}
	for _, r := range rels {
		r.Provider = e.Cfg.Name
	}
	return rels, nil
}

func (e *BaseExchange) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	path := e.Eps.ListAssets
	if class != "" {
		path += "?class=" + class
	}
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var assets []*types.Asset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, fmt.Errorf("decode assets: %w", err)
	}
	for _, a := range assets {
		a.Provider = e.Cfg.Name
	}
	return assets, nil
}

func (e *BaseExchange) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	path := e.resolve(e.Eps.GetAsset, map[string]string{"symbol": symbolOrID})
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var asset types.Asset
	if err := json.Unmarshal(data, &asset); err != nil {
		return nil, fmt.Errorf("decode asset: %w", err)
	}
	asset.Provider = e.Cfg.Name
	return &asset, nil
}

func (e *BaseExchange) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	path := e.resolve(e.Eps.GetSnapshot, map[string]string{"symbol": symbol})
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var snap types.MarketSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	snap.Symbol = symbol
	return &snap, nil
}

func (e *BaseExchange) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	path := e.Eps.GetSnapshots + "?symbols=" + strings.Join(symbols, ",")
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var snaps map[string]*types.MarketSnapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		return nil, fmt.Errorf("decode snapshots: %w", err)
	}
	return snaps, nil
}

func (e *BaseExchange) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	path := e.resolve(e.Eps.GetBars, map[string]string{"symbol": symbol})
	path += fmt.Sprintf("?timeframe=%s&start=%s&end=%s&limit=%d", timeframe, start, end, limit)
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var bars []*types.Bar
	if err := json.Unmarshal(data, &bars); err != nil {
		return nil, fmt.Errorf("decode bars: %w", err)
	}
	return bars, nil
}

func (e *BaseExchange) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	path := e.Eps.GetLatestTrades + "?symbols=" + strings.Join(symbols, ",")
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var trades map[string]*types.Trade
	if err := json.Unmarshal(data, &trades); err != nil {
		return nil, fmt.Errorf("decode trades: %w", err)
	}
	return trades, nil
}

func (e *BaseExchange) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	path := e.Eps.GetLatestQuotes + "?symbols=" + strings.Join(symbols, ",")
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var quotes map[string]*types.Quote
	if err := json.Unmarshal(data, &quotes); err != nil {
		return nil, fmt.Errorf("decode quotes: %w", err)
	}
	return quotes, nil
}

func (e *BaseExchange) GetClock(ctx context.Context) (*types.MarketClock, error) {
	if e.Eps.GetClock == "" {
		return &types.MarketClock{IsOpen: true, Timestamp: time.Now().UTC().Format(time.RFC3339)}, nil
	}
	data, _, err := e.Do(ctx, "GET", e.Eps.GetClock, nil)
	if err != nil {
		return nil, err
	}
	var clock types.MarketClock
	if err := json.Unmarshal(data, &clock); err != nil {
		return nil, fmt.Errorf("decode clock: %w", err)
	}
	return &clock, nil
}

func (e *BaseExchange) GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error) {
	if e.Eps.GetCalendar == "" {
		return nil, nil // crypto exchanges trade 24/7
	}
	path := e.Eps.GetCalendar + fmt.Sprintf("?start=%s&end=%s", start, end)
	data, _, err := e.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var days []*types.MarketCalendarDay
	if err := json.Unmarshal(data, &days); err != nil {
		return nil, fmt.Errorf("decode calendar: %w", err)
	}
	return days, nil
}
