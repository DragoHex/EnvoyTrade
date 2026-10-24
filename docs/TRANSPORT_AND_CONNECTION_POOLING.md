# Broker Transport & Persistent Connection Pooling Architecture

This document describes the transport layer, per-account proxy egress isolation, two-tier connection pooling, and continuous socket warming backstop in EnvoyTrade.

---

## 1. Overview & Operational Requirements

In automated copy-trading, placing follower orders with sub-25ms execution latency requires eliminating network handshakes on the hot path:
- **Cold TCP 3-Way Handshake + TLS 1.3 Negotiation**: Adds 60–120ms of latency per order if a fresh socket is opened when a trade signal arrives.
- **Regulatory Egress Isolation**: Brokers (e.g. Zerodha Kite, Kotak Neo) mandate static IP whitelisting per client app. Each follower account must route outbound requests through its own assigned static-IP proxy.
- **Active Connection Guarantee**: Upstream NAT firewalls, proxy gateways, and cloud load balancers drop idle TCP connections after 30–60 seconds of silence.

To solve this, EnvoyTrade implements a **Two-Tier Architecture** where every customer account reuses a single, long-lived `*http.Client`, and periodic reconciliation acts as a connection warmer to keep TCP sockets continuously open 24/7.

---

## 2. Two-Tier Pooling Architecture

```mermaid
flowchart TD
    subgraph Callers["System Callers"]
        W["Worker Goroutines (1..N)<br/>b.PlaceOrder()"]
        R["Reconciliation Poller (10s)<br/>b.GetOrders()"]
        SO["SquareOff Service<br/>b.CancelOrder()"]
        RB["Rebalance Service<br/>b.GetPositions()"]
    end

    subgraph Tier2["Tier 2: broker.AccountPool (Client Instance Layer)"]
        AP["AccountPool (sync.RWMutex)<br/>Keyed by accountID (UUID)"]
        B1["Master Broker Instance<br/>(*kiteconnect.Client / Broker)"]
        B2["Follower 1 Broker Instance<br/>(Dedicated *http.Client)"]
        BN["Follower N Broker Instance<br/>(Dedicated *http.Client)"]
    end

    subgraph Tier1["Tier 1: broker.TransportPool (Socket & TLS Layer)"]
        TP["TransportPool (sync.RWMutex)<br/>Keyed by Proxy URL"]
        TR1["*http.Transport 1<br/>MaxIdleConnsPerHost: 4<br/>IdleConnTimeout: 90s"]
        TR2["*http.Transport 2<br/>MaxIdleConnsPerHost: 4<br/>IdleConnTimeout: 90s"]
        TRN["*http.Transport N<br/>MaxIdleConnsPerHost: 4<br/>IdleConnTimeout: 90s"]
    end

    subgraph Egress["Egress & Proxies"]
        P1["Dedicated Static Proxy 1<br/>(Customer 1 IP)"]
        P2["Dedicated Static Proxy 2<br/>(Customer 2 IP)"]
        PN["Dedicated Static Proxy N<br/>(Customer N IP)"]
    end

    subgraph Edge["Broker Edge Gateways"]
        K1["Broker API Edge"]
        K2["Broker API Edge"]
        KN["Broker API Edge"]
    end

    Callers --> AP
    AP --> B1 & B2 & BN
    B1 --> TR1 --> P1 --> K1
    B2 --> TR2 --> P2 --> K2
    BN --> TRN --> PN --> KN
```

---

