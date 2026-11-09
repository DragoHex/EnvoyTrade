# Change Request: Critical Production Fixes for `github.com/DragoHex/gokotakneo`

**Target Repository**: `../EnvoyTradeSDKs/gokotakneo` (`github.com/DragoHex/gokotakneo`)  
**Proposed Release Tag**: `v0.2.0`  
**Priority**: High (Blocks EnvoyTrade Kotak Neo Copy-Trading Integration)

---

## 1. Summary & Motivation

EnvoyTrade is integrating Kotak Neo Trade API as a supported broker for real-time copy-trading. During pre-integration architectural audit of `github.com/DragoHex/gokotakneo` (tag `v0.1.0`), several critical blockers and inconsistencies were identified in the WebSocket order stream, session lifecycle, and JSON deserialization layers.

This request specifies the exact code fixes, unit tests, and verification steps required to bring `gokotakneo` to production readiness under tag `v0.2.0`.

---

## 2. Issues & Proposed Fixes

### Issue 1 (P0): Aggressive 6-Second Inactivity Watchdog Disconnect
* **File**: `orderstream/stream.go`
* **Root Cause**: Line 159 checks `time.Since(last) > 6*time.Second` and forcibly closes the WebSocket connection. `lastActivity` is only updated when a new trade/order message arrives. In live trading, an account with no order activity for 6 seconds is abruptly disconnected, triggering continuous reconnect loops. Additionally, gorilla/websocket handles incoming Ping frames internally without returning them from `ReadMessage()`, so keepalive pings from the server do not reset `lastActivity`.
* **Fix**:
  1. Register `conn.SetPingHandler` to record `lastActivity` on incoming Ping frames and return a Pong.
  2. Increase the inactivity threshold from `6*time.Second` to `90*time.Second`.
  3. Send client-side ping frames every 30 seconds to maintain connection through NAT/firewalls.

### Issue 2 (P0): Missing Core Execution Fields in `OrderUpdate`
* **File**: `orderstream/types.go`
* **Root Cause**: `orderstream.OrderUpdate` currently only defines 6 fields (`nOrdNo`, `trdSym`, `ordSt`, `fldQty`, `avgPrc`, `rejRsn`). Kotak Neo's actual real-time execution payload delivers critical trading metadata: `trnsTp` (Buy/Sell), `prod` (CNC/MIS/NRML), `exSeg`, `prcTp`, `qty`, `tok`, `unFldSz`, `exOrdId`, and `ordDtTm`. Without `trnsTp` and `prod`, copy-trading engines cannot reconstruct the order action.
* **Fix**: Expand `OrderUpdate` with all standard execution fields matching Kotak Neo's payload.

### Issue 3 (P1): Missing `Close()` Method on `orderstream.Client`
* **File**: `orderstream/stream.go`
* **Root Cause**: Callers have no way to cleanly terminate an active WebSocket connection. When an account session expires, or during worker pool teardown, cancelling context does not immediately unblock `conn.ReadMessage()`.
* **Fix**: Add thread-safe `func (c *Client) Close() error` that disables auto-reconnect, cancels context, and closes the underlying WebSocket connection.

### Issue 4 (P1): `SetSessionTokens` Does Not Update Authenticated State
* **File**: `client.go`
* **Root Cause**: When restoring valid credentials from persistent storage via `client.SetSessionTokens(token, sid)`, `c.authenticated` remains `false`. Consequently, `client.IsAuthenticated()` returns `false`.
* **Fix**: Update `SetSessionTokens` to set `c.authenticated = strings.TrimSpace(tradeToken) != "" && strings.TrimSpace(tradeSid) != ""`.

### Issue 5 (P2): `models.BrokerTime` Rejects Standard Kotak Date Formats
* **File**: `models/time.go`
* **Root Cause**: `acceptedFormats` only accepts ISO-8601 (`"2006-01-02 15:04:05"`). Kotak Neo order history and execution endpoints frequently return `"02-Jan-2006 15:04:05"` (e.g. `"16-Feb-2026 12:29:31"`) or slash format `"02/01/2006 15:04:05"`. Unmarshaling fails with `unable to parse datetime into BrokerTime`.
* **Fix**: Add `"02-Jan-2006 15:04:05"`, `"02-Jan-2006"`, `"02/01/2006 15:04:05"`, and `"02/01/2006"` to `acceptedFormats`.

