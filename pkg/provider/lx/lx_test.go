package lx

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/types"
	"github.com/luxfi/zap"
)

// startTestDEX boots a ZAP node that speaks the lx schema — decodes the
// request by opcode, encodes a typed response using schema.go helpers.
// Returns the listen address and a stop func.
func startTestDEX(t *testing.T) (string, func()) {
	t.Helper()

	// Pick an ephemeral port up front.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	node := zap.NewNode(zap.NodeConfig{
		NodeID:      "luxdex",
		ServiceType: dexServiceType,
		Port:        port,
		NoDiscovery: true,
	})

	handler := func(_ context.Context, _ string, msg *zap.Message) (*zap.Message, error) {
		op, _ := unpackFlags(msg.Flags())
		root := msg.Root()

		var respBytes []byte
		switch op {
		case OpGetSnapshot:
			sym := readSymbolReq(root)
			respBytes = buildSnapshotResp(snapshot{
				symbol:    sym,
				lastPrice: 100.50,
				lastSize:  0.25,
				lastNs:    1_700_000_000_000_000_000,
				bidPrice:  100.45,
				bidSize:   1.5,
				askPrice:  100.55,
				askSize:   1.8,
				quoteNs:   1_700_000_000_500_000_000,
			})

		case OpGetAsset:
			sym := readSymbolReq(root)
			respBytes = buildAssetResp(asset{
				id:       "asset-1",
				symbol:   sym,
				name:     "Bitcoin",
				class:    "crypto",
				exchange: "lx",
				status:   "active",
				provider: "lx",
				tradable: true,
			})

		case OpGetOrder:
			account, orderID := readAccountOrderReq(root)
			respBytes = buildOrderResp(OpGetOrder, order{
				id:         orderID,
				clientID:   "cli-1",
				account:    account,
				symbol:     "BTC/USD",
				side:       "buy",
				orderType:  "limit",
				tif:        "gtc",
				status:     "filled",
				qty:        0.1,
				filledQty:  0.1,
				limitPrice: 100.0,
				avgFillPrice: 100.25,
				createdNs:  1_700_000_000_000_000_000,
				filledNs:   1_700_000_000_500_000_000,
			})

		case OpCreateOrder:
			req := readCreateOrderReq(root)
			respBytes = buildOrderResp(OpCreateOrder, order{
				id:         "new-order",
				clientID:   req.clientID,
				account:    req.account,
				symbol:     req.symbol,
				side:       req.side,
				orderType:  req.orderType,
				tif:        req.tif,
				status:     "accepted",
				qty:        req.qty,
				limitPrice: req.limitPrice,
				createdNs:  1_700_000_000_000_000_000,
			})

		case OpCancelOrder:
			respBytes = buildEmptyResp(OpCancelOrder)

		default:
			respBytes = buildError("not_implemented", "opcode not handled in test", op)
		}

		return zap.Parse(respBytes)
	}

	for _, op := range []uint16{
		OpGetSnapshot, OpGetAsset, OpGetOrder, OpCreateOrder, OpCancelOrder,
	} {
		node.Handle(op, handler)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("start dex node: %v", err)
	}

	return "127.0.0.1:" + strconv.Itoa(port), node.Stop
}

// ── Schema round-trip sanity checks ────────────────────────────────────

func TestSchema_SymbolReq_RoundTrip(t *testing.T) {
	raw := buildSymbolReq(OpGetSnapshot, "BTC/USD")
	msg, err := zap.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	op, status := unpackFlags(msg.Flags())
	if op != OpGetSnapshot || status != StatusOK {
		t.Fatalf("flags: op=0x%x status=0x%x", op, status)
	}
	if got := readSymbolReq(msg.Root()); got != "BTC/USD" {
		t.Fatalf("symbol: got %q", got)
	}
}

func TestSchema_Snapshot_RoundTrip(t *testing.T) {
	raw := buildSnapshotResp(snapshot{
		symbol:    "ETH/USD",
		lastPrice: 2500.0,
		bidPrice:  2499.0,
		askPrice:  2501.0,
		lastNs:    1_700_000_000_000_000_000,
	})
	msg, err := zap.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s := readSnapshotRoot(msg.Root())
	if s.symbol != "ETH/USD" || s.lastPrice != 2500.0 {
		t.Fatalf("snapshot: %+v", s)
	}
	if s.bidPrice != 2499.0 || s.askPrice != 2501.0 {
		t.Fatalf("book: %+v", s)
	}
}

func TestSchema_CreateOrder_RoundTrip(t *testing.T) {
	raw := buildCreateOrderReq(createOrderReq{
		account:    "acct-1",
		symbol:     "BTC/USD",
		side:       "buy",
		orderType:  "limit",
		tif:        "gtc",
		clientID:   "cli-1",
		qty:        0.5,
		limitPrice: 95000.0,
	})
	msg, err := zap.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	req := readCreateOrderReq(msg.Root())
	if req.account != "acct-1" || req.symbol != "BTC/USD" {
		t.Fatalf("strings: %+v", req)
	}
	if req.qty != 0.5 || req.limitPrice != 95000.0 {
		t.Fatalf("floats: %+v", req)
	}
}

