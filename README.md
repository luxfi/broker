# Broker

Multi-venue trading router with smart order routing, pre-trade risk, instant settlement, and real-time market data aggregation across 16 institutional providers.

```
go build -o brokerd ./cmd/brokerd/
ALPACA_API_KEY=pk_... ALPACA_API_SECRET=sk_... ./brokerd
```

## Providers

| Provider | Asset Classes | Type |
|----------|-------------|------|
| Alpaca | Equities, Crypto | Broker-dealer (SEC/FINRA) |
| Interactive Brokers | Equities, Options, Futures, FX | Prime broker |
| Tradier | Equities, Options | Broker-dealer |
| Coinbase | Crypto | Exchange (Advanced Trade API) |
| Binance | Crypto | Exchange |
| Kraken | Crypto | Exchange |
| Gemini | Crypto | Exchange |
| SFOX | Crypto | Prime dealer / SOR |
| FalconX | Crypto | Institutional RFQ |
| Fireblocks | Crypto | Custody + OTC |
| BitGo | Crypto | Custody + Prime |
| Circle | USDC / Stablecoins | Stablecoin infrastructure |
| CurrencyCloud | FX (35+ pairs) | Institutional FX (Visa) |
| LMAX | FX, Crypto, Metals | Institutional CLOB |
| Polygon.io | Equities, FX, Crypto | Market data |
| Finix | Payments | Payment processing |

## Architecture

```
                        ┌──────────────┐
                        │  brokerd     │
                        │  :8090       │
                        └──────┬───────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
       ┌──────┴──────┐  ┌─────┴──────┐  ┌──────┴──────┐
       │ Smart Order │  │   Risk     │  │   Audit     │
       │   Router    │  │  Engine    │  │    Log      │
       └──────┬──────┘  └────────────┘  └─────────────┘
              │
    ┌────┬────┼────┬────┬────┬────┐
    │    │    │    │    │    │    │
   CEX  CEX  OTC  LP   FX  Data  Pay   (16 providers)
    │    │    │    │    │    │    │
   CB  KRK  FLX  SFX  CC  POLY  FNX
```

**Smart Order Router** polls all venues in parallel, ranks by net price (spread + fees), and can split large orders across venues using VWAP.

**Risk Engine** enforces per-account and global limits: order value, daily volume, open order count, rate limits, symbol whitelists/blacklists.

**Settlement Engine** manages a prefunding pool for instant crypto buys while ACH settles. Margin monitoring auto-liquidates at configurable drawdown thresholds.

**Audit Log** is an immutable append-only trail of every order, fill, cancellation, and transfer. Queryable, exportable, with hook support for external logging.

## API Reference

Base URL: `http://localhost:8090/v1`

Auth: `Authorization: Bearer <api-key>` or `X-API-Key: <api-key>` header. Health check at `/healthz` requires no auth.

### Accounts

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/accounts` | Create account (with provider, identity, contact) |
| GET | `/v1/accounts` | List all accounts across providers |
| GET | `/v1/accounts/{provider}/{accountId}` | Get account detail |
| GET | `/v1/accounts/{provider}/{accountId}/portfolio` | Portfolio snapshot |
| PATCH | `/v1/accounts/{provider}/{accountId}` | Update account |
| DELETE | `/v1/accounts/{provider}/{accountId}` | Close account |
| GET | `/v1/accounts/{provider}/{accountId}/activities` | Account activity feed |

### Orders

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/accounts/{provider}/{accountId}/orders` | Place order |
| GET | `/v1/accounts/{provider}/{accountId}/orders` | List orders (`?status=open\|closed\|all`) |
| GET | `/v1/accounts/{provider}/{accountId}/orders/{orderId}` | Get order |
| PATCH | `/v1/accounts/{provider}/{accountId}/orders/{orderId}` | Replace/modify order |
| DELETE | `/v1/accounts/{provider}/{accountId}/orders/{orderId}` | Cancel order |
| DELETE | `/v1/accounts/{provider}/{accountId}/orders` | Cancel all orders |