### Issue 6 (P2): Missing `GetOrderBook` Method Documented in README
* **File**: `reports.go`
* **Root Cause**: `README.md` instructs users to call `client.GetOrderBook(ctx)`, but only `client.GetOrders(ctx)` exists in code.
* **Fix**: Add `GetOrderBook(ctx context.Context) ([]models.Order, error)` as an alias for `GetOrders(ctx)`.

---

## 3. Exact Code Changes

### A. `orderstream/types.go`
```go
package orderstream

import (
	"time"

	"github.com/shopspring/decimal"
)

// OrderUpdate represents an asynchronous order state transition event.
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

// PositionUpdate represents a live position exposure update.
type PositionUpdate struct {
	TradingSymbol string          `json:"trdSym"`
	Product       string          `json:"prod"`
	NetQuantity   int             `json:"netQty"`
	MTM           decimal.Decimal `json:"mtm"`
}

// StreamCallbacks represents callbacks for live order/position events.
type StreamCallbacks struct {
	OnConnect        func()
	OnOrderUpdate    func(OrderUpdate)
	OnPositionUpdate func(PositionUpdate)
	OnError          func(error)
	OnClose          func(code int, reason string)
	OnReconnect      func(attempt int, delay time.Duration)
}
```

---

### B. `orderstream/stream.go`
```go
package orderstream

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Client manages live order lifecycle events and position updates.
type Client struct {
	rtURL            string
	tradeToken       string
	tradeSid         string
	callbacks        StreamCallbacks
	conn             *websocket.Conn
	lastActivity     atomic.Value
	autoReconnect    bool
	reconnectAttempt int
	cancel           context.CancelFunc
	proxyURL         *url.URL
	mu               sync.Mutex
}

// SetProxyURL sets proxy URL on the order stream client.
func (c *Client) SetProxyURL(rawURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	trimmed := strings.TrimSpace(rawURL)
	if trimmed != "" {
		if u, err := url.Parse(trimmed); err == nil && u.Host != "" {
			c.proxyURL = u
		}
	}
}

// New creates a new order/position streaming client.
func New(rtURL, tradeToken, tradeSid string, callbacks StreamCallbacks) *Client {
	wsURL := rtURL
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(wsURL, "https://")
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	}

	c := &Client{
		rtURL:         wsURL,
		tradeToken:    tradeToken,
		tradeSid:      tradeSid,
		callbacks:     callbacks,
		autoReconnect: true,
	}
	c.lastActivity.Store(time.Now())
	return c
}

// Close gracefully terminates the order stream connection and stops auto-reconnect.
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

// Serve starts the connection and message loop until context cancellation or Close().
func (c *Client) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := c.connectAndRead(ctx)
			if err != nil && c.callbacks.OnError != nil {
				c.callbacks.OnError(err)
			}

			c.mu.Lock()
			shouldReconnect := c.autoReconnect
			c.mu.Unlock()

			if !shouldReconnect {
				return err
			}

			c.reconnectAttempt++
			delay := c.calculateBackoff()
			if c.callbacks.OnReconnect != nil {
				c.callbacks.OnReconnect(c.reconnectAttempt, delay)
			}

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (c *Client) calculateBackoff() time.Duration {
	base := 500 * time.Millisecond
	delay := base * time.Duration(math.Exp2(float64(c.reconnectAttempt)))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	jitter := time.Duration(rand.Intn(500)) * time.Millisecond
	return delay + jitter
}

func (c *Client) connectAndRead(ctx context.Context) error {
	u, err := url.Parse(c.rtURL)
	if err != nil {
		return fmt.Errorf("invalid rt url: %w", err)
	}

	c.mu.Lock()
	proxyURL := c.proxyURL
	c.mu.Unlock()

	dialer := websocket.DefaultDialer
	if proxyURL != nil {
		dialer = &websocket.Dialer{
			Proxy:            http.ProxyURL(proxyURL),
			HandshakeTimeout: 10 * time.Second,
		}
	}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("order stream websocket dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		conn.Close()
	}()

	c.reconnectAttempt = 0
	c.lastActivity.Store(time.Now())

	// Step 1: Send Raw Non-JSON Handshake String (per Kotak spec)
	handshake := fmt.Sprintf("{type:cn,Authorization:%s,Sid:%s,src:WEB}", c.tradeToken, c.tradeSid)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(handshake)); err != nil {
		return fmt.Errorf("send raw handshake: %w", err)
	}

	// Register Ping Handler to update activity timer and respond with Pong
	conn.SetPingHandler(func(appData string) error {
		c.lastActivity.Store(time.Now())
		c.mu.Lock()
		defer c.mu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	if c.callbacks.OnConnect != nil {
		c.callbacks.OnConnect()
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	readCtx, readCancel := context.WithCancel(ctx)
	defer readCancel()

	// Inactivity Watchdog: 90 seconds threshold
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-readCtx.Done():
				return
			case <-ticker.C:
				last := c.lastActivity.Load().(time.Time)
				if time.Since(last) > 90*time.Second {
					conn.Close()
					return
				}
			}
		}
	}()

	// Ping keep-alive: send Ping every 30 seconds
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-readCtx.Done():
				return
			case <-ticker.C:
				c.mu.Lock()
				err := conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(time.Second))
				c.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		c.lastActivity.Store(time.Now())

		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal(data, &envelope); err == nil {
			if envelope.Type == "order" && c.callbacks.OnOrderUpdate != nil {
				var order OrderUpdate
				if err := json.Unmarshal(envelope.Data, &order); err == nil {
					c.callbacks.OnOrderUpdate(order)
				}
			} else if envelope.Type == "position" && c.callbacks.OnPositionUpdate != nil {
				var pos PositionUpdate
				if err := json.Unmarshal(envelope.Data, &pos); err == nil {
					c.callbacks.OnPositionUpdate(pos)
				}
			}
		}
	}
}
```

