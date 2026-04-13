// Package lx is the broker provider for the on-chain Lux DEX
// (luxfi/dex precompile + native ZAP transport). Works against any chain
// that ships the DEX precompile -- Lux mainnet, Lux subnets, Liquidity L1.
//
// Transport is luxfi/zap: zero-copy binary RPC over TCP with opcode
// dispatch via message flags. No JSON, no gRPC, no HTTP. Every request
// and response has a fixed schema defined in schema.go.
//
// The smart order router prefers `lx` over external venues for any
// symbol with on-chain depth.
//
// Required env vars:
//
//	LX_DEX_ADDR    DEX ZAP endpoint (e.g. dex.chain.svc.cluster.local:6336)
//	LX_MPC_ADDR    MPC ZAP endpoint (e.g. mpc.liquid-mpc.svc:6337)
//	LX_USDL_ADDR   USDL ERC-20 address (chain-specific, optional)
package lx

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
	"github.com/luxfi/zap"
)

const (
	DefaultDEXAddr = "dex.chain.svc.cluster.local:6336"
	DefaultMPCAddr = "mpc.liquid-mpc.svc.cluster.local:6337"

	dexServiceType = "_luxdex._tcp"
	dexPeerID      = "luxdex"
)

// Compile-time interface assertion.
var _ provider.Provider = (*Provider)(nil)

// Config wires the provider to the chain + signing service.
type Config struct {
	DEXAddr     string
	MPCAddr     string
	USDLAddress string
	NodeID      string
	Logger      *slog.Logger
}

// Provider implements broker.Provider using native ZAP transport to the
// on-chain DEX precompile. Every RPC is a zero-copy ZAP message pair.
type Provider struct {
	cfg Config

	mu      sync.Mutex
	dexNode *zap.Node
}

// New returns a configured DEX provider. Zero-valued fields fall back to
// safe defaults.
func New(cfg Config) *Provider {
	if cfg.DEXAddr == "" {
		cfg.DEXAddr = DefaultDEXAddr
	}
	if cfg.MPCAddr == "" {
		cfg.MPCAddr = DefaultMPCAddr
	}
	if cfg.NodeID == "" {
		cfg.NodeID = "lx-client"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Provider{cfg: cfg}
}

// Name is the registry key.
func (p *Provider) Name() string { return "lx" }

// dial lazily starts the ZAP node + direct connection on first use and
// reuses the open connection on subsequent calls.
func (p *Provider) dial() (*zap.Node, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dexNode != nil {
		return p.dexNode, nil
	}
	node := zap.NewNode(zap.NodeConfig{
		NodeID:      p.cfg.NodeID,
		ServiceType: dexServiceType,
		Port:        0, // ephemeral
		NoDiscovery: true,
		Logger:      p.cfg.Logger,
	})
	if err := node.Start(); err != nil {
		return nil, fmt.Errorf("lx: start zap node: %w", err)
	}
	if err := node.ConnectDirect(p.cfg.DEXAddr); err != nil {
		node.Stop()
		return nil, fmt.Errorf("lx: dial dex %s: %w", p.cfg.DEXAddr, err)
	}
	p.dexNode = node
	return node, nil
}

// Close shuts down the ZAP transport. Safe to call repeatedly.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dexNode != nil {
		p.dexNode.Stop()
		p.dexNode = nil
	}
	return nil
}

// call performs a ZAP request/response round-trip. The request message is
// already built by the caller (with opcode + status in flags). On success
// the response Message is returned; on protocol-level error the decoded
// ErrorResp is surfaced as a Go error.
func (p *Provider) call(ctx context.Context, reqBytes []byte) (*zap.Message, error) {
	node, err := p.dial()
	if err != nil {
		return nil, err
	}
	msg, err := zap.Parse(reqBytes)
	if err != nil {
		return nil, fmt.Errorf("lx: build request: %w", err)
	}
	resp, err := node.Call(ctx, dexPeerID, msg)
	if err != nil {
		return nil, fmt.Errorf("lx: call: %w", err)
	}
	_, status := unpackFlags(resp.Flags())
	if status != StatusOK {
		code, message := readError(resp.Root())
		return nil, &RPCError{Code: code, Message: message}
	}
	return resp, nil
}

