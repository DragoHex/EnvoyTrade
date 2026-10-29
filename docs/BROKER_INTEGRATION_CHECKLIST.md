# Broker Integration Checklist

This document defines the mandatory architectural checklist when integrating a new broker (e.g. Dhan, Zerodha Kite, AngelOne, Groww, Kotak Neo, Symphony) or extending an existing broker adapter in EnvoyTrade.

Every broker integration must follow the consumer-first seam rule (PLAN.md §1), enforce strict follower isolation, utilize dedicated static proxy egress, and adhere to deterministic idempotency and money math standards.

---

## 1. Instrument Master Lifecycle (`internal/instrument`)

Every broker maintains unique symbol naming conventions, exchange segment identifiers, security tokens, and contract lot sizes. EnvoyTrade's copy-trade engine strictly requires canonical lot sizes and broker-native symbols to prevent mis-sizing and placement rejections.

### A. Fetch Instruments
- [ ] **Stream Without Heap Bloat**: Implement streaming download and row-by-row parsing (e.g. `csv.Reader` or streaming JSON decoder) in `internal/instrument/syncer.go`. Never load unparsed raw files entirely into memory.
- [ ] **Strict Failure Isolation**: If the broker's endpoint times out or returns HTTP 5xx, fail gracefully without mutating or clearing existing database records.
- [ ] **Schedule Integration**: Register the broker's sync method in `Syncer.StartDailyRoutine` (scheduled at `05:00:00 AM IST` daily).

### B. Save to Database (`instruments` Table)
- [ ] **Broker Tagging**: Every inserted row must have `broker = '<broker_name>'` (e.g. `'kite'`, `'dhan'`).
- [ ] **Conflict Resolution & Unique Keys**: Ensure uniqueness on `(broker, exchange, tradingsymbol)`. If using integer tokens, offset tokens (e.g. `10^12 + security_id`) to prevent collisions across brokers.
- [ ] **Transactional Pruning (All-or-Nothing Guard)**:
  - Prune existing records (`DELETE FROM instruments WHERE broker = $1`) **strictly only after 100% of the feed is successfully downloaded and parsed**.
  - Execute prune and batch inserts within a transaction.
- [ ] **Chunked Batch Insertion**: Insert parsed records in chunks of 5,000 using PostgreSQL parameterized `UNNEST` queries (`store.BatchInsertInstruments`) to avoid memory spikes and query parameter limits.
- [ ] **Storage & Planner Reclaim**: Execute `VACUUM (ANALYZE) instruments` immediately after batch insertion to reclaim dead tuples and refresh planner statistics.

### C. Fetch and Save to In-Memory Cache
- [ ] **Single Database Query**: Query active F&O derivatives for the current calendar month and next 3 weeks rolling via `store.ActiveFOInstrumentsForCache(ctx)`. Plain equities without expiry must **not** be pre-loaded into the cache.
- [ ] **Pre-allocated 10,000 Capacity**:
  - Auto-load records into `instrument.Cache` on server startup and immediately after the 05:00 AM daily sync.
  - Reset existing entries using Go 1.21+ `clear(map)` to retain backing memory allocations without garbage collection churn.
  - Never allow unbounded map growth during market hours; enforce the 10,000 entry ceiling with DB fallback on miss.
- [ ] **Forward Translation (`ResolveToSecurityID`)**:
  - Cache broker-native parameters (e.g. `SecurityID`, `ExchangeSegment`, `LotSize`) so the broker adapter can map them in sub-microsecond time during order placement.
- [ ] **Reverse Translation (`ResolveToSymbol`)**:
  - Maintain reverse index `(exchange_segment:security_id -> exchange:tradingsymbol)` for incoming order updates, WebSocket ticker feeds, and postback webhooks.

---

## 2. Credential, Auth & Token Lifecycle (`internal/crypto`, `internal/<broker>`)

- [ ] **Credential Encryption at Rest**:
  - Passwords and TOTP secret keys must be encrypted with AES-GCM (`internal/crypto`) before persistence to `accounts.encrypted_password` and `accounts.encrypted_totp_secret`.
  - Plaintext credentials must never be written to logs or database tables.
- [ ] **Token Validation & Claim Parsing**:
  - **JWT Brokers (e.g. Dhan)**: Implement token parser (`Parse<Broker>JWT`) to extract claims (`client_id`, `exp`, `iss`). Assert that the token's client ID matches the account's `broker_user_id` to prevent credential misassignment. Extract exact token expiration time.
  - **Headless Login Brokers (e.g. Kite)**: Implement automated 5-step headless login (`HeadlessLogin`) utilizing stored encrypted password and TOTP secret to exchange request tokens for session access tokens.
