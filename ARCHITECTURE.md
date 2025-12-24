# Broker Architecture

Smart order router and multi-venue execution engine. Single Go binary (`brokerd`), 16 venue adapters, institutional-grade order routing with net-price optimization.

## Design Principles

1. Single process. No microservices. One binary handles routing, risk, execution, settlement, compliance, market data, and streaming.
2. Venue adapters are the only abstraction. Everything else is concrete.
3. All order flow passes through risk engine before reaching any venue.
4. Market data is aggregated in-process. No external message bus required (NATS available but optional).
5. Secrets from KMS. Never in manifests, never in env files committed to git.

## System Topology

```
                                  Clients
                                    |
                              TLS (Cloudflare)
                                    |
                           hanzoai/gateway
                           (api.lux.network)
                                    |
                          +---------+---------+
                          |     brokerd       |
                          |     :8090         |
                          +---+---+---+---+---+
                              |   |   |   |
               +--------------+   |   |   +--------------+
               |                  |   |                   |
          +----+----+      +------+------+         +------+------+
          |  Router |      | Risk Engine |         |  Settlement |
          |  (SOR)  |      | (pre-trade) |         |   (pool)    |
          +----+----+      +-------------+         +------+------+
               |                                          |
     +---------+---------+                          +-----+-----+
     |    Market Data    |                          |   Funding  |
     |  Feed (BBO agg)  |                          | (deposit/  |
     +---+---+---+---+--+                          |  withdraw) |
         |   |   |   |                             +------------+
     +---+---+---+---+---+---+---+---+
     | Provider Adapters (16 venues) |
     +---+---+---+---+---+---+---+---+
         |   |   |   |   |   |   |
        CB  KRK BIN GEM SFOX FLX LMAX  ...
```

## Component Responsibilities

### Router (`pkg/router/`)

The smart order router. Stateless decision engine.

**Input:** Symbol, side (buy/sell), quantity.

**Decision algorithm:**

1. Fan out `GetAsset` + `GetSnapshot` to all registered providers in parallel (goroutines, channel collection).
2. Filter: drop providers where asset is not tradable or snapshot fails.
3. Score each venue: `net_price = raw_price * (1 + taker_fee_bps / 10000)` for buys; `raw_price * (1 - taker_fee_bps / 10000)` for sells (negated so higher net = better).
4. Sort ascending by score. Index 0 is best.
5. Return ranked list.

**Split execution (VWAP):**

1. Get all routes (same as above).
2. Filter to routes with live quotes.
3. Weight inversely proportional to score: better score = more volume.
4. Allocate quantity proportionally. Last leg absorbs rounding remainder.
5. Calculate estimated VWAP, fees, net price, and savings vs. single-venue worst case.
6. Execute all legs in parallel via `CreateOrder` with `type=market`, `time_in_force=ioc`.
7. Aggregate fills: compute actual VWAP from weighted average of fill prices.

**Fee schedule:** Hard-coded defaults per provider (see `defaultFees()`). Override at runtime via `SetFees()`.

```
Provider     Maker/Taker (bps)    Notes
-----------  ------------------   -----
alpaca       0/0                  Commission-free equities
binance      10/10                Standard tier
kraken       16/26                Standard tier
sfox         15/25                Prime dealer
coinbase     40/60                Advanced Trade
gemini       20/40                ActiveTrader
falcon       5/10                 Institutional RFQ
lmax         2/3                  Institutional CLOB
ibkr         0.5/0.5             Per-share approximation
```

### Market Data Feed (`pkg/marketdata/`)

In-process consolidated order book and ticker aggregation.

**Data model:**
- `Ticker`: per-symbol, aggregates quotes from all providers. Tracks best bid, best ask, spread (absolute + bps), provider attribution for each side.
- `ConsolidatedBook`: per-symbol, sorted bid/ask levels with provider tags.
- `Quote`: per-provider raw quote (bid/ask price+size, last price).

**Update flow:**
1. Provider snapshot polled or WebSocket update arrives.
2. `UpdateQuote(symbol, provider, bid, ask, bidSize, askSize, last)` called.
3. Feed recalculates BBO across all providers: highest bid wins, lowest ask wins.
4. Computes cross-venue spread: `best_ask - best_bid` (can be negative = arbitrage).
5. Notifies all subscribers via non-blocking channel send.

**Arbitrage detection:** When `best_bid_provider != best_ask_provider` and `best_bid > best_ask`, there is a cross-venue arbitrage. The router exposes this through the `/v1/bbo/{symbol}` endpoint. Execution is the client's responsibility.

