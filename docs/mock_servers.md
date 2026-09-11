# Test Broker & Proxy Infrastructure Specification

## 1. Overview & Purpose

To validate and test **EnvoyTrade** in local, staging, and automated integration environments without hitting Zerodha's live Kite Connect production endpoints or requiring actual leased VPS proxy hardware, this specification defines two companion test services:

1. **Forward Proxy Service (`cmd/testproxy`)**: A minimal HTTP forward proxy running on port `3128` that authenticates incoming requests via Basic Auth (`user:pass`), maps the user to their assigned virtual/static IP from a JSON configuration, and forwards traffic with `X-Forwarded-For` injection to mimic per-account proxy egress.
2. **Test Broker Server (`cmd/testbroker`)**: A mock Zerodha Kite Connect server running on port `8089` providing Kite-compatible REST APIs, multi-user order isolation, NIFTY50 F&O instrument master & option chain, strict IP whitelisting verification, comprehensive order validation & rejection rules, simulated postback webhooks to EnvoyTrade, and an embedded single-page Web UI (`/ui`).

---

## 2. High-Level Architecture & Data Flow

```mermaid
flowchart TD
    subgraph EnvoyTradeSystem [EnvoyTrade Copy-Trade Platform]
        ETWorker[Worker Pool / Kite Client]
        ETCallback[Callback Listener /broker-callback]
    end

    subgraph ProxyService [Test Forward Proxy :3128]
        ProxyAuth[Proxy Basic Auth Validator]
        ProxyMap[User-to-IP Mapper proxy_config.json]
        ProxyForward[HTTP Forwarder & X-Forwarded-For Injector]
        ProxyAuth --> ProxyMap --> ProxyForward
    end

    subgraph TestBrokerService [Test Broker Mock Server :8089]
        BrokerIPCheck[IP Whitelist remote_addr + X-Forwarded-For]
        BrokerRouter[Kite REST API /orders/regular, /orders, /quote]
        RuleEngine[Order Validation & Execution Engine]
        OptChain[NIFTY50 Option Chain In-Memory Store]
        OrderStore[Multi-User In-Memory Order Book]
        WebUI[Embedded Web UI /ui]
        PostbackSender[Simulated Postback Webhook Sender]

        BrokerIPCheck --> BrokerRouter
        BrokerRouter --> RuleEngine
        RuleEngine --> OptChain
        RuleEngine --> OrderStore
        OrderStore --> PostbackSender
        WebUI <--> OptChain
        WebUI <--> OrderStore
    end

    ETWorker -->|"HTTP via proxyURL (user:pass)"| ProxyAuth
    ProxyForward -->|"Forwarded Request (X-Forwarded-For: user-ip)"| BrokerIPCheck
    PostbackSender -.->|"POST /broker-callback (SHA-256 Checksum)"| ETCallback
```

---

## 3. Forward Proxy Service (`cmd/testproxy`, `internal/testproxy`)

### 3.1 Functionality
- Listens on `http://0.0.0.0:3128` (configurable via `PORT` or flag `-port`).
- Acts as a standard HTTP forward proxy for outbound broker requests.
- Requires Proxy Authentication:
  - Header: `Proxy-Authorization: Basic <base64(username:password)>`.
  - If missing or invalid, returns HTTP `407 Proxy Authentication Required`.
- **IP Diversion & Mapping**:
  - Matches the authenticated username against `configs/proxy_config.json`.
  - Obtains the assigned virtual client IP (e.g., `user1` -> `192.168.1.101`, `user2` -> `192.168.1.102`).
  - Injects `X-Forwarded-For: <assigned_ip>` header when proxying the request to the target broker server.
- Forwards HTTP request bodies and headers, returning the upstream broker's HTTP status and response payload transparently to the caller.

### 3.2 Configuration Schema (`configs/proxy_config.json`)
```json
{
  "port": 3128,
  "users": [
    {
      "username": "user1",
      "password": "pass1",
      "assigned_ip": "192.168.1.101"
    },
    {
      "username": "user2",
      "password": "pass2",
      "assigned_ip": "192.168.1.102"
    },
    {
      "username": "master_user",
      "password": "master_pass",
      "assigned_ip": "192.168.1.100"
    }
  ]
}
```

### 3.3 EnvoyTrade Client Integration
Compatible with standard Go `http.Client` proxy configuration and `internal/kite/proxy.go` (`kite.RESTClientFor`):
```go
proxyURL, _ := url.Parse("http://user1:pass1@localhost:3128")
client := &http.Client{
    Transport: &http.Transport{
        Proxy: http.ProxyURL(proxyURL),
    },
    Timeout: 10 * time.Second,
}
// Pass into kiteconnect client config or test harness
```

---

## 4. Test Broker Mock Server (`cmd/testbroker`, `internal/testbroker`)

