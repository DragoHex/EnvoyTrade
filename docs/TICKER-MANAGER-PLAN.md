# Multi-Master TickerManager Architecture & Implementation Plan

## Goal Description

In EnvoyTrade, the WebSocket ticker provides ultra-low latency (50–100ms faster than webhook postbacks) for detecting master orders and triggering fan-out copy-trading.

The existing implementation was built under a single-master assumption:
1. **Static lifecycle**: Tickers only launch at server boot. If a master logs in, refreshes an access token, or is created/activated later, no WebSocket ticker is started.
2. **Vendored SDK global dialer mutation**: Even in the latest `github.com/zerodha/gokiteconnect/v4` (`v4.4.3`), `kiteticker.Ticker` mutates package-level `websocket.DefaultDialer`, causing concurrency issues across multiple tickers dialing simultaneously.
3. **Reconnection storm**: When any ticker reconnects, it triggers `poller.RunOnce()`, executing a heavy full reconciliation across *all* masters and followers in the system.

Per updated specifications:
- **`gokiteconnect` Dependency**: `github.com/zerodha/gokiteconnect/v4` is upgraded to the latest version (`v4.4.3`) in `go.mod`. The `./gokiteconnect` folder in the repository root is kept strictly for reference.
- **WebSockets do not need any proxy**: Direct connections to Zerodha (`wss://ws.kite.trade`) ensure the lowest possible execution latency and maximum connection stability.

This plan introduces the **`TickerManager`** service, an isolated direct **`WebSocketConn`** client, per-master scoped reconciliation in `recon.Poller`, and dynamic lifecycle hooks triggered on login, token refresh, and account updates.

---

## User Review Required

> [!IMPORTANT]
> **gokiteconnect v4.4.3 Upgrade**:
> `go.mod` is updated to `github.com/zerodha/gokiteconnect/v4 v4.4.3`. All unit tests (`kite`, `callback`, `fake`, `domain`, `auth`, `worker`, `httpapi`, `listener`, `recon`) build and pass cleanly against `v4.4.3`.

> [!IMPORTANT]
> **Direct Connections for WebSockets**:
> WebSockets connect directly to `wss://ws.kite.trade` without routing through the REST proxy IPs. This eliminates proxy latency (saving 10–50ms) and prevents idle drops from proxy servers.

> [!NOTE]
> **Postback remains the universal safety net**:
> Even during network disconnects or if a master runs without WebSocket credentials, webhook postback (`POST /broker-callback`) and the periodic reconciliation poller continue to backfill and guarantee zero missed fills.

---

## Architecture Overview

```
                      ┌────────────────────────────────────────────────────────┐
                      │                     HTTP API                           │
                      │  - Account Login / Headless Token Refresh              │
                      │  - Account Active / Inactive Toggle                    │
                      │  - Account Delete                                      │
                      └──────────────────────────┬─────────────────────────────┘
                                                 │ Notify (Start/Stop/Restart)
                                                 ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                            TickerManager                                     │
│                                                                              │
│  activeTickers: map[uuid.UUID]*ManagedTicker                                 │
│                                                                              │
│  ┌────────────────────────┐                    ┌───────────────────────────┐ │
│  │     Master A Ticker    │                    │      Master B Ticker      │ │
│  │ ┌────────────────────┐ │                    │ ┌───────────────────────┐ │ │
│  │ │ WebSocketConn      │ │                    │ │ WebSocketConn         │ │ │
│  │ │ (Direct Connection │ │                    │ │ (Direct Connection    │ │ │
│  │ │  Isolated Dialer)  │ │                    │ │  Isolated Dialer)     │ │ │
│  │ └─────────┬──────────┘ │                    │ └───────────┬───────────┘ │ │
│  │           │            │                    │             │             │ │
│  │           ▼            │                    │             ▼             │ │
│  │  OnConnect:            │                    │  OnConnect:               │ │
│  │  ReconcileMaster(A)    │                    │  ReconcileMaster(B)       │ │
│  └───────────┬────────────┘                    └─────────────┬─────────────┘ │
└──────────────┼───────────────────────────────────────────────┼───────────────┘
               │                                               │
               ▼                                               ▼
         MasterFill (Master A)                           MasterFill (Master B)
               └───────────────────────┬───────────────────────┘
                                       ▼
                       masterFillQueue (memchan.Queue)
                                       │
                                       ▼
                              MasterFillConsumer
                                       │
                                       ▼
                          engine.HandleMasterFill
```