- [ ] **Token Renewal & Scheduled Daemon**:
  - Schedule in-process token renewal daemon (e.g. Dhan's 00:15 & 06:00 IST renewer, or morning headless login).
  - Persist refreshed tokens via `SetAccountAccessToken(ctx, accountID, token, expiresAt, "authenticated", "")`.
- [ ] **Token Refresh Callbacks (`OnTokenRefreshed`)**:
  - **Evict Cached Broker**: Call `accountPool.Evict(accountID)` so next resolution builds a fresh broker client with new token.
  - **Master Accounts**: Trigger dynamic WebSocket ticker restart via `tickerManager.RestartMaster(ctx, masterID)`.
  - **Follower Accounts**: Re-register the follower's dedicated worker goroutine with new broker client via `followerRegistrar.RegisterFollower(ctx, followerID)`.
- [ ] **Auth Expiry & Failure State**:
  - If a token is expired or renewal fails with non-retryable 401:
    - Mark account `accounts.auth_status = "unauthenticated"`, `accounts.auth_error = err.Error()`.
    - Set account status to `error` (`domain.AccountStatusError`).

---

## 3. Broker Adapter & Execution Seam (`internal/broker`, `internal/<broker>`)

- [ ] **Consumer Seam Implementation**: Implement `broker.Broker` in `internal/<broker>`:
  - `PlaceOrder(ctx, variety, params)`
  - `GetPositions(ctx)`
  - `GetOpenOrders(ctx)`
  - `GetOrders(ctx)`
  - `GetOrderHistory(ctx, orderID)`
  - `CancelOrder(ctx, variety, orderID)`
- [ ] **Broker Factory & Registry**:
  - Implement `broker.FactoryFunc` accepting `broker.BrokerAccount` and register with `broker.Registry` in `cmd/server/main.go` (`brokerRegistry.Register("<broker>", factory)`).
  - Wrap instantiation with `broker.AccountPool` for Tier 2 caching and connection reuse.
- [ ] **Strict Static Proxy Egress**:
  - Follower orders must egress strictly through their assigned static proxy IP (`ProxyConfig`).
  - Return `domain.ErrUnproxiedNotAllowed` if unproxied where required.
  - Reuse persistent transports via `broker.RESTClientFor(proxyCfg)` / `broker.DefaultTransportPool` to keep TLS sessions warm.
- [ ] **Domain Sentinel Error Mapping**:
  - Map broker HTTP/JSON error responses to domain sentinels:
    - `domain.ErrAuthExpired`: HTTP 401, expired session, invalid JWT, DH-901/DH-906.
    - `domain.ErrBrokerUnreachable`: HTTP 502/503/504, connection timeouts, network connection refused.
    - `domain.ErrNotFound`: Unknown order ID or symbol.
- [ ] **Broker-Native Parameter Mapping**:
  - Map canonical `OrderParams` (Product: `CNC`/`MIS`/`NRML`, OrderType: `MARKET`/`LIMIT`/`SL`/`SL-M`, TransactionType: `BUY`/`SELL`, Tag: `IdempotencyTag`) to broker's native schema.
  - Use `instrument.Cache.ResolveToSecurityID` to resolve native security ID and exchange segment in sub-microsecond time.
- [ ] **Strict Decimal Math**:
  - All prices, trigger prices, quantities, and ratio math must strictly use `shopspring/decimal`. Floating-point math is strictly forbidden.

---

## 4. Master WebSocket Ticker (`internal/ticker`, `internal/<broker>/ws`)

- [ ] **Implement `ticker.Factory`**:
  - Implement `CreateTicker(ctx, masterID, auth, catchUp)` and register with `ticker.Manager` (`tickerManager.RegisterFactory("<broker>", factory)`).
  - Master accounts stream execution updates over WebSocket; strictly 1 ticker per active master account.
- [ ] **Terminal Order Status Filtering**:
  - Drop intermediate statuses (`OPEN`, `TRIGGER_PENDING`, `MODIFY`).
  - Forward only terminal executions (`COMPLETE`, `REJECTED`, `CANCELLED`) to `queue.Publisher[domain.MasterFill]`.
- [ ] **Reverse Symbol Resolution**:
  - Translate broker-native security IDs or integer tokens to canonical `(exchange, tradingsymbol)` using `instrument.Cache.ResolveToSymbol`.
- [ ] **Automatic Reconnect & Catch-Up Hook**:
  - Implement backoff reconnection on WebSocket disconnects.
  - On reconnection, execute `catchUp(ctx)` (`reconciler.ReconcileMaster`) to reconcile missed fills via REST.
- [ ] **Goroutine Leak Prevention**:
  - `Stop()` must cleanly close the WebSocket and cancel background goroutines. Verify with `go.uber.org/goleak` in unit tests.

---

## 5. Postback Webhooks (`internal/callback`, `internal/<broker>/callback`)

- [ ] **Register with Adaptive Router**:
  - Register postback handler with `unifiedcallback.AdaptiveRouter` (`postbackRouter.Register("<broker>", handler)`).
  - Add zero-allocation payload fingerprint signature to `fingerprintPayload` in `internal/callback/router.go` (checking broker-specific JSON keys with `bytes.Contains`).
- [ ] **Signature / Checksum Verification**:
  - Verify payload checksum or secret using the account's stored `api_secret` (resolved via `AccountLookup.AccountByBrokerUserID`).
- [ ] **HTTP Response Contract**:
  - Always respond HTTP 200 OK unless the raw body itself cannot be parsed as JSON (HTTP 400).
  - Unknown accounts, bad checksums, non-terminal states, or full queues must be acknowledged with 200 and dropped to prevent infinite retry loops from broker webhook dispatchers.
- [ ] **Canonical Event Dispatching**:
  - Master executions -> convert to `domain.MasterFill` and publish to `queue.Publisher[domain.MasterFill]`.
  - Follower executions -> convert to `domain.OrderUpdate` and publish to `queue.Publisher[domain.OrderUpdate]`.
  - Use `instrument.Cache.ResolveToSymbol` for reverse translation if the postback carries broker-specific security IDs.

---

## 6. Reconciliation & Drift Poller (`internal/recon`)

- [ ] **REST Order & History Inspection**:
  - Ensure adapter implements `GetOrders(ctx)` and `GetOrderHistory(ctx, orderID)`.
  - The reconciler backstop calls `GetMasterOrders` to backfill missing master fills and `GetFollowerOrderHistory` to resolve stuck follower orders (>2 min).
- [ ] **Fail-Fast on Unreachable Broker**:
  - When evaluating position drift or rebalance, return `domain.ErrBrokerUnreachable` immediately if the broker cannot be reached. Never evaluate drift on partial or stale state.
- [ ] **Position Normalization**:
  - Ensure `GetPositions(ctx)` returns canonical `broker.Position` structs with normalized products (`CNC`, `MIS`, `NRML`) and net quantities.

---

## 7. Portfolio & Margin Synchronization (`internal/<broker>/sync.go`)

- [ ] **Implement `PortfolioSyncer`**:
  - Implement position, holding, and fund balance synchronization for the broker.
  - Normalization:
    - **Positions**: map to `domain.PositionSyncParam` (`Instrument`, `Product`, `Quantity`, `BuyPrice`, `SellPrice`, `Mtm`, `Pnl`).
    - **Holdings**: map to `domain.HoldingSyncParam` (`Instrument`, `SellableQuantity`, `BuyAveragePrice`).
    - **Margins**: map to `domain.MarginSyncParam` (`NetQty`, `TotalMtm`, `RealizedPnl`, `AccountValue`, `AvailableCash`, `AvailableMargin`, `Status = "online"`).
- [ ] **Account State Transition**:
  - On successful sync: update `accounts.status = "active"`, `accounts.auth_status = "authenticated"`, clear `auth_error`, refresh `token_expires_at`.
  - On auth failure: update `accounts.auth_status = "unauthenticated"`, `accounts.status = "error"`.
- [ ] **Debouncing & Rate Limiting**:
  - Register syncer with `multiBrokerSyncer` in `cmd/server/main.go` and wrap with `kite.NewDebouncedSyncer` (500ms window) to prevent burst polling.

---

## 8. Admin API & Tenant Validation (`internal/httpapi`)

- [ ] **Broker Enum Whitelisting**:
  - Add the broker name (lowercase) to supported brokers in `internal/httpapi/accounts.go` (`postAccount`, `patchAccount`).
- [ ] **Quota Enforcement**:
  - Ensure `store.CheckAccountQuota(ctx, userID, broker)` is called before account creation to respect user plan limits.
- [ ] **Proxy IP Whitelist Check**:
  - Validate that the assigned proxy IP is whitelisted for this broker via `proxyIP.MatchesBroker(broker)`.
- [ ] **Sensitive Field Encryption**:
  - Encrypt passwords and TOTP secrets using `internal/crypto` before storing in PostgreSQL.

---

## 9. Scheduled Jobs & Documentation Updates

- [ ] **Update `docs/CRON_AND_PERIODIC_ROUTINES.md`**:
  - Any new in-process routine, cron job, or scheduled timer added for the broker must be documented in [`docs/CRON_AND_PERIODIC_ROUTINES.md`](CRON_AND_PERIODIC_ROUTINES.md) in the exact same change.
