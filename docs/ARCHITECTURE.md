# EnvoyTrade V1 — Architecture

Companion to [PLAN.md](./PLAN.md). This document captures the system's structure and the
interactions between its components. It is derived from, not a substitute for, the plan —
see PLAN.md for rationale and alternatives considered.

---

## 1. System context

Single Go binary, single Postgres instance, one master trading account, N follower accounts,
each follower egressing through its own dedicated static-IP proxy. Zerodha Kite Connect is the
only external system.

```mermaid
C4Context
  title EnvoyTrade — System Context

  Person(operator, "Operator", "Runs daily auth, watches dashboard, engages kill switch")
  Person(master, "Master trader", "Places orders manually or via app/web — the signal source")

  System_Boundary(envoytrade, "EnvoyTrade") {
    System(binary, "envoytrade binary", "Go service: listener, engine, workers, recon, dashboard")
    SystemDb(pg, "PostgreSQL", "Event log + derived state, single source of truth")
  }

  System_Ext(kite, "Zerodha Kite Connect", "REST + WebSocket broker API")
  System_Ext(proxy, "Proxy vendor (algoip.in / staticip.in)", "Per-account dedicated static-IP egress")
  System_Ext(alert, "Alert webhook", "Slack / Telegram")
  System_Ext(secrets, "Secret manager", "KMS / sops+age — API secrets & access tokens")

  Rel(master, kite, "1. Places a trade directly on own account (web/mobile/desktop) — not through EnvoyTrade")
  Rel(kite, binary, "2. WS: pushes the master's order-update event to the running service")
  Rel(binary, pg, "Durable write of the master fill, then read/write derived state")
  Rel(binary, kite, "3. REST: places the copied order on each follower account")
  Rel(binary, proxy, "Every follower REST call egresses via that account's dedicated static IP")
  Rel(proxy, kite, "Forwards the follower's order call under its Zerodha-whitelisted IP")
  Rel(operator, binary, "Daily auth, dashboard, kill switch")
  Rel(binary, alert, "Pages on drift, disconnects, kill switch, un-authed accounts")
  Rel(binary, secrets, "Reads API secrets / access tokens by reference")
```

---

## 2. Component / package map

One process; components are Go packages communicating in-memory. The only two seams meant to
survive a V2 rewrite are `queue` (dispatch transport) and `kite.Broker`/`BrokerFactory`
(broker abstraction) — see PLAN.md §1.

```mermaid
flowchart TB
  subgraph cmd["cmd/"]
    main["cmd/envoytrade\n(wiring, shutdown)"]
    authcli["cmd/envoytrade-auth\n(daily re-auth callback)"]
  end

  subgraph internal["internal/"]
    config["config"]
    domain["domain\n(pure types + sizing rules)"]
    store["store\n(Postgres, sqlc)"]
    kite["kite\n(gokiteconnect wrapper)\nclient · ticker · session · fake"]
    queueP["queue\n(Publisher/Consumer, memchan impl)"]
    listener["listener\n(master + follower ingestion)"]
    engine["engine\n(fan-out, sizing, dispatch)"]
    worker["worker\n(per-follower goroutine)"]
    recon["recon\n(reconciliation + drift)"]
    obs["obs\n(log, metrics, trace facades)"]
    admin["admin\n(HTTP dashboard)"]
  end

  main --> config & store & kite & queueP & listener & engine & worker & recon & obs & admin
  authcli --> kite
  authcli --> store

  listener -->|reads| kite
  listener -->|durable write| store
  listener -->|publish MasterFill| queueP
  engine -->|consume| queueP
  engine -->|read follow_links, write follower_orders| store
  engine -->|dispatch Job| worker
  worker -->|PlaceOrder| kite
  worker -->|write order_events| store
  recon -->|GetOrderHistory, GetOrders| kite
  recon -->|append transitions| store
  admin -->|read-mostly + kill switch writes| store

  engine -. depends on interface only .-> kite
  worker -. depends on interface only .-> kite
  engine -. depends on interface only .-> queueP
```

