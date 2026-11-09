# Phase 4: Master Order Stream (WebSocket)

This document specifies Phase 4 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Establish real-time WebSocket order stream ingestion for master trading accounts using Angel One's interactive order socket (`orderstream.OrderStream`), filtering non-terminal statuses, resolving tokens to canonical symbols, and publishing execution events to `queue.Publisher[domain.MasterFill]`.

---

## 2. Order Stream Architecture (`ws/ticker.go`)

### 2.1 WebSocket Lifecycle & Connection
- **Stream Client**: Wraps `goangelone/orderstream.OrderStream`, connecting to `wss://smartapisocket.angelone.in/websocket-order-update` using the master account's active JWT access token.
- **Forward Proxy Egress**: Configured via `orderstream.WithOrderStreamProxyURL(proxyURL)` to ensure WebSocket handshakes route through assigned static proxy IPs.
- **Automatic Reconnection**: Re-establishes connection on unexpected disconnects with exponential backoff.
- **Catch-Up Reconciliation Hook**: Upon successful reconnection, triggers the registered `catchUp(ctx)` hook to poll `GetOrders` via REST and backfill any executions missed during socket downtime.

### 2.2 Event Processing Pipeline
```
orderstream.OrderUpdate
  ├── Filter: Drop non-terminal statuses (keep only COMPLETE, REJECTED, CANCELLED)
  ├── Resolve: instrument.Cache.ResolveToSymbol(exchange, symboltoken) -> (Exchange, Tradingsymbol)
  ├── Normalize: Product (DELIVERY -> CNC), Transaction Type, Order Type
  ├── Construct: domain.MasterFill
  └── Publish: queue.Publisher[domain.MasterFill] -> Copy-Trade Engine
```

### 2.3 Goroutine Leak Prevention & Seam Conformance
- Satisfies `ticker.Ticker` interface (`Start(ctx)`, `Stop() error`).
- `Stop()` cleanly cancels context, closes WebSocket, and waits for background read goroutines to terminate.
- Verified in unit tests with `go.uber.org/goleak`.

---

## 3. Implementation Details

### 3.1 New Files
- `internal/angelone/ws/ticker.go`: `MasterTicker`, `TickerFactory` (implementing `ticker.Factory`), normalizers, and reverse symbol mapping.
- `internal/angelone/ws/ticker_test.go`: Pure TDD unit test suite testing streaming, filtering, reconnection, and leak-free shutdown.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestMasterTicker_TerminalExecution` | Incoming `COMPLETE` status update | Successfully parses, resolves symbol, and publishes `domain.MasterFill`. |
| `TestMasterTicker_IgnoreNonTerminal` | Incoming `OPEN` or `TRIGGER PENDING` update | Dropped immediately; 0 messages published to queue. |
| `TestMasterTicker_ReverseSymbolResolution` | Payload has `symboltoken: "3045"`, `exchange: "NSE"` | Resolves canonical `"NSE:SBIN-EQ"` via `instrument.Cache`. |
| `TestMasterTicker_ReconnectCatchUp` | WebSocket disconnects and reconnects | Fires registered `catchUp` reconciliation hook. |
| `TestMasterTicker_ProxyDialing` | Configured proxy URL | Dialer directs WebSocket handshake through proxy. |
| `TestMasterTicker_CleanShutdown` | `Stop()` called | Context canceled, socket closed, zero goroutine leaks (`goleak.VerifyNone`). |
