# brokerd — integration patterns

`brokerd` is a venue-agnostic execution engine. It accepts an
order intent (`symbol + side + quantity`), scores every venue that
can fill it on net price, runs the trade through a pre-trade risk
gate, and settles the resulting fill against a shared capital pool.
The same binary trades spot, derivatives, fixed income, and
on-chain assets — each surface is reached through a uniform venue
adapter and a uniform order request.

This document describes the architectural patterns. It does not
name individual counterparties; concrete adapters in
`pkg/provider/<name>/` are pluggable backends behind the same
interface and are configured at deploy time.

## 1. Smart order router (SOR)

`pkg/router` is the front door. The request shape is intentionally
small:

```go
RouteRequest{Symbol, Side, Qty}
```

The router consults its `provider.Registry`, asks every registered
adapter for a quote, and ranks the responses by **net execution
price**:

```
net_price(buy)  = ask + (taker_fee_bps / 10_000) * ask
net_price(sell) = bid - (taker_fee_bps / 10_000) * bid
score          = net_price        // lower wins for buy, higher for sell
```

Fee schedules per adapter are registered with
`router.SetFees(name, makerBps, takerBps)`. Capabilities (assets
supported, order types, min/max sizes, region restrictions) are
registered with `router.SetCapability(*ProviderCapability)`.
`FindBestProvider` returns the winning venue; `GetAllRoutes`
returns the full ranked slate for transparency and audit.

For institutional flow, `pkg/router/twap.go` slices a parent
order into a time-weighted schedule across the same SOR. Order
splitting across two or more venues for a single parent order is
supported when one venue cannot fill the full quantity at the
posted top-of-book size.

## 2. Venue adapter abstraction

Every external trading venue — spot exchanges, OTC liquidity
providers, regulated US exchanges, international ECNs, on-chain
liquidity pools, fixed-income desks — implements one interface in
`pkg/venue/venue.go`:

```go
type Venue interface {
    Name() string
    GetQuote(ctx, symbol)                           (*Quote, error)
    PlaceOrder(ctx, *PlaceOrderRequest)             (*Order, error)
    CancelOrder(ctx, accountID, orderID)            error
    GetOrderBook(ctx, symbol, depth)                (*OrderBook, error)
    StreamQuotes(ctx, symbols)                      (<-chan *Quote, error)
}
```

`Venue` is the narrow surface the SOR, the market-data aggregator,
and the arbitrage detector consume. Capability beyond the narrow
surface — bracket orders, fractional shares, ACATS, journal
transfers, fixed-income trading, crypto deposits — is expressed
through the optional interfaces in `pkg/provider/provider.go`
(`TradingExtended`, `JournalManager`, `ACATSManager`,
`FixedIncomeTrader`, `CryptoDataProvider`, etc.). A venue
implements only the optional interfaces that apply to it; callers
type-assert before invoking.

### Adding a new venue

1. **Create `pkg/provider/<venue_key>/`** with a package that
   implements `provider.Provider` and any optional interfaces the
   venue supports.
2. **Construct from env.** Use `pkg/provider/envconfig` so
   secrets are sourced from `BROKER_<VENUE>_API_KEY`,
   `BROKER_<VENUE>_API_SECRET`, etc. — never compiled in. In
   production secrets are pulled from KMS into env at process
   start.
3. **Register at boot.** In `cmd/brokerd`, register the venue
   with `provider.Registry.Register(name, instance)` behind a
   build tag or env-driven feature flag so deployments enable
   only the venues they are licensed to use.
4. **Declare capabilities.** Call `router.SetCapability` with
   asset classes, order types, regions, and min/max sizes so the
   SOR can pre-filter.
5. **Declare fees.** Call `router.SetFees` with the venue's
   maker/taker bps. The SOR will weight net price against them.
6. **Write a contract test** under `pkg/provider/<venue_key>/`
   that drives the adapter against the venue's sandbox / paper
   environment via `httptest` recordings. No live keys in CI.

The adapter is self-contained: the SOR, risk engine, settlement
pool, and market-data feed do not learn anything venue-specific —
they see only `Venue`, `Quote`, `Order`.

