# Fan-Out Mechanism V2 (Two-Tier Connection Pool & Low-Latency Copy Trading)

This document describes the V2 fan-out copy-trading pipeline in EnvoyTrade, designed for sub-25ms order placement across N follower accounts on a 2-core 8GB system.

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
        MF --> DBM["PostgreSQL: INSERT master_fills<br/>(Idempotency Guard)"]:::db
        DBM --> HMF["engine.HandleMasterFill()"]:::engine
    end

    %% 2. Engine Fan-Out & Sizing
    subgraph S2["2. Fan-Out & Sizing"]
        HMF --> CHK["Master Active?"]:::engine
        CHK -- Yes --> EFL["PostgreSQL: Fetch EnabledFollowLinks"]:::db
        
        EFL --> F1["Follower 1 Sizing<br/>domain.SizeOrder()"]:::engine
        EFL --> F2["Follower 2 Sizing<br/>domain.SizeOrder()"]:::engine
        EFL --> FN["Follower N Sizing<br/>domain.SizeOrder()"]:::engine

        F1 --> DB1["PostgreSQL: INSERT follower_orders<br/>(Idempotency Tag Persisted)"]:::db
        F2 --> DB2["PostgreSQL: INSERT follower_orders<br/>(Idempotency Tag Persisted)"]:::db
        FN --> DBN["PostgreSQL: INSERT follower_orders<br/>(Idempotency Tag Persisted)"]:::db

        DB1 --> DISP1["Non-Blocking Dispatch<br/>(item.ch <- job)"]:::engine
        DB2 --> DISP2["Non-Blocking Dispatch<br/>(item.ch <- job)"]:::engine
        DBN --> DISPN["Non-Blocking Dispatch<br/>(item.ch <- job)"]:::engine
    end

    %% 3. Worker Pool (Parallel Execution)
    subgraph S3["3. Dedicated Worker Goroutines (100% Concurrent)"]
        DISP1 -.-> W1["Worker Goroutine 1"]:::worker
        DISP2 -.-> W2["Worker Goroutine 2"]:::worker
        DISPN -.-> WN["Worker Goroutine N"]:::worker

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
    participant W as Worker Goroutines (Followers 1..N)
    participant BP as Tier 2: AccountPool (Cached Broker)
    participant TP as Tier 1: TransportPool (Warm TCP/TLS)
    participant P as Proxy & Broker Egress

    Note over K,L: 1. Ingestion Phase
    K->>L: Master Order Filled (COMPLETE)
    L->>DB: INSERT master_fills (idempotency guard)
    L->>E: HandleMasterFill(fill)

    Note over E,DB: 2. Fan-Out & Sizing Phase
    E->>DB: EnabledFollowLinks(masterID)
    loop For each Follower (1..N)
        E->>E: SizeOrder (ratio, lot sizes, caps)
        E->>DB: INSERT follower_orders (idempotency tag persisted)
        E->>W: dispatcher.Dispatch(followerID, job) [Microsecond Channel Send]
    end

    Note over W,P: 3. Parallel Execution Phase (100% Concurrent)
    par Follower 1
        W->>BP: ResolveBroker(Follower 1) [Cache Hit ~0ms]
        BP-->>W: Broker Instance (Ready)
        W->>TP: Get warm HTTP Client [Zero Handshake]
        W->>P: b.PlaceOrder() [15-25ms via Warm Keep-Alive Tunnel]
        P-->>W: Order Placed (OrderID)
        W->>DB: UpdateFollowerOrderPlaced & AppendOrderEvent
    and Follower 2
        W->>BP: ResolveBroker(Follower 2) [Cache Hit ~0ms]
        BP-->>W: Broker Instance (Ready)
        W->>TP: Get warm HTTP Client [Zero Handshake]
        W->>P: b.PlaceOrder() [15-25ms via Warm Keep-Alive Tunnel]
        P-->>W: Order Placed (OrderID)
        W->>DB: UpdateFollowerOrderPlaced & AppendOrderEvent
    and Follower N
        W->>BP: ResolveBroker(Follower N) [Cache Hit ~0ms]
        BP-->>W: Broker Instance (Ready)
        W->>TP: Get warm HTTP Client [Zero Handshake]
        W->>P: b.PlaceOrder() [15-25ms via Warm Keep-Alive Tunnel]
        P-->>W: Order Placed (OrderID)
        W->>DB: UpdateFollowerOrderPlaced & AppendOrderEvent
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

1. **Idempotency Persisted Before Network:**
   The `follower_orders` row with deterministic `idempotency_tag` is written to PostgreSQL **before** any broker dispatch. Redeliveries safely no-op without double-ordering.
2. **Microsecond Channel Dispatch:**
   `dispatcher.Dispatch` pushes to an in-memory buffered channel (`ch <- job`) and returns immediately. If a follower's channel is full, it fails fast and dead-letters without stalling fan-out to other followers.
3. **Dedicated Goroutines per Follower:**
   Worker goroutines execute 100% concurrently in parallel, each with its own egress proxy client and rate limiter (~8–10 orders/s).
4. **Two-Tier Cache:**
   - **Tier 1 (`TransportPool`):** Retains open TCP/TLS Keep-Alive sockets per egress proxy URL.
   - **Tier 2 (`AccountPool`):** Caches `broker.Broker` instances per account ID.
5. **Continuous Keep-Alive:**
   The Reconciler Poller (10s) and MTM Syncer (60s) keep the underlying sockets warm, avoiding the ~100ms cold handshake penalty when the master trades.
