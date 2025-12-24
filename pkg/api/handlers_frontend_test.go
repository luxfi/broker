package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/auth"
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
	os.Setenv("ADMIN_SECRET", "test-secret")
	t.Cleanup(func() {
		os.Unsetenv("ADMIN_SECRET")
	})

	registry := provider.NewRegistry()
	registry.Register(mp)
	srv := NewServer(registry, ":0")
	srv.AuthStore().Add(&auth.APIKey{
		Key: testAPIKey, Name: "test", OrgID: "test-org",
		Permissions: []string{"admin"}, CreatedAt: time.Now(),
	})
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

	req, _ := http.NewRequest("GET", ts.URL+"/v1/exchange/orders", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("X-Gateway-User-Id", "user1")
	resp, err := http.DefaultClient.Do(req)
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
	req, _ := http.NewRequest("POST", ts.URL+"/v1/exchange/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("X-Gateway-User-Id", "user1")
	resp, err := http.DefaultClient.Do(req)
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
			{ID: "a1", ProviderID: "pa1", Provider: "testprov"},
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

	req, _ := http.NewRequest("GET", ts.URL+"/v1/exchange/portfolio", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("X-Hanzo-User-Id", "user1")
	resp, err := http.DefaultClient.Do(req)
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
