# alpaca_omnisub

Alpaca OmniSub (Omnibus + Sub-accounts) provider for lux/broker.

## How it differs from `alpaca`

| | `alpaca` | `alpaca_omnisub` |
|---|---|---|
| Account model | Standalone individual/joint accounts | Sub-accounts under a single omnibus master |
| Account creation | `account_type` = individual/joint | `account_type` = `omnibus_sub_account`, `omnibus_master_id` set |
| Orders | Scoped to each account | Scoped to each sub-account |
| Positions | Per-account | Per-sub-account |
| Transfers (ACH/wire) | Per-account | Routed through omnibus master; internal ledger via journals |
| Bank relationships | Per-account | On omnibus master |
| Reconciliation | Per-account | Omnibus aggregate via `GetOmnibusSnapshot()` |
| Market data | Alpaca data API | Same Alpaca data API |

## Environment variables

```
ALPACA_OMNISUB_API_KEY          # Alpaca broker API key (omnisub-specific)
ALPACA_OMNISUB_API_SECRET       # Alpaca broker API secret
ALPACA_OMNISUB_BASE_URL         # defaults to sandbox
ALPACA_OMNISUB_OMNIBUS_ACCOUNT_ID  # the omnibus master account ID
```

Both `alpaca` and `alpaca_omnisub` can run in parallel on the same registry with independent credentials.