// RPCError is surfaced when the DEX responds with a non-OK status flag.
type RPCError struct {
	Code    string
	Message string
}

func (e *RPCError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ── Accounts ───────────────────────────────────────────────────────────
// On-chain accounts are EVM addresses derived by MPC keygen. The lx
// provider doesn't own the user lifecycle — CreateAccount returns an
// explicit error so callers route through MPC.

func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	return nil, fmt.Errorf("lx: account creation handled by MPC keygen")
}

func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	return nil, fmt.Errorf("lx: GetAccount not implemented")
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	return nil, fmt.Errorf("lx: chain has no account enumeration")
}

// ── Portfolio & Positions ──────────────────────────────────────────────

func (p *Provider) GetPortfolio(ctx context.Context, providerAccountID string) (*types.Portfolio, error) {
	return nil, fmt.Errorf("lx: GetPortfolio wire schema not yet defined")
}

// ── Orders ─────────────────────────────────────────────────────────────

func (p *Provider) CreateOrder(ctx context.Context, providerAccountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	msg := buildCreateOrderReq(createOrderReq{
		account:    providerAccountID,
		symbol:     req.Symbol,
		side:       req.Side,
		orderType:  req.Type,
		tif:        req.TimeInForce,
		clientID:   req.ClientOrderID,
		qty:        parseFloat(req.Qty),
		notional:   parseFloat(req.Notional),
		limitPrice: parseFloat(req.LimitPrice),
		stopPrice:  parseFloat(req.StopPrice),
	})
	resp, err := p.call(ctx, msg)
	if err != nil {
		return nil, err
	}
	o := readOrderRoot(resp.Root())
	return orderToType(o), nil
}

func (p *Provider) ListOrders(ctx context.Context, providerAccountID string) ([]*types.Order, error) {
	// List responses not yet defined (list-of-object schema in progress).
	// Use provider-native GetOrder for now.
	return nil, fmt.Errorf("lx: ListOrders wire schema not yet defined")
}

func (p *Provider) GetOrder(ctx context.Context, providerAccountID, providerOrderID string) (*types.Order, error) {
	msg := buildAccountOrderReq(OpGetOrder, providerAccountID, providerOrderID)
	resp, err := p.call(ctx, msg)
	if err != nil {
		return nil, err
	}
	return orderToType(readOrderRoot(resp.Root())), nil
}

func (p *Provider) CancelOrder(ctx context.Context, providerAccountID, providerOrderID string) error {
	msg := buildAccountOrderReq(OpCancelOrder, providerAccountID, providerOrderID)
	_, err := p.call(ctx, msg)
	return err
}

// ── Transfers / Banks ──────────────────────────────────────────────────
// Fiat rails live elsewhere (Lux Bank). The DEX provider is on-chain only.

func (p *Provider) CreateTransfer(ctx context.Context, providerAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("lx: transfers handled by treasury")
}

func (p *Provider) ListTransfers(ctx context.Context, providerAccountID string) ([]*types.Transfer, error) {
	return nil, fmt.Errorf("lx: ListTransfers not implemented")
}

func (p *Provider) CreateBankRelationship(ctx context.Context, providerAccountID string, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error) {
	return nil, fmt.Errorf("lx: not applicable on-chain")
}

func (p *Provider) ListBankRelationships(ctx context.Context, providerAccountID string) ([]*types.BankRelationship, error) {
	return nil, fmt.Errorf("lx: not applicable on-chain")
}

// ── Assets ─────────────────────────────────────────────────────────────

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	return nil, fmt.Errorf("lx: ListAssets wire schema not yet defined")
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	msg := buildSymbolReq(OpGetAsset, symbolOrID)
	resp, err := p.call(ctx, msg)
	if err != nil {
		return nil, err
	}
	a := readAssetRoot(resp.Root())
	return &types.Asset{
		ID:       a.id,
		Symbol:   a.symbol,
		Name:     a.name,
		Class:    a.class,
		Exchange: a.exchange,
		Status:   a.status,
		Provider: a.provider,
		Tradable: a.tradable,
	}, nil
}