### Positions

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/accounts/{provider}/{accountId}/positions/{symbol}` | Get position |
| DELETE | `/v1/accounts/{provider}/{accountId}/positions/{symbol}` | Close position |
| DELETE | `/v1/accounts/{provider}/{accountId}/positions` | Close all positions |

### Smart Order Routing

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/route/{symbol}` | Best venues for symbol |
| GET | `/v1/route/{symbol}/{quote}` | Best venues for pair (e.g. BTC/USD) |
| GET | `/v1/route/{symbol}/split` | VWAP split plan |
| GET | `/v1/route/{symbol}/{quote}/split` | VWAP split plan for pair |
| POST | `/v1/smart-order` | Execute smart order (routes to best venue) |
| POST | `/v1/smart-order/split` | Execute split across venues |
| GET | `/v1/assets` | Aggregated assets across all providers |

### Market Data

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/market/{provider}/snapshot/{symbol}` | Real-time price snapshot |
| GET | `/v1/market/{provider}/snapshots` | Multiple snapshots (`?symbols=`) |
| GET | `/v1/market/{provider}/bars/{symbol}` | OHLCV bars (`?timeframe=&start=&end=&limit=`) |
| GET | `/v1/market/{provider}/trades/latest` | Latest trades (`?symbols=`) |
| GET | `/v1/market/{provider}/quotes/latest` | Latest quotes (`?symbols=`) |
| GET | `/v1/market/{provider}/clock` | Market clock |
| GET | `/v1/market/{provider}/calendar` | Market calendar |
| GET | `/v1/market/{provider}/crypto/bars` | Crypto OHLCV bars |
| GET | `/v1/market/{provider}/crypto/quotes` | Crypto quotes |
| GET | `/v1/market/{provider}/crypto/trades` | Crypto trades |
| GET | `/v1/market/{provider}/crypto/snapshots` | Crypto snapshots |

### Consolidated Data

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/bbo/{symbol}` | Best bid/offer across all venues |
| GET | `/v1/bbo/{symbol}/{quote}` | BBO for pair |
| GET | `/v1/stream` | SSE stream (real-time consolidated tickers) |

### Transfers & Banking

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/accounts/{provider}/{accountId}/transfers` | List transfers |
| POST | `/v1/accounts/{provider}/{accountId}/transfers` | Create transfer (ACH/wire/crypto) |
| DELETE | `/v1/accounts/{provider}/{accountId}/transfers/{transferId}` | Cancel transfer |
| GET | `/v1/accounts/{provider}/{accountId}/bank-relationships` | List bank relationships |
| POST | `/v1/accounts/{provider}/{accountId}/bank-relationships` | Create bank relationship |
| DELETE | `/v1/accounts/{provider}/{accountId}/ach-relationships/{achId}` | Delete ACH |
| POST | `/v1/accounts/{provider}/{accountId}/recipient-banks` | Add wire recipient |
| GET | `/v1/accounts/{provider}/{accountId}/recipient-banks` | List wire recipients |
| DELETE | `/v1/accounts/{provider}/{accountId}/recipient-banks/{bankId}` | Remove wire recipient |

### Journals (Inter-Account Transfers)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/journals/{provider}` | Create journal (JNLC cash / JNLS security) |
| GET | `/v1/journals/{provider}` | List journals |
| GET | `/v1/journals/{provider}/{journalId}` | Get journal |
| DELETE | `/v1/journals/{provider}/{journalId}` | Delete journal |
| POST | `/v1/journals/{provider}/batch` | Batch journal |
| POST | `/v1/journals/{provider}/reverse_batch` | Reverse batch |