## 3. Risk engine — pre-trade gate

`pkg/risk` runs before any order reaches a venue adapter. It
enforces, in order:

- **Symbol allow/block lists** (global + per-account).
- **Per-order notional ceiling** (`MaxOrderValue` USD).
- **Open-order count ceiling** per account.
- **Position-value ceiling** per account.
- **Daily-volume ceiling** per account (rolling 24h window).
- **Rate limit** per account (orders per minute).
- **Provider whitelist** per global config — only adapters
  explicitly allowed for an account/jurisdiction can be selected
  by the SOR.
- **Cooldown after significant loss** to dampen runaway
  algorithmic flow.

Account limits override global limits where set. Every decision
returns a typed reason on rejection (`RejectReason`) so the API
surface can return structured 4xx responses and the audit log can
attribute every block.

## 4. Settlement pool

`pkg/settlement` separates **execution** from **funding**. A user
can buy before their fiat funding settles by drawing on a shared
pool of pre-funded capital, sized by KYC tier:

| Tier            | Default instant-buy ceiling (USD) |
|-----------------|----------------------------------:|
| Basic           |   250                             |
| Standard        | 5,000                             |
| Enhanced        | 25,000                            |
| Institutional   | 250,000                           |

Lifecycle events (`ach_initiated`, `ach_pending`, `ach_cleared`,
`ach_failed`, `margin_call`, `liquidated`) drive the state
machine. The pool tracks outstanding prefunded credit per
account, reconciles against incoming fiat settlements, and emits
margin calls / liquidations when an account's outstanding draw
breaches its tier ceiling for too long. `pkg/settlement/margin.go`
holds the margin maintenance math; `pkg/settlement/pool.go` holds
the pool-capital accounting.

## 5. Market-data aggregation

`pkg/marketdata` builds an **in-process consolidated book** by
fanning every adapter's `StreamQuotes` output into one feed keyed
by symbol. Two derived structures are maintained continuously:

- **Consolidated BBO** — best bid and best ask across all
  connected venues, with the source venue attributed at each
  price level (`PriceLevel{Provider, Price, Size}`).
- **Ticker cache** — most-recent quote per symbol, fanned out to
  subscribers via per-symbol channels.

The SOR reads the consolidated book at decision time so it never
makes a routing decision on a stale single-venue quote.
`pkg/marketdata/arbitrage.go` runs a separate scan over the same
feed: when `best_bid(venue_A) > best_ask(venue_B) + threshold_bps`
for the same symbol, an `ArbitrageOpportunity` is emitted (sized
by the smaller of the two top-of-book sizes). The detector is an
observer only — it does not auto-trade; downstream policy decides.

## 6. Funding rails — deposit and withdraw

`pkg/funding` delegates fiat and on-chain movement to the
processor registry in `hanzoai/commerce`. The request shape is
uniform regardless of method:

```go
DepositRequest{
    AccountID, Provider, Amount /* cents */, Currency,
    PaymentMethod,                 // card | bank_transfer | crypto
    Token,                         // card nonce
    TxHash, Chain, Address,        // on-chain
}

WithdrawRequest{
    AccountID, Provider, Amount, Currency,
    PaymentMethod,                 // bank_transfer | crypto
    RelationshipID,                // bank linkage id
    DestAddress, Chain,            // on-chain
}
```

The processor registry resolves `Provider` to a concrete
payment-processor adapter (card networks, bank-rail processors,
on-chain custody / wallet services) and returns a typed result
including a settlement ETA. Funding events are pushed into the
same settlement pool that the execution path draws from — there
is one capital ledger, not two.

## 7. End-to-end request flow

```
client → API (pkg/api)
       → risk.Engine.Check()                       — pre-trade gate
       → router.FindBestProvider(symbol, side)     — net-price ranking
       → venue.PlaceOrder()                        — winning adapter
       → settlement.Pool.Reserve()                 — capital draw
       → audit.Log()                               — content-addressed record
       → ws / webhook fan-out                      — fill notifications
```

Every layer is independently testable; every layer is independently
swappable. New venues, new payment processors, new risk policies,
new settlement tiers all plug into the same boundaries without
touching the others.
