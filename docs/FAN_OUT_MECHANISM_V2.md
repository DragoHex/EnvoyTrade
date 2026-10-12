# Fan-Out Mechanism V2.1 (Atomic UNNEST Bulk Insert & Two-Tier Connection Pool)

This document describes the V2.1 fan-out copy-trading pipeline in EnvoyTrade, designed for sub-millisecond in-memory engine fan-out and sub-25ms order placement across N follower accounts on a 2-core 8GB system.

---

## 1. End-to-End Fan-Out Flowchart

```mermaid
flowchart TD
    %% Styling
    classDef signal fill:#3b82f6,stroke:#1d4ed8,color:#ffffff,stroke-width:2px;
    classDef engine fill:#8b5cf6,stroke:#6d28d9,color:#ffffff,stroke-width:2px;
    classDef worker fill:#06b6d4,stroke:#0891b2,color:#ffffff,stroke-width:2px;
    classDef cache fill:#10b981,stroke:#047857,color:#ffffff,stroke-width:2px;
    classDef network fill:#f59e0b,stroke:#d97706,color:#ffffff,stroke-width:2px;
    classDef db fill:#64748b,stroke:#475569,color:#ffffff,stroke-width:2px;

    %% 1. Ingestion
    subgraph S1["1. Master Fill Ingestion"]
        WS["Kite WS Ticker / Postback"]:::signal --> MF["MasterFill Signal (COMPLETE)"]:::signal
        MF --> DBM["PostgreSQL: INSERT master_fills<br/>(Order Tx: sync_commit = off)"]:::db
        DBM --> HMF["engine.HandleMasterFill()"]:::engine
    end

    %% 2. Engine Fan-Out & Bulk Sizing
    subgraph S2["2. In-Memory Fan-Out & Atomic Bulk Insert"]
        HMF --> CHK["Master Active?"]:::engine
        CHK -- Yes --> EFL["PostgreSQL: Fetch EnabledFollowLinks"]:::db
        
        EFL --> LOT["sync.Map In-Memory Cache<br/>resolveLotSize(exchange, symbol)"]:::cache
        LOT --> SIZE["Pre-Size All Follower Orders in Memory<br/>domain.SizeOrder() for 1..N"]:::engine

        SIZE --> BULK["PostgreSQL: Single Atomic UNNEST Query<br/>INSERT INTO follower_orders ... SELECT * FROM UNNEST(...)<br/>ON CONFLICT (idempotency_tag) DO NOTHING<br/>(Order Tx: sync_commit = off, ~1-2ms total)"]:::db

        BULK --> DISP["In-Memory Non-Blocking Dispatch Loop<br/>(item.ch <- job for 1..N)"]:::engine
    end

    %% 3. Worker Pool (Parallel Execution)
    subgraph S3["3. Dedicated Worker Goroutines (100% Concurrent)"]
        DISP -.-> W1["Worker Goroutine 1"]:::worker
        DISP -.-> W2["Worker Goroutine 2"]:::worker
        DISP -.-> WN["Worker Goroutine N"]:::worker

        W1 --> T2_1["Tier 2: AccountPool<br/>(ResolveBroker Hit: ~0ms)"]:::cache
        W2 --> T2_2["Tier 2: AccountPool<br/>(ResolveBroker Hit: ~0ms)"]:::cache
        WN --> T2_N["Tier 2: AccountPool<br/>(ResolveBroker Hit: ~0ms)"]:::cache

        T2_1 --> T1_1["Tier 1: TransportPool<br/>(Pre-Connected TCP/TLS Socket)"]:::cache
        T2_2 --> T1_2["Tier 1: TransportPool<br/>(Pre-Connected TCP/TLS Socket)"]:::cache
        T2_N --> T1_N["Tier 1: TransportPool<br/>(Pre-Connected TCP/TLS Socket)"]:::cache
    end

    %% 4. Network Egress
    subgraph S4["4. Parallel Egress over Dedicated Static Proxies"]
        T1_1 --> P1["Dedicated Proxy 1<br/>b.PlaceOrder()"]:::network
        T1_2 --> P2["Dedicated Proxy 2<br/>b.PlaceOrder()"]:::network
        T1_N --> PN["Dedicated Proxy N<br/>b.PlaceOrder()"]:::network

        P1 --> Z1["Zerodha Kite Edge<br/>(~15–25ms Execution)"]:::network
        P2 --> Z2["Zerodha Kite Edge<br/>(~15–25ms Execution)"]:::network
        PN --> ZN["Zerodha Kite Edge<br/>(~15–25ms Execution)"]:::network
    end

    %% 5. Continuous Keep-Alive Backstop
    subgraph S5["5. Continuous Socket Warmer (Background)"]
        RP["Reconciler Poller (Every 10s)<br/>GET /orders"]:::signal -.-> T2_1 & T2_2 & T2_N
        MS["MTM Syncer (Every 60s)<br/>GET /positions"]:::signal -.-> T2_1 & T2_2 & T2_N
    end

    %% 6. Invalidation Hook
    subgraph S6["6. Safe Mutation Lifecycle"]
        REF["Token Refresh / PATCH Account"]:::signal --> EVICT["AccountPool.Evict(id)<br/>(Next call recreates broker with warm socket)"]:::cache
        EVICT -.-> T2_1 & T2_2 & T2_N
    end
```

---