**Polling vs. streaming:**
- SSE streaming via `pkg/ws/`: clients subscribe to symbols, receive consolidated ticker updates.
- Snapshot polling via `Feed.PollSnapshots()`: background goroutine polls all providers at configurable interval (default: provider-dependent, typically 1-5s for crypto).
- WebSocket feeds from venues (Binance, Kraken, Coinbase) update the feed directly when available.

### Risk Engine (`pkg/risk/`)

Pre-trade validation. Synchronous. Blocks order submission if any check fails.

**Checks (in order):**
1. Provider allowlist (global).
2. Symbol blocklist (global + per-account).
3. Symbol allowlist (per-account, if configured).
4. Order value vs. max single order limit.
5. Daily volume accumulator vs. daily limit.
6. Open order count vs. concurrent order limit.
7. Rate limit: orders per minute (sliding window from timestamps).

**Limits hierarchy:** Per-account overrides global. Global defaults: $1M max order, $10M daily, 100 open orders, 60 orders/min.

**Usage tracking:** In-memory. Daily counters reset after 24h. Rate limit uses a sliding window of order timestamps.

### Settlement Engine (`pkg/settlement/`)

Prefunding pool for instant crypto buys while ACH settles.

**Flow:**
```
User "Buy 1 BTC" -> Validate KYC tier limit
                  -> Check outstanding exposure
                  -> Reserve pool capital (amount = qty * price)
                  -> Execute trade on best venue (caller)
                  -> Initiate ACH (caller)
                  -> Track lifecycle:
                       ACH clears  -> Release pool credit
                       ACH fails   -> Mark for liquidation
                       Price drops  -> Margin call / auto-liquidate
```

**KYC tiers:**
```
basic          $250       Email-verified
standard       $5,000     ID-verified
enhanced       $25,000    Full KYC + accredited
institutional  $250,000   Institutional accounts
```

**Margin policy:** Configurable thresholds. Default: 20% warning, 30% margin call, 50% auto-liquidation. `CheckMarginHealth()` scans all pending reservations against current prices.

**Pool:** In-memory reservation store. Each reservation tracks: account, asset, amount, entry price, status, timestamps, events. Statuses: `pending_settlement`, `settled`, `failed`, `margin_called`, `liquidated`.

### Provider Interface (`pkg/provider/`)

```go
type Provider interface {
    Name() string
    CreateAccount(ctx, req)    // Onboarding
    GetAccount(ctx, id)        // Account detail
    ListAccounts(ctx)          // All accounts
    GetPortfolio(ctx, id)      // Holdings snapshot
    CreateOrder(ctx, id, req)  // Place order
    ListOrders(ctx, id)        // Order history
    GetOrder(ctx, id, oid)     // Order detail
    CancelOrder(ctx, id, oid)  // Cancel
    CreateTransfer(ctx, id, r) // Fund movement
    ListTransfers(ctx, id)     // Transfer history
    CreateBankRelationship()   // Link bank
    ListBankRelationships()    // List banks
    ListAssets(ctx, class)     // Tradable instruments
    GetAsset(ctx, symbol)      // Asset detail
    GetSnapshot(ctx, symbol)   // Real-time price
    GetSnapshots(ctx, syms)    // Batch snapshots
    GetBars(ctx, ...)          // Historical OHLCV
    GetLatestTrades(ctx, syms) // Latest trades
    GetLatestQuotes(ctx, syms) // Latest quotes
    GetClock(ctx)              // Market hours
    GetCalendar(ctx, s, e)     // Trading calendar
}
```

**Optional capability interfaces** (type-asserted at runtime):
- `TradingExtended` -- Replace, cancel-all, positions.
- `AccountManager` -- Update, close, activities.
- `DocumentManager` -- Upload, list, download.
- `JournalManager` -- Inter-account transfers (JNLC/JNLS).
- `TransferExtended` -- Cancel transfer, ACH deletion, wire banks.
- `CryptoDataProvider` -- Crypto-specific bars/quotes/trades/snapshots.
- `EventStreamer` -- SSE streams for trade/account/transfer/journal events.
- `PortfolioAnalyzer` -- Portfolio equity history.
- `WatchlistManager` -- CRUD watchlists.

**Provider registration:** `pkg/provider/envconfig/` reads 16 sets of env vars and registers configured providers. Breaks import cycle: envconfig imports provider sub-packages, provider sub-packages import the provider interface package.

### Venues

