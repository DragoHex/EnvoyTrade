# Phase 3: Portfolio & Margin Synchronization

This document specifies Phase 3 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Synchronize client positions, demat holdings, and real-time margin/collateral balances from Angel One to EnvoyTrade's database, updating account status transitions and providing real-time portfolio tracking for the dashboard.

---

## 2. Synchronization Architecture (`PortfolioSyncer`)

### 2.1 Normalized Domain Mappings
`internal/angelone/sync.go` implements portfolio synchronization:
- **Positions** (`client.GetPositions`):
  - Maps to `domain.PositionSyncParam`
  - Calculates net quantity, buy price, sell price, unrealized/realized P&L, and MTM.
- **Holdings** (`client.GetHoldings`):
  - Maps to `domain.HoldingSyncParam`
  - Normalizes `Tradingsymbol`, `SellableQuantity` (from `AuthorisedQuantity` / `Quantity`), and `BuyAveragePrice`.
- **Margins & Limits** (`client.GetUserMargins`):
  - Maps `RMSLimit` to `domain.MarginSyncParam`:
    - `AvailableCash`: `rms.AvailableCash`
    - `AvailableMargin`: `rms.AvailableMargin`
    - `RealizedPnl`: `rms.M2MRealized`
    - `TotalMtm`: `rms.M2MRealized + rms.M2MUnrealized`
    - `AccountValue`: `rms.Net`
    - `Status`: `"online"`

### 2.2 Transparent Re-Authentication on 401
If any portfolio query fails with `domain.ErrAuthExpired` (e.g. JWT token invalidated upstream):
1. Syncer captures the auth expiration error.
2. Invokes `Renewer` to refresh or re-authenticate the account via `RenewSession` / `HeadlessLogin`.
3. Retries the portfolio synchronization pipeline once.
4. If retry succeeds, updates database session tokens without failing the periodic sync routine.

### 2.3 Account Status Transitions
- **On Success**:
  - `accounts.status` &rarr; `"active"`
  - `accounts.auth_status` &rarr; `"authenticated"`
  - `accounts.auth_error` &rarr; `""` (cleared)
  - `accounts.token_expires_at` &rarr; refreshed to next `06:00:00 AM IST`
- **On Auth Failure** (after retry):
  - `accounts.status` &rarr; `"error"`
  - `accounts.auth_status` &rarr; `"unauthenticated"`
  - `accounts.auth_error` &rarr; recorded with error description

---

## 3. Implementation Details

### 3.1 New Files
- `internal/angelone/sync.go`: `PortfolioSyncer` struct, `SyncAccountPortfolio`, normalizers, and transparent re-auth retry logic.
- `internal/angelone/sync_test.go`: Pure TDD unit test suite.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestSync_Success` | Valid active session | Persists positions, holdings, margins; account marked `"active"`. |
| `TestSync_PositionsEmpty` | Account with 0 open positions | Clears stale positions in database without error. |
| `TestSync_HoldingsMapping` | Demat holdings returned | Maps sellable quantities and average buy prices accurately. |
| `TestSync_MarginsCalculation` | RMS limits with realized & unrealized MTM | Accurately maps `AvailableCash`, `AvailableMargin`, and `TotalMtm`. |
| `TestSync_TransparentReAuth` | SmartAPI returns 401 on first call | Refreshes session via renewer and succeeds on retry. |
| `TestSync_TerminalAuthFailure` | Re-auth fails after invalid credentials | Sets `auth_status = "unauthenticated"` and logs error. |
| `TestSync_UnreachableBroker` | Network connection drop | Returns `domain.ErrBrokerUnreachable` without clobbering existing DB data. |
