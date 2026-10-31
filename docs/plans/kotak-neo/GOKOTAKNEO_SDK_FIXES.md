# Kotak Neo SDK (`gokotakneo`): Issues & Fix Specification

This document details critical issues, production gaps, and proposed fixes for `github.com/DragoHex/gokotakneo` (located at `../EnvoyTradeSDKs/gokotakneo`).

---

## 1. Summary of Identified Issues

| Severity | Issue | Affected File(s) | Impact on Production Integration |
|---|---|---|---|
| **P0 - Critical** | **Aggressive 6-Second Inactivity Watchdog** | `orderstream/stream.go:159` | Kills the WebSocket connection every 6 seconds on idle accounts, causing endless reconnect loops. |
| **P0 - Critical** | **Missing Core Execution Fields in `OrderUpdate`** | `orderstream/types.go:10` | WebSocket order updates omit `trnsTp` (Buy/Sell), `prod` (CNC/MIS/NRML), `exSeg`, `prcTp`, `qty`, `tok`, and `exOrdId`. EnvoyTrade cannot identify what was traded. |
| **P1 - High** | **`SetSessionTokens` Does Not Set `authenticated = true`** | `client.go:64` | Restoring session credentials from database leaves `client.IsAuthenticated() == false`. |
| **P1 - High** | **Missing `Close()` Method on `orderstream.Client`** | `orderstream/stream.go` | No way for callers to deterministically close the WebSocket; cancelling context leaves blocking socket reads hanging. |
| **P2 - Medium** | **`models.BrokerTime` Fails on Standard Kotak Date Format** | `models/time.go:14` | Rejects Kotak's standard `"02-Jan-2006 15:04:05"` (e.g. `"16-Feb-2026 12:29:31"`) format, crashing JSON deserialization on order history. |
| **P2 - Medium** | **Missing `GetOrderBook` Method Documented in README** | `reports.go:14`, `README.md:154` | README examples call `client.GetOrderBook(ctx)`, but only `client.GetOrders(ctx)` exists in code. |
| **P3 - Low** | **No Proxy Support in `orderstream.New` Constructor** | `orderstream/stream.go:44` | Order stream proxy can only be configured via post-construction setter `SetProxyURL`. |

---

## 2. Deep Dive & Proposed Fixes

---

### Issue 1: Aggressive 6-Second Inactivity Watchdog Disconnect

#### Root Cause
In `orderstream/stream.go`:
```go
// Watchdog Goroutine
wg.Add(1)
go func() {
    defer wg.Done()
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-readCtx.Done():
            return
        case <-ticker.C:
            last := c.lastActivity.Load().(time.Time)
            if time.Since(last) > 6*time.Second {
                c.conn.Close()
                return
            }
        }
    }
}()
```
`lastActivity` is only updated when an incoming message is parsed. In live algorithmic trading, orders do not execute every 6 seconds. An idle account with no orders for 6 seconds triggers `c.conn.Close()`, entering an infinite reconnection loop. Furthermore, WebSocket ping control frames (Opcode 9) handled by `gorilla/websocket` do not return from `ReadMessage()`, so `lastActivity` is never updated on ping.

#### Proposed Fix
1. Register `conn.SetPingHandler` to update `lastActivity` and respond with Pong.
2. Send periodic Ping keep-alive frames from client to server (every 30 seconds).
3. Increase the inactivity watchdog threshold from **6 seconds** to **90 seconds**.

```go
// In connectAndRead:
conn.SetPingHandler(func(appData string) error {
    c.lastActivity.Store(time.Now())
    return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
})

// Increase watchdog threshold to 90 seconds
if time.Since(last) > 90*time.Second {
    c.conn.Close()
    return
}
```

---

### Issue 2: Missing Core Execution Fields in `orderstream.OrderUpdate`

#### Root Cause
In `orderstream/types.go`:
```go
type OrderUpdate struct {
    OrderNumber     string          `json:"nOrdNo"`
    TradingSymbol   string          `json:"trdSym"`
    Status          string          `json:"ordSt"`
    FilledQuantity  int             `json:"fldQty"`
    AveragePrice    decimal.Decimal `json:"avgPrc"`
    RejectionReason string          `json:"rejRsn,omitempty"`
}
```
Kotak Neo's actual real-time WebSocket order stream payload contains:
```json
{
  "type": "order",
  "data": {
    "nOrdNo": "260216000308219",
    "ordSt": "complete",
    "avgPrc": "35.88",
    "qty": 1,
    "fldQty": 1,
    "unFldSz": 0,
    "sym": "ITBEES",
    "trnsTp": "B",
    "prcTp": "MKT",
    "prod": "NRML",
    "exSeg": "nse_cm",
    "trdSym": "ITBEES",
    "tok": "11536",
    "ordDtTm": "16-Feb-2026 12:29:31",
    "exOrdId": "1100000049435826"
  }
}
```
Because `trnsTp` (Transaction Type: Buy/Sell) and `prod` (Product: CNC/MIS/NRML) are missing, consumers cannot construct a valid copy-trade `MasterFill`.