### Documents

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/accounts/{provider}/{accountId}/documents` | Upload document |
| GET | `/v1/accounts/{provider}/{accountId}/documents` | List documents |
| GET | `/v1/accounts/{provider}/{accountId}/documents/{documentId}` | Get document |
| GET | `/v1/accounts/{provider}/{accountId}/documents/{documentId}/download` | Download |

### Watchlists

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/accounts/{provider}/{accountId}/watchlists` | Create watchlist |
| GET | `/v1/accounts/{provider}/{accountId}/watchlists` | List watchlists |
| GET | `/v1/accounts/{provider}/{accountId}/watchlists/{id}` | Get watchlist |
| PUT | `/v1/accounts/{provider}/{accountId}/watchlists/{id}` | Update watchlist |
| DELETE | `/v1/accounts/{provider}/{accountId}/watchlists/{id}` | Delete watchlist |
| POST | `/v1/accounts/{provider}/{accountId}/watchlists/{id}/assets` | Add asset |
| DELETE | `/v1/accounts/{provider}/{accountId}/watchlists/{id}/{symbol}` | Remove asset |

### Portfolio History

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/accounts/{provider}/{accountId}/portfolio/history` | Equity time series (`?period=&timeframe=`) |

### Event Streaming (SSE)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/events/{provider}/trades` | Trade events |
| GET | `/v1/events/{provider}/accounts` | Account events |
| GET | `/v1/events/{provider}/transfers` | Transfer events |
| GET | `/v1/events/{provider}/journals` | Journal events |

### Risk & Audit

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/risk/check` | Pre-trade risk check (`?provider=&account_id=&symbol=&side=&qty=&price=&type=`) |
| GET | `/v1/audit` | Query audit log (`?action=&provider=&account_id=&symbol=&since=&until=`) |
| GET | `/v1/audit/stats` | Audit statistics |
| GET | `/v1/audit/export` | Export audit log (JSON download) |

### Funding

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/fund/deposit` | Deposit via payment processor |
| POST | `/v1/fund/withdraw` | Withdraw via payment processor |
| POST | `/v1/fund/webhook/{processor}` | Payment processor webhook |
| GET | `/v1/fund/processors` | List available processors |

### Providers

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/providers` | List registered providers |
| GET | `/v1/providers/capabilities` | Provider capabilities (asset classes, fees, features) |

## Settlement Engine

The prefunding pool enables instant crypto execution while ACH/wire settlement is pending.

```
User: "Buy 1 BTC"  →  Pool reserves $50k  →  Executes at market  →  ACH initiated
                                                                        │
                                              ACH clears (2-5 days) ────┤
                                              → Pool credit released    │
                                                                        │
                                              ACH fails ────────────────┘
                                              → Auto-liquidate position
                                              → Recover pool capital
```

**KYC-tiered instant buy limits:**

| Tier | Instant Limit | Use Case |
|------|--------------|----------|
| basic | $250 | Email-verified users |
| standard | $5,000 | ID-verified users |
| enhanced | $25,000 | Full KYC + accredited |
| institutional | $250,000 | Institutional accounts |

**Margin thresholds** (configurable): 20% warning, 30% margin call, 50% auto-liquidation.

## Configuration

### Required

At least one provider must be configured. Set the API key environment variables for each provider you want to use.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BROKER_LISTEN` | `:8090` | HTTP listen address |
| `BROKER_API_KEY` | — | Initial API key to register |
| `BROKER_ORG_ID` | — | Organization ID for API key |
| `BROKER_DEV_MODE` | `false` | Bypass auth (development only) |
| `ADMIN_USERNAME` | `admin` | Admin login username |
| `ADMIN_PASSWORD` | — | Admin login password (hashed, never plaintext) |
| `ADMIN_SECRET` | — | JWT signing secret (required for production) |

**Provider credentials** — set the pair for each provider you want to enable:

