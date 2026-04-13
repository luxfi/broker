// Schema definitions for Lux DEX ZAP messages. Every RPC has a dedicated
// request/response struct with fixed field offsets; payload is zero-copy.
//
// Wire layout:
//
//	Header (16 bytes):
//	  magic    (4): "ZAP\x00"
//	  version  (2): 1
//	  flags    (2): [opcode:8 | status:4 | reserved:4]
//	  root off (4): offset into body where the root struct starts
//	  size     (4): total message size
//
//	Body: one root struct, fields at fixed aligned offsets.
//
// Status encoding (flags lower nibble of upper byte):
//
//	0x0  OK
//	0x1  ERR       — root is ErrorResp{code, message}
//	0x2  NOT_IMPL  — root is ErrorResp with code "not_implemented"
//
// All float values use IEEE-754 binary64 little-endian. All timestamps are
// int64 Unix nanoseconds. Strings are encoded via ZAP text fields
// (4-byte relative offset + 4-byte length).
package lx

import (
	"github.com/luxfi/zap"
)

// Opcodes — must match the dex-server ZAP handler table.
const (
	OpListAssets   uint16 = 0x01
	OpGetAsset     uint16 = 0x02
	OpGetSnapshot  uint16 = 0x10
	OpGetSnapshots uint16 = 0x11
	OpGetBars      uint16 = 0x12
	OpGetQuotes    uint16 = 0x13
	OpGetTrades    uint16 = 0x14
	OpGetBook      uint16 = 0x15
	OpCreateOrder  uint16 = 0x20
	OpCancelOrder  uint16 = 0x21
	OpGetOrder     uint16 = 0x22
	OpListOrders   uint16 = 0x23
	OpGetPortfolio uint16 = 0x30
)

// Status bits in the flags lower byte. The opcode occupies the upper byte.
const (
	StatusOK       uint16 = 0x0000
	StatusErr      uint16 = 0x0001
	StatusNotImpl  uint16 = 0x0002
)

// packFlags encodes opcode + status into the 16-bit flags header field.
func packFlags(op, status uint16) uint16 {
	return (op << 8) | (status & 0x00FF)
}

// unpackFlags extracts opcode + status from the flags header field.
func unpackFlags(flags uint16) (op, status uint16) {
	return flags >> 8, flags & 0x00FF
}

// ───────────────────────────────────────────────────────────────────────
//  ErrorResp (flags.status != OK)
// ───────────────────────────────────────────────────────────────────────
//
//   offset  type    name
//   ──────  ──────  ───────────
//        0  text    code     (8 bytes: 4 off + 4 len)
//        8  text    message  (8 bytes: 4 off + 4 len)
//       16 (total)
const (
	errOffCode    = 0
	errOffMessage = 8
	errSize       = 16
)

func buildError(code, msg string, op uint16) []byte {
	b := zap.NewBuilder(64 + len(code) + len(msg))
	ob := b.StartObject(errSize)
	ob.SetText(errOffCode, code)
	ob.SetText(errOffMessage, msg)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusErr))
}

func readError(root zap.Object) (code, message string) {
	return root.Text(errOffCode), root.Text(errOffMessage)
}

// ───────────────────────────────────────────────────────────────────────
//  SymbolReq — single-symbol request (GetSnapshot, GetAsset, GetBook)
// ───────────────────────────────────────────────────────────────────────
//
//        0  text    symbol   (8)
//        8 (total)
const (
	symReqOffSymbol = 0
	symReqSize      = 8
)

