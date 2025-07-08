# Lux Broker

Institutional federated broker with smart order routing across 16 providers.

## Providers

| Provider | Asset Classes | Type |
|----------|-------------|------|
| **Alpaca** | Equities, Crypto | Broker-dealer |
| **Interactive Brokers** | Equities, Options, Futures, FX | Prime broker |
| **Tradier** | Equities, Options | Broker-dealer |
| **Coinbase** | Crypto | Exchange (Advanced Trade) |
| **Binance** | Crypto | Exchange |
| **Kraken** | Crypto | Exchange |
| **Gemini** | Crypto | Exchange |
| **SFOX** | Crypto | Prime dealer / SOR |
| **FalconX** | Crypto | Institutional RFQ |
| **Fireblocks** | Crypto | Custody + OTC |
| **BitGo** | Crypto | Custody + Prime |
| **Circle** | USDC / Stablecoins | Stablecoin infrastructure |
| **CurrencyCloud** | FX (35 pairs) | Institutional FX (Visa) |
| **LMAX** | FX, Crypto, Metals | Institutional CLOB |
| **Polygon.io** | Equities, FX, Crypto | Market data |
| **Finix** | Payments | Payment processing |

## Features

- **Smart Order Routing** — Fee-aware net-price optimization across all venues
- **Order Splitting** — Automatic split across venues for large orders (VWAP)
- **Pre-Trade Risk** — Position limits, rate limits, concentration checks
- **Best Execution** — FINRA Rule 5310 / MiFID II compliant routing
- **Multi-Asset** — Crypto, equities, FX, options, futures, precious metals
- **TWAP Scheduling** — Time-weighted execution for large orders

## Architecture

```
Client Request
      |
  [brokerd :8090]
      |
  [Smart Order Router]
      |
  +---+---+---+---+---+
  |   |   |   |   |   |
 CEX DEX  LP  LP  LP ...  (16 providers)
      |
  [Risk Engine]
      |
  [Fee Calculator]  →  Net-price ranking
      |
  [Split Engine]    →  Multi-venue fills
      |
  Response (fills, VWAP)
```

## Quick Start

```bash
# Build
go build -o brokerd ./cmd/brokerd/

# Run (configure at least one provider)
ALPACA_API_KEY=... ALPACA_API_SECRET=... ./brokerd

# Or with multiple providers
ALPACA_API_KEY=... COINBASE_API_KEY=... KRAKEN_API_KEY=... ./brokerd
```

## API

All endpoints under `/api/v1/` on port `:8090` (configurable via `BROKER_LISTEN`).

### Accounts
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/{provider}/accounts` | Create account |
| `GET` | `/api/v1/{provider}/accounts` | List accounts |
| `GET` | `/api/v1/{provider}/accounts/{id}` | Get account |
| `GET` | `/api/v1/{provider}/accounts/{id}/portfolio` | Get portfolio |

### Orders
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/{provider}/accounts/{id}/orders` | Create order |
| `GET` | `/api/v1/{provider}/accounts/{id}/orders` | List orders |
| `GET` | `/api/v1/{provider}/accounts/{id}/orders/{oid}` | Get order |
| `DELETE` | `/api/v1/{provider}/accounts/{id}/orders/{oid}` | Cancel order |

### Smart Router
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/route/order` | Route order to best venue |
| `GET` | `/api/v1/route/capabilities` | Provider capabilities |

### Market Data
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/{provider}/market/snapshot/{symbol}` | Price snapshot |
| `GET` | `/api/v1/{provider}/market/bars/{symbol}` | OHLCV bars |
| `GET` | `/api/v1/{provider}/market/trades/{symbols}` | Latest trades |
| `GET` | `/api/v1/{provider}/market/quotes/{symbols}` | Latest quotes |
| `GET` | `/api/v1/{provider}/market/clock` | Market clock |

### Assets
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/{provider}/assets` | List assets |
| `GET` | `/api/v1/{provider}/assets/{symbol}` | Get asset |

## Configuration

| Env Var | Description |
|---------|-------------|
| `BROKER_LISTEN` | Listen address (default `:8090`) |
| `ALPACA_API_KEY` / `ALPACA_API_SECRET` | Alpaca credentials |
| `IBKR_ACCESS_TOKEN` / `IBKR_ACCOUNT_ID` | Interactive Brokers |
| `COINBASE_API_KEY` / `COINBASE_API_SECRET` | Coinbase Advanced Trade |
| `BINANCE_API_KEY` / `BINANCE_API_SECRET` | Binance |
| `KRAKEN_API_KEY` / `KRAKEN_API_SECRET` | Kraken |
| `GEMINI_API_KEY` / `GEMINI_API_SECRET` | Gemini |
| `SFOX_API_KEY` | SFOX Prime |
| `FALCON_API_KEY` / `FALCON_API_SECRET` / `FALCON_PASSPHRASE` | FalconX |
| `FIREBLOCKS_API_KEY` / `FIREBLOCKS_PRIVATE_KEY` | Fireblocks |
| `BITGO_ACCESS_TOKEN` / `BITGO_ENTERPRISE` | BitGo |
| `CIRCLE_API_KEY` | Circle |
| `TRADIER_ACCESS_TOKEN` / `TRADIER_ACCOUNT_ID` | Tradier |
| `POLYGON_API_KEY` | Polygon.io (market data) |
| `CURRENCYCLOUD_LOGIN_ID` / `CURRENCYCLOUD_API_KEY` | CurrencyCloud FX |
| `LMAX_API_KEY` / `LMAX_USERNAME` / `LMAX_PASSWORD` | LMAX Digital |
| `FINIX_USERNAME` / `FINIX_PASSWORD` | Finix payments |

## Docker

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o brokerd ./cmd/brokerd/
docker build --platform linux/amd64 -t ghcr.io/luxfi/broker:latest .
```

## License

Copyright 2024-2026, Lux Partners Limited. All rights reserved.