**Dependency rule (PLAN.md §1):** `engine` and `worker` import only the `queue`, `kite`,
`store`, `obs` *interfaces* — never `memchan` or `gokiteconnect` concretely. `main.go` is the
sole place concrete implementations are named.

---

## 3. Master signal capture (WS ticker)

Kite offers two channels for order events — HTTP postback and WS `OnOrderUpdate`. Postback only
notifies for orders placed via the app's own `api_key`; since the master places orders manually
through the Zerodha app, only the WS ticker sees them (PLAN.md §3.0).

```mermaid
sequenceDiagram
  participant M as Master (Kite app/web)
  participant KWS as Kite WS (ticker)
  participant L as internal/listener
  participant DB as Postgres (master_fills)
  participant Q as queue (memchan)
  participant E as internal/engine

  M->>KWS: Places/modifies order (any origin)
  KWS-->>L: OnOrderUpdate(order) [text frame]
  L->>L: filter: status==COMPLETE, not seen
  L->>DB: INSERT master_fills (dispatch_state='pending')
  Note over L,DB: Durable write — the source of truth
  L-->>Q: non-blocking Publish(MasterFill)
  Q-->>E: Consume(MasterFill)
  Note over E: If channel send was dropped,<br/>outbox drainer re-injects pending rows every 2s
```