---

## Proposed Changes

### Component 1: Dependency & Documentation (`go.mod`, `CLAUDE.md`)

#### [MODIFY] [go.mod](file:///Users/msp/MSP/Projects/EnvoyTrade/go.mod)
- Upgrade `github.com/zerodha/gokiteconnect/v4` to `v4.4.3`.
- Run `go mod tidy` to clean up indirect dependencies.

#### [MODIFY] [CLAUDE.md](file:///Users/msp/MSP/Projects/EnvoyTrade/CLAUDE.md)
- Clarify that `./gokiteconnect` is kept for reference only and that `github.com/zerodha/gokiteconnect/v4 v4.4.3` is consumed directly from Go modules.

---

### Component 2: Per-Master Scoped Reconciliation (`internal/recon`)

#### [MODIFY] [reconciler.go](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/recon/reconciler.go)
- Extract single-master polling from `RunOnce` into `ReconcileMaster(ctx context.Context, masterID uuid.UUID) error`.
- Refactor `RunOnce` to iterate over active masters and delegate to `ReconcileMaster(ctx, m.ID)`.
- When a master ticker reconnects, its catch-up hook calls `ReconcileMaster(ctx, masterID)`, eliminating reconnection storms for other masters.

```go
// ReconcileMaster checks and backfills recent fills for a single master account.
func (p *Poller) ReconcileMaster(ctx context.Context, masterID uuid.UUID) error {
    now := time.Now()
    windowStart := now.Add(-p.cfg.PollingWindow)

    orders, err := p.masterReader.GetMasterOrders(ctx, masterID)
    if err != nil {
        p.alerter.Alert(ctx, "failed to read master orders", map[string]any{
            "master_id": masterID,
            "error":     err.Error(),
        })
        return err
    }

    for _, o := range orders {
        if !callback.IsTerminal(o.Status) {
            continue
        }
        if !o.OrderTimestamp.Time.IsZero() && o.OrderTimestamp.Time.Before(windowStart) {
            continue
        }
        fill := callback.ToMasterFill(o, masterID)
        if err := p.masterConsumer.Handle(ctx, fill); err != nil {
            p.alerter.Alert(ctx, "failed to handle master fill from recon", map[string]any{
                "master_id":       masterID,
                "broker_order_id": o.OrderID,
                "error":           err.Error(),
            })
        }
    }
    return nil
}
```

---

### Component 3: Isolated Direct WebSocket Connection (`internal/kite/ws`)

#### [NEW] [conn.go](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/kite/ws/conn.go)
- Implements `callback.Conn` and `callback.ConnectNotifier`.
- Uses an independent `*websocket.Dialer{}` instance per connection (never touching or mutating `websocket.DefaultDialer`).
- Connects directly to `wss://ws.kite.trade?api_key=...&access_token=...` (no proxy).
- Handles ping/pong keep-alives and parses incoming text JSON frames: `{"type":"order","data":<kiteconnect.Order>}`.
- Implements auto-reconnect with exponential backoff on disconnect.
- Deterministic shutdown upon `Close()` or context cancellation.

```go
package ws

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/url"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

type Config struct {
    APIKey      string
    AccessToken string
    RootURL     string // defaults to "wss://ws.kite.trade"
}

type Conn struct {
    cfg           Config
    orderCallback func(kiteconnect.Order)
    connectHook   func()
    logger        *slog.Logger
    mu            sync.Mutex
    wsConn        *websocket.Conn
    closed        bool
    closeCh       chan struct{}
}

func NewConn(cfg Config, logger *slog.Logger) *Conn { ... }
func (c *Conn) OnOrderUpdate(f func(kiteconnect.Order)) { c.orderCallback = f }
func (c *Conn) OnConnect(f func()) { c.connectHook = f }
func (c *Conn) ServeWithContext(ctx context.Context) { ... }
func (c *Conn) Close() error { ... }
```

---

### Component 4: TickerManager Service (`internal/kite`)

