# Debugging & Log Filtering Guide

Guide for tracing orders, debugging WebSocket/postback signals, and inspecting structured logs.

## Log Locations & Configuration

EnvoyTrade uses Go structured logging (`log/slog`).

- **Default File**: `/var/log/envoytrade/app.log` (falls back to `stdout` if unprivileged)
- **Environment Variables**:
  - `LOG_FILE`: Custom log file path (e.g. `LOG_FILE=/tmp/envoytrade.log`)
  - `LOG_FORMAT`: `json` (default) or `text`
  - `LOG_LEVEL`: `debug`, `info` (default), `warn`, `error`
  - `LOG_TO_STDOUT`: Set to `true` to tee output to both stdout and log file

---

## Order Ingestion Logging

When master orders arrive via WebSocket (`MasterTicker`) or Kite Postback (`Handler`), terminal updates (`COMPLETE`, `REJECTED`, `CANCELLED`) are logged across pipeline stages:

1. **Listener queue consumption** (`internal/listener/listener.go`):
   - Message: `listener: consuming master fill`
   - Fields: `broker_order_id`, `master_id`, `symbol`, `qty`
2. **Engine fan-out processing** (`internal/engine/engine.go`):
   - Message: `engine: processing master fill`
   - Fields: `master_id`, `broker_order_id`, `symbol`, `qty`
3. **Follower dispatch** (`internal/engine/engine.go`):
   - Message: `engine: follower order dispatched`
   - Fields: `follower_id`, `order_id`, `qty`
4. **Ticker warnings / errors** (`internal/kite/callback/ticker.go`):
   - Queue saturation: `ticker: master fill queue full, dropping`
   - Catch-up triggers: `ticker: reconnected, running catch-up order pull`

---

## Ripgrep (`rg`) Cheat Sheet

Run `rg` on log file (adjust path if `LOG_FILE` differs):

### 1. Master Fills Ingestion
Find incoming master fills consumed by listener:
```sh
rg 'listener: consuming master fill' /var/log/envoytrade/app.log
```

### 2. Full Order Flow (Master Ingest -> Engine -> Follower Dispatch)
Trace complete copy-trade execution:
```sh
rg 'consuming master fill|processing master fill|follower order dispatched' /var/log/envoytrade/app.log
```

### 3. Trace Specific Broker Order ID
Search Kite order ID across all components:
```sh
# Matches JSON ("broker_order_id":"...") and logfmt (broker_order_id=...)
rg 'broker_order_id["=:\s]+<ORDER_ID>' /var/log/envoytrade/app.log
```

### 4. Ticker Events (Reconnects, Drops)
```sh
rg 'ticker:' /var/log/envoytrade/app.log
```

### 5. Follower Order Failures & Dead-Letter Events
```sh
rg 'dead-lettered|follower worker channel full|insert master fill failed' /var/log/envoytrade/app.log
```

### 6. Container Logs (Podman / Docker)
If service runs in container:
```sh
podman logs -f envoytrade | rg 'consuming master fill'
```
