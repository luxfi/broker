// Package lx is the broker provider for the on-chain Lux DEX
// (luxfi/dex precompile + ZAP RPC). It works against any chain that
// ships the DEX precompile -- Lux mainnet, Lux subnets, Liquidity L1.
//
// Transport is luxfi/zap (zero-copy binary RPC over TCP). The DEX
// exposes a ZAP listener; we connect, dial direct (no mDNS), and call
// opcodes for orderbook reads + signed order writes. JSON-RPC and gRPC
// are not used.
//
// The smart order router prefers `lx` over external venues for any
// symbol with on-chain depth.
//
// Required env vars:
//
//	LX_DEX_ADDR    DEX ZAP endpoint (e.g. dex.chain.svc.cluster.local:6336)
//	LX_RPC_URL     fallback EVM JSON-RPC for chain reads (optional)
//	LX_USDL_ADDR   USDL ERC-20 address (chain-specific, optional)
//	LX_MPC_ADDR    MPC ZAP endpoint (e.g. mpc.liquid-mpc.svc:6337)
package lx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
	"github.com/luxfi/zap"
)

const (
	// DefaultDEXAddr is the in-cluster DEX ZAP listener.
	DefaultDEXAddr = "dex.chain.svc.cluster.local:6336"
	// DefaultMPCAddr is the in-cluster MPC ZAP listener.
	DefaultMPCAddr = "mpc.liquid-mpc.svc.cluster.local:6337"
	// DefaultRPCURL is the in-cluster gateway (used only for read fallback).
	DefaultRPCURL = "http://gateway.chain.svc.cluster.local:8080"

	// Service identifiers for ZAP discovery.
	dexServiceType = "_luxdex._tcp"
	dexPeerID      = "luxdex"
	mpcPeerID      = "luxmpc"
)

// DEX RPC opcodes — must match the dex-server ZAP handler table.
const (
	OpListAssets    uint16 = 0x01
	OpGetAsset      uint16 = 0x02
	OpGetSnapshot   uint16 = 0x10
	OpGetSnapshots  uint16 = 0x11
	OpGetBars       uint16 = 0x12
	OpGetQuotes     uint16 = 0x13
	OpGetTrades     uint16 = 0x14
	OpGetBook       uint16 = 0x15
	OpCreateOrder   uint16 = 0x20
	OpCancelOrder   uint16 = 0x21
	OpGetOrder      uint16 = 0x22
	OpListOrders    uint16 = 0x23
	OpGetPortfolio  uint16 = 0x30
)

// Compile-time interface assertion.
var _ provider.Provider = (*Provider)(nil)

// Config wires the provider to the chain + signing service.
type Config struct {
	// DEXAddr is the ZAP listener for the DEX precompile RPC.
	DEXAddr string
	// MPCAddr is the ZAP listener for the MPC signing service.
	MPCAddr string
	// USDLAddress is the chain-local USDL ERC-20 contract.
	USDLAddress string
	// RPCURL is an optional EVM JSON-RPC fallback for read paths
	// when ZAP is unreachable. Empty disables the fallback.
	RPCURL string
	// NodeID names this client in ZAP discovery + logs.
	NodeID string
	// Logger receives ZAP transport logs. nil falls back to slog.Default().
	Logger *slog.Logger
}

// Provider implements broker.Provider using ZAP transport to the on-chain
// DEX precompile. Reads call OpGet*; writes go through MPC for signing.
type Provider struct {
	cfg Config

	mu      sync.Mutex
	dexNode *zap.Node // lazy-started on first call
}

// New returns a configured DEX provider. Zero-valued fields fall back to
// safe defaults so this is always callable.
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

// Name is the registry key. The router uses this to address the provider.
func (p *Provider) Name() string { return "lx" }

// dial establishes the ZAP client node + direct connection on first use.
// Subsequent calls reuse the open connection.
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

