package lx

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/luxfi/broker/pkg/types"
	"github.com/luxfi/zap"
)

// startTestDEX boots a ZAP node that emulates the dex-server handler
// table. Returns the listen address ("host:port") and a stop func.
func startTestDEX(t *testing.T) (string, func()) {
	t.Helper()

	// Pick an ephemeral port up front so the client can dial it.
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

	// Echo handler — decode the JSON payload, build a simple response.
	handler := func(_ context.Context, _ string, msg *zap.Message) (*zap.Message, error) {
		op := uint16(msg.Flags() >> 8)
		reqPayload := msg.Root().Bytes(0)

		var resp any
		switch op {
		case OpGetSnapshot:
			var req map[string]string
			_ = json.Unmarshal(reqPayload, &req)
			resp = &types.MarketSnapshot{
				Symbol:      req["symbol"],
				LatestTrade: &types.Trade{Price: 100.0},
			}
		case OpListAssets:
			resp = []*types.Asset{{Symbol: "BTC/USD", Provider: "lx"}}
		case OpCancelOrder:
			resp = nil
		default:
			resp = map[string]string{"echo": strconv.Itoa(int(op))}
		}

		out, _ := json.Marshal(resp)
		b := zap.NewBuilder(len(out) + 32)
		obj := b.StartObject(8)
		obj.SetBytes(0, out)
		obj.FinishAsRoot()
		respMsg, err := zap.Parse(b.Finish())
		return respMsg, err
	}

	// Register a handler for every opcode the test exercises.
	for _, op := range []uint16{OpGetSnapshot, OpListAssets, OpCancelOrder, OpGetPortfolio} {
		node.Handle(op, handler)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("start dex node: %v", err)
	}

	addr := "127.0.0.1:" + strconv.Itoa(port)
	return addr, node.Stop
}

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
		t.Errorf("symbol: got %q, want %q", snap.Symbol, "BTC/USD")
	}
	if snap.LatestTrade == nil || snap.LatestTrade.Price != 100.0 {
		t.Errorf("price: got %v, want 100.0", snap.LatestTrade)
	}
}

func TestProvider_ListAssets(t *testing.T) {
	addr, stop := startTestDEX(t)
	defer stop()

	p := New(Config{DEXAddr: addr})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assets, err := p.ListAssets(ctx, "crypto")
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(assets) != 1 || assets[0].Symbol != "BTC/USD" {
		t.Errorf("assets: got %+v", assets)
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

func TestProvider_Name(t *testing.T) {
	if (&Provider{}).Name() != "lx" {
		t.Fatal("Name should be \"lx\"")
	}
}

func TestProvider_DefaultsApplied(t *testing.T) {
	p := New(Config{})
	if p.cfg.DEXAddr != DefaultDEXAddr {
		t.Errorf("DEXAddr: got %q, want %q", p.cfg.DEXAddr, DefaultDEXAddr)
	}
	if p.cfg.MPCAddr != DefaultMPCAddr {
		t.Errorf("MPCAddr: got %q, want %q", p.cfg.MPCAddr, DefaultMPCAddr)
	}
	if p.cfg.NodeID != "lx-client" {
		t.Errorf("NodeID: got %q, want lx-client", p.cfg.NodeID)
	}
}
