# EnvoyTrade

EnvoyTrade is a high-reliability, low-latency copy-trading platform built for Zerodha Kite Connect. It synchronizes trades from a designated master trading account to multiple follower accounts in real time using a strictly enforced 1-master-to-N-followers topology (at most one master per follower).

The system is designed as a single, self-contained Go binary backed by PostgreSQL, paired with a fine-grained, reactive SolidJS web dashboard for monitoring positions, orders, holdings, and trade execution.

```mermaid
flowchart TD
    subgraph SignalIngestion [Signal Ingestion]
        KT["Kite Ticker WS (Master Fast-Path)"]
        KP["Kite HTTP Postbacks (Master & Followers)"]
        Q["In-Memory Dispatch Queue (memchan)"]
        KT --> Q
        KP --> Q
    end

    subgraph CoreEngine [Core Engine & Sizing]
        L["Ingestion Consumer (listener)"]
        DB[("PostgreSQL Store")]
        ENG["Execution Engine"]
        
        Q --> L
        L --> DB
        L --> ENG
        DB -.->|"Canonical Lot Sizes & Follow Links"| ENG
    end

    subgraph ExecutionWorkers [Isolated Worker Pool]
        W1["Follower Worker 1 (Goroutine + Rate Limiter)"]
        W2["Follower Worker 2 (Goroutine + Rate Limiter)"]
        WN["Follower Worker N (Goroutine + Rate Limiter)"]
        
        ENG -->|"Non-blocking Dispatch"| W1
        ENG -->|"Non-blocking Dispatch"| W2
        ENG -->|"Non-blocking Dispatch"| WN
    end

    subgraph BrokerEgress [Broker Egress]
        BR1["Kite REST Client (Proxy IP 1)"]
        BR2["Kite REST Client (Proxy IP 2)"]
        BRN["Kite REST Client (Proxy IP N)"]
        
        W1 --> BR1
        W2 --> BR2
        WN --> BRN
    end

    subgraph Backstop [State Reconciliation]
        REC["Periodic Reconciler"] -.->|"Drift Detection"| DB
        REC -.->|"Kite Order Status Check"| BR1
    end
```

---

## Key Guarantees & Design Principles

- **Database-Enforced Idempotency**: Master fills and follower orders carry deterministic idempotency tags (`IdempotencyTag(masterFillID, followerID)`). Database unique constraints guarantee that duplicate signals from concurrent WebSocket tickers, postback retries, or server restarts are recognized as safe no-ops rather than placing duplicate orders.
- **Canonical Lot Sizing**: Lot sizes are never trusted from signal payloads. Sizing logic looks up canonical exchange lot sizes directly from the `instruments` table. Sizing calculation uses exact decimal arithmetic (`shopspring/decimal`, zero floating-point math), floors to valid lot multiples, and caps orders to configured risk limits (`max_qty_per_order`). Sizing results (including zero-quantity skips) are fully audited in the database.
- **Worker Isolation**: Each follower operates within its own dedicated goroutine and buffered dispatch channel. Dispatch is non-blocking: if a follower's queue fills up or its broker connection hangs, it is marked as dead-lettered without delaying or blocking execution for other followers.
- **Dedicated Egress Proxy Support**: Egress requests to Kite Connect can be routed through per-account HTTP proxies, satisfying broker static-IP authorization requirements.
- **Dual-Path Ingestion with Reconnect Catch-Up**: Trades are ingested via Kite WebSocket ticker (for sub-millisecond master detection) and HTTP postback webhooks (with HMAC verification per account). On WebSocket reconnects, an automated catch-up hook queries the Kite REST API to ingest any executions missed during disconnection.
- **Reconciliation Backstop**: A background reconciler periodically audits order and position states between the local database and Kite Connect to resolve drift and detect out-of-band updates.

---

## User Interface Tour

The frontend is built with SolidJS, leveraging fine-grained signals for targeted DOM updates without unnecessary component re-renders. It includes complete dark and light themes matching the dark slate and mint accent palette (`#44475b`, `#04b488`, `#06d896`).

### 1. Dashboard (`/`)

The primary operations center for copy-trading. The Dashboard displays trading groups organized by master account and provides real-time portfolio metrics, connection health, and administrative controls.