---

### C. `client.go`
Update `SetSessionTokens`:
```go
// SetSessionTokens updates active trade session credentials and sets authenticated state.
func (c *Client) SetSessionTokens(tradeToken, tradeSid string) {
	c.transport.SetSessionHeaders(tradeToken, tradeSid)
	c.mu.Lock()
	c.authenticated = strings.TrimSpace(tradeToken) != "" && strings.TrimSpace(tradeSid) != ""
	c.mu.Unlock()
}
```

---

### D. `models/time.go`
Update `acceptedFormats`:
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

### E. `reports.go`
Add `GetOrderBook` alias:
```go
// GetOrderBook retrieves all orders for the current trading day (alias for GetOrders).
func (c *Client) GetOrderBook(ctx context.Context) ([]models.Order, error) {
	return c.GetOrders(ctx)
}
```

---

## 4. TDD Verification Matrix

Add the following tests in `../EnvoyTradeSDKs/gokotakneo`:

| Test Suite | Test Function | Scenario | Expected Outcome |
|---|---|---|---|
| `orderstream` | `TestOrderStream_FullOrderUpdate` | Push complete Kotak Neo JSON execution message with all 16 fields | All fields (`trnsTp`, `prod`, `exSeg`, `prcTp`, `qty`, `tok`, `exOrdId`, `ordDtTm`) parse accurately into `OrderUpdate` |
| `orderstream` | `TestOrderStream_Close` | Call `client.Close()` on active streaming client | Graceful exit without goroutine leak; `Serve` terminates promptly |
| `orderstream` | `TestOrderStream_PingPongKeepalive` | Push Ping control frames across 10-second idle window | `conn.SetPingHandler` handles frame, resets `lastActivity`, connection stays open |
| `client` | `TestClient_SetSessionTokens_Authenticated` | Call `SetSessionTokens("token", "sid")` | `c.IsAuthenticated() == true`; calling with `("", "")` sets `false` |
| `models` | `TestBrokerTime_KotakFormats` | Unmarshal `"16-Feb-2026 12:29:31"`, `"02-Jan-2006"`, `"16/02/2026 12:29:31"` | Deserializes into valid `time.Time` without error |
| `reports` | `TestReports_GetOrderBook_Alias` | Call `client.GetOrderBook(ctx)` | Executes successfully and matches `GetOrders(ctx)` behavior |

---

## 5. Execution Steps

1. Apply code changes across:
   - `orderstream/types.go`
   - `orderstream/stream.go`
   - `orderstream/stream_test.go`
   - `client.go`
   - `models/time.go`
   - `models/time_test.go`
   - `reports.go`
   - `reports_test.go`
2. Run test verification:
   ```sh
   cd ../EnvoyTradeSDKs/gokotakneo
   go test -v -race ./...
   ```
3. Commit and tag release:
   ```sh
   git add .
   git commit -m "fix: orderstream watchdog, execution fields, auth state, and time parsing"
   git tag v0.2.0
   git push origin main --tags
   ```
4. Update EnvoyTrade dependency:
   ```sh
   cd /Users/msp/MSP/Projects/EnvoyTrade
   go get github.com/DragoHex/gokotakneo@v0.2.0
   ```
