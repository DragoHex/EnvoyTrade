# WebSocket & Postback Architecture (V1)

Dual-channel ingestion, execution, and reconciliation design for Zerodha Kite Connect copy-trading.

## 1. Overview & Data Flow

```
Trader (Kite Web/App)
   │ manual order
   ▼
Kite Broker — Master Session
   │ WS on_order_update (status: COMPLETE)
   ▼
WS Listener (`callback.MasterTicker` via `memchan.Queue[MasterFill]`)
   │ durable write FIRST (Postgres `master_fills`), then fan-out
   ▼
Fan-out Engine (`internal/engine`)
   │ read `follow_links`, scale qty (`domain.SizeOrder`, lot-rounded), insert `follower_orders`
   ▼
Order Worker Pool (`internal/worker.Pool`)
   │ goroutine-per-follower, throttled (~10 orders/sec), timeout retry
   ▼
Kite Broker — Follower Accounts
   │ Postback webhook (SHA256 signature verified) → `/broker-callback`
   ▼
Postback Handler (`callback.Handler` via `memchan.Queue[OrderUpdate]`)
   │ update `follower_orders` terminal status & append `order_events`
   ▼
Reconciliation Poller (`internal/recon.Poller`, every 30–60s)
   │ diffs master REST history vs DB & resolves stuck follower orders
   ▼
Drift & Alert Hooks
```

---

## 2. Ingestion Channels

| Leg | Channel | Rationale |
|---|---|---|
| Master fill detection | WebSocket (`MasterTicker`) | Postback only fires for orders placed with app's own API key. Master trades manually on Kite web/app; WebSocket captures manual orders across all origins. |
| Follower order status | HTTP Postback (`/broker-callback`) | Orders are placed via platform's own API key. Webhook scales flat across N followers without maintaining N persistent WS connections. |
| Safety backstop | Periodic reconciliation poll (`recon.Poller`) | Closes delivery gaps from WS reconnect windows or dropped postbacks. |

---

## 3. Component Details

### Master WebSocket Listener (`internal/kite/callback/ticker.go`)
- Wraps `Conn` (subset of `*kiteticker.Ticker`).
- Per PLAN.md §3.4: only one WS Ticker runs per process (the master) due to gorilla's package-global `DefaultDialer`.
- Filters for terminal order statuses (`COMPLETE`, `REJECTED`, `CANCELLED`).
- Publishes `domain.MasterFill` to `queue.Publisher[domain.MasterFill]`.
- Implements `SetCatchUpHook`: on reconnect (`OnConnect` count > 1), automatically triggers catch-up REST sweep to backfill any fills executed during the disconnect window.

### Adaptive Postback Webhook Router (`internal/callback/router.go`)
- Mounted at `POST /broker-callback` and `POST /broker-callback/`.
- Routes any broker's webhook through an adaptive 5-stage pipeline:
  1. `?broker=name` query param
  2. `X-Broker: name` HTTP header
  3. `/broker-callback/{broker}` subpath
  4. Sub-microsecond zero-allocation byte fingerprinting (`groww_order_id`, `uniqueorderid`+`filledshares`, `checksum`+`user_id`)
  5. Default fallback to Kite handler
- Request body is preserved for downstream signature validation.
- See `docs/CALLBACKS.md` for full specification.

### Kite Postback Webhook Handler (`internal/kite/callback/postback.go`)
- Handles Zerodha Kite and TestBroker postback payloads.
- Computes SHA256 signature `order_id + order_timestamp + api_secret` and verifies with constant-time compare against payload checksum.
- Resolves account via `AccountByBrokerAndUserID(ctx, "kite", userID)` with fallback to `AccountByBrokerUserID`.
- Responds `200 OK` for invalid checksum, unknown user, non-terminal status, or queue-full to prevent broker retry spam.
- Routes master fills to `MasterFills` queue and follower order updates to `FollowerUpdates` queue.

### Worker Pool (`internal/worker/pool.go`)
- Dedicated goroutine and bounded channel per follower.
- Rate limiter: defaults to 100ms spacing between orders to stay within Kite's ~10 orders/s per account limit.
- Retry policy:
  - Network timeouts / context deadlines: retried up to 3 times with exponential backoff.
  - Margin / RMS / input rejections: terminal immediately with no retry (avoids duplicate executions).
- Non-blocking dispatch: full worker queue marks order dead-lettered without blocking fan-out to other followers.

### Reconciliation Poller (`internal/recon/reconciler.go`)
- Periodic safety net running every 30–60s.
- **Master History Poll**: pulls master orders via REST over polling window (default 15m); converts to `domain.MasterFill` and passes to `MasterFillConsumer.Handle`. Idempotent `InsertMasterFill` safely absorbs duplicates.
- **Stuck Follower Resolution**: queries `follower_orders` where `terminal_status IS NULL` and `created_at < now() - 2m`. Fetches order history via REST; if terminal, updates status and appends audit event.
- **Drift Detection**: triggers `Alerter.Alert` if order rejected, quantity drifts from expected lot-scaled intent, or order remains unresolved past timeout.

### Kite Broker Adapter (`internal/kite/broker.go` & `proxy.go`)
- Adapts `*kiteconnect.Client` to `broker.Broker`.
- Handles `PlaceOrder`, `GetOrders`, and `GetOrderHistory`.
- Supports per-account static proxy egress via `RESTClientFor(ProxyConfig)` to comply with Zerodha static IP requirements.

### Server Wiring (`cmd/server/main.go`)
- Initializes Postgres store and runs migrations.
- Instantiates `worker.Pool` and registers follower accounts.
- Launches background goroutines for `MasterFillConsumer`, `FollowerStatusConsumer`, and `recon.Poller`.
- Mounts `/broker-callback` alongside HTTP REST API routes.
- Configures structured logging (`slog`) writing to `/var/log/envoytrade/app.log` (or `os.Stdout` fallback) with HTTP request logging middleware.
- Handles graceful shutdown on `SIGINT`/`SIGTERM`.
