# Hanzo Broker

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

### Bearer token encoding (pkg/token/)
One canonical text encoding for every bearer credential: unpadded base64url,
and nothing else. `token.Decode` rejects any other spelling of the same bytes;
`token.ID` is the hash to key revocation, audit, dedup and rate limiting on.

The rule exists because Go's base64 decoders are lenient in two independent
ways, and a lenient decoder hands out one credential under several names:

- the unused low bits of the final quantum are ignored, so an HS256 signature
  (32 bytes in 43 chars, 2 slack bits) has 4 spellings and an RS256 signature
  (256 bytes in 342 chars, 4 slack bits) has 16;
- `\r` and `\n` are skipped wherever they appear, which `Encoding.Strict()`
  does **not** close — so Strict alone is not a fix.

`Decode` round-trips instead: a segment is canonical exactly when it is the
encoding of the bytes it decodes to. One predicate, nothing to enumerate.
Non-canonical input is rejected, never normalized.

Both JWT verifiers decode every segment through it. The one deliberate
exception is the JWKS `n`/`e` in `pkg/auth` — public key material from the
trusted IAM endpoint, where every spelling yields the same key and rejecting
one would take the whole JWKS offline.

Never key a denylist, audit record, dedup cache or rate-limit bucket on a
submitted token string, and never write the token itself to a log or a column.

### Admin Auth (pkg/admin/)
- `admin.go` -- HS256 JWT; passwords bcrypt cost 12, never plaintext
- `NewStore(jwtSecret)` takes the signing secret from the caller; source it
  from KMS. No env var, no default, no fallback.
- The verifier requires `alg: HS256` and never dispatches on the token header
- Middleware **strips** X-Admin-User/X-Admin-Role and passes claims through
  the request context — claims never travel as headers
- **Not mounted.** Nothing imports this package; `cmd/brokerd` does not serve
  an admin surface. Wire it up and the whole path goes live at once.

### Provider Registration from Env (pkg/provider/envconfig/)
`envconfig.RegisterFromEnv(registry)` reads all 16 provider env vars and registers
configured providers. This replaces the 16 if-blocks that were duplicated in
`cmd/brokerd/main.go`. Any ATS, BD, or TA binary
imports `envconfig` instead of importing all 16 provider sub-packages directly.

The `envconfig` package lives as a sub-package of `provider` (not in `provider` itself)
to avoid import cycles: `provider/alpaca` has compile-time interface assertions that
import `provider`, so `provider` cannot import `provider/alpaca`.

### Compliance (pkg/compliance/)
Thin HTTP adapter layer over `github.com/luxfi/compliance` library.
All domain types, store interface, MemoryStore, RBAC, and onboarding
logic live in the library. This package re-exports types as aliases and provides:
- HTTP handlers (chi router with RBAC guards, rate limiting, CORS)
- PostgresStore (pgx-backed, implements library's ComplianceStore interface)
- Webhook dispatcher for cross-BD trade events (`pkg/compliance/webhooks/`)
- Seed data for development mode

Mounted at `/compliance` on the main server. Supports in-memory store (default) or
PostgreSQL via `DATABASE_URL` env var. AML screening uses OFAC SDN + ScamSniffer
locally, backed by the native `github.com/luxfi/aml` engine (`amld`).

Key endpoint groups under `/compliance`:
- `/kyc` -- Identity verification, document upload, status tracking
- `/aml` -- AML screening (OFAC/ScamSniffer + native luxfi/aml), risk assessment, flagged review
- `/applications` -- 5-step onboarding flow (SSN hashed with HMAC-SHA256)
- `/pipelines` + `/sessions` -- Configurable onboarding pipeline engine
- `/funds` -- Fund management with investor tracking
- `/esign` -- Envelope/template eSign workflow
- `/roles` + `/modules` -- RBAC permission matrix

Library dependency (go.mod):
```
require github.com/luxfi/compliance v0.1.0
replace github.com/luxfi/compliance => ../compliance
```

### Database (pkg/db/)
PostgreSQL connection pool and auto-migrations. Used by compliance when `DATABASE_URL` is set.

### Endpoint Groups
| Prefix | Auth | Purpose |
|--------|------|---------|
| `/v1/*` | API Key/Bearer | Trading API |
| `/healthz` | none | Health check |