| Provider | Pkg | Asset Classes | Execution | Market Data |
|----------|-----|--------------|-----------|-------------|
| Alpaca | `provider/alpaca/` | Equities, Crypto | Full (100% API) | Stocks + Crypto |
| Interactive Brokers | `provider/ibkr/` | Equities, Options, Futures, FX, Crypto | Full | Full |
| Tradier | `provider/tradier/` | Equities, Options | Full | Full |
| Coinbase | `provider/coinbase/` | Crypto | Advanced Trade | Full |
| Binance | `provider/binance/` | Crypto | Spot/Margin | Full |
| Kraken | `provider/kraken/` | Crypto | Full | Full |
| Gemini | `provider/gemini/` | Crypto | Full | Full |
| SFOX | `provider/sfox/` | Crypto | Market/Limit + Algo | Best-price + OHLCV |
| FalconX | `provider/falcon/` | Crypto | RFQ | Quotes |
| Fireblocks | `provider/fireblocks/` | Crypto | Transfer/Custody | Balances |
| BitGo | `provider/bitgo/` | Crypto | Custody + Prime | Balances |
| Circle | `provider/circle/` | Stablecoins | Mint/Burn/Transfer | Balances |
| CurrencyCloud | `provider/currencycloud/` | FX (35+ pairs) | Spot/Forward | Rates |
| LMAX | `provider/lmax/` | FX, Crypto, Metals | CLOB | Full |
| Polygon.io | `provider/polygon/` | Equities, FX, Crypto, Options | Data only | Full |
| Finix | `provider/finix/` | Payments | ACH/Wire/Card | N/A |

### Compliance (`pkg/compliance/`)

KYC/KYB onboarding, fund management, eSign, RBAC, reporting. Mounted at `/compliance`. PostgreSQL or in-memory store. Includes Jube AML/fraud integration (`compliance/jube/`) and webhook dispatcher (`compliance/webhooks/`).

### Audit (`pkg/audit/`)

Append-only log of every order, fill, cancellation, transfer. Queryable by action, provider, account, symbol, time range. Exportable as JSON.

### Auth (`pkg/auth/` + `pkg/admin/`)

- **API auth:** API key or Bearer token validation. Keys scoped to org via IAM OIDC `owner` claim.
- **Admin auth:** JWT (HMAC-SHA256). Password hashed (SHA-256 + salt). Admin users from env vars.

## Data Flow: Smart Order Execution

```
Client POST /v1/smart-order
  |
  v
API Server (auth middleware validates JWT/API key)
  |
  v
Risk Engine: PreTradeCheck(provider, account, symbol, side, qty, price)
  |-- REJECT if any check fails (returns errors)
  |-- WARN if approaching limits (returns warnings)
  |
  v
Router: FindBestProvider(ctx, symbol, side)
  |-- Fan out GetAsset + GetSnapshot to all providers (parallel)
  |-- Score: net_price = raw_price * (1 +/- fee_bps)
  |-- Return best provider
  |
  v
Provider: CreateOrder(ctx, accountID, request)
  |-- HTTP to venue API
  |-- Return unified Order struct
  |
  v
Audit: Record(action=order_created, provider, account, symbol, ...)
  |
  v
Risk Engine: RecordOrder(provider, account, orderValue)
  |
  v
Client receives Order response
```

## Data Flow: Split Order (VWAP)

```
Client POST /v1/smart-order/split
  |
  v
Router: BuildSplitPlan(ctx, symbol, side, qty)
  |-- GetAllRoutes (parallel venue polling)
  |-- Score-weighted allocation
  |-- Return SplitPlan with legs
  |
  v
Router: ExecuteSplitPlan(ctx, plan, accounts)
  |-- For each leg (parallel goroutines):
  |     Provider.CreateOrder(ctx, accountID, {type:market, tif:ioc})
  |-- Collect results via channel
  |-- Compute actual VWAP = sum(price_i * filled_i) / sum(filled_i)
  |
  v
Return ExecutionResult {vwap, legs[], status, latency}
```

## Data Flow: Market Data Aggregation

```
Background poller (configurable interval)
  |
  v
For each registered provider (parallel):
  Provider.GetSnapshot(ctx, symbol)
  |
  v
Feed.UpdateQuote(symbol, provider, bid, ask, sizes, last)
  |-- Update per-provider quote in ticker
  |-- Recalculate BBO: max(bids), min(asks) across providers
  |-- Compute spread + spread_bps
  |-- Notify subscribers (non-blocking channel send)
  |
  v
SSE Server: HandleSSE streams to connected clients
  |-- event: ticker
  |-- data: {symbol, best_bid, best_ask, spread, sources{}}
```