<p align="center">
  <img src="ui_scs/Dashboard.png" alt="EnvoyTrade Dashboard" width="900" />
</p>

- **Multi-Group Hierarchy**: Groups follower accounts under their respective master account headers (e.g., `MASTER01`, `MASTER02`) with real-time group-level status indicators.
- **Master Account Rollup**: Displays master account status, aggregated MTM (Mark-to-Market), net positions, active order count, available cash, and available margin.
- **Follower Account Rows**: Each follower displays its individual connection status, current MTM, balance, and a responsive **Copy Toggle** switch to enable or pause trade replication.
- **Real-Time Account Metrics**: Columns for `Net Qty`, `Positions (O/C)`, `Open Orders`, `Total MTM`, `Available Cash`, and `Available Margin`.
- **Emergency Action Controls**:
  - **Refresh**: Refreshes account metrics and balances on demand.
  - **Square Off**: Closes open intraday and delivery positions across followers (gated behind confirmation modal).
  - **Exit Open Orders**: Cancels all outstanding limit and trigger orders.
  - **Expand Drawer**: Chevrons on each account row expand an inline drawer detailing positions, orders, and holdings.

---

### 2. Account Orders & Portfolio Drawer

Accessible from the Dashboard by expanding any account row, the Account Drawer provides an in-depth view of trading activity for any selected master or follower account. The tables use a fixed layout (`table-layout: fixed`) with strict column widths to eliminate visual flickering and layout shifting across pagination and tab changes.

<p align="center">
  <img src="ui_scs/Holdings_Positions.png" alt="EnvoyTrade Account Drawer - Open Positions" width="900" />
</p>

- **Account Summary Bar**: Highlights live account-level metrics at a glance:
  - `Net Qty` (e.g. `-890`)
  - `Open / Closed Positions` (e.g. `8 / 2`)
  - `Pending Orders` (e.g. `0`)
  - `Total MTM` (color-coded, e.g. `₹380.00`)
  - `Realized P&L` (e.g. `₹0.00`)
  - `Margin / Value` (total portfolio equity, e.g. `₹21,63,520.84`)
  - Live connection status dot (`online`)
- **Sub-Tabs Filter**:
  - **Open Position**: Live intraday and overnight positions with product type (`CNC`, `NRML`, `MIS`), instrument symbol, net quantity, average buy/sell prices, LTP, live MTM, and row-level exit action.
  - **Closed Position**: Completed intraday positions with realized profit/loss.
  - **Holding**: Long-term demat equity holdings, sellable quantity, buy average, LTP, and P&L.
  - **Open Order**: Pending limit and trigger orders with trigger/limit prices and cancellation controls.
  - **Closed Order**: Completed executions with timestamps, quantities, prices, and execution types (`B`/`S`).
  - **Rejected Order**: Rejected orders with broker and exchange rejection reasons.
- **Stable Pagination**: In-table pagination controls ("Prev", "Page X of Y", "Next") with fixed table headers that remain visible and static even during loading or empty states.

---

### 3. Accounts Directory (`/accounts`)

The central directory for registering and managing trading accounts across Master and Follower roles.

<p align="center">
  <img src="ui_scs/Accounts.png" alt="EnvoyTrade Accounts Management" width="900" />
</p>

- **Role Visualization**: Distinguishes Master accounts (wizard icon) from Follower accounts (monkey icon).
- **Broker Identifiers & Badges**: Displays broker user IDs (`MASTER01`, `FOLLOW02A`, etc.) alongside broker visual branding.
- **Group Assignment**: Shows the assigned master group for each follower account.
- **Risk Configuration**: Displays configured `Capital Ratio` multipliers (e.g., `0.5`, `1.0`, `0.75`) and `Max Qty/Order` limits.
- **Account Controls**: Individual enable/disable toggles, edit modal, delete action, and follower unlink capability.
- **Add Account**: Modal workflow for onboarding new broker accounts and configuring credentials.

---

### 4. Group Management (`/accounts` - Groups View)

The Groups view provides group-level governance over trade copy links between master accounts and their followers.

<p align="center">
  <img src="ui_scs/Groups.png" alt="EnvoyTrade Groups Management" width="900" />
</p>