### 4.1 Kite Connect REST API Endpoints
The test broker exposes endpoints matching Zerodha's Kite Connect v4 API specifications:

| Endpoint | Method | Description |
|---|---|---|
| `/orders/:variety` (e.g. `/orders/regular`) | `POST` | Places a new order. Returns `{ "status": "success", "data": { "order_id": "..." } }`. |
| `/orders` | `GET` | Returns list of all orders for the authenticated user. |
| `/orders/:order_id` | `GET` | Returns order details and transition history. |
| `/orders/:variety/:order_id` | `DELETE` | Cancels an open order. Returns `{ "status": "success", "data": { "order_id": "..." } }`. |
| `/quote/ltp` | `GET` | Returns Last Traded Price for requested symbols (e.g. `?i=NFO:NIFTY26SEPFUT`). |
| `/quote` | `GET` | Full quotes with market depth, OHLC, volume, and OI. |
| `/instruments` and `/instruments/NFO` | `GET` | Lists available instruments in JSON or Kite CSV format. |
| `/api/instruments/:symbol` | `PUT` | Updates LTP, bid/ask, and volume for a symbol dynamically at runtime. |
| `/test/postback` | `POST` | Manually triggers a simulated Kite postback webhook to EnvoyTrade. |
| `/ui` and `/ui/*` | `GET` | Serves the embedded single-page test broker web interface. |

### 4.2 Multi-User Isolation & State Management
- **User Identification**: Extracted from:
  1. `Authorization: token <api_key>:<access_token>` (standard Kite Connect header).
  2. Request parameter or header `X-User-ID` / `user_id`.
- **In-Memory Store**:
  - Thread-safe storage guarded by `sync.RWMutex`.
  - Isolates order books and positions per user account.
  - Queries to `GET /orders` return only the authenticated user's orders (admin UI can inspect all users).

### 4.3 Instruments & NIFTY50 Option Chain (`configs/testbroker_instruments.json`)
The mock broker pre-loads NIFTY50 F&O products on startup:
- **Underlying Index**: `NIFTY 50` spot (reference price 25,000.00).
- **Index Futures**: `NIFTY26SEPFUT` (exchange `NFO`, lot size 75, tick size 0.05).
- **Option Chain Ladder**:
  - Strikes from 24,500 to 25,500 at 50-point intervals.
  - Both Call (`CE`) and Put (`PE`) contracts for each strike (e.g. `NIFTY26SEP25000CE`, `NIFTY26SEP25000PE`).
  - Lot size: 75.
  - Tick size: 0.05.
  - Default LTP, bid, ask, open interest, and volume pre-populated.
- **Dynamic Updates**:
  - Web UI and REST API (`PUT /api/instruments/:symbol`) allow operators to alter LTP, bid/ask, and volume during testing without restarting the server.

### 4.4 IP Whitelisting & Verification Middleware
- Every incoming request to protected broker endpoints is evaluated by IP whitelisting middleware:
  1. Checks `r.RemoteAddr` (socket IP).
  2. If the request was forwarded by a proxy, inspects `X-Forwarded-For`.
  3. Matches the resolved client IP against the user's whitelisted IPs in `configs/testbroker_rules.json`.
- **Rejection**: If the IP does not match the whitelist for that account:
  - HTTP `403 Forbidden` is returned:
    ```json
    {
      "status": "error",
      "error_type": "NetworkException",
      "message": "Client IP not allowed: IP 192.168.1.999 is not whitelisted for user user1"
    }
    ```

### 4.5 Order Validation & Execution Rules (`configs/testbroker_rules.json`)
```json
{
  "allowed_exchanges": ["NFO", "NSE"],
  "allowed_products": ["NRML", "MIS", "CNC"],
  "allowed_order_types": ["MARKET", "LIMIT", "SL", "SL-M"],
  "allowed_varieties": ["regular", "amo"],
  "lot_size_check": true,
  "circuit_limit_pct": 10.0,
  "max_quantity_per_order": 1800,
  "execution_mode": "instant",
  "postback_url": "http://localhost:8080/broker-callback",
  "users": {
    "user1": {
      "api_key": "kite_key_user1",
      "api_secret": "secret_user1",
      "whitelisted_ips": ["192.168.1.101", "127.0.0.1"]
    },
    "user2": {
      "api_key": "kite_key_user2",
      "api_secret": "secret_user2",
      "whitelisted_ips": ["192.168.1.102"]
    },
    "master_user": {
      "api_key": "kite_key_master",
      "api_secret": "secret_master",
      "whitelisted_ips": ["192.168.1.100", "127.0.0.1"]
    }
  },
  "auto_reject_symbols": ["REJECT_ME"]
}
```