## API Surface

```
/healthz                              Health check (no auth)

/v1/route/{symbol}                    Best venues for symbol
/v1/route/{symbol}/split              VWAP split plan
/v1/smart-order                       Execute via best venue
/v1/smart-order/split                 Execute split across venues
/v1/bbo/{symbol}                      Best bid/offer (cross-venue)
/v1/stream                            SSE consolidated tickers
/v1/assets                            Aggregated tradable assets

/v1/accounts                          CRUD accounts
/v1/accounts/{p}/{id}/orders          CRUD orders
/v1/accounts/{p}/{id}/positions       Positions
/v1/accounts/{p}/{id}/transfers       Transfers
/v1/accounts/{p}/{id}/portfolio       Portfolio snapshot

/v1/market/{provider}/snapshot/{sym}  Per-venue snapshot
/v1/market/{provider}/bars/{sym}      Historical OHLCV
/v1/risk/check                        Pre-trade risk check
/v1/audit                             Query audit log

/v1/providers                         Registered providers
/v1/providers/capabilities            Provider feature matrix

/compliance/*                         KYC/KYB, onboarding, RBAC
```

## Deployment

### Single Binary

```
brokerd
  |- HTTP server (:8090)
  |- Provider registry (16 adapters, env-configured)
  |- Router (SOR)
  |- Risk engine
  |- Market data feed + SSE
  |- Settlement service + pool
  |- Compliance router (PostgreSQL or in-memory)
  |- Audit log
  |- Admin JWT auth
```

### K8s (lux-k8s cluster)

```yaml
Namespace: broker
Deployment: broker (1 replica, scales horizontally)
  Image: ghcr.io/luxfi/broker:{tag}
  Port: 8090
  Resources: 100m-500m CPU, 64Mi-256Mi RAM
  Probes: /healthz (readiness 5s, liveness 30s)
  Secrets: broker-secrets (KMSSecret CRD -> kms.hanzo.ai)
Service: broker:8090
Ingress: via hanzoai/gateway at api.lux.network
```

**Secrets flow:**
```
kms.hanzo.ai (Infisical)
  -> KMSSecret CRD in broker namespace
  -> K8s Secret: broker-secrets
  -> Pod env: DATABASE_URL, ALPACA_API_KEY, etc.
```

### compose.yml (local dev)

```
services:
  sql: postgres:17-alpine on :5434
```

Broker runs natively (`make run`) against the local Postgres and configured provider sandboxes.

### CI/CD

Image built by PaaS (platform.hanzo.ai) via Tekton pipeline. Cloud Build config exists for GCP fallback (`cloudbuild.yaml`). Images pushed to `ghcr.io/luxfi/broker:{tag}`.

## Technology Choices

| Choice | Rationale |
|--------|-----------|
| Go | Low-latency order routing requires predictable GC, goroutines for parallel venue polling, stdlib HTTP. |
| Single binary | No inter-service latency for routing decisions. One process = one failure domain. |
| chi router | stdlib-compatible, middleware chains, zero allocation routing. |
| zerolog | Structured JSON logging, zero allocation. |
| pgx/v5 | PostgreSQL driver for compliance store. Direct, no ORM. |
| NATS (go.mod) | Available for inter-process pub/sub if broker scales to multiple instances. Not required for single-instance. |
| SSE over WebSocket | Works through all proxies/load balancers. Simpler. Sufficient for ticker updates. |
| In-memory risk/settlement | Latency-critical paths. PostgreSQL for durable compliance data. |

## Scaling Path

The broker is designed to scale horizontally when needed:

1. **Current:** Single instance handles all routing, risk, execution. Sufficient for <10k orders/day.
2. **Next:** NATS for cross-instance market data fan-out. Sticky sessions for settlement state. PostgreSQL for risk state.
3. **Future:** Dedicated market data service (separate binary from same codebase). Sharded by asset class.

The single-binary architecture means "scaling" is primarily about running more replicas behind the gateway, not decomposing into microservices.

## Related Systems

| System | Relationship |
|--------|-------------|
| `luxfi/compliance` | Extracted compliance module (KYC/AML) |
| `luxfi/bank` | Customer-facing apps consuming broker API |
| `hanzoai/commerce` | Payment processors (Plaid, Braintree, Stripe) used by funding service |
| `hanzoai/iam` | Identity provider. Broker validates OIDC tokens, scopes to org. |
| `kms.hanzo.ai` | All provider credentials. KMSSecret CRDs sync to K8s. |