- **Master Group Overview**: Lists all trade groups with assigned Master account, broker badge, and total linked follower count.
- **Connection Status**: Real-time status indicators for master account connectivity.
- **Create Group**: Interactive modal to assemble new master-follower trading clusters.
- **Group Configuration**: Direct navigation to manage follower link parameters (`capital_ratio`, `max_qty_per_order`, active toggle) per group.

---

### 5. Analytics (`/analytics`)

*Note: The Analytics module (PnL equity curves, win/loss streak strips, slippage analysis, and trade log reporting) is under active development and planned for an upcoming milestone.*

---

## Technology Stack

| Layer | Technologies |
|---|---|
| **Backend** | Go 1.22+, `net/http`, `log/slog` |
| **Database** | PostgreSQL 15+, `pgx/v5`, `sqlc` query generation |
| **Broker SDK** | Zerodha Kite Connect SDK (`github.com/zerodha/gokiteconnect/v4`) |
| **Math & Precision** | `shopspring/decimal` for financial and ratio calculations |
| **Frontend** | SolidJS 1.9+, Solid Router, TypeScript, Vite |
| **Styling** | Native CSS Variables, responsive theme provider (light & dark mode) |
| **Testing** | Go standard testing, `testcontainers-go` (PostgreSQL), Vitest, Solid Testing Library |

---

## Getting Started

### Prerequisites

- **Go**: Version 1.22 or higher
- **Node.js**: Version 18+ (or Node 20+)
- **PostgreSQL**: Version 15+ (or Podman / Docker for running local containers)
- **Zerodha Kite Connect Account**: API Key and Secret for live broker connectivity

---

### 1. Database Setup

Create a PostgreSQL database and configure the connection string:

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/envoytrade?sslmode=disable"
```

Database migrations are embedded in the Go binary (`internal/store/postgres/migrations/`) and are applied automatically upon server startup.

---

### 2. Running the Backend Server

Start the core server daemon:

```bash
# Compile and run the server daemon
go run cmd/server/main.go
```

By default, the HTTP API server starts on `http://localhost:8080`.

---

### 3. Running the Frontend

Navigate to the `frontend/` directory, install dependencies, and launch the Vite development server:

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173` in your browser to access the EnvoyTrade dashboard.

---

### 4. Running Tests

#### Backend Unit Tests (No external dependencies)
```bash
go test ./...
go vet ./...
```

#### Backend Integration Tests (Requires Podman/Docker for PostgreSQL testcontainers)
```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test -tags integration ./...
```

#### Frontend Test Suite
```bash
cd frontend
npm test
npm run build
```

---

## Repository Structure

```
.
├── cmd/
│   └── server/                 # Server daemon entrypoint and process wiring
├── docs/
│   ├── APIs/                   # REST API endpoint specifications
│   ├── ARCHITECTURE.md         # Architecture diagrams and system design
│   ├── PLAN.md                 # Full V1 implementation plan
│   ├── SCHEMA.md               # PostgreSQL schema and data relationships
│   └── UI-PLAN.md              # Frontend UI specifications and component design
├── frontend/                   # SolidJS frontend application
│   ├── src/
│   │   ├── components/         # Reusable UI components (AccountOrderDetails, AccountTable, Modals)
│   │   ├── pages/              # Top-level route pages (Dashboard, Accounts, Groups, Analytics)
│   │   └── theme/              # Theme provider and color definitions
├── gokiteconnect/              # Vendored Zerodha Kite Connect SDK
├── internal/
│   ├── broker/                 # Broker interface and order parameters
│   ├── domain/                 # Domain types, order sizing math, and sentinel errors
│   ├── engine/                 # Fan-out execution engine and dispatch decisions
│   ├── httpapi/                # HTTP API router and account order endpoints
│   ├── kite/                   # Kite broker adapter, proxy transport, and callback ingestion
│   ├── listener/               # Order update and master fill consumers
│   ├── queue/                  # Queue abstraction and in-memory channel implementation
│   ├── recon/                  # Background state reconciliation engine
│   ├── store/                  # PostgreSQL store implementation and sqlc generated queries
│   └── worker/                 # Isolated per-follower goroutine workers and rate limiter
└── ui_scs/                     # UI screenshots and visual references
```

---