| Provider | Variables |
|----------|-----------|
| Alpaca | `ALPACA_API_KEY`, `ALPACA_API_SECRET` |
| Interactive Brokers | `IBKR_ACCESS_TOKEN`, `IBKR_ACCOUNT_ID`, `IBKR_GATEWAY_URL`, `IBKR_CONSUMER_KEY` |
| Coinbase | `COINBASE_API_KEY`, `COINBASE_API_SECRET` |
| Binance | `BINANCE_API_KEY`, `BINANCE_API_SECRET` |
| Kraken | `KRAKEN_API_KEY`, `KRAKEN_API_SECRET` |
| Gemini | `GEMINI_API_KEY`, `GEMINI_API_SECRET` |
| SFOX | `SFOX_API_KEY` |
| FalconX | `FALCON_API_KEY`, `FALCON_API_SECRET`, `FALCON_PASSPHRASE` |
| Fireblocks | `FIREBLOCKS_API_KEY`, `FIREBLOCKS_PRIVATE_KEY` |
| BitGo | `BITGO_ACCESS_TOKEN`, `BITGO_ENTERPRISE` |
| Circle | `CIRCLE_API_KEY` |
| Tradier | `TRADIER_ACCESS_TOKEN`, `TRADIER_ACCOUNT_ID` |
| Polygon.io | `POLYGON_API_KEY` |
| CurrencyCloud | `CURRENCYCLOUD_LOGIN_ID`, `CURRENCYCLOUD_API_KEY` |
| LMAX | `LMAX_API_KEY`, `LMAX_USERNAME`, `LMAX_PASSWORD` |
| Finix | `FINIX_USERNAME`, `FINIX_PASSWORD` |
| Braintree | `BRAINTREE_PUBLIC_KEY`, `BRAINTREE_PRIVATE_KEY`, `BRAINTREE_MERCHANT_ID` |

## Build & Test

```bash
make build          # Build binary
make test           # Run tests
make test-race      # Run tests with race detector
make lint           # go vet
make docker         # Build Docker image
make docker-push    # Push to ghcr.io
```

## Docker

```bash
docker build --platform linux/amd64 -t ghcr.io/luxfi/broker:latest .
docker run -p 8090:8090 -e ALPACA_API_KEY=... -e ALPACA_API_SECRET=... ghcr.io/luxfi/broker:latest
```

Image: `ghcr.io/luxfi/broker` — 7.5 MB, alpine-based, healthcheck on `/healthz`.

## Project Structure

```
broker/
├── cmd/brokerd/          Entry point, provider wiring
├── pkg/
│   ├── api/              HTTP handlers, chi router, middleware
│   ├── admin/            JWT auth (HMAC-SHA256), admin users
│   ├── auth/             API key validation, permissions
│   ├── audit/            Immutable audit trail
│   ├── funding/          Deposit/withdraw via payment processors
│   ├── marketdata/       Real-time ticker aggregation, BBO
│   ├── provider/         Provider interface + 16 implementations
│   │   ├── alpaca/       Full Alpaca Broker API (100% coverage)
│   │   ├── binance/      Binance spot/margin
│   │   ├── bitgo/        BitGo custody + Prime
│   │   ├── circle/       Circle USDC operations
│   │   ├── coinbase/     Coinbase Advanced Trade
│   │   ├── currencycloud/ CurrencyCloud FX
│   │   ├── falcon/       FalconX RFQ
│   │   ├── finix/        Finix payments
│   │   ├── fireblocks/   Fireblocks custody
│   │   ├── gemini/       Gemini exchange
│   │   ├── ibkr/         Interactive Brokers
│   │   ├── kraken/       Kraken exchange
│   │   ├── lmax/         LMAX Digital
│   │   ├── polygon/      Polygon.io data
│   │   ├── sfox/         SFOX prime dealer
│   │   └── tradier/      Tradier equities
│   ├── risk/             Pre-trade risk engine
│   ├── router/           Smart order routing (net-price + VWAP)
│   ├── settlement/       Prefunding pool, instant buy, margin
│   ├── types/            Unified type definitions
│   └── ws/               WebSocket / SSE server
├── Dockerfile
├── Makefile
└── go.mod
```

## Related Projects

| Module | Purpose |
|--------|---------|
| [luxfi/compliance](https://github.com/luxfi/compliance) | KYC/AML, identity verification, regulatory frameworks |
| [luxfi/bank](https://github.com/luxfi/bank) | Customer apps, payments, admin dashboard |
| [hanzoai/commerce](https://github.com/hanzoai/commerce) | Payment processors (Plaid, Braintree, Stripe) |
| [hanzoai/iam](https://github.com/hanzoai/iam) | Identity and access management (OAuth2/OIDC) |

## License

Copyright 2024-2026 Lux Partners Limited. All rights reserved.