#### [NEW] [ticker_manager.go](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/kite/ticker_manager.go)
- Maintains a thread-safe registry `map[uuid.UUID]*managedTicker`.
- `managedTicker` bundles the `*callback.MasterTicker`, `*ws.Conn`, context cancellation function, and status.
- Core methods:
  - `StartMaster(ctx context.Context, masterID uuid.UUID) error`:
    1. Checks if already running; if so, returns early (idempotent).
    2. Fetches credentials from store: `store.AccountAuthInfo(ctx, masterID)`.
    3. Checks active flag: if master is inactive or token is missing/expired, skips start.
    4. Creates `ws.NewConn(cfg, logger)` and `callback.NewMasterTicker(conn, masterID, queue, logger)`.
    5. Registers `poller.ReconcileMaster(ctx, masterID)` as catch-up hook on reconnect.
    6. Starts background goroutine and stores in registry.
  - `StopMaster(masterID uuid.UUID) error`:
    1. Looks up ticker; cancels its context and closes the connection.
    2. Removes from registry.
  - `RestartMaster(ctx context.Context, masterID uuid.UUID) error`:
    Performs atomic `StopMaster` then `StartMaster`.
  - `SyncActiveMasters(ctx context.Context) error`:
    Queries all active masters from DB, starts tickers for any unstarted ones, and stops tickers for any deactivated ones.
  - `Shutdown()`:
    Stops all active tickers cleanly on server shutdown.

```go
type TickerStore interface {
    Groups(ctx context.Context) ([]domain.Group, error)
    AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
    MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error)
}

type Reconciler interface {
    ReconcileMaster(ctx context.Context, masterID uuid.UUID) error
}

type TickerManager struct {
    store      TickerStore
    queue      queue.Publisher[domain.MasterFill]
    reconciler Reconciler
    logger     *slog.Logger
    mu         sync.RWMutex
    tickers    map[uuid.UUID]*managedTicker
}
```

---

### Component 5: Integration with Lifecycle Events

#### [MODIFY] [sync.go](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/kite/sync.go)
- Add optional hook: `OnTokenRefreshed func(ctx context.Context, accountID uuid.UUID)` to `PortfolioSyncer`.
- When headless login succeeds and a new access token is persisted in `SyncAccountPortfolio`, call `OnTokenRefreshed(ctx, accountID)`.
- The hook triggers `tickerManager.RestartMaster(ctx, accountID)` so the WebSocket immediately spins up with the fresh token.

#### [MODIFY] [accounts.go](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/httpapi/accounts.go)
- When toggling master active status via `SetAccountActive(id, active)`:
  - If `active == true`, call `tickerManager.StartMaster(ctx, id)`.
  - If `active == false`, call `tickerManager.StopMaster(id)`.
- When deleting an account via `DELETE /api/v1/accounts/{id}`, call `tickerManager.StopMaster(id)`.

#### [MODIFY] [main.go](file:///Users/msp/MSP/Projects/EnvoyTrade/cmd/server/main.go)
- Replace static `startMasterTicker` function with `tickerManager := kite.NewTickerManager(store, masterFillQueue, poller, logger)`.
- On startup, invoke `tickerManager.SyncActiveMasters(ctx)`.
- Defer `tickerManager.Shutdown()`.
- Pass `tickerManager` into `httpapi.NewRouter` and `PortfolioSyncer`.

---

## Verification Plan

### Automated Tests
1. **Dependency Verification**:
   - `go build ./...`
   - `go test -v ./internal/kite/...`
   - Verify zero compiler warnings or broken types against `gokiteconnect/v4 v4.4.3`.
2. **Scoped Reconciler Unit Test**:
   - `go test -v ./internal/recon/... -run TestReconcileMaster`
   - Assert `ReconcileMaster` only queries the target master's orders and does not touch other accounts.
3. **WebSocket Connection Unit & Fake Server Tests**:
   - `go test -v ./internal/kite/ws/...`
   - Use `httptest.NewServer` with gorilla WebSocket upgrader to test connection, ping/pong, JSON order frame decoding, reconnect behavior, and context cancellation.
4. **TickerManager Concurrency & Lifecycle Tests**:
   - `go test -v ./internal/kite/... -run TestTickerManager`
   - Test starting multiple masters simultaneously, stopping individual masters, duplicate start idempotency, token restart, and leak-free `Shutdown()` using `goleak`.
5. **Integration Suite**:
   - `go test -tags integration ./...`

### Manual Verification
1. Start server with 2 configured master accounts.
2. Verify both tickers log successful direct connections to Kite.
3. Simulate disconnect on Master 1; verify only Master 1 triggers a scoped catch-up pull, while Master 2 remains unaffected.
4. Toggle Master 1 to inactive in the dashboard; verify Master 1's ticker shuts down while Master 2 remains running.
5. Perform login or trigger position sync on Master 1; verify Master 1's ticker starts automatically.