Only **one** `Ticker` instance runs in the whole process (the master's) — gorilla's shared
package-level `websocket.DefaultDialer` makes per-instance proxying unsafe, so follower tickers
are never created (PLAN.md §3.4). Follower status instead comes from REST reconciliation.

---

## 4. Fan-out, sizing, and dispatch (transactional outbox)

The in-memory channel is a latency optimization only; Postgres is the durability boundary.

```mermaid
sequenceDiagram
  participant Q as queue.Consumer
  participant E as engine
  participant DB as Postgres
  participant W as worker (per follower)
  participant K as Kite REST (via proxy)

  Q->>E: MasterFill
  E->>DB: SELECT follow_links WHERE master_id=? AND enabled
  loop each follower
    E->>E: SizeOrder(masterQty, ratio, lotSize, maxQty)
    E->>DB: TX INSERT follower_orders (idempotency_tag, algo_id, intended_qty)
  end
  E->>DB: UPDATE master_fills SET dispatch_state='dispatched'
  E-->>W: hand off Job (non-blocking send to follower's chan)
  W->>W: rate limiter (8/s) + circuit breaker check
  W->>K: PlaceOrder(variety, OrderParams{AlgoID, Tag=idempotency_tag})
  K-->>W: order_id / error
  W->>DB: INSERT order_events (api_request, api_response, http_status, latency_ms)
  W->>DB: UPDATE follower_orders SET kite_order_id, placed_qty
```

**Isolation:** each follower has its own goroutine, inbound buffered channel, `Broker` client
(own proxy egress), rate limiter, and circuit breaker. A stuck or panicking follower cannot
block another; an overflowing channel dead-letters that one `follower_order` (PLAN.md §4.3).

**Outbox drainer** (every 2s): sweeps `master_fills` still `pending` after 2s and re-injects
them into the engine — guarantees at-least-once even across a process crash between the
listener's write and the engine's consume (PLAN.md §4.1).

---

## 5. Reconciliation & drift detection

WS is the fast path; the reconciliation loop polling REST is the **authoritative** one — it is
also the only status channel for followers, since they have no ticker of their own.

```mermaid
sequenceDiagram
  participant R as recon (every 5s + close pass)
  participant DB as Postgres
  participant K as Kite REST (per-follower proxy)

  loop each non-terminal follower_order
    R->>DB: SELECT follower_orders WHERE terminal_status IS NULL
    R->>K: GetOrderHistory(kite_order_id)
    K-->>R: order transitions
    R->>DB: INSERT order_events (ws_update/reconciled) for unseen transitions
    R->>DB: UPDATE follower_orders (filled_qty, terminal_status, average_price)
    R->>R: compute drift = filled_qty - expected(master_filled x ratio / lot)
    alt drift beyond 1 lot, or rejection, or partial-fill mismatch
      R->>DB: (drift recorded, visible on dashboard)
      R-->>obs.Alerter: webhook alert
    end
  end
```

---

## 6. Daily authentication lifecycle

Kite access tokens expire ~06:00 IST daily; login requires interactive 2FA (PLAN.md §3.2).

```mermaid
sequenceDiagram
  participant Op as Operator
  participant Auth as envoytrade-auth
  participant Kite as Kite login
  participant DB as Postgres
  participant Eng as engine and listener

  Note over Op,Eng: Pre-open window, 07:00 to 09:00 IST
  Op->>Auth: Open auth board, per-account live-session check
  Auth-->>Op: red or green board, plus Login links
  Op->>Kite: Follow login link, complete 2FA
  Kite-->>Auth: redirect with request_token
  Auth->>Kite: GenerateSession(request_token, api_secret)
  Kite-->>Auth: access_token
  Auth->>DB: revoke prior session, insert new account_sessions row
  Eng->>DB: readiness gate, refuse dispatch or listen without live session

  Note over Eng,Kite: Mid-session token death
  Eng->>Kite: any API call
  Kite-->>Eng: 403 TokenException
  Eng->>DB: mark session revoked
  Eng->>Eng: engage per-account kill switch, page operator
  Note over Eng: Other followers keep trading, isolation holds
```

---

## 7. Egress isolation (static IP)

Every account's REST client is bound to its own dedicated proxy at construction time via
`Client.SetHTTPClient` — no change to the vendored SDK (PLAN.md §3.3).

```mermaid
flowchart LR
  subgraph Accounts["Accounts"]
    M["Master account"]
    F1["Follower 1"]
    F2["Follower 2"]
    FN["Follower N"]
  end

  subgraph Proxies["Proxy vendor: algoip.in / staticip.in"]
    PM["Dedicated IP for master"]
    P1["Dedicated IP for Follower 1"]
    P2["Dedicated IP for Follower 2"]
    PN["Dedicated IP for Follower N"]
  end

  KiteAPI[("Kite Connect API")]
  SelfCheck["Startup and hourly self check.\nCompares observed IP per account against the whitelisted IP.\nA mismatch engages that account's kill switch."]

  M -->|"REST, own dedicated IP"| PM --> KiteAPI
  F1 --> P1 --> KiteAPI
  F2 --> P2 --> KiteAPI
  FN --> PN --> KiteAPI
  M -.->|"WS ticker: single process-wide connection, no per-account proxy"| KiteAPI

  PM -.-> SelfCheck
  P1 -.-> SelfCheck
  P2 -.-> SelfCheck
  PN -.-> SelfCheck
```

---

## 8. Data model relationships

```mermaid
erDiagram
  ACCOUNTS ||--o{ ACCOUNT_SESSIONS : "has sessions"
  ACCOUNTS ||--o| FOLLOW_LINKS : "is follower (PK)"
  ACCOUNTS ||--o{ FOLLOW_LINKS : "is master (FK)"
  ACCOUNTS ||--o{ MASTER_FILLS : "master_id"
  ACCOUNTS ||--o{ FOLLOWER_ORDERS : "follower_id"
  ACCOUNTS ||--o{ ORDER_EVENTS : "account_id"
  MASTER_FILLS ||--o{ FOLLOWER_ORDERS : "fans out to"
  FOLLOWER_ORDERS ||--o{ ORDER_EVENTS : "transition log"
  MASTER_FILLS ||--o{ ORDER_EVENTS : "signal_received"

  ACCOUNTS {
    uuid id PK
    account_role role
    text broker "adapter key, e.g. zerodha"
    text broker_user_id "broker's own client id"
    text egress_proxy_url
    text status
  }
  FOLLOW_LINKS {
    uuid follower_id PK "1 master per follower, structural"
    uuid master_id FK
    numeric capital_ratio
    boolean enabled
  }
  MASTER_FILLS {
    bigint id PK
    uuid master_id FK
    text broker_order_id "broker-native order id"
    text dispatch_state "pending|dispatched|dead"
  }
  FOLLOWER_ORDERS {
    bigint id PK
    bigint master_fill_id FK
    uuid follower_id FK
    text idempotency_tag UK
    text broker_order_id "broker-native order id"
    text terminal_status
  }
  ORDER_EVENTS {
    bigint id PK
    bigint follower_order_id FK
    text event_type
    jsonb payload
  }
```

`follow_links.follower_id` as primary key structurally enforces "one master per follower" with
no trigger or application check (PLAN.md §2). `order_events` is the append-only SEBI audit
trail; `master_fills` + `follower_orders` are derived, mutable projections.

**Broker-agnostic by design.** V1 only ever writes `broker = 'zerodha'`, but no table encodes
Kite-specific identifiers as first-class columns — `broker_user_id` and `broker_order_id` hold
whatever the adapter's native id looks like, and `raw_payload`/`payload` (jsonb) absorb the
rest. Adding a second broker means a new `internal/<broker>` package implementing the same
`kite.Broker` interface (PLAN.md §1) and a new `accounts.broker` value — no migration to the
core event-log tables.

---

## 9. Kill switch — check points

```mermaid
flowchart TD
  A[Global kill switch] -->|checked| B[engine, before fan-out]
  C[Per-account kill switch] -->|checked| D[worker, before each PlaceOrder]
  E[Admin dashboard] -->|engage/disengage| A
  E -->|engage/disengage| C

  F[TokenException 403] -->|auto-engage| C
  G[Egress IP mismatch] -->|auto-engage| C
  H[5 consecutive failures] -->|auto-engage| C
  I[Drift beyond hard threshold] -->|auto-engage| C
```

---

## 10. Deployment topology (V1)

```mermaid
flowchart TB
  subgraph Local["Local machine — operator's premises, systemd Restart=always"]
    Bin["envoytrade binary\n(all internal/* components in one process)"]
    PG[(PostgreSQL\nsame machine)]
    Bin <--> PG
  end

  Bin -->|REST, per-account proxy| Vendor["Proxy vendor\nalgoip.in / staticip.in\n1 dedicated IP per account + spares"]
  Vendor --> KiteAPI[(Kite Connect)]
  Bin -->|single WS connection, master only| KiteAPI

  Bin --> Secrets[(Secret manager\nKMS / sops+age)]
  Bin --> Webhook[Alert webhook\nSlack/Telegram]
  PG -.PITR + nightly dump.-> Backup[(Off-host backup\n5yr+ retention)]

  Cron1["07:00 IST: auth checklist"] -.-> Bin
  Cron2["07:30 IST: instrument sync"] -.-> Bin
  Cron3["15:35 IST: recon full pass"] -.-> Bin
```

No k8s, no containers, no microservices — one process, one database, one machine by design
(PLAN.md §10). Static proxy IPs (§7 above) are what satisfy Zerodha's whitelisting
requirement, so the service does not need to be network-close to the exchange — it runs
wherever the operator's machine is.

---

## 11. Cross-reference to PLAN.md

| Architecture section | PLAN.md source |
|---|---|
| §2 Component map | §1 Package/module layout |
| §3 Master signal capture | §3.0, §3.1, §3.4 |
| §4 Fan-out & dispatch | §4.1 Durable-write-then-dispatch, §4.2 Sizing, §4.3 Worker isolation |
| §5 Reconciliation & drift | §5 Failure handling |
| §6 Auth lifecycle | §3.2 Token lifecycle |
| §7 Egress isolation | §3.3 Static IP for REST |
| §8 Data model | §2 Data model |
| §9 Kill switch | §5 Kill switch |
| §10 Deployment | §8 Deployment |