func TestSchema_Error_RoundTrip(t *testing.T) {
	raw := buildError("not_found", "no such order", OpGetOrder)
	msg, err := zap.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, status := unpackFlags(msg.Flags())
	if status != StatusErr {
		t.Fatalf("status: 0x%x", status)
	}
	code, message := readError(msg.Root())
	if code != "not_found" || message != "no such order" {
		t.Fatalf("err: %q %q", code, message)
	}
}

func TestSchema_CSV(t *testing.T) {
	parts := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
	got := splitCSV(joinCSV(parts))
	if len(got) != 3 || got[0] != "BTC/USD" || got[2] != "SOL/USD" {
		t.Fatalf("csv round-trip: %v", got)
	}
	if splitCSV("") != nil {
		t.Fatal("empty csv should split to nil")
	}
}

// ── Provider-level ZAP roundtrip tests ─────────────────────────────────

func TestProvider_GetSnapshot(t *testing.T) {
	addr, stop := startTestDEX(t)
	defer stop()

	p := New(Config{DEXAddr: addr})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap, err := p.GetSnapshot(ctx, "BTC/USD")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.Symbol != "BTC/USD" {
		t.Errorf("symbol: got %q", snap.Symbol)
	}
	if snap.LatestTrade == nil || snap.LatestTrade.Price != 100.50 {
		t.Errorf("trade: got %+v", snap.LatestTrade)
	}
	if snap.LatestQuote == nil || snap.LatestQuote.BidPrice != 100.45 {
		t.Errorf("quote: got %+v", snap.LatestQuote)
	}
}

func TestProvider_GetAsset(t *testing.T) {
	addr, stop := startTestDEX(t)
	defer stop()

	p := New(Config{DEXAddr: addr})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a, err := p.GetAsset(ctx, "BTC/USD")
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if a.Symbol != "BTC/USD" || a.Name != "Bitcoin" || a.Class != "crypto" {
		t.Errorf("asset: %+v", a)
	}
	if !a.Tradable {
		t.Error("should be tradable")
	}
}

func TestProvider_GetOrder(t *testing.T) {
	addr, stop := startTestDEX(t)
	defer stop()

	p := New(Config{DEXAddr: addr})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	o, err := p.GetOrder(ctx, "acct-1", "ord-42")
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if o.ID != "ord-42" || o.Symbol != "BTC/USD" {
		t.Errorf("order: %+v", o)
	}
	if o.Status != "filled" || o.Qty != "0.1" {
		t.Errorf("fields: status=%q qty=%q", o.Status, o.Qty)
	}
}

func TestProvider_CreateOrder(t *testing.T) {
	addr, stop := startTestDEX(t)
	defer stop()

	p := New(Config{DEXAddr: addr})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &types.CreateOrderRequest{
		Symbol:      "BTC/USD",
		Side:        "buy",
		Type:        "limit",
		TimeInForce: "gtc",
		Qty:         "0.5",
		LimitPrice:  "95000.00",
	}
	o, err := p.CreateOrder(ctx, "acct-1", req)
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.ID != "new-order" || o.Side != "buy" {
		t.Errorf("order: %+v", o)
	}
	if o.Status != "accepted" {
		t.Errorf("status: %q", o.Status)
	}
}

func TestProvider_CancelOrder(t *testing.T) {
	addr, stop := startTestDEX(t)
	defer stop()

	p := New(Config{DEXAddr: addr})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.CancelOrder(ctx, "acct-1", "ord-1"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
}

func TestProvider_ErrorPropagation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	node := zap.NewNode(zap.NodeConfig{
		NodeID:      "luxdex",
		ServiceType: dexServiceType,
		Port:        port,
		NoDiscovery: true,
	})
	node.Handle(OpGetAsset, func(_ context.Context, _ string, msg *zap.Message) (*zap.Message, error) {
		raw := buildError("not_found", "no such symbol", OpGetAsset)
		return zap.Parse(raw)
	})
	if err := node.Start(); err != nil {
		t.Fatal(err)
	}
	defer node.Stop()

	p := New(Config{DEXAddr: "127.0.0.1:" + strconv.Itoa(port)})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = p.GetAsset(ctx, "XXX")
	if err == nil {
		t.Fatal("expected error")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("want *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != "not_found" || rpcErr.Message != "no such symbol" {
		t.Errorf("err: %+v", rpcErr)
	}
}

func TestProvider_Name(t *testing.T) {
	if (&Provider{}).Name() != "lx" {
		t.Fatal("Name should be \"lx\"")
	}
}

func TestProvider_DefaultsApplied(t *testing.T) {
	p := New(Config{})
	if p.cfg.DEXAddr != DefaultDEXAddr {
		t.Errorf("DEXAddr: got %q", p.cfg.DEXAddr)
	}
	if p.cfg.MPCAddr != DefaultMPCAddr {
		t.Errorf("MPCAddr: got %q", p.cfg.MPCAddr)
	}
}