func buildSymbolReq(op uint16, symbol string) []byte {
	b := zap.NewBuilder(64 + len(symbol))
	ob := b.StartObject(symReqSize)
	ob.SetText(symReqOffSymbol, symbol)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

func readSymbolReq(root zap.Object) string {
	return root.Text(symReqOffSymbol)
}

// ───────────────────────────────────────────────────────────────────────
//  ClassReq — asset class filter (ListAssets)
// ───────────────────────────────────────────────────────────────────────
//
//        0  text    class   (8)
//        8 (total)
const (
	classReqOffClass = 0
	classReqSize     = 8
)

func buildClassReq(op uint16, class string) []byte {
	b := zap.NewBuilder(64)
	ob := b.StartObject(classReqSize)
	ob.SetText(classReqOffClass, class)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

func readClassReq(root zap.Object) string {
	return root.Text(classReqOffClass)
}

// ───────────────────────────────────────────────────────────────────────
//  SymbolsReq — multi-symbol request (GetSnapshots/Quotes/Trades)
// ───────────────────────────────────────────────────────────────────────
//
// Symbols are packed as a single comma-separated text field. This keeps the
// wire layout compact without needing a heterogeneous list-of-text encoding.
//
//        0  text    symbols  (8) — CSV: "BTC/USD,ETH/USD,SOL/USD"
//        8 (total)
const (
	symsReqOffSymbols = 0
	symsReqSize       = 8
)

func buildSymbolsReq(op uint16, symbols []string) []byte {
	csv := joinCSV(symbols)
	b := zap.NewBuilder(64 + len(csv))
	ob := b.StartObject(symsReqSize)
	ob.SetText(symsReqOffSymbols, csv)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

func readSymbolsReq(root zap.Object) []string {
	return splitCSV(root.Text(symsReqOffSymbols))
}

// ───────────────────────────────────────────────────────────────────────
//  BarsReq — historical bars query (GetBars)
// ───────────────────────────────────────────────────────────────────────
//
//        0  text    symbol     (8)
//        8  text    timeframe  (8)
//       16  text    start      (8)
//       24  text    end        (8)
//       32  int64   limit      (8)
//       40 (total)
const (
	barsReqOffSymbol    = 0
	barsReqOffTimeframe = 8
	barsReqOffStart     = 16
	barsReqOffEnd       = 24
	barsReqOffLimit     = 32
	barsReqSize         = 40
)

type barsReq struct {
	symbol, timeframe, start, end string
	limit                         int64
}

func buildBarsReq(r barsReq) []byte {
	b := zap.NewBuilder(128)
	ob := b.StartObject(barsReqSize)
	ob.SetText(barsReqOffSymbol, r.symbol)
	ob.SetText(barsReqOffTimeframe, r.timeframe)
	ob.SetText(barsReqOffStart, r.start)
	ob.SetText(barsReqOffEnd, r.end)
	ob.SetInt64(barsReqOffLimit, r.limit)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(OpGetBars, StatusOK))
}

func readBarsReq(root zap.Object) barsReq {
	return barsReq{
		symbol:    root.Text(barsReqOffSymbol),
		timeframe: root.Text(barsReqOffTimeframe),
		start:     root.Text(barsReqOffStart),
		end:       root.Text(barsReqOffEnd),
		limit:     root.Int64(barsReqOffLimit),
	}
}

// ───────────────────────────────────────────────────────────────────────
//  AccountReq — account-scoped request (GetPortfolio, ListOrders)
// ───────────────────────────────────────────────────────────────────────
//
//        0  text    account  (8)
//        8 (total)
const (
	acctReqOffAccount = 0
	acctReqSize       = 8
)

func buildAccountReq(op uint16, account string) []byte {
	b := zap.NewBuilder(64)
	ob := b.StartObject(acctReqSize)
	ob.SetText(acctReqOffAccount, account)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

func readAccountReq(root zap.Object) string {
	return root.Text(acctReqOffAccount)
}

// ───────────────────────────────────────────────────────────────────────
//  AccountOrderReq — (account, order_id) (GetOrder, CancelOrder)
// ───────────────────────────────────────────────────────────────────────
//
//        0  text    account   (8)
//        8  text    order_id  (8)
//       16 (total)
const (
	acctOrderReqOffAccount = 0
	acctOrderReqOffOrder   = 8
	acctOrderReqSize       = 16
)

func buildAccountOrderReq(op uint16, account, order string) []byte {
	b := zap.NewBuilder(64)
	ob := b.StartObject(acctOrderReqSize)
	ob.SetText(acctOrderReqOffAccount, account)
	ob.SetText(acctOrderReqOffOrder, order)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

func readAccountOrderReq(root zap.Object) (account, order string) {
	return root.Text(acctOrderReqOffAccount), root.Text(acctOrderReqOffOrder)
}

// ───────────────────────────────────────────────────────────────────────
//  CreateOrderReq — place an order
// ───────────────────────────────────────────────────────────────────────
//
//        0  text     account           (8)
//        8  text     symbol            (8)
//       16  text     side              (8)  — "buy" | "sell"
//       24  text     order_type        (8)  — "market" | "limit" | "stop" | "stop_limit"
//       32  text     time_in_force     (8)  — "day" | "gtc" | "ioc" | "fok"
//       40  text     client_order_id   (8)
//       48  float64  qty               (8)
//       56  float64  notional          (8)
//       64  float64  limit_price       (8)
//       72  float64  stop_price        (8)
//       80 (total)
const (
	coReqOffAccount   = 0
	coReqOffSymbol    = 8
	coReqOffSide      = 16
	coReqOffOrderType = 24
	coReqOffTIF       = 32
	coReqOffCliID     = 40
	coReqOffQty       = 48
	coReqOffNotional  = 56
	coReqOffLimit     = 64
	coReqOffStop      = 72
	coReqSize         = 80
)

type createOrderReq struct {
	account, symbol, side, orderType, tif, clientID string
	qty, notional, limitPrice, stopPrice            float64
}

func buildCreateOrderReq(r createOrderReq) []byte {
	b := zap.NewBuilder(256)
	ob := b.StartObject(coReqSize)
	ob.SetText(coReqOffAccount, r.account)
	ob.SetText(coReqOffSymbol, r.symbol)
	ob.SetText(coReqOffSide, r.side)
	ob.SetText(coReqOffOrderType, r.orderType)
	ob.SetText(coReqOffTIF, r.tif)
	ob.SetText(coReqOffCliID, r.clientID)
	ob.SetFloat64(coReqOffQty, r.qty)
	ob.SetFloat64(coReqOffNotional, r.notional)
	ob.SetFloat64(coReqOffLimit, r.limitPrice)
	ob.SetFloat64(coReqOffStop, r.stopPrice)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(OpCreateOrder, StatusOK))
}

func readCreateOrderReq(root zap.Object) createOrderReq {
	return createOrderReq{
		account:    root.Text(coReqOffAccount),
		symbol:     root.Text(coReqOffSymbol),
		side:       root.Text(coReqOffSide),
		orderType:  root.Text(coReqOffOrderType),
		tif:        root.Text(coReqOffTIF),
		clientID:   root.Text(coReqOffCliID),
		qty:        root.Float64(coReqOffQty),
		notional:   root.Float64(coReqOffNotional),
		limitPrice: root.Float64(coReqOffLimit),
		stopPrice:  root.Float64(coReqOffStop),
	}
}

// ───────────────────────────────────────────────────────────────────────
//  SnapshotResp — single quote snapshot
// ───────────────────────────────────────────────────────────────────────
//
//        0  text     symbol        (8)
//        8  float64  last_price    (8)
//       16  float64  last_size     (8)
//       24  int64    last_time_ns  (8)
//       32  float64  bid_price     (8)
//       40  float64  bid_size      (8)
//       48  float64  ask_price     (8)
//       56  float64  ask_size      (8)
//       64  int64    quote_time_ns (8)
//       72 (total)
const (
	snapOffSymbol    = 0
	snapOffLastPrice = 8
	snapOffLastSize  = 16
	snapOffLastNS    = 24
	snapOffBidPrice  = 32
	snapOffBidSize   = 40
	snapOffAskPrice  = 48
	snapOffAskSize   = 56
	snapOffQuoteNS   = 64
	snapSize         = 72
)

type snapshot struct {
	symbol                                            string
	lastPrice, lastSize, bidPrice, bidSize            float64
	askPrice, askSize                                 float64
	lastNs, quoteNs                                   int64
}

// writeSnapshot writes a snapshot struct into b and returns its abs offset.
// Caller is responsible for referencing the offset from a parent object.
func writeSnapshot(b *zap.Builder, s snapshot) int {
	ob := b.StartObject(snapSize)
	ob.SetText(snapOffSymbol, s.symbol)
	ob.SetFloat64(snapOffLastPrice, s.lastPrice)
	ob.SetFloat64(snapOffLastSize, s.lastSize)
	ob.SetInt64(snapOffLastNS, s.lastNs)
	ob.SetFloat64(snapOffBidPrice, s.bidPrice)
	ob.SetFloat64(snapOffBidSize, s.bidSize)
	ob.SetFloat64(snapOffAskPrice, s.askPrice)
	ob.SetFloat64(snapOffAskSize, s.askSize)
	ob.SetInt64(snapOffQuoteNS, s.quoteNs)
	return ob.Finish()
}

func buildSnapshotResp(s snapshot) []byte {
	b := zap.NewBuilder(128 + len(s.symbol))
	ob := b.StartObject(snapSize)
	ob.SetText(snapOffSymbol, s.symbol)
	ob.SetFloat64(snapOffLastPrice, s.lastPrice)
	ob.SetFloat64(snapOffLastSize, s.lastSize)
	ob.SetInt64(snapOffLastNS, s.lastNs)
	ob.SetFloat64(snapOffBidPrice, s.bidPrice)
	ob.SetFloat64(snapOffBidSize, s.bidSize)
	ob.SetFloat64(snapOffAskPrice, s.askPrice)
	ob.SetFloat64(snapOffAskSize, s.askSize)
	ob.SetInt64(snapOffQuoteNS, s.quoteNs)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(OpGetSnapshot, StatusOK))
}

func readSnapshotRoot(root zap.Object) snapshot {
	return snapshot{
		symbol:    root.Text(snapOffSymbol),
		lastPrice: root.Float64(snapOffLastPrice),
		lastSize:  root.Float64(snapOffLastSize),
		lastNs:    root.Int64(snapOffLastNS),
		bidPrice:  root.Float64(snapOffBidPrice),
		bidSize:   root.Float64(snapOffBidSize),
		askPrice:  root.Float64(snapOffAskPrice),
		askSize:   root.Float64(snapOffAskSize),
		quoteNs:   root.Int64(snapOffQuoteNS),
	}
}

// ───────────────────────────────────────────────────────────────────────
//  AssetResp — single asset record
// ───────────────────────────────────────────────────────────────────────
//
//        0  text     id        (8)
//        8  text     symbol    (8)
//       16  text     name      (8)
//       24  text     class     (8)
//       32  text     exchange  (8)
//       40  text     status    (8)
//       48  text     provider  (8)
//       56  bool     tradable  (1, padded to 8)
//       64 (total, 8-aligned)
const (
	assetOffID       = 0
	assetOffSymbol   = 8
	assetOffName     = 16
	assetOffClass    = 24
	assetOffExchange = 32
	assetOffStatus   = 40
	assetOffProvider = 48
	assetOffTradable = 56
	assetSize        = 64
)

type asset struct {
	id, symbol, name, class, exchange, status, provider string
	tradable                                             bool
}

func buildAssetResp(a asset) []byte {
	b := zap.NewBuilder(256)
	ob := b.StartObject(assetSize)
	ob.SetText(assetOffID, a.id)
	ob.SetText(assetOffSymbol, a.symbol)
	ob.SetText(assetOffName, a.name)
	ob.SetText(assetOffClass, a.class)
	ob.SetText(assetOffExchange, a.exchange)
	ob.SetText(assetOffStatus, a.status)
	ob.SetText(assetOffProvider, a.provider)
	ob.SetBool(assetOffTradable, a.tradable)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(OpGetAsset, StatusOK))
}

func readAssetRoot(root zap.Object) asset {
	return asset{
		id:       root.Text(assetOffID),
		symbol:   root.Text(assetOffSymbol),
		name:     root.Text(assetOffName),
		class:    root.Text(assetOffClass),
		exchange: root.Text(assetOffExchange),
		status:   root.Text(assetOffStatus),
		provider: root.Text(assetOffProvider),
		tradable: root.Bool(assetOffTradable),
	}
}

// ───────────────────────────────────────────────────────────────────────
//  OrderResp — single order record
// ───────────────────────────────────────────────────────────────────────
//
//        0  text     id               (8)
//        8  text     client_order_id  (8)
//       16  text     account          (8)
//       24  text     symbol           (8)
//       32  text     side             (8)
//       40  text     order_type       (8)
//       48  text     time_in_force    (8)
//       56  text     status           (8)
//       64  float64  qty              (8)
//       72  float64  filled_qty       (8)
//       80  float64  limit_price      (8)
//       88  float64  stop_price       (8)
//       96  float64  avg_fill_price   (8)
//      104  int64    created_ns       (8)
//      112  int64    updated_ns       (8)
//      120  int64    filled_ns        (8)
//      128 (total)
const (
	ordOffID        = 0
	ordOffCliID     = 8
	ordOffAccount   = 16
	ordOffSymbol    = 24
	ordOffSide      = 32
	ordOffOrderType = 40
	ordOffTIF       = 48
	ordOffStatus    = 56
	ordOffQty       = 64
	ordOffFilledQty = 72
	ordOffLimit     = 80
	ordOffStop      = 88
	ordOffAvgFill   = 96
	ordOffCreatedNS = 104
	ordOffUpdatedNS = 112
	ordOffFilledNS  = 120
	ordSize         = 128
)

type order struct {
	id, clientID, account, symbol, side, orderType, tif, status string
	qty, filledQty, limitPrice, stopPrice, avgFillPrice         float64
	createdNs, updatedNs, filledNs                              int64
}

func writeOrder(b *zap.Builder, o order) int {
	ob := b.StartObject(ordSize)
	ob.SetText(ordOffID, o.id)
	ob.SetText(ordOffCliID, o.clientID)
	ob.SetText(ordOffAccount, o.account)
	ob.SetText(ordOffSymbol, o.symbol)
	ob.SetText(ordOffSide, o.side)
	ob.SetText(ordOffOrderType, o.orderType)
	ob.SetText(ordOffTIF, o.tif)
	ob.SetText(ordOffStatus, o.status)
	ob.SetFloat64(ordOffQty, o.qty)
	ob.SetFloat64(ordOffFilledQty, o.filledQty)
	ob.SetFloat64(ordOffLimit, o.limitPrice)
	ob.SetFloat64(ordOffStop, o.stopPrice)
	ob.SetFloat64(ordOffAvgFill, o.avgFillPrice)
	ob.SetInt64(ordOffCreatedNS, o.createdNs)
	ob.SetInt64(ordOffUpdatedNS, o.updatedNs)
	ob.SetInt64(ordOffFilledNS, o.filledNs)
	return ob.Finish()
}

func buildOrderResp(op uint16, o order) []byte {
	b := zap.NewBuilder(512)
	ob := b.StartObject(ordSize)
	ob.SetText(ordOffID, o.id)
	ob.SetText(ordOffCliID, o.clientID)
	ob.SetText(ordOffAccount, o.account)
	ob.SetText(ordOffSymbol, o.symbol)
	ob.SetText(ordOffSide, o.side)
	ob.SetText(ordOffOrderType, o.orderType)
	ob.SetText(ordOffTIF, o.tif)
	ob.SetText(ordOffStatus, o.status)
	ob.SetFloat64(ordOffQty, o.qty)
	ob.SetFloat64(ordOffFilledQty, o.filledQty)
	ob.SetFloat64(ordOffLimit, o.limitPrice)
	ob.SetFloat64(ordOffStop, o.stopPrice)
	ob.SetFloat64(ordOffAvgFill, o.avgFillPrice)
	ob.SetInt64(ordOffCreatedNS, o.createdNs)
	ob.SetInt64(ordOffUpdatedNS, o.updatedNs)
	ob.SetInt64(ordOffFilledNS, o.filledNs)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

func readOrderRoot(root zap.Object) order {
	return order{
		id:           root.Text(ordOffID),
		clientID:     root.Text(ordOffCliID),
		account:      root.Text(ordOffAccount),
		symbol:       root.Text(ordOffSymbol),
		side:         root.Text(ordOffSide),
		orderType:    root.Text(ordOffOrderType),
		tif:          root.Text(ordOffTIF),
		status:       root.Text(ordOffStatus),
		qty:          root.Float64(ordOffQty),
		filledQty:    root.Float64(ordOffFilledQty),
		limitPrice:   root.Float64(ordOffLimit),
		stopPrice:    root.Float64(ordOffStop),
		avgFillPrice: root.Float64(ordOffAvgFill),
		createdNs:    root.Int64(ordOffCreatedNS),
		updatedNs:    root.Int64(ordOffUpdatedNS),
		filledNs:     root.Int64(ordOffFilledNS),
	}
}

// ───────────────────────────────────────────────────────────────────────
//  EmptyResp — acknowledgment payload (CancelOrder success, etc.)
// ───────────────────────────────────────────────────────────────────────
//
//        0  bool    ok   (1, padded to 8)
//        8 (total)
const (
	emptyOffOK = 0
	emptySize  = 8
)

func buildEmptyResp(op uint16) []byte {
	b := zap.NewBuilder(32)
	ob := b.StartObject(emptySize)
	ob.SetBool(emptyOffOK, true)
	ob.FinishAsRoot()
	return b.FinishWithFlags(packFlags(op, StatusOK))
}

// ───────────────────────────────────────────────────────────────────────
// CSV helpers — compact encoding for list-of-string requests.
// ───────────────────────────────────────────────────────────────────────

func joinCSV(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(parts) - 1 // commas
	for _, p := range parts {
		n += len(p)
	}
	buf := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, p...)
	}
	return string(buf)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	// Count commas first to size the output exactly.
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			n++
		}
	}
	out := make([]string, 0, n)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
