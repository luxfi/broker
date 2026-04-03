package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// mockProvider implements provider.Provider for testing frontend handlers.
type mockProvider struct {
	name     string
	accounts []*types.Account
	assets   []*types.Asset
	snapshot map[string]*types.MarketSnapshot
	bars     []*types.Bar
	orders   []*types.Order
	portfolio *types.Portfolio
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, nil
}
func (m *mockProvider) GetAccount(_ context.Context, id string) (*types.Account, error) {
	for _, a := range m.accounts {
		if a.ProviderID == id {
			return a, nil
		}
	}
	return nil, nil
}
func (m *mockProvider) ListAccounts(_ context.Context) ([]*types.Account, error) {
	return m.accounts, nil
}
func (m *mockProvider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error) {
	if m.portfolio != nil {
		return m.portfolio, nil
	}
	return &types.Portfolio{Cash: "0", Equity: "0", BuyingPower: "0", PortfolioValue: "0"}, nil
}
func (m *mockProvider) CreateOrder(_ context.Context, _ string, req *types.CreateOrderRequest) (*types.Order, error) {
	return &types.Order{
		ID:     "ord-1",
		Symbol: req.Symbol,
		Side:   req.Side,
		Type:   req.Type,
		Status: "accepted",
	}, nil
}
func (m *mockProvider) ListOrders(_ context.Context, _ string) ([]*types.Order, error) {
	return m.orders, nil
}
func (m *mockProvider) GetOrder(_ context.Context, _, _ string) (*types.Order, error) {
	return nil, nil
}
func (m *mockProvider) CancelOrder(_ context.Context, _, _ string) error { return nil }
func (m *mockProvider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, nil
}
func (m *mockProvider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return nil, nil
}
func (m *mockProvider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, nil
}
func (m *mockProvider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return nil, nil
}
func (m *mockProvider) ListAssets(_ context.Context, class string) ([]*types.Asset, error) {
	if class == "" {
		return m.assets, nil
	}
	var filtered []*types.Asset
	for _, a := range m.assets {
		if a.Class == class {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}
func (m *mockProvider) GetAsset(_ context.Context, _ string) (*types.Asset, error) {
	return nil, nil
}
func (m *mockProvider) GetSnapshot(_ context.Context, symbol string) (*types.MarketSnapshot, error) {
	if snap, ok := m.snapshot[symbol]; ok {
		return snap, nil
	}
	return nil, nil
}
func (m *mockProvider) GetSnapshots(_ context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	result := make(map[string]*types.MarketSnapshot)
	for _, s := range symbols {
		if snap, ok := m.snapshot[s]; ok {
			result[s] = snap
		}
	}
	return result, nil
}
func (m *mockProvider) GetBars(_ context.Context, _ string, _ string, _ string, _ string, _ int) ([]*types.Bar, error) {
	return m.bars, nil
}
func (m *mockProvider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, nil
}
func (m *mockProvider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, nil
}
func (m *mockProvider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, nil
}
func (m *mockProvider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, nil
}

func setupTestServerWithMock(t *testing.T, mp *mockProvider) *httptest.Server {
	t.Helper()
	t.Setenv("IAM_ENDPOINT", testJWKS.server.URL)
	registry := provider.NewRegistry()
	registry.Register(mp)
	srv := NewServer(registry, ":0")
	return httptest.NewServer(srv.Handler())
}

func TestFrontendAssetsEmpty(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/assets")
	if err != nil {
		t.Fatalf("GET /v1/exchange/assets: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	total, ok := body["total"].(float64)
	if !ok {
		t.Fatal("expected total field")
	}
	if total != 0 {
		t.Fatalf("expected 0 assets, got %v", total)
	}
}

func TestFrontendAssetsWithProvider(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		assets: []*types.Asset{
			{ID: "1", Symbol: "AAPL", Name: "Apple", Class: "us_equity", Status: "active", Tradable: true},
			{ID: "2", Symbol: "BTC/USD", Name: "Bitcoin", Class: "crypto", Status: "active", Tradable: true},
			{ID: "3", Symbol: "INACTIVE", Name: "Inactive", Class: "us_equity", Status: "inactive", Tradable: false},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	// All assets (tradable only)
	resp, err := authedGet(ts.URL + "/v1/exchange/assets")
	if err != nil {
		t.Fatalf("GET /v1/exchange/assets: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	total := body["total"].(float64)
	if total != 2 {
		t.Fatalf("expected 2 tradable assets, got %v", total)
	}

	assets := body["assets"].([]interface{})
	first := assets[0].(map[string]interface{})
	if first["type"] != "stocks" {
		t.Fatalf("expected type 'stocks' for us_equity, got %v", first["type"])
	}
}

func TestFrontendAssetsTypeFilter(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		assets: []*types.Asset{
			{ID: "1", Symbol: "AAPL", Name: "Apple", Class: "us_equity", Status: "active", Tradable: true},
			{ID: "2", Symbol: "BTC/USD", Name: "Bitcoin", Class: "crypto", Status: "active", Tradable: true},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/assets?type=crypto")
	if err != nil {
		t.Fatalf("GET /v1/exchange/assets?type=crypto: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	total := body["total"].(float64)
	if total != 1 {
		t.Fatalf("expected 1 crypto asset, got %v", total)
	}
}

func TestFrontendAssetsInvalidType(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/assets?type=invalid")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCryptoPricesEmpty(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/crypto-prices")
	if err != nil {
		t.Fatalf("GET /v1/exchange/crypto-prices: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	prices := body["prices"].(map[string]interface{})
	if len(prices) != 0 {
		t.Fatalf("expected empty prices, got %d", len(prices))
	}
}

func TestCryptoPricesWithData(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		snapshot: map[string]*types.MarketSnapshot{
			"BTC/USD": {
				Symbol:      "BTC/USD",
				LatestTrade: &types.Trade{Price: 50000.0},
				DailyBar:    &types.Bar{Open: 49000, High: 51000, Low: 48000, Close: 50000, Volume: 1234},
				PrevDailyBar: &types.Bar{Close: 49000},
			},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/crypto-prices")
	if err != nil {
		t.Fatalf("GET /v1/exchange/crypto-prices: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	prices := body["prices"].(map[string]interface{})
	btc, ok := prices["BTC/USD"].(map[string]interface{})
	if !ok {
		t.Fatal("expected BTC/USD price entry")
	}
	if btc["price"].(float64) != 50000.0 {
		t.Fatalf("expected price 50000, got %v", btc["price"])
	}
	if btc["change_pct"] == nil {
		t.Fatal("expected change_pct field")
	}
}

func TestChartDataEmpty(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/charts/AAPL")
	if err != nil {
		t.Fatalf("GET /v1/exchange/charts/AAPL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	bars := body["bars"].([]interface{})
	if len(bars) != 0 {
		t.Fatalf("expected empty bars, got %d", len(bars))
	}
}

func TestChartDataWithBars(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		bars: []*types.Bar{
			{Timestamp: "2026-01-01T00:00:00Z", Open: 100, High: 110, Low: 90, Close: 105, Volume: 500},
			{Timestamp: "2026-01-02T00:00:00Z", Open: 105, High: 115, Low: 95, Close: 110, Volume: 600},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/charts/AAPL?timeframe=1D&limit=10")
	if err != nil {
		t.Fatalf("GET /v1/exchange/charts/AAPL: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	bars := body["bars"].([]interface{})
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	first := bars[0].(map[string]interface{})
	if first["open"].(float64) != 100 {
		t.Fatalf("expected open=100, got %v", first["open"])
	}
	if first["time"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("expected time field, got %v", first["time"])
	}
}

func TestFrontendOrdersNoAccounts(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/orders")
	if err != nil {
		t.Fatalf("GET /v1/exchange/orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestFrontendOrdersWithAccount(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov", UserID: "user1"},
		},
		orders: []*types.Order{
			{ID: "o1", Symbol: "AAPL", Side: "buy", Status: "filled"},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedRequest("GET", ts.URL+"/v1/exchange/orders", nil, "user1")
	if err != nil {
		t.Fatalf("GET /v1/exchange/orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	orders := body["orders"].([]interface{})
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
}

func TestFrontendCreateOrderNoAccounts(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{"symbol":"AAPL","qty":"1","side":"buy"}`
	resp, err := authedPost(ts.URL+"/v1/exchange/orders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/exchange/orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestFrontendCreateOrderSuccess(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov", UserID: "user1"},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	body := `{"symbol":"AAPL","qty":"10","side":"buy"}`
	resp, err := authedRequest("POST", ts.URL+"/v1/exchange/orders", strings.NewReader(body), "user1")
	if err != nil {
		t.Fatalf("POST /v1/exchange/orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var order map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&order)
	if order["symbol"] != "AAPL" {
		t.Fatalf("expected symbol AAPL, got %v", order["symbol"])
	}
}

func TestFrontendCreateOrderValidation(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov"},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	// Missing symbol
	body := `{"qty":"10","side":"buy"}`
	resp, err := authedPost(ts.URL+"/v1/exchange/orders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/exchange/orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	// Missing side
	body2 := `{"symbol":"AAPL","qty":"10"}`
	resp2, err := authedPost(ts.URL+"/v1/exchange/orders", "application/json", strings.NewReader(body2))
	if err != nil {
		t.Fatalf("POST /v1/exchange/orders: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp2.StatusCode)
	}
}

func TestFrontendPositionsNoAccounts(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/positions")
	if err != nil {
		t.Fatalf("GET /v1/exchange/positions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestFrontendPositionsWithAccount(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov", UserID: testUserID},
		},
		portfolio: &types.Portfolio{
			Cash:           "10000",
			Equity:         "15000",
			BuyingPower:    "20000",
			PortfolioValue: "15000",
			Positions: []types.Position{
				{Symbol: "AAPL", Qty: "10", AvgEntryPrice: "150", MarketValue: "1600", CurrentPrice: "160", Side: "long"},
			},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	// No user ID -> falls back to first account
	resp, err := authedGet(ts.URL + "/v1/exchange/positions")
	if err != nil {
		t.Fatalf("GET /v1/exchange/positions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	positions := body["positions"].([]interface{})
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
}

func TestFrontendPortfolioSingleAccount(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov", UserID: "user1"},
		},
		portfolio: &types.Portfolio{
			AccountID:      "pa1",
			Cash:           "10000.00",
			Equity:         "15000.00",
			BuyingPower:    "20000.00",
			PortfolioValue: "15000.00",
			Positions: []types.Position{
				{Symbol: "AAPL", Qty: "10"},
			},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedRequest("GET", ts.URL+"/v1/exchange/portfolio", nil, "user1")
	if err != nil {
		t.Fatalf("GET /v1/exchange/portfolio: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["cash"] != "10000.00" {
		t.Fatalf("expected cash 10000.00, got %v", body["cash"])
	}
	if body["equity"] != "15000.00" {
		t.Fatalf("expected equity 15000.00, got %v", body["equity"])
	}
}

func TestFrontendPortfolioNoAccounts(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/portfolio")
	if err != nil {
		t.Fatalf("GET /v1/exchange/portfolio: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAssetClassMapping(t *testing.T) {
	tests := []struct {
		provider string
		frontend string
	}{
		{"us_equity", "stocks"},
		{"crypto", "crypto"},
		{"fixed_income", "bonds"},
		{"commodities", "commodity"},
		{"forex", "forex"},
		{"unknown", "unknown"}, // passthrough
	}
	for _, tt := range tests {
		got := mapAssetClass(tt.provider)
		if got != tt.frontend {
			t.Errorf("mapAssetClass(%q) = %q, want %q", tt.provider, got, tt.frontend)
		}
	}
}

func TestIsCryptoSymbol(t *testing.T) {
	if !isCryptoSymbol("BTC/USD") {
		t.Error("expected BTC/USD to be crypto")
	}
	if isCryptoSymbol("AAPL") {
		t.Error("expected AAPL to not be crypto")
	}
}

func TestChartDataCustomTimeframeAndLimit(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		bars: []*types.Bar{
			{Timestamp: "2026-01-01T09:30:00Z", Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 200},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/charts/AAPL?timeframe=5Min&limit=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	bars := body["bars"].([]interface{})
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
	bar := bars[0].(map[string]interface{})
	if bar["high"].(float64) != 101 {
		t.Fatalf("expected high=101, got %v", bar["high"])
	}
	if bar["volume"].(float64) != 200 {
		t.Fatalf("expected volume=200, got %v", bar["volume"])
	}
}

func TestChartDataDefaultTimeframe(t *testing.T) {
	// Verify default timeframe is used when not specified
	mp := &mockProvider{
		name: "testprov",
		bars: []*types.Bar{
			{Timestamp: "2026-01-01T00:00:00Z", Open: 50, High: 55, Low: 45, Close: 52, Volume: 1000},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	// No timeframe param
	resp, err := authedGet(ts.URL + "/v1/exchange/charts/ETH-USD")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	bars := body["bars"].([]interface{})
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
}

func TestCryptoPricesMultipleSymbols(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		snapshot: map[string]*types.MarketSnapshot{
			"BTC/USD": {
				Symbol:      "BTC/USD",
				LatestTrade: &types.Trade{Price: 50000.0},
				DailyBar:    &types.Bar{Open: 49000, High: 51000, Low: 48000, Close: 50000, Volume: 1234},
				PrevDailyBar: &types.Bar{Close: 49000},
			},
			"ETH/USD": {
				Symbol:      "ETH/USD",
				LatestTrade: &types.Trade{Price: 3000.0},
				DailyBar:    &types.Bar{Open: 2900, High: 3100, Low: 2800, Close: 3000, Volume: 5678},
				PrevDailyBar: &types.Bar{Close: 2900},
			},
			"SOL/USD": {
				Symbol:      "SOL/USD",
				LatestTrade: &types.Trade{Price: 100.0},
			},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedGet(ts.URL + "/v1/exchange/crypto-prices")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	prices := body["prices"].(map[string]interface{})

	if len(prices) != 3 {
		t.Fatalf("expected 3 price entries, got %d", len(prices))
	}

	// Verify ETH data
	eth := prices["ETH/USD"].(map[string]interface{})
	if eth["price"].(float64) != 3000.0 {
		t.Fatalf("expected ETH price 3000, got %v", eth["price"])
	}
	if eth["open"].(float64) != 2900 {
		t.Fatalf("expected ETH open 2900, got %v", eth["open"])
	}
	changePct := eth["change_pct"].(float64)
	// (3000 - 2900) / 2900 * 100 = 3.448...
	if changePct < 3.4 || changePct > 3.5 {
		t.Fatalf("expected ETH change_pct ~3.45, got %v", changePct)
	}

	// Verify SOL has price but no bar data
	sol := prices["SOL/USD"].(map[string]interface{})
	if sol["price"].(float64) != 100.0 {
		t.Fatalf("expected SOL price 100, got %v", sol["price"])
	}
	if _, hasOpen := sol["open"]; hasOpen {
		t.Fatal("SOL should not have open field (no DailyBar)")
	}
}

func TestBuildPriceEntryPartialData(t *testing.T) {
	// Only LatestTrade, no bars
	snap := &types.MarketSnapshot{
		LatestTrade: &types.Trade{Price: 42000},
	}
	entry := buildPriceEntry(snap)
	if entry["price"].(float64) != 42000 {
		t.Fatalf("expected price 42000, got %v", entry["price"])
	}
	if _, ok := entry["open"]; ok {
		t.Fatal("should not have open without DailyBar")
	}
	if _, ok := entry["prev_close"]; ok {
		t.Fatal("should not have prev_close without PrevDailyBar")
	}

	// Only DailyBar, no trade
	snap2 := &types.MarketSnapshot{
		DailyBar: &types.Bar{Open: 100, High: 110, Low: 90, Close: 105, Volume: 500},
	}
	entry2 := buildPriceEntry(snap2)
	if _, ok := entry2["price"]; ok {
		t.Fatal("should not have price without LatestTrade")
	}
	if entry2["close"].(float64) != 105 {
		t.Fatalf("expected close 105, got %v", entry2["close"])
	}

	// PrevDailyBar with zero close should not produce change_pct
	snap3 := &types.MarketSnapshot{
		LatestTrade:  &types.Trade{Price: 100},
		PrevDailyBar: &types.Bar{Close: 0},
	}
	entry3 := buildPriceEntry(snap3)
	if _, ok := entry3["change_pct"]; ok {
		t.Fatal("should not have change_pct when prev close is 0")
	}
}

func TestAssetClassMappingAllTypes(t *testing.T) {
	// Test all frontend-to-provider mappings
	fToP := map[string]string{
		"stocks":    "us_equity",
		"crypto":    "crypto",
		"bonds":     "fixed_income",
		"commodity": "commodities",
		"forex":     "forex",
	}
	for frontend, providerClass := range fToP {
		got := frontendToProvider[frontend]
		if got != providerClass {
			t.Errorf("frontendToProvider[%q] = %q, want %q", frontend, got, providerClass)
		}
	}

	// Test all provider-to-frontend mappings
	pToF := map[string]string{
		"us_equity":    "stocks",
		"crypto":       "crypto",
		"fixed_income": "bonds",
		"commodities":  "commodity",
		"forex":        "forex",
	}
	for providerClass, frontend := range pToF {
		got := providerToFrontend[providerClass]
		if got != frontend {
			t.Errorf("providerToFrontend[%q] = %q, want %q", providerClass, got, frontend)
		}
	}
}

func TestFrontendAssetsFilterAllTypes(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		assets: []*types.Asset{
			{ID: "1", Symbol: "AAPL", Name: "Apple", Class: "us_equity", Status: "active", Tradable: true},
			{ID: "2", Symbol: "BTC/USD", Name: "Bitcoin", Class: "crypto", Status: "active", Tradable: true},
			{ID: "3", Symbol: "BOND1", Name: "Treasury", Class: "fixed_income", Status: "active", Tradable: true},
			{ID: "4", Symbol: "GOLD", Name: "Gold", Class: "commodities", Status: "active", Tradable: true},
			{ID: "5", Symbol: "EUR/USD", Name: "Euro", Class: "forex", Status: "active", Tradable: true},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	for _, tt := range []struct {
		queryType string
		wantType  string
	}{
		{"stocks", "stocks"},
		{"crypto", "crypto"},
		{"bonds", "bonds"},
		{"commodity", "commodity"},
		{"forex", "forex"},
	} {
		t.Run(tt.queryType, func(t *testing.T) {
			resp, err := authedGet(ts.URL + "/v1/exchange/assets?type=" + tt.queryType)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			var body map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&body)
			total := body["total"].(float64)
			if total != 1 {
				t.Fatalf("expected 1 %s asset, got %v", tt.queryType, total)
			}
			assets := body["assets"].([]interface{})
			first := assets[0].(map[string]interface{})
			if first["type"] != tt.wantType {
				t.Fatalf("expected type %q, got %v", tt.wantType, first["type"])
			}
		})
	}
}

func TestResolveUserIDHeaders(t *testing.T) {
	// Only X-User-Id is trusted (set by auth middleware from JWT claims).
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("X-User-Id", "jwt-user")
	r.Header.Set("X-Gateway-User-Id", "gateway-user")
	r.Header.Set("X-Hanzo-User-Id", "hanzo-user")
	if got := resolveUserID(r); got != "jwt-user" {
		t.Fatalf("expected jwt-user, got %s", got)
	}

	// X-Gateway-User-Id and X-Hanzo-User-Id are ignored.
	r2, _ := http.NewRequest("GET", "/", nil)
	r2.Header.Set("X-Gateway-User-Id", "gateway-user")
	r2.Header.Set("X-Hanzo-User-Id", "hanzo-user")
	if got := resolveUserID(r2); got != "" {
		t.Fatalf("expected empty (untrusted headers ignored), got %s", got)
	}

	// No headers returns empty.
	r3, _ := http.NewRequest("GET", "/", nil)
	if got := resolveUserID(r3); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestFrontendCreateOrderInvalidJSON(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov"},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	resp, err := authedPost(ts.URL+"/v1/exchange/orders", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestFrontendCreateOrderDefaultTypeAndTIF(t *testing.T) {
	mp := &mockProvider{
		name: "testprov",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "testprov", UserID: testUserID},
		},
	}
	ts := setupTestServerWithMock(t, mp)
	defer ts.Close()

	// Only symbol and side — type and time_in_force should default
	body := `{"symbol":"AAPL","qty":"1","side":"buy"}`
	resp, err := authedRequest("POST", ts.URL+"/v1/exchange/orders", strings.NewReader(body), testUserID)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var order map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&order)
	if order["type"] != "market" {
		t.Fatalf("expected default type 'market', got %v", order["type"])
	}
}

func TestFrontendPortfolioMultipleAccounts(t *testing.T) {
	mp1 := &mockProvider{
		name: "prov1",
		accounts: []*types.Account{
			{ID: "a1", ProviderID: "pa1", Provider: "prov1", UserID: "user1"},
		},
		portfolio: &types.Portfolio{
			Cash: "5000.00", Equity: "10000.00", BuyingPower: "8000.00", PortfolioValue: "10000.00",
			Positions: []types.Position{
				{Symbol: "AAPL", Qty: "10"},
			},
		},
	}
	mp2 := &mockProvider{
		name: "prov2",
		accounts: []*types.Account{
			{ID: "a2", ProviderID: "pa2", Provider: "prov2", UserID: "user1"},
		},
		portfolio: &types.Portfolio{
			Cash: "3000.00", Equity: "7000.00", BuyingPower: "5000.00", PortfolioValue: "7000.00",
			Positions: []types.Position{
				{Symbol: "BTC/USD", Qty: "0.5"},
			},
		},
	}

	t.Setenv("IAM_ENDPOINT", testJWKS.server.URL)

	registry := provider.NewRegistry()
	registry.Register(mp1)
	registry.Register(mp2)
	srv := NewServer(registry, ":0")
	tsSrv := httptest.NewServer(srv.Handler())
	defer tsSrv.Close()

	// Use a JWT with sub=user1 to match the account UserID.
	resp, err := authedRequest("GET", tsSrv.URL+"/v1/exchange/portfolio", nil, "user1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	// Aggregated: 5000 + 3000 = 8000 cash
	if body["cash"] != "8000.00" {
		t.Fatalf("expected aggregated cash 8000.00, got %v", body["cash"])
	}
	// Aggregated: 10000 + 7000 = 17000 equity
	if body["equity"] != "17000.00" {
		t.Fatalf("expected aggregated equity 17000.00, got %v", body["equity"])
	}
	positions := body["positions"].([]interface{})
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}
}