### Tier 1: Socket & TLS Pool (`broker.TransportPool`)
Located in [`internal/broker/transport.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/broker/transport.go).

* **Keying**: Keyed strictly by the account's proxy connection string (`scheme://user:pass@host:port`).
* **Persistent Transports**: Maintains cached `*http.Transport` instances. In Go's runtime, `http.Transport` owns the underlying TCP socket connection pool, HTTP keep-alive tables, and TLS session tickets.
* **Socket Tuning Parameters**:
  * `IdleConnTimeout: 90 * time.Second`: Sockets remain idle in the pool for up to 90 seconds before Go considers closing them.
  * `MaxIdleConnsPerHost: 4`: Ensures up to 4 idle persistent TCP connections are kept open per proxy host for immediate burst availability.
  * `TLSClientConfig: &tls.Config{InsecureSkipVerify: false}`: Enforces strict TLS certificate verification over proxy tunnels.
* **Multi-Broker Reusability**: Extracted from Kite-specific packages into the generic `internal/broker` layer so Kotak Neo, Dhan, Symphony, and Zerodha adapters share identical transport caching mechanics.

---

### Tier 2: Account Instance Pool (`broker.AccountPool`)
Located in [`internal/broker/pool.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/broker/pool.go).

* **Keying**: Keyed by customer `accountID` (`uuid.UUID`).
* **Single Client per Customer**: All subsystems (Worker Pool order execution, Reconciler status poller, MTM Syncer, Rebalance, Square-Off) call:
  ```go
  b, err := accountPool.ResolveBroker(ctx, accountID)
  ```
* **Thread-Safety**: Uses double-checked `sync.RWMutex` locking to guarantee that concurrent callers receive the same broker adapter and `*http.Client` without duplicate construction or race conditions.
* **Safe Invalidation Hook**: On daily interactive 2FA, token expiration, or credential rotation:
  ```go
  accountPool.Evict(accountID)
  ```
  This discards the stale broker instance while preserving Tier 1's underlying warm TCP/TLS socket in `TransportPool`. The next call recreates the broker client on top of the already-warm connection.

---

## 3. Continuous Socket Warming via Reconciler Heartbeat

A persistent transport is only as good as its activity; if no trades occur for several minutes, intermediate NAT gateways drop the connection table entries.

EnvoyTrade prevents socket death by turning the **Reconciliation Poller** into an automated connection warmer:

```mermaid
sequenceDiagram
    autonumber
    participant R as Reconciler Poller (recon.Poller)
    participant AP as AccountPool (Tier 2)
    participant TP as TransportPool (Tier 1)
    participant P as Customer Static Proxy
    participant B as Broker REST API

    Note over R: Poll Interval: 10 Seconds<br/>(Idle Timeout: 90 Seconds)

    loop Every 10 Seconds
        R->>AP: ResolveBroker(masterID)
        AP->>TP: Get cached *http.Transport
        TP->>P: Reused TCP Socket: GET /orders
        P->>B: HTTP Keep-Alive Request
        B-->>R: Order Book (200 OK)
        Note over TP,B: TCP Keep-Alive Timer Reset to 90s

        loop For each Active Follower
            R->>AP: ResolveBroker(followerID)
            AP->>TP: Get cached *http.Transport
            TP->>P: Reused TCP Socket: GET /orders
            P->>B: HTTP Keep-Alive Request
            B-->>R: Order Book (200 OK)
            Note over TP,B: TCP Keep-Alive Timer Reset to 90s
        end
    end
```

### Heartbeat Timing Matrix
* **Reconciler Interval**: `PollInterval: 10 * time.Second` ([`recon.DefaultConfig`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/recon/reconciler.go#L84-L90)).
* **Transport Idle Timeout**: `IdleConnTimeout: 90 * time.Second` ([`broker.TransportPool`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/broker/transport.go#L87)).
* **Safety Margin**: **9x frequency multiplier** ($90s / 10s = 9$). Every customer's proxy TCP socket sends and receives HTTP traffic at least 9 times before Go's transport or the proxy's keep-alive window can expire.

---

## 4. Latency Impact: Cold vs Warm Hot Path

```
Cold Connection (No Pooling):
[Resolve Account DB: ~2ms] ──> [TCP SYN/ACK: ~30ms] ──> [TLS 1.3 Handshake: ~45ms] ──> [HTTP POST Order: ~25ms]
Total Latency: ~102ms

Warm Pooled Connection (EnvoyTrade Hot Path):
[AccountPool Hit: ~0ms] ──> [Warm Socket HTTP POST Order: ~18ms]
Total Latency: ~18ms (82% reduction)
```

---

## 5. Key Source Code References

* [`internal/broker/transport.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/broker/transport.go): `ProxyConfig`, `TransportPool`, `RESTClientFor`
* [`internal/broker/pool.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/broker/pool.go): `AccountPool`, `ResolveBroker`, `Evict`
* [`internal/recon/reconciler.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/recon/reconciler.go): `Poller.Run`, `ReconcileMaster`, `ReconcileFollower`
* [`cmd/server/main.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/cmd/server/main.go): Instantiation of `accountPool` and unified distribution across `workerPool`, `poller`, `squareoff`, and `rebalance`.