#### Proposed Fix
Update `OrderUpdate` in `orderstream/types.go`:
```go
type OrderUpdate struct {
    OrderNumber      string          `json:"nOrdNo"`
    ExchangeOrderID  string          `json:"exOrdId"`
    TradingSymbol    string          `json:"trdSym"`
    Symbol           string          `json:"sym"`
    InstrumentToken  string          `json:"tok"`
    ExchangeSegment  string          `json:"exSeg"`
    TransactionType  string          `json:"trnsTp"`
    ProductCode      string          `json:"prod"`
    PriceType        string          `json:"prcTp"`
    Status           string          `json:"ordSt"`
    Quantity         int             `json:"qty"`
    FilledQuantity   int             `json:"fldQty"`
    UnfilledQuantity int             `json:"unFldSz"`
    AveragePrice     decimal.Decimal `json:"avgPrc"`
    OrderTimestamp   string          `json:"ordDtTm"`
    RejectionReason  string          `json:"rejRsn,omitempty"`
}
```

---

### Issue 3: Missing `Close()` Method on `orderstream.Client`

#### Root Cause
Callers cannot gracefully shut down an `orderstream.Client`. When an account token is refreshed or server shuts down, the client goroutines remain stuck until the TCP connection drops.

#### Proposed Fix
Add thread-safe `Close() error` to `orderstream.Client`:
```go
// Close terminates the order stream connection and watchdog goroutine.
func (c *Client) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.autoReconnect = false
    if c.cancel != nil {
        c.cancel()
    }
    if c.conn != nil {
        return c.conn.Close()
    }
    return nil
}
```

---

### Issue 4: `SetSessionTokens` Does Not Update Authenticated State

#### Root Cause
In `client.go`:
```go
func (c *Client) SetSessionTokens(tradeToken, tradeSid string) {
    c.transport.SetSessionHeaders(tradeToken, tradeSid)
}
```
`c.authenticated` remains `false`. Restoring a valid session from the database causes `c.IsAuthenticated()` to return `false`.

#### Proposed Fix
```go
func (c *Client) SetSessionTokens(tradeToken, tradeSid string) {
    c.transport.SetSessionHeaders(tradeToken, tradeSid)
    c.mu.Lock()
    c.authenticated = strings.TrimSpace(tradeToken) != "" && strings.TrimSpace(tradeSid) != ""
    c.mu.Unlock()
}
```

---

### Issue 5: `models.BrokerTime` Missing Standard Kotak Date Formats

#### Root Cause
In `models/time.go`:
```go
var acceptedFormats = []string{
    "2006/01/02 15:04:05",
    "2006-01-02 15:04:05",
    "2006-01-02T15:04:05Z07:00",
    "2006-01-02T15:04:05",
    "2006-01-02",
}
```
Kotak Neo timestamps often arrive as `"16-Feb-2026 12:29:31"` or `"02-Jan-2006 15:04:05"`. Deserializing causes an unmarshal error.

#### Proposed Fix
Add standard Kotak formats to `acceptedFormats`:
```go
var acceptedFormats = []string{
    "02-Jan-2006 15:04:05",
    "02-Jan-2006",
    "02/01/2006 15:04:05",
    "02/01/2006",
    "2006/01/02 15:04:05",
    "2006-01-02 15:04:05",
    "2006-01-02T15:04:05Z07:00",
    "2006-01-02T15:04:05",
    "2006-01-02",
}
```

---

### Issue 6: Missing `GetOrderBook` Method in `reports.go`

#### Root Cause
The `README.md` instructs users to call:
```go
orders, err := client.GetOrderBook(ctx)
```
However, the code in `reports.go` only defines `GetOrders(ctx)`.

#### Proposed Fix
Add `GetOrderBook` as an alias in `reports.go`:
```go
// GetOrderBook retrieves all orders for the current trading day (alias for GetOrders).
func (c *Client) GetOrderBook(ctx context.Context) ([]models.Order, error) {
    return c.GetOrders(ctx)
}
```

---

### Issue 7: `orderstream.Client` Proxy Constructor Support

#### Root Cause
`orderstream.New` does not accept proxy configuration. It requires a subsequent call to `SetProxyURL`.

#### Proposed Fix
Add Option pattern or support `WithStreamProxyConfig(cfg ProxyConfig)`:
```go
type StreamOption func(*Client)

func WithStreamProxy(proxyURL *url.URL) StreamOption {
    return func(c *Client) {
        c.proxyURL = proxyURL
    }
}
```

---

## 3. Recommended Execution Strategy

Because `../EnvoyTradeSDKs/gokotakneo` is a sibling repository in your workspace (`git@github.com:DragoHex/gokotakneo.git`), we can:
1. Apply these fixes directly in `../EnvoyTradeSDKs/gokotakneo`.
2. Add comprehensive unit tests in `../EnvoyTradeSDKs/gokotakneo` to verify all edge cases.
3. Commit and tag a new release (e.g., `v0.2.0`).
4. Import `v0.2.0` in EnvoyTrade's `go.mod`.
