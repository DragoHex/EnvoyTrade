# End-to-End Integration Testing Plan (TestBroker + Seeded Data)

## Goal Description
Build an external, black-box end-to-end integration test suite (`tests/e2e/`) in Go that simulates real-world trading traffic across the EnvoyTrade platform using the seeded test database and the paper trading mock broker (`testbroker`). The test operates outside the application code boundary, driving actions through the `testbroker/sdk` and EnvoyTrade HTTP APIs, observing effects at the broker and trader levels, and performing read-only database assertions to verify internal audit trails (sizing reasons, idempotency tags, order events).

---

## User Review Required

> [!IMPORTANT]
> **Bug Reporting Policy**: Per user instructions, if the integration tests expose any behavioral irregularities, timing race conditions, or discrepancies in EnvoyTrade's engine/workers, **we will NOT adjust the test expectations to accommodate them**. Instead, we will halt, diagnose the root cause, and consult the user before proceeding.

> [!NOTE]
> **Seeded Instruments Requirement**: EnvoyTrade's fan-out engine strictly resolves contract lot sizes from the `instruments` table. To prevent false `ReasonBadInstrument` sizing rejections, `scripts/seed_test_data.sql` will be updated to insert the 12 core test instruments (`CRUDEOIL*`, `NIFTY*`, `RELIANCE`).

---

## Architecture & Test Harness Design

```
                                  ┌───────────────────────────────┐
                                  │      E2E Test Runner          │
                                  │       (`tests/e2e`)           │
                                  └───────┬───────────────┬───────┘
                                          │               │
                     1. Drive Trades &    │               │ 3. Verify Dashboard
                        Manage Mock State │               │    & Group Portfolio
                                          │               │
                                  ┌───────▼───────┐ ┌─────▼──────────┐
                                  │  TestBroker   │ │   EnvoyTrade   │
                                  │ (:8089 REST & │ │  Server (:8080 │
                                  │   WebSocket)  │ │   HTTP API)    │
                                  └───────┬───────┘ └─────▲──────────┘
                                          │               │
                                          │ 2. WS Ticker  │
                                          │    & Postback │
                                          └───────────────┘
                                                  │
                                                  │ 4. Read-Only Audit
                                          ┌───────▼───────┐
                                          │ Postgres DB   │
                                          │ (:5434 seeded)│
                                          └───────────────┘
```

### 1. Hybrid Process Lifecycle
The test harness (`tests/e2e/harness.go`) will:
1. Probe `:8089` (TestBroker) and `:8080` (EnvoyTrade).
2. If both are healthy (`/healthz` or status 200), reuse existing instances.
3. If not running, launch them as background subprocesses (`testbroker` and `cmd/server`) and ensure clean teardown via `t.Cleanup()`.
4. Connect to Postgres (`DATABASE_URL`) with read-only helper functions for verifying audit tables (`master_fills`, `follower_orders`, `order_events`).

### 2. Continuous Trading Day Timeline
Rather than tearing down and resetting state after every assertion, tests will run as a sequential chronological timeline simulating a trading day:
- **Phase 1: Pre-Market & Initialization** (Auth, session check, master/follower health, initial balances)
- **Phase 2: Standard Trading Sessions** (Market orders, limit orders, lot multipliers, multi-master isolation)
- **Phase 3: Real-World Corner Cases & Turbulence** (Sub-lot sizing, duplicate postbacks/ticker spikes, manual execution queues, disabled followers)
- **Phase 4: Error Injection & Failure Handling** (RMS margin rejection, timeout retries)
- **Phase 5: Reconciliation & Day-End** (Missed fill backstop, stuck order resolution, group rebalance, portfolio sync)

---

## Enumerated Test Cases

### Positive & Execution Scenarios
| ID | Test Case | Action / Trigger | Expected Effect (Broker & API) | DB Verification |
|---|---|---|---|---|
| **POS-01** | **Master Market Order Fan-Out** | Master 1 places MARKET BUY (qty 400) on `CRUDEOIL21SEP26`. | Instant fill at LTP on TestBroker. WS & postback dispatched. `FOLLOW01D` (ratio 1.0) receives MARKET BUY qty 400. `FOLLOW01C` (ratio 0.75) receives qty 300. | `sizing_reason = 'OK'`, `dispatch_state = 'dispatched'`, unique idempotency tags created. |
| **POS-02** | **Master Limit Order Fan-Out** | Master 1 places LIMIT SELL (qty 400, price 9650.0) on `CRUDEOIL21SEP26`. | Filled at limit price. Followers receive LIMIT SELL with matching price `9650.0` and scaled quantities. | Sizing reason `OK`, correct limit price recorded. |
| **POS-03** | **Multi-Master & Group Isolation** | Master 2 places BUY on `CRUDEOIL17SEP26C10600` (qty 200). | ONLY Group 2 followers (`FOLLOW02A`, `FOLLOW02B`, `FOLLOW02C`) receive orders. Zero orders placed for Group 1 followers. | Group 2 follow links resolved; Group 1 untouched. |
| **POS-04** | **Partial Lot Sizing Floor** | Master 1 places BUY qty 100 on contract with lot size 100. Follower has ratio 0.5. | Follower receives 0 lots (50 < 100 floored) or if master qty 200, follower gets exactly 100. | `sizing_reason = 'OK'` or `'BELOW_ONE_LOT'`. Math verified to nearest lot. |
| **POS-05** | **Dual-Delivery Idempotency** | Both WS ticker and HTTP postback deliver the same master fill to EnvoyTrade, or TestBroker re-sends postback. | EnvoyTrade acknowledges with HTTP 200. Follower orders are placed **only once**. Zero duplicate follower orders at TestBroker. | Exactly one `follower_orders` row per follower; `domain.ErrDuplicate` handled as silent no-op. |
| **POS-06** | **Follower Status Update & Audit Trail** | Follower order transitions to COMPLETE or CANCELLED on TestBroker. | Follower postback received by `/broker-callback`. | `follower_orders.status` updated to `COMPLETE`. Audit event appended to `order_events`. |
| **POS-07** | **Group Rebalance Action** | Authenticated trader invokes `POST /api/v1/groups/:id/rebalance`. | Group rebalance engine calculates position drift and issues sync orders to align followers with master. | Order events logged, group positions reconciled. |

