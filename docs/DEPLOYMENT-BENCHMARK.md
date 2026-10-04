# Deployment Benchmark & Decision Guide

Two ways to deploy EnvoyTrade:
1. **Single Embedded Binary** (Option 1): Native Go daemon with embedded SolidJS UI + native/local PostgreSQL.
2. **Docker Compose** (Option 2): 3 isolated containers (Headless Go API + Nginx Web UI + PostgreSQL 16).

---

## 1. Quick Decision Matrix (TL;DR)

| Your Situation | Recommended Choice | Why |
|---|---|---|
| **Budget VPS (1 GB – 2 GB RAM)** | **Single Binary** | Uses half the RAM (~90 MB total host RAM). Leaves memory for Postgres cache. |
| **Lowest Latency Trade Execution** | **Single Binary** | Direct IPC via Unix Domain Socket (`<0.15ms`). Bypasses Docker network bridge and proxy hops. |
| **Simple Systemd Management** | **Single Binary** | Single service file (`/etc/systemd/system/envoytrade.service`), standard logs in journald. |
| **Existing Docker / Container Stack** | **Docker Compose** | Standard `docker compose up -d`, zero host dependencies except Docker. |
| **Edge Caching / High Web Traffic** | **Docker Compose** | Nginx shields Go API. Caches static assets, gzip, and SSL termination at edge. |

---

## 2. Benchmark Results

Measured with 200 HTTP API & SPA requests under 10 concurrent connections on PostgreSQL 16 with canonical tuning (`shared_buffers = 1GB`, micro-autovacuum):

```
======================================================================================
                          BENCHMARK & FOOTPRINT ANALYSIS
======================================================================================

Metric                           | Single Binary (Option 1) | Docker Compose (Option 2)
--------------------------------------------------------------------------------------
Throughput (req/sec)             | 3,616.7 req/s (+14.8%)   | 3,151.0 req/s
Average Latency (ms)             | 2.59 ms                  | 2.89 ms
P50 Latency (ms)                 | 1.96 ms                  | 2.56 ms
P95 Latency (ms)                 | 4.53 ms                  | 5.23 ms
P99 Latency (ms)                 | 18.32 ms                 | 8.66 ms
Failed Requests                  | 0                        | 0
Sanity Integration Tests         | 8/8 Passed               | 8/8 Passed
======================================================================================
```

---

## 3. Real Host Memory Breakdown

`docker stats` hides host container runtime costs. Below is the **real host RAM** paid by your VPS:

```
======================================================================================
                        TRUE HOST MEMORY CONSUMPTION
======================================================================================
Component                        | Single Binary (Option 1) | Docker Compose (Option 2)
--------------------------------------------------------------------------------------
Application Processes            | ~27 MB (Go + Web + API)  | ~13 MB (API ~7MB + Web ~6MB)
PostgreSQL Database              | ~56 MB – 78 MB           | ~55 MB – 65 MB
Container Shims (3x)             | 0 MB (None)              | ~15 MB (containerd-shim)
Docker/Containerd Daemons        | 0 MB (None)              | ~90 MB (dockerd + containerd)
Network Bridge & OverlayFS Cache | 0 MB (Direct IPC/Loop)   | ~12 MB (veth + iptables + cache)
--------------------------------------------------------------------------------------
TOTAL HOST RAM USAGE             | ~83 MB – 105 MB          | ~185 MB – 225 MB
======================================================================================
```

> [!NOTE]
> Single Binary uses **~50% less total system RAM** than Docker Compose on a host machine.

---

## 4. In-Depth Comparison

### Option 1: Single Embedded Binary

**How it works:**
- Single compiled binary (`/usr/local/bin/envoytrade`, ~12 MB).
- SolidJS frontend built and embedded directly (`//go:embed`).
- On Linux VPS: Connects to local PostgreSQL via Unix Domain Socket (`host=/var/run/postgresql`).
- Managed by `systemd` (Linux) or `launchd` (macOS).

**Pros:**
- **Fastest IPC**: Unix domain socket eliminates TCP/IP stack overhead (`<0.15ms` query latency).
- **Lowest RAM**: ~90 MB total host memory with DB. Fits easily on $4/mo VPS (1 GB RAM).
- **Zero Dependencies**: Runs without Docker, containerd, or rootless daemons.
- **Atomic Rollback**: Replace one binary file, restart service.

**Cons:**
- Requires PostgreSQL installed natively on host (or running separately).
- Static assets served by Go web server instead of dedicated edge proxy.

---

### Option 2: Docker Compose (3 Containers)

**How it works:**
- `db`: PostgreSQL 16 container with tuned configuration.
- `api`: Headless Go backend container (0 UI bytes, stripped binary).
- `web`: Nginx container serving SolidJS static assets and reverse-proxying `/api/` & `/broker-callback`.

**Pros:**
- **One-Command Setup**: `make deploy-compose` starts everything (DB + API + Web) with zero host setup.
- **Headless API**: API binary is tiny (~8 MB) and contains zero frontend assets.
- **Nginx Edge Layer**: Nginx handles static file compression, keepalive pools, and rate limiting.
- **Strict Isolation**: Sandboxed namespaces, easy cleanup with `make decommission-compose`.

**Cons:**
- **Higher RAM Footprint**: Needs ~200 MB host RAM minimum due to Docker engine, shims, and container networking.
- **Slightly Higher Latency**: Extra TCP proxy hops (Client -> Nginx -> Bridge Network -> Go API -> Bridge Network -> DB).

---

## 5. How to Run the Benchmark Yourself

Run automated benchmark anytime:

```bash
make benchmark
# Or: ./scripts/benchmark_comparison.sh
```

**What the script does:**
1. Spins up Scenario 1 (Single Binary).
2. Executes 8 automated integration/sanity tests.
3. Fires 200 concurrent requests across 10 threads, logging latency & memory.
4. Spins down Scenario 1.
5. Launches Scenario 2 (Docker Compose stack).
6. Executes identical sanity and load tests.
7. Decommissions compose stack and restores local dev DB.
8. Prints side-by-side comparison table.
