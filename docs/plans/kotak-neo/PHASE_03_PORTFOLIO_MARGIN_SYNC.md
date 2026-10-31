# Phase 3: Portfolio & Margin Synchronization

This document specifies Phase 3 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Synchronize live open positions, Demat holdings, and cash/margin fund balances between Kotak Neo and PostgreSQL. Handle transparent on-demand re-login on expired sessions, maintain account status transitions, and register with the server's debounced syncer.

---

## 2. Technical Design

### 2.1 Syncer Implementation (`internal/kotak/sync.go`)
Implements:
```go
type PortfolioSyncer struct {
    Store            Store
    AccountPool      *broker.AccountPool
    LoginFunc        func(ctx context.Context, u, p, t, k, sec string) (*TradeSessionJSON, error)
    OnTokenRefreshed func(ctx context.Context, accountID uuid.UUID) error
    Logger           *slog.Logger
}

func (s *PortfolioSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error
```

### 2.2 Data Ingestion & Mapping
- **Positions**:
  - Endpoint: `GET <BaseURL>/quick/user/positions`
  - Normalized to `domain.PositionSyncParam`:
    - `Product`: `CNC`, `MIS`, `NRML`
    - `Quantity`: Net quantity
    - `BuyPrice`, `SellPrice`, `Ltp`, `Mtm`, `Pnl` (Strictly `shopspring/decimal`)
- **Holdings**:
  - Endpoint: `GET <BaseURL>/portfolio/v1/holdings`
  - Normalized to `domain.HoldingSyncParam`:
    - `Instrument`, `SellableQuantity`, `BuyAveragePrice`, `Ltp`, `Pnl`
- **Fund & Margin Limits**:
  - Endpoint: `GET <BaseURL>/quick/user/limits`
  - Normalized to `domain.MarginSyncParam`:
    - `AvailableCash`, `AvailableMargin`, `Utilised`, `NetQty`, `TotalMtm`, `Status = "online"`

### 2.3 Account State Transitions
- **On Successful Sync**:
  - `accounts.status = "active"`
  - `accounts.auth_status = "authenticated"`
  - Clear `auth_error`
- **On Auth Failure (Permanent)**:
  - `accounts.auth_status = "unauthenticated"`
  - `accounts.status = "error"`
  - Set `auth_error = SanitizeAuthError(err.Error())`

### 2.4 On-Demand Headless Re-Authentication
If `SyncAccountPortfolio` encounters an empty or expired access token, or receives an HTTP 401 error from Kotak Neo:
1. Automatically invokes `HeadlessLogin` using stored encrypted credentials and newly generated TOTP.
2. Updates `accounts.access_token` and `token_expires_at` in the database.
3. Fires `OnTokenRefreshed` callback.
4. Retries the portfolio and margin sync with the fresh session.

### 2.5 Debounced Syncer Integration
In `cmd/server/main.go`, register `kotakSyncer` inside `multiBrokerSyncer` wrapped with `kite.NewDebouncedSyncer` (500ms debounce window) to prevent polling stampedes during market hours.

---

## 3. Test-Driven Development (TDD) Test Matrix

| Case Type | Test Name | Scenario / Setup | Expected Outcome |
|---|---|---|---|
| **Positive** | `TestSyncAccountPortfolio_ValidToken` | Account has active session; mock server returns positions, holdings, and limits. | Persists all rows to DB; updates `accounts.status = "active"` and `auth_status = "authenticated"`. |
| **Positive** | `TestSyncAccountPortfolio_ExpiredToken_PerformsLogin` | Token expired; mock server accepts login and returns fresh session. | Executes headless login, updates session in DB, successfully syncs portfolio. |
| **Positive** | `TestSyncAccountPortfolio_TokenRejected_ReloginsAndRetries` | First GET returns HTTP 401; syncer re-logins and retries. | Transparently recovers from upstream 401 without user intervention. |
| **Negative** | `TestSyncAccountPortfolio_MissingCredentials` | Token expired and no stored password/TOTP present. | Returns error, sets `accounts.status = "error"`. |
| **Negative** | `TestSyncAccountPortfolio_InvalidMPIN` | Re-login fails with invalid MPIN. | Records sanitized `auth_error`, marks `auth_status = "unauthenticated"`. |
| **Edge Case** | `TestSyncAccountPortfolio_ZeroPositions` | User has no open positions or holdings. | Syncs empty arrays cleanly, updates cash margins without error. |
| **Edge Case** | `TestSyncAccountPortfolio_DebounceCoalescing` | Multiple sync requests dispatched within 500ms. | Coalesced into a single broker fetch. |