Validation Logic:
1. **Lot Multiples**: `quantity % instrument.LotSize == 0`. Fails with `InputError` if violated.
2. **Freeze Limit**: `quantity <= max_quantity_per_order` (1800 for NIFTY). Fails with `OrderException` if exceeded.
3. **Price Band**: For `LIMIT` orders, price must be within `±circuit_limit_pct%` of LTP.
4. **Execution Mode**:
   - `"instant"` (default): Order immediately transitions to `COMPLETE` at the current market/limit price, and a postback webhook is dispatched.
   - `"manual"`: Order remains `OPEN` until a user clicks "Fill" or "Reject" in the Web UI.
   - `"auto_reject"`: Triggers immediate `REJECTED` status for testing failure handling in EnvoyTrade.

### 4.6 Simulated Postback Webhooks
When an order reaches a terminal state (`COMPLETE`, `REJECTED`, `CANCELLED`):
1. Format postback JSON payload matching Kite Connect's postback schema.
2. Calculate Kite checksum:
   $$\text{checksum} = \text{hex}(\text{sha256}(\text{order\_id} + \text{order\_timestamp} + \text{api\_secret}))$$
3. Send HTTP `POST` to `postback_url` (EnvoyTrade's `/broker-callback`).

---

## 5. Embedded Test Broker Web UI (`/ui`)

### 5.1 Technology & Zero-Dependency Delivery
- Standard Go `//go:embed` compiles HTML, CSS, and JS directly into the test broker binary.
- Requires no external Node.js/pnpm build steps during runtime.
- Styled according to EnvoyTrade's design tokens:
  - Primary Slate: `#44475b` / `#1a1c24`
  - Accent Teal: `#04b488` / `#06d896`
  - Surface: `#f5f5f7` / `#1a1c24`

### 5.2 UI Views
1. **Option Chain View**:
   - Tabular ladder of NIFTY50 strikes (Calls on left, Strike in center, Puts on right).
   - Shows LTP, Bid, Ask, Volume, OI.
   - Inline editable input to simulate live price movements.
2. **Order Book View**:
   - Real-time list of all placed orders across users.
   - User filter dropdown.
   - Status indicators (`COMPLETE`, `OPEN`, `REJECTED`, `CANCELLED`).
   - Action buttons for `OPEN` orders: **Fill** (triggers `COMPLETE`), **Reject** (triggers `REJECTED`), **Cancel**.
3. **Postback Tester**:
   - Form allowing manual dispatch of master fills or follower updates directly to EnvoyTrade with custom prices, quantities, and tags.
4. **Whitelisting & Rules View**:
   - Visual dashboard of active users, their assigned IPs, and active validation rules.

---

## 6. Directory Layout

```
configs/
  testbroker_instruments.json   # NIFTY50 spot, futures, and option chain ladder
  testbroker_rules.json         # Order validation rules, execution mode, user IP whitelists
  proxy_config.json             # Proxy port, basic auth credentials, assigned user IPs

cmd/
  testbroker/
    main.go                     # Test broker entrypoint
  testproxy/
    main.go                     # Forward proxy service entrypoint

internal/
  testbroker/
    server.go                   # HTTP router, middleware, and Kite API handlers
    instruments.go              # Instruments loader, option chain model, and live updater
    orders.go                   # Multi-user order engine, state machine, and validator
    whitelist.go                # IP whitelisting and X-Forwarded-For resolution
    postback.go                 # Kite-compliant postback webhook generator and dispatcher
    ui.go                       # Embedded Web UI handler and static file serving
    ui/
      index.html                # Self-contained single-page UI (HTML/CSS/JS)
    server_test.go              # Unit and integration tests for test broker
  testproxy/
    proxy.go                    # HTTP forward proxy implementation
    config.go                   # Proxy config loader
    auth.go                     # Proxy Basic Auth and IP assignment mapper
    proxy_test.go               # Proxy unit and integration tests
```

---

## 7. Verification & Testing Strategy

1. **Test Proxy Unit Tests (`internal/testproxy/proxy_test.go`)**:
   - Verify missing/bad credentials return HTTP 407.
   - Verify valid credentials map correctly to assigned client IP.
   - Verify HTTP request forwarding and header injection (`X-Forwarded-For`).
2. **Test Broker Unit Tests (`internal/testbroker/server_test.go`)**:
   - Verify instrument loading and option chain JSON structure.
   - Verify order validation (lot size check, price bands, freeze limits).
   - Verify IP whitelisting middleware (authorized IP -> 200, unauthorized IP -> 403).
   - Verify multi-user order isolation (`user1` cannot view `user2`'s orders).
   - Verify postback checksum generation against Zerodha's specification.
3. **End-to-End Pipeline Test**:
   - EnvoyTrade `worker.Pool` dispatching through `kite.RESTClientFor` -> `testproxy (:3128)` -> `testbroker (:8089)`.
   - Test broker auto-filling order and dispatching postback to EnvoyTrade `/broker-callback`.
