# Phase 4: Master Order Stream (`orderstream`)

This document specifies Phase 4 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Establish a dedicated WebSocket connection for each active master account using `gokotakneo/orderstream.Client`. Stream execution events in real time, filter for terminal states, and forward master fills to `queue.Publisher[domain.MasterFill]`. On reconnect, invoke the on-demand catch-up reconciliation hook to backfill any fills that occurred during disconnection.

---

## 2. Technical Design

### 2.1 MasterOrderStream Architecture (`internal/kotak/ws/stream.go`)
Implements `ticker.Factory` (satisfying EnvoyTrade's existing `ticker.Manager` seam) and manages the live socket connection:
```go
type MasterOrderStream struct {
    masterID    uuid.UUID
    ucc         string
    tradeToken  string
    tradeSid    string
    rtURL       string
    queue       queue.Publisher[domain.MasterFill]
    resolver    SymbolResolver
    catchUp     func(context.Context) error
    logger      *slog.Logger
    client      *orderstream.Client
    cancel      context.CancelFunc
    mu          sync.Mutex
}
```

### 2.2 Terminal Status Filtering
Kotak Neo pushes intermediate states (`OPEN`, `TRIGGER PENDING`, `MODIFIED`) and terminal states (`COMPLETE`, `REJECTED`, `CANCELLED`).
EnvoyTrade strictly forwards only terminal executions:
```go
func (t *MasterTicker) handleOrderUpdate(update orderstream.OrderUpdate) {
    if !domain.IsTerminal(update.Status) {
        return // drop non-terminal intermediate updates
    }
    // Reverse translate symbol if required
    fill := domain.MasterFill{
        MasterID:        t.masterID,
        BrokerOrderID:   update.OrderNumber,
        Exchange:        normalizeExchange(update.ExchangeSegment),
        Tradingsymbol:   update.TradingSymbol,
        TransactionType: normalizeTransactionType(update.TransactionType), // "B" -> "BUY", "S" -> "SELL"
        Product:         normalizeProduct(update.ProductCode),            // "CNC", "MIS", "NRML"
        FilledQuantity:  update.FilledQuantity,
        AveragePrice:    update.AveragePrice,
        Status:          update.Status,
        ReceivedAt:      time.Now(),
    }
    _ = t.queue.Publish(context.Background(), fill)
}
```

### 2.3 Automatic Reconnect & Catch-Up Hook
- When the socket reconnects (`OnReconnect` callback):
  - Invokes `t.catchUp(ctx)` (`reconciler.ReconcileMaster`).
  - Calls Kotak Neo REST `GetOrders` to backfill any execution fills that occurred during network downtime.

### 2.4 Clean Shutdown & Leak Prevention
- `Stop()` cancels the internal context and closes the WebSocket connection.
- Verified with `go.uber.org/goleak` to guarantee zero dangling goroutines survive termination.

---

## 3. Test-Driven Development (TDD) Test Matrix

| Case Type | Test Name | Scenario / Setup | Expected Outcome |
|---|---|---|---|
| **Positive** | `TestMasterTicker_TerminalComplete_Publishes` | Incoming WebSocket order event with `ordSt = "COMPLETE"`. | Converted into `domain.MasterFill` and published to `masterFillQueue`. |
| **Positive** | `TestMasterTicker_TerminalRejected_Publishes` | Incoming WebSocket order event with `ordSt = "REJECTED"`. | Published to queue for audit logging. |
| **Positive** | `TestMasterTicker_Reconnect_TriggersCatchUp` | WebSocket connection drops and reconnects. | `catchUp` reconciliation hook is triggered. |
| **Negative** | `TestMasterTicker_NonTerminalDropped` | Incoming event with `ordSt = "OPEN"` or `"TRIGGER PENDING"`. | Dropped immediately; queue publication count is 0. |
| **Edge Case** | `TestMasterTicker_GoroutineLeakPrevention` | Create ticker, start reading mock WebSocket, invoke `Stop()`. | All goroutines terminate cleanly; passes `goleak.VerifyNone`. |
| **Edge Case** | `TestMasterTicker_ZeroQuantityFill` | Terminal order received with `fldQty = 0`. | Handled safely without division-by-zero or panic. |