// call performs a ZAP request/response round-trip against the DEX, with
// the JSON-encoded request body in field 0 and an opcode in flags upper
// 8 bits. This matches the ledger replication wire format.
func (p *Provider) call(ctx context.Context, op uint16, req any, out any) error {
	node, err := p.dial()
	if err != nil {
		return err
	}

	var payload []byte
	if req != nil {
		payload, err = json.Marshal(req)
		if err != nil {
			return fmt.Errorf("lx: marshal request: %w", err)
		}
	}

	b := zap.NewBuilder(len(payload) + 64)
	obj := b.StartObject(8)
	obj.SetBytes(0, payload)
	obj.FinishAsRoot()
	msgBytes := b.FinishWithFlags(uint16(op) << 8)

	msg, err := zap.Parse(msgBytes)
	if err != nil {
		return fmt.Errorf("lx: build message: %w", err)
	}

	resp, err := node.Call(ctx, dexPeerID, msg)
	if err != nil {
		return fmt.Errorf("lx: call op=0x%02x: %w", op, err)
	}
	if out == nil {
		return nil
	}
	respPayload := resp.Root().Bytes(0)
	if len(respPayload) == 0 {
		return nil
	}
	if err := json.Unmarshal(respPayload, out); err != nil {
		return fmt.Errorf("lx: decode response: %w", err)
	}
	return nil
}

// ── Accounts ───────────────────────────────────────────────────────────
// On-chain accounts are EVM addresses derived by MPC keygen. The lx
// provider does not own the user lifecycle; CreateAccount returns an
// explicit error so the caller routes through MPC instead.

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
	var out types.Portfolio
	req := map[string]string{"account": providerAccountID}
	if err := p.call(ctx, OpGetPortfolio, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Orders ─────────────────────────────────────────────────────────────
// Order writes flow through MPC: the DEX-side handler receives the order
// intent, calls MPC for signing, and broadcasts the signed tx.

func (p *Provider) CreateOrder(ctx context.Context, providerAccountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	var out types.Order
	body := struct {
		Account string                     `json:"account"`
		Order   *types.CreateOrderRequest  `json:"order"`
	}{providerAccountID, req}
	if err := p.call(ctx, OpCreateOrder, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Provider) ListOrders(ctx context.Context, providerAccountID string) ([]*types.Order, error) {
	var out []*types.Order
	req := map[string]string{"account": providerAccountID}
	if err := p.call(ctx, OpListOrders, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) GetOrder(ctx context.Context, providerAccountID, providerOrderID string) (*types.Order, error) {
	var out types.Order
	req := map[string]string{"account": providerAccountID, "order": providerOrderID}
	if err := p.call(ctx, OpGetOrder, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Provider) CancelOrder(ctx context.Context, providerAccountID, providerOrderID string) error {
	req := map[string]string{"account": providerAccountID, "order": providerOrderID}
	return p.call(ctx, OpCancelOrder, req, nil)
}

// ── Transfers / Banks ──────────────────────────────────────────────────
// Fiat rails live elsewhere (Lux Bank). The DEX provider is on-chain only.

func (p *Provider) CreateTransfer(ctx context.Context, providerAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	return nil, fmt.Errorf("lx: transfers handled by treasury, not provider")
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
	var out []*types.Asset
	req := map[string]string{"class": class}
	if err := p.call(ctx, OpListAssets, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	var out types.Asset
	req := map[string]string{"symbol": symbolOrID}
	if err := p.call(ctx, OpGetAsset, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Market Data ────────────────────────────────────────────────────────
// All live data comes from the on-chain orderbook (DEX precompile).

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	var out types.MarketSnapshot
	req := map[string]string{"symbol": symbol}
	if err := p.call(ctx, OpGetSnapshot, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	var out map[string]*types.MarketSnapshot
	req := map[string][]string{"symbols": symbols}
	if err := p.call(ctx, OpGetSnapshots, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	var out []*types.Bar
	req := struct {
		Symbol    string `json:"symbol"`
		Timeframe string `json:"timeframe"`
		Start     string `json:"start"`
		End       string `json:"end"`
		Limit     int    `json:"limit"`
	}{symbol, timeframe, start, end, limit}
	if err := p.call(ctx, OpGetBars, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	var out map[string]*types.Trade
	req := map[string][]string{"symbols": symbols}
	if err := p.call(ctx, OpGetTrades, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	var out map[string]*types.Quote
	req := map[string][]string{"symbols": symbols}
	if err := p.call(ctx, OpGetQuotes, req, &out); err != nil {
		return nil, err
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
