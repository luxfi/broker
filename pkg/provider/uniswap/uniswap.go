package uniswap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// Compile-time interface check.
var _ provider.Provider = (*Provider)(nil)

var ErrNotImplemented = fmt.Errorf("uniswap: not implemented (requires on-chain tx signing)")

var (
	DefaultRPCURL   = "https://eth.llamarpc.com"
	EthMainnet      = 1
	DefaultV2Router = "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"
	DefaultV3Router = "0xE592427A0AEce92De3Edee1F18E0157C05861564"

	// Uniswap V3 subgraph on The Graph decentralized network.
	DefaultV3Subgraph = "https://gateway.thegraph.com/api/subgraphs/id/5zvR82QoaXYFyDEKLZ9t6v9adgnptxYpKpSbxtgVENFV"
	// Uniswap V2 subgraph.
	DefaultV2Subgraph = "https://gateway.thegraph.com/api/subgraphs/id/A3Np3RQbaBA6oKJgKstG2GzMWVLp7ndMbkXcEBqLm6Ke"
)

// Config for the Uniswap provider.
type Config struct {
	RPCURL      string `json:"rpc_url"`
	ChainIDs    []int  `json:"chain_ids"`
	V2Router    string `json:"v2_router"`
	V3Router    string `json:"v3_router"`
	V4Router    string `json:"v4_router"`
	V3Subgraph  string `json:"v3_subgraph"`
	GraphAPIKey string `json:"graph_api_key"`
}

// Provider implements the broker Provider interface for Uniswap DEX data.
// Read-only: fetches token prices and pool data from the V3 subgraph.
type Provider struct {
	cfg    Config
	client *http.Client
	assets map[string]*types.Asset // symbol -> asset (lowercase key)
}

// New creates a Uniswap provider with the given config.
func New(cfg Config) *Provider {
	if cfg.RPCURL == "" {
		cfg.RPCURL = DefaultRPCURL
	}
	if len(cfg.ChainIDs) == 0 {
		cfg.ChainIDs = []int{EthMainnet}
	}
	if cfg.V2Router == "" {
		cfg.V2Router = DefaultV2Router
	}
	if cfg.V3Router == "" {
		cfg.V3Router = DefaultV3Router
	}
	if cfg.V3Subgraph == "" {
		cfg.V3Subgraph = DefaultV3Subgraph
	}
	p := &Provider{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
	p.assets = buildAssetMap()
	return p
}

func (p *Provider) Name() string { return "uniswap" }

// --- Assets ---

func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	out := make([]*types.Asset, 0, len(topTokens))
	for _, t := range topTokens {
		out = append(out, makeAsset(t))
	}
	return out, nil
}

func (p *Provider) GetAsset(_ context.Context, symbolOrID string) (*types.Asset, error) {
	key := strings.ToLower(symbolOrID)
	if a, ok := p.assets[key]; ok {
		return a, nil
	}
	// Try matching by contract address.
	addr := strings.ToLower(symbolOrID)
	for _, t := range topTokens {
		if strings.ToLower(t.address) == addr {
			return makeAsset(t), nil
		}
	}
	return nil, fmt.Errorf("uniswap: asset %q not found", symbolOrID)
}

// --- Market Data ---

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	tok := p.resolveToken(symbol)
	if tok == nil {
		return nil, fmt.Errorf("uniswap: unknown token %q", symbol)
	}
	price, err := p.fetchTokenPrice(ctx, tok.address)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return &types.MarketSnapshot{
		Symbol:      symbol,
		LatestTrade: &types.Trade{Timestamp: now, Price: price},
		LatestQuote: &types.Quote{Timestamp: now, BidPrice: price, AskPrice: price},
	}, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	out := make(map[string]*types.MarketSnapshot, len(symbols))
	for _, sym := range symbols {
		snap, err := p.GetSnapshot(ctx, sym)
		if err != nil {
			continue
		}
		out[sym] = snap
	}
	return out, nil
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, nil // subgraph does not provide OHLCV candle data
}