// ── Market Data ────────────────────────────────────────────────────────

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	msg := buildSymbolReq(OpGetSnapshot, symbol)
	resp, err := p.call(ctx, msg)
	if err != nil {
		return nil, err
	}
	return snapToType(readSnapshotRoot(resp.Root())), nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	// List-of-snapshot schema is in-flight; until then issue one call per
	// symbol. N network round-trips but zero JSON — keeps the wire pure
	// ZAP and avoids a half-baked list encoding.
	out := make(map[string]*types.MarketSnapshot, len(symbols))
	for _, s := range symbols {
		snap, err := p.GetSnapshot(ctx, s)
		if err != nil {
			return nil, err
		}
		out[s] = snap
	}
	return out, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("lx: GetBars response schema not yet defined")
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("lx: GetLatestTrades response schema not yet defined")
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	// Derive quotes from snapshots until a dedicated Quote schema ships.
	snaps, err := p.GetSnapshots(ctx, symbols)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*types.Quote, len(snaps))
	for sym, s := range snaps {
		if s.LatestQuote != nil {
			out[sym] = s.LatestQuote
		}
	}
	return out, nil
}

// GetClock returns the chain clock. The DEX runs 24/7.
func (p *Provider) GetClock(ctx context.Context) (*types.MarketClock, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	return &types.MarketClock{
		Timestamp: now,
		IsOpen:    true,
		NextOpen:  now,
		NextClose: now,
	}, nil
}

func (p *Provider) GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error) {
	return nil, nil
}

// ── internal conversion helpers ────────────────────────────────────────

func snapToType(s snapshot) *types.MarketSnapshot {
	out := &types.MarketSnapshot{Symbol: s.symbol}
	if s.lastPrice != 0 || s.lastSize != 0 || s.lastNs != 0 {
		out.LatestTrade = &types.Trade{
			Price:     s.lastPrice,
			Size:      s.lastSize,
			Timestamp: time.Unix(0, s.lastNs).UTC().Format(time.RFC3339Nano),
		}
	}
	if s.bidPrice != 0 || s.askPrice != 0 {
		out.LatestQuote = &types.Quote{
			BidPrice:  s.bidPrice,
			BidSize:   s.bidSize,
			AskPrice:  s.askPrice,
			AskSize:   s.askSize,
			Timestamp: time.Unix(0, s.quoteNs).UTC().Format(time.RFC3339Nano),
		}
	}
	return out
}

func orderToType(o order) *types.Order {
	out := &types.Order{
		ID:             o.id,
		ProviderID:     o.id,
		AccountID:      o.account,
		Symbol:         o.symbol,
		Side:           o.side,
		Type:           o.orderType,
		TimeInForce:    o.tif,
		Status:         o.status,
		Qty:            formatFloat(o.qty),
		FilledQty:      formatFloat(o.filledQty),
		LimitPrice:     formatFloat(o.limitPrice),
		StopPrice:      formatFloat(o.stopPrice),
		FilledAvgPrice: formatFloat(o.avgFillPrice),
		Provider:       "lx",
	}
	if o.createdNs != 0 {
		out.CreatedAt = time.Unix(0, o.createdNs).UTC()
	}
	if o.filledNs != 0 {
		t := time.Unix(0, o.filledNs).UTC()
		out.FilledAt = &t
	}
	return out
}

// parseFloat coerces the string-typed types.CreateOrderRequest fields to
// float64 for wire transport. Empty / unparseable values map to 0.
func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	var f float64
	_, _ = fmt.Sscanf(s, "%f", &f)
	return f
}

func formatFloat(f float64) string {
	if f == 0 {
		return ""
	}
	return fmt.Sprintf("%g", f)
}
