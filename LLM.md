# LLM.md - Hanzo Broker

## Overview
Go module: github.com/luxfi/broker

## Tech Stack
- **Language**: Go

## Build & Run
```bash
go build ./...
go test ./...
```

## Structure
```
broker/
  Dockerfile
  Makefile
  README.md
  bin/
  brokerd
  cmd/
  go.mod
  go.sum
  k8s/
  pkg/
  sdk/
```

## Key Files
- `README.md` -- Project documentation
- `go.mod` -- Go module definition
- `Makefile` -- Build automation
- `Dockerfile` -- Container build

## Architecture

### Provider Interface Pattern
The core `provider.Provider` interface defines the baseline contract. Optional capabilities are expressed as Go sub-interfaces that providers implement selectively. The API layer uses runtime type assertions to expose routes:

```go
if am, ok := p.(provider.AccountManager); ok { ... }
```

### Optional Capability Interfaces (pkg/provider/provider.go)
- `TradingExtended` -- ReplaceOrder, CancelAll, positions management
- `AccountManager` -- UpdateAccount, CloseAccount, GetAccountActivities
- `DocumentManager` -- Upload/List/Get/Download documents
- `JournalManager` -- Inter-account cash/security transfers (JNLC/JNLS)
- `TransferExtended` -- CancelTransfer, ACH deletion, wire recipient banks
- `CryptoDataProvider` -- Crypto bars/quotes/trades/snapshots via /v1beta3
- `EventStreamer` -- SSE streams for trade/account/transfer/journal events
- `PortfolioAnalyzer` -- Portfolio equity history
- `WatchlistManager` -- CRUD watchlists with symbol management

### Alpaca Provider Files (pkg/provider/alpaca/)
- `alpaca.go` -- Core provider, accounts, orders, positions, market data
- `alpaca_accounts.go` -- AccountManager (update, close, activities)
- `alpaca_documents.go` -- DocumentManager
- `alpaca_journals.go` -- JournalManager (single, batch, reverse)
- `alpaca_transfers.go` -- TransferExtended (cancel, ACH, wire banks)
- `alpaca_crypto.go` -- CryptoDataProvider (/v1beta3/crypto/us)
- `alpaca_events.go` -- EventStreamer (SSE from /v1/events/)
- `alpaca_portfolio.go` -- PortfolioAnalyzer
- `alpaca_watchlists.go` -- WatchlistManager

### Market Data Routing
- Stock symbols (no "/") route to `/v2/stocks/`
- Crypto symbols (contain "/") route to `/v1beta3/crypto/us/`
- `isCryptoSymbol()` helper in alpaca.go
- GetBars supports pagination via `next_page_token`
- GetSnapshots/GetLatestTrades/GetLatestQuotes split by asset class

### API Routes (pkg/api/)
- `server.go` -- Core routes, middleware, and handler wiring
- `handlers_extended.go` -- All capability-based route handlers
- `handlers_funding.go` -- Payment processor deposit/withdraw

### Admin Auth (pkg/admin/)
- `admin.go` -- JWT auth (HMAC-SHA256), password hashing (SHA-256 + salt, never plaintext)
- Admin users configured via ADMIN_USERNAME + ADMIN_PASSWORD env vars
- JWT secret from ADMIN_SECRET env var (required for production)
- Middleware validates Bearer tokens, sets X-Admin-User/X-Admin-Role headers

### Provider Registration from Env (pkg/provider/envconfig/)
`envconfig.RegisterFromEnv(registry)` reads all 16 provider env vars and registers
configured providers. This replaces the 16 if-blocks that were duplicated in
`cmd/brokerd/main.go`. Any ATS, BD, or TA binary
imports `envconfig` instead of importing all 16 provider sub-packages directly.

The `envconfig` package lives as a sub-package of `provider` (not in `provider` itself)
to avoid import cycles: `provider/alpaca` has compile-time interface assertions that
import `provider`, so `provider` cannot import `provider/alpaca`.

### Compliance (pkg/compliance/)
KYC/KYB, onboarding, fund management, eSign, RBAC, and reporting.
Mounted at `/compliance` on the main server. Supports in-memory store (default) or
PostgreSQL via `DATABASE_URL` env var. Includes Jube AML/fraud client (`pkg/compliance/jube/`)
and webhook dispatcher (`pkg/compliance/webhooks/`).

### Database (pkg/db/)
PostgreSQL connection pool and auto-migrations. Used by compliance when `DATABASE_URL` is set.

### Endpoint Groups
| Prefix | Auth | Purpose |
|--------|------|---------|
| `/v1/*` | API Key/Bearer | Trading API |
| `/healthz` | none | Health check |