func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return map[string]*types.Trade{}, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	out := make(map[string]*types.Quote, len(symbols))
	for _, sym := range symbols {
		tok := p.resolveToken(sym)
		if tok == nil {
			continue
		}
		price, err := p.fetchTokenPrice(ctx, tok.address)
		if err != nil {
			continue
		}
		out[sym] = &types.Quote{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			BidPrice:  price, AskPrice: price,
		}
	}
	return out, nil
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return &types.MarketClock{Timestamp: time.Now().UTC().Format(time.RFC3339), IsOpen: true}, nil
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, nil // DEX trades 24/7
}

// --- Not Implemented (requires wallet / on-chain signing) ---

func (p *Provider) CreateAccount(_ context.Context, _ *types.CreateAccountRequest) (*types.Account, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) GetAccount(_ context.Context, _ string) (*types.Account, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListAccounts(_ context.Context) ([]*types.Account, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListOrders(_ context.Context, _ string) ([]*types.Order, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) GetOrder(_ context.Context, _, _ string) (*types.Order, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) CancelOrder(_ context.Context, _, _ string) error { return ErrNotImplemented }
func (p *Provider) CreateTransfer(_ context.Context, _ string, _ *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListTransfers(_ context.Context, _ string) ([]*types.Transfer, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) CreateBankRelationship(_ context.Context, _, _, _, _, _ string) (*types.BankRelationship, error) {
	return nil, ErrNotImplemented
}
func (p *Provider) ListBankRelationships(_ context.Context, _ string) ([]*types.BankRelationship, error) {
	return nil, ErrNotImplemented
}

// --- Subgraph Client ---

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

func (p *Provider) querySubgraph(ctx context.Context, query string, vars map[string]any) (json.RawMessage, error) {
	body, _ := json.Marshal(graphQLRequest{Query: query, Variables: vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.V3Subgraph, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.GraphAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.GraphAPIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uniswap: subgraph request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("uniswap: subgraph %d: %s", resp.StatusCode, string(data))
	}
	var gqlResp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &gqlResp); err != nil {
		return nil, fmt.Errorf("uniswap: parse subgraph response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("uniswap: subgraph error: %s", gqlResp.Errors[0].Message)
	}
	return gqlResp.Data, nil
}

const tokenPriceQuery = `query ($addr: String!) {
  token(id: $addr) {
    derivedETH
  }
  bundle(id: "1") {
    ethPriceUSD
  }
}`

func (p *Provider) fetchTokenPrice(ctx context.Context, address string) (float64, error) {
	addr := strings.ToLower(address)
	data, err := p.querySubgraph(ctx, tokenPriceQuery, map[string]any{"addr": addr})
	if err != nil {
		return 0, err
	}
	var result struct {
		Token  *struct{ DerivedETH string `json:"derivedETH"` } `json:"token"`
		Bundle *struct{ EthPriceUSD string `json:"ethPriceUSD"` } `json:"bundle"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, fmt.Errorf("uniswap: parse token price: %w", err)
	}
	if result.Token == nil || result.Bundle == nil {
		return 0, fmt.Errorf("uniswap: token %s not found in subgraph", address)
	}
	derivedETH := parseFloat(result.Token.DerivedETH)
	ethUSD := parseFloat(result.Bundle.EthPriceUSD)
	return derivedETH * ethUSD, nil
}

// --- Token Registry ---

type token struct {
	symbol   string
	name     string
	address  string // Ethereum mainnet contract address
	decimals int
}

var topTokens = []token{
	{"ETH", "Ether", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", 18},           // WETH
	{"USDC", "USD Coin", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", 6},
	{"USDT", "Tether USD", "0xdAC17F958D2ee523a2206206994597C13D831ec7", 6},
	{"WBTC", "Wrapped BTC", "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", 8},
	{"DAI", "Dai Stablecoin", "0x6B175474E89094C44Da98b954EedeAC495271d0F", 18},
	{"UNI", "Uniswap", "0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984", 18},
	{"LINK", "Chainlink", "0x514910771AF9Ca656af840dff83E8264EcF986CA", 18},
	{"AAVE", "Aave", "0x7Fc66500c84A76Ad7e9c93437bFc5Ac33E2DDaE9", 18},
	{"MKR", "Maker", "0x9f8F72aA9304c8B593d555F12eF6589cC3A579A2", 18},
	{"SNX", "Synthetix", "0xC011a73ee8576Fb46F5E1c5751cA3B9Fe0af2a6F", 18},
	{"COMP", "Compound", "0xc00e94Cb662C3520282E6f5717214004A7f26888", 18},
	{"CRV", "Curve DAO", "0xD533a949740bb3306d119CC777fa900bA034cd52", 18},
	{"LDO", "Lido DAO", "0x5A98FcBEA516Cf06857215779Fd812CA3beF1B32", 18},
	{"RPL", "Rocket Pool", "0xD33526068D116cE69F19A9ee46F0bd304F21A51f", 18},
	{"ENS", "Ethereum Name Service", "0xC18360217D8F7Ab5e7c516566761Ea12Ce7F9D72", 18},
	{"GRT", "The Graph", "0xc944E90C64B2c07662A292be6244BDf05Cda44a7", 18},
	{"MATIC", "Polygon", "0x7D1AfA7B718fb893dB30A3aBc0Cfc608AaCfeBB0", 18},
	{"SHIB", "Shiba Inu", "0x95aD61b0a150d79219dCF64E1E6Cc01f0B64C4cE", 18},
	{"APE", "ApeCoin", "0x4d224452801ACEd8B2F0aebE155379bb5D594381", 18},
	{"FET", "Fetch.ai", "0xaea46A60368A7bD060eec7DF8CBa43b7EF41Ad85", 18},
	{"PEPE", "Pepe", "0x6982508145454Ce325dDbE47a25d4ec3d2311933", 18},
	{"ARB", "Arbitrum", "0xB50721BCf8d664c30412Cfbc6cf7a15145234ad1", 18},
	{"OP", "Optimism", "0x4200000000000000000000000000000000000042", 18},
	{"BLUR", "Blur", "0x5283D291DBCF85356A21bA090E6db59121208b44", 18},
	{"WLD", "Worldcoin", "0x163f8C2467924be0ae7B5347228CABF260318753", 18},
	{"DYDX", "dYdX", "0x92D6C1e31e14520e676a687F0a93788B716BEff5", 18},
	{"IMX", "Immutable X", "0xF57e7e7C23978C3cAEC3C3548E3D615c346e79fF", 18},
	{"SUSHI", "SushiSwap", "0x6B3595068778DD592e39A122f4f5a5cF09C90fE2", 18},
	{"1INCH", "1inch", "0x111111111117dC0aa78b770fA6A738034120C302", 18},
	{"BAL", "Balancer", "0xba100000625a3754423978a60c9317c58a424e3D", 18},
}

func makeAsset(t token) *types.Asset {
	return &types.Asset{
		ID: t.address, Provider: "uniswap", Symbol: t.symbol,
		Name: t.name, Class: "crypto", Exchange: "uniswap",
		Status: "active", Tradable: false, Fractionable: true,
	}
}

func buildAssetMap() map[string]*types.Asset {
	m := make(map[string]*types.Asset, len(topTokens)*2)
	for _, t := range topTokens {
		a := makeAsset(t)
		m[strings.ToLower(t.symbol)] = a
		m[strings.ToLower(t.address)] = a
	}
	return m
}

func (p *Provider) resolveToken(symbol string) *token {
	key := strings.ToLower(strings.TrimSuffix(symbol, "/USD"))
	for i := range topTokens {
		if strings.ToLower(topTokens[i].symbol) == key || strings.ToLower(topTokens[i].address) == key {
			return &topTokens[i]
		}
	}
	return nil
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