### Negative & Failure Scenarios
| ID | Test Case | Action / Trigger | Expected Effect (Broker & API) | DB Verification |
|---|---|---|---|---|
| **NEG-01** | **Disabled Follower Exclusion** | Master 1 fills an order. `FOLLOW01B` is configured with `enabled = false`. | `FOLLOW01B` receives **no order** at TestBroker. Other active followers execute normally. | `follower_orders` row created with zero qty or not dispatched; no worker job spawned. |
| **NEG-02** | **Sub-Lot Rejection (`ReasonBelowOneLot`)** | Master 1 buys 1 lot (qty 100). Follower ratio is 0.25 (scaled qty = 25 < 100). | No order sent to TestBroker for this follower. Sizing is visible and audited. | Row persisted in `follower_orders` with qty 0 and `sizing_reason = 'BELOW_ONE_LOT'`. |
| **NEG-03** | **Unregistered / Bad Instrument** | Master trades a symbol not present in `instruments` table (`UNKNOWN_CONTRACT`). | Sizing detects missing lot size. Fan-out gracefully aborts without worker panics. | Row persisted in `follower_orders` with `sizing_reason = 'BAD_INSTRUMENT'`. Server continues operating. |
| **NEG-04** | **Non-Terminal Master Order (Delayed/Manual)** | TestBroker switched to `manual` mode. Master places order (remains `OPEN`). | No follower orders placed while master order is `OPEN`. Later, Admin API calls `ManualFill(orderID)` -> immediate follower fan-out. | No premature fan-out on non-terminal states (`OPEN`, `TRIGGER PENDING`). |
| **NEG-05** | **Follower Margin / RMS Terminal Rejection** | TestBroker rejects follower order with `InsufficientMargin` or invalid credentials. | Worker pool marks placement as terminal error on first attempt (no infinite retry loop). | Order recorded as failed; no duplicate attempts dispatched. |
| **NEG-06** | **Network Timeout & Retry** | TestBroker simulates delayed response / network glitch for a follower. | Worker retries up to 3 times with exponential backoff before succeeding or terminating. | Retry attempts reflected in server logs and worker execution. |
| **NEG-07** | **Reconciliation Poller Backstop** | Master order filled while WS/postback delivery was interrupted. | Recon poller runs periodic check (or manual trigger), detects missing fill from broker order book, and fans out. | Master fill backfilled, follower orders generated, drift resolved. |

---

## Proposed Changes

### 1. Database Seed Updates
#### [MODIFY] `scripts/seed_test_data.sql`
- Add instrument seed block inserting all MCX, NFO, and NSE test contracts (`lot_size`, `tick_size`, `exchange`, `tradingsymbol`).
- Ensure all accounts have matching active credentials in `testbroker_config.json`.

---

### 2. Test Harness & Framework
#### [NEW] `tests/e2e/harness.go`
- `Harness` struct managing configuration (`EnvoyURL`, `TestBrokerURL`, `DatabaseURL`, `AdminAuthToken`).
- Health checking and hybrid subprocess management (`StartServersIfDown`, `StopServers`).
- Pre-authenticated HTTP client for EnvoyTrade API (`POST /api/v1/auth/login`).
- `testbroker/sdk.Client` instance for driving mock broker operations as `MASTER01`, `MASTER02`, and followers.
- Read-only Postgres connection pool for querying verification rows.

---

### 3. End-to-End Test Suite
#### [NEW] `tests/e2e/e2e_test.go`
- Single entrypoint with `//go:build e2e`.
- Subtests organized along the continuous trading day timeline:
  - `TestE2E_TradingDay/Phase1_PreMarket_HealthAndBalances`
  - `TestE2E_TradingDay/Phase2_MarketOrders_FanOut`
  - `TestE2E_TradingDay/Phase3_LimitOrders_PriceMatching`
  - `TestE2E_TradingDay/Phase4_MultiMaster_Isolation`
  - `TestE2E_TradingDay/Phase5_Idempotency_DuplicatePostback`
  - `TestE2E_TradingDay/Phase6_CornerCases_SubLotAndDisabled`
  - `TestE2E_TradingDay/Phase7_ManualExecution_DelayedFills`
  - `TestE2E_TradingDay/Phase8_Rejection_RMSFailureHandling`
  - `TestE2E_TradingDay/Phase9_Reconciliation_PollerBackstop`
  - `TestE2E_TradingDay/Phase10_GroupRebalance_Alignment`

---

### 4. Makefile Integration
#### [MODIFY] `Makefile`
- Add `make e2e` target:
  ```makefile
  e2e:
  	go test -v -tags e2e -count=1 ./tests/e2e/...
  ```

---

## Verification Plan

### Automated Verification
```sh
# 1. Start database and seed instruments & accounts
make db-up
make db-seed

# 2. Run full E2E integration test suite
make e2e

# 3. Verify normal fast unit test suites remain untouched & clean
go test ./...
pnpm -C frontend test --run
```

### Manual Verification
- View TestBroker UI at `http://localhost:8089/ui` to inspect orders during/after test execution.
- View EnvoyTrade Dashboard at `http://localhost:8080/api/v1/accounts` to verify account statuses and MTM.