## 2. Fan-Out Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    participant K as Zerodha / Master WS
    participant L as Listener (MasterFillConsumer)
    participant DB as PostgreSQL
    participant E as Engine (HandleMasterFill)
    participant C as In-Memory Lot Size Cache
    participant W as Worker Goroutines (Followers 1..N)
    participant BP as Tier 2: AccountPool (Cached Broker)
    participant TP as Tier 1: TransportPool (Warm TCP/TLS)
    participant P as Proxy & Broker Egress

    Note over K,L: 1. Ingestion Phase
    K->>L: Master Order Filled (COMPLETE)
    L->>DB: INSERT master_fills (Order Tx: sync_commit = off)
    L->>E: HandleMasterFill(fill)

    Note over E,DB: 2. Fan-Out & Bulk Sizing Phase
    E->>DB: EnabledFollowLinks(masterID)
    opt Scaled Ratios Exist
        E->>C: resolveLotSize(exchange, symbol) [sync.Map Cache Hit ~0ms]
    end
    E->>E: Pre-size all 1..N follower orders in memory
    E->>DB: INSERT follower_orders Batch (Single Atomic UNNEST Query, sync_commit = off)
    DB-->>E: Map of [idempotency_tag]orderID
    loop For each Follower (1..N)
        E->>W: dispatcher.Dispatch(followerID, job) [Microsecond Channel Send]
    end

    Note over W,P: 3. Parallel Execution Phase (100% Concurrent)
    par Follower 1
        W->>BP: ResolveBroker(Follower 1) [Cache Hit ~0ms]
        BP-->>W: Broker Instance (Ready)
        W->>TP: Get warm HTTP Client [Zero Handshake]
        W->>P: b.PlaceOrder() [15-25ms via Warm Keep-Alive Tunnel]
        P-->>W: Order Placed (OrderID)
        W->>DB: UpdateFollowerOrderPlaced & AppendOrderEvent (Order Tx: sync_commit = off)
    and Follower 2
        W->>BP: ResolveBroker(Follower 2) [Cache Hit ~0ms]
        BP-->>W: Broker Instance (Ready)
        W->>TP: Get warm HTTP Client [Zero Handshake]
        W->>P: b.PlaceOrder() [15-25ms via Warm Keep-Alive Tunnel]
        P-->>W: Order Placed (OrderID)
        W->>DB: UpdateFollowerOrderPlaced & AppendOrderEvent (Order Tx: sync_commit = off)
    and Follower N
        W->>BP: ResolveBroker(Follower N) [Cache Hit ~0ms]
        BP-->>W: Broker Instance (Ready)
        W->>TP: Get warm HTTP Client [Zero Handshake]
        W->>P: b.PlaceOrder() [15-25ms via Warm Keep-Alive Tunnel]
        P-->>W: Order Placed (OrderID)
        W->>DB: UpdateFollowerOrderPlaced & AppendOrderEvent (Order Tx: sync_commit = off)
    end

    Note over R,P: Continuous Keep-Alive Backstop
    loop Every 10s (Reconciler Poller) & Every 60s (MTM Syncer)
        R->>BP: ResolveBroker(Account) [Cache Hit]
        BP->>TP: Reuse warm TCP/TLS Socket
        TP->>P: GET /orders & Portfolio Sync [Keeps Sockets Hot 24/7]
    end
```

---

## 3. Core Architectural Guarantees

1. **Atomic Bulk Insertion via PostgreSQL `UNNEST`:**
   Instead of sequential per-follower database inserts, the engine pre-sizes all follower orders in RAM and persists them in a **single atomic PostgreSQL `UNNEST` query**. A master fill fanning out to 10–50 followers takes **1 database roundtrip** instead of N roundtrips.
2. **Transaction-Scoped `synchronous_commit = off`:**
   Order write transactions (`master_fills`, `follower_orders`, `order_events`, `pending_order_updates`) execute with `SET LOCAL synchronous_commit = 'off'`. WAL records write directly to RAM buffer cache and flush asynchronously, dropping database write latency from **2–5ms down to ~0.2ms** while preserving full ACID constraints. Accounts, credentials, and authentication tables remain on default `synchronous_commit = 'on'`.
3. **In-Memory Lot Size Cache:**
   Instrument lot sizes are cached in a thread-safe `sync.Map` (`EXCHANGE:TRADINGSYMBOL`), purged daily at **01:00:00 IST**. Eliminates redundant database roundtrips on the execution hot path.
4. **Idempotency Persisted Before Network:**
   The `follower_orders` row with deterministic `idempotency_tag` is written to PostgreSQL **before** any broker dispatch. Redeliveries safely no-op via `ON CONFLICT (idempotency_tag) DO NOTHING`.
5. **Microsecond Channel Dispatch:**
   `dispatcher.Dispatch` pushes to an in-memory buffered channel (`ch <- job`) and returns immediately. If a follower's channel is full, it fails fast and dead-letters without stalling fan-out to other followers.
6. **Dedicated Goroutines per Follower:**
   Worker goroutines execute 100% concurrently in parallel, each with its own egress proxy client and rate limiter (~8–10 orders/s).
7. **Two-Tier Cache:**
   - **Tier 1 (`TransportPool`):** Retains open TCP/TLS Keep-Alive sockets per egress proxy URL.
   - **Tier 2 (`AccountPool`):** Caches `broker.Broker` instances per account ID.
8. **Continuous Keep-Alive:**
   The Reconciler Poller (10s) and MTM Syncer (60s) keep the underlying sockets warm, avoiding the ~100ms cold handshake penalty when the master trades.
