# Paper Trading Mock Broker — Design Plan

> **Goal:** A single Go binary (`cmd/testbroker`) that fully simulates Zerodha Kite Connect for E2E testing of EnvoyTrade's copy-trade pipeline — REST API, WebSocket ticker, and postback webhooks — with an embedded lightweight UI for control.

---

## What It Replaces

The existing spec ([mock_servers.md](file:///Users/msp/MSP/Projects/EnvoyTrade/docs/mock_servers.md)) defined the target. This plan implements it, with two major additions the spec was missing:

1. **WebSocket ticker endpoint** — EnvoyTrade's `TickerManager` connects to `ws://<host>?api_key=...&access_token=...` and expects `{"type":"order","data":{...}}` messages. The mock must serve this.
2. **Quick user switching in UI** — A single-click dropdown to act as any registered user (master or follower) without restarting.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  cmd/testbroker (:8089)                      │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  ┌───────────┐  │
│  │ REST API │  │ WS Ticker│  │ Postback  │  │ Embedded  │  │
│  │ Handlers │  │ Hub      │  │ Dispatcher│  │ UI (/ui)  │  │
│  └────┬─────┘  └────┬─────┘  └─────┬─────┘  └─────┬─────┘  │
│       │              │              │              │         │
│       └──────────────┴──────┬───────┴──────────────┘         │
│                             │                                │
│                    ┌────────▼────────┐                       │
│                    │   OrderEngine   │                       │
│                    │ (per-user books,│                       │
│                    │  instruments,   │                       │
│                    │  validation)    │                       │
│                    └─────────────────┘                       │
└─────────────────────────────────────────────────────────────┘
         │                                    ▲
         │ POST /broker-callback              │ WS connect
         ▼                                    │
┌─────────────────────────────────────────────────────────────┐
│              EnvoyTrade Server (:8080)                       │
│   callback.Handler          TickerManager (ws.Conn)         │
└─────────────────────────────────────────────────────────────┘
```

---

## Scope — What Gets Built

### Core (Milestone 1)

| Component | File | What it does |
|---|---|---|
| **Entrypoint** | `cmd/testbroker/main.go` | Loads config, starts HTTP+WS server on `:8089` |
| **Config & Users** | `internal/testbroker/config.go` | Loads `configs/testbroker_config.json` — users, instruments, rules, postback URL |
| **Order Engine** | `internal/testbroker/engine.go` | Per-user order book, order ID generation, validation (lot size, freeze, price band), state machine (OPEN→COMPLETE/REJECTED/CANCELLED) |
| **REST Handlers** | `internal/testbroker/rest.go` | Kite-compatible REST: `POST /orders/{variety}`, `GET /orders`, `GET /orders/{id}`, `DELETE /orders/{variety}/{id}`, `GET /portfolio/positions`, `GET /user/margins` |
| **Auth Middleware** | `internal/testbroker/auth.go` | Parses `Authorization: token api_key:access_token`, resolves user |
| **WebSocket Hub** | `internal/testbroker/wshub.go` | Upgrades `GET /?api_key=...&access_token=...` to WS, pushes `{"type":"order","data":{...}}` on order state transitions |
| **Postback Dispatcher** | `internal/testbroker/postback.go` | On terminal state, `POST`s Kite-format JSON with `sha256(order_id + timestamp + api_secret)` checksum to EnvoyTrade's `/broker-callback` |
| **Instruments Store** | `internal/testbroker/instruments.go` | In-memory NIFTY50 futures + option chain, LTP/bid/ask updatable at runtime |
| **Embedded UI** | `internal/testbroker/ui/` | Single-page HTML+CSS+JS, embedded via `//go:embed` |

### Config File

Single config file `configs/testbroker_config.json` (merges old `testbroker_rules.json` + `testbroker_instruments.json`):

```json
{
  "port": 8089,
  "postback_url": "http://localhost:8080/broker-callback",
  "execution_mode": "instant",
  "users": {
    "MASTER01": {
      "api_key": "key_master",
      "api_secret": "secret_master",
      "access_token": "token_master",
      "role": "master"
    },
    "FOLLOWER01": {
      "api_key": "key_f1",
      "api_secret": "secret_f1",
      "access_token": "token_f1",
      "role": "follower"
    },
    "FOLLOWER02": {
      "api_key": "key_f2",
      "api_secret": "secret_f2",
      "access_token": "token_f2",
      "role": "follower"
    }
  },
  "instruments": [
    {
      "exchange": "NFO",
      "tradingsymbol": "NIFTY26SEPFUT",
      "instrument_token": 408065,
      "lot_size": 75,
      "tick_size": 0.05,
      "ltp": 25000.0
    }
  ],
  "validation": {
    "lot_size_check": true,
    "max_quantity_per_order": 1800,
    "circuit_limit_pct": 10.0,
    "allowed_exchanges": ["NFO", "NSE"],
    "allowed_products": ["NRML", "MIS", "CNC"],
    "allowed_order_types": ["MARKET", "LIMIT", "SL", "SL-M"]
  }
}
```

---

## Key Design Decisions

### 1. WebSocket Ticker

EnvoyTrade's [`ws.Conn`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/kite/ws/conn.go) connects to `{RootURL}?api_key=X&access_token=Y` and reads JSON text messages:

```json
{"type": "order", "data": { <kiteconnect.Order fields> }}
```

The mock's WS hub:
- Accepts upgrade at the root path (`/`) — matching Kite's `wss://ws.kite.trade` path
- Authenticates via query params `api_key` + `access_token` → resolves user
- Keeps a registry of connected clients per user
- When an order reaches terminal state, broadcasts to all WS clients of that user
- **Both** WS and postback fire on the same event — EnvoyTrade's dedup (`InsertMasterFill` unique constraint) handles it

### 2. Execution Modes (per order or global)

| Mode | Behavior |
|---|---|
| `instant` (default) | Order immediately fills at LTP, triggers postback+WS |
| `manual` | Order stays `OPEN`, operator fills/rejects via UI |
| `delayed` | Order fills after configurable delay (e.g. 500ms) — tests async flow |

The UI can override per-order (click Fill/Reject on OPEN orders).

### 3. Quick User Switching in UI

The UI is an **admin console**, not scoped to a single user. It shows:
- All users and their roles in a top bar
- A "Place Order As" dropdown to submit orders as any user (simulating master trades)
- Per-user order book filter

This lets a tester:
1. Place a master trade via the UI
2. Watch EnvoyTrade's engine fan out follower orders
3. See follower orders arrive in the mock's order book
4. Verify postbacks were sent

### 4. Dual Notification (WS + Postback)

Every terminal order fires **both** a WS message and a postback HTTP POST. This exactly mirrors Kite's real behavior and tests EnvoyTrade's dedup path — `InsertMasterFill`'s unique constraint no-ops the second delivery.

### 5. Session/Login Endpoint (simplified)

The mock provides a trivial token endpoint:
- `POST /session/token` — accepts any `api_key` + `request_token`, returns `{"data":{"access_token":"<configured_token>"}}`

No real auth flow — just enough for EnvoyTrade's `kite.Login` to succeed.

---

## Embedded UI Design

Single-page, zero-dependency HTML/CSS/JS using EnvoyTrade's design tokens. Three panels:

### Panel 1: Order Book
- Table: Order ID, User, Symbol, Side, Qty, Price, Status, Timestamp
- Filter by user (dropdown)
- Action buttons for OPEN orders: **Fill**, **Reject**, **Cancel**
- Auto-updates via polling (`GET /api/orders` every 1s) or SSE

### Panel 2: Quick Trade
- "Place Order As" user dropdown (shows role badge: master/follower)
- Symbol dropdown (from loaded instruments)
- Side toggle: BUY / SELL
- Quantity input (pre-filled to lot size)
- Order type: MARKET / LIMIT (with price input)
- Submit button → `POST /orders/regular` internally

### Panel 3: Instruments & Settings
- Table of loaded instruments with editable LTP
- Global execution mode toggle: instant / manual / delayed
- Postback URL display and override
- Activity log: last N postbacks sent (status, target, response code)

---

## Directory Layout

```
cmd/testbroker/
  main.go

internal/testbroker/
  config.go           # Config struct + JSON loader
  engine.go           # OrderEngine: per-user order book, validation, state machine
  rest.go             # Kite REST API handlers (mux routing)
  auth.go             # Auth middleware: token header → user resolution
  wshub.go            # WebSocket hub: upgrade, per-user client registry, broadcast
  postback.go         # Postback dispatcher: checksum + HTTP POST
  instruments.go      # Instrument store: load, query, update LTP
  server.go           # Server wiring: http.Server setup, graceful shutdown
  ui.go               # //go:embed handler for static files
  ui/
    index.html         # Single-page admin UI

configs/
  testbroker_config.json  # Users, instruments, rules
```

> [!NOTE]
> The forward proxy (`cmd/testproxy`) from the spec is out of scope for this milestone — the mock broker works fine without it for E2E testing by pointing `TickerManager.SetWSRootURL` and Kite REST base URL directly at `:8089`.

---

## Integration with EnvoyTrade

To run E2E tests, point EnvoyTrade at the mock:

```sh
# Terminal 1: start the mock broker
go run ./cmd/testbroker

# Terminal 2: start EnvoyTrade with mock endpoints
# (via env vars or config that overrides Kite base URLs)
```

EnvoyTrade needs these overrides:
- **Kite REST base URL** → `http://localhost:8089` (workers call `PlaceOrder`, recon calls `GetOrders`)
- **Kite WS root URL** → `ws://localhost:8089` (via `TickerManager.SetWSRootURL`)
- Account `api_key`, `api_secret`, `access_token` in DB must match `testbroker_config.json`

The `make db-seed` should be updated to insert accounts matching the mock's config.

---

## Implementation Order

1. **Config + Engine** — load config, in-memory order book with validation
2. **REST handlers** — `PlaceOrder`, `GetOrders`, `GetOrderHistory`
3. **Auth middleware** — parse Kite auth header
4. **Postback dispatcher** — fire on terminal state
5. **WebSocket hub** — upgrade + broadcast
6. **Instruments store** — load + runtime LTP update
7. **Embedded UI** — order book, quick trade, instruments
8. **Entrypoint** — wire everything in `main.go`
9. **Seed data alignment** — update `make db-seed` to match mock config

---

## What This Intentionally Skips

- IP whitelisting (test complexity with zero E2E value — add later if needed)
- Option chain ladder (start with futures only, extend trivially)
- Portfolio positions tracking (orders are the critical path)
- Proxy service (`cmd/testproxy`) — separate concern, not needed for core E2E
