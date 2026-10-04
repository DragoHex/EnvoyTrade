# Proxy IP Management, Strict 1:1 Assignment & Broker Egress Plan

## 1. Context & Objectives

Zerodha Kite Connect enforces strict static IP whitelisting for trading accounts using their API. Every Kite Connect application must place orders from an IP pre-registered in Zerodha's developer console.

This specification establishes:
1. A dedicated `proxy_ips` table in PostgreSQL storing static proxy configurations with the **IP address as the primary key**.
2. Omission of `whitelisted_brokers` in the DB for now.
3. Strict **1-to-1 account-to-IP assignment** enforced via a partial unique index in PostgreSQL (`idx_accounts_ip_address_unique`) and API validation.
4. Seeding the 4 initial AlgoIP proxies (two IPv4 and two IPv6).
5. UI updates in `AccountDetailDrawer` to present an IP Type selector (`IPv4` / `IPv6` for followers; `NA` / `IPv4` / `IPv6` for masters) with automatic reservation of an available IP of that type.
6. Broker adapter integration in `internal/kite` and `cmd/server/main.go` ensuring live REST calls, headless authentication, and portfolio sync route through the account's assigned proxy.

---

## 2. Seed Proxy IP Inventory

All initial proxies are provisioned via **`dc46-mum-01.algoip.in:443`** with validity `2026-09-29 19:35:15 IST` to `2026-12-29 19:35:15 IST` on plan `QUARTERLY`:

| IP Address | Version | Username | Password |
|---|---|---|---|
| `2402:1f00:8302:91e6:6d08:9249:eca8:8252` | `ipv6` | `aip_live_1lsqjsm823jhpkqw` | `aip_sec_xdyvcubp4z2jzvledwmdro2l8t4ckaxc` |
| `2402:1f00:8302:9102:794b:a90f:c2bb:9293` | `ipv6` | `aip_live_4ubp4mve0b5fzbkd` | `aip_sec_ic9rfljxngjh27gbi5my4lyj0qpqcs2l` |
| `148.113.41.42` | `ipv4` | `aip_live_trnbam1he8ndlerw` | `aip_sec_qxgf0tu7317loli1p44s1d12y4dfgiyh` |
| `148.113.41.41` | `ipv4` | `aip_live_uco8bc0guf0cj9wu` | `aip_sec_ipyxzu2mrixua0j0lh76ubu5ec66p66o` |

---

## 3. Database Architecture

### Migration `0014_proxy_ips.sql`
```sql
CREATE TABLE IF NOT EXISTS proxy_ips (
    ip_address  TEXT PRIMARY KEY,
    ip_type     TEXT NOT NULL CHECK (ip_type IN ('ipv4', 'ipv6')),
    host        TEXT NOT NULL,
    port        INTEGER NOT NULL DEFAULT 443,
    username    TEXT NOT NULL,
    password    TEXT NOT NULL,
    valid_from  TIMESTAMPTZ NOT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    plan        TEXT NOT NULL DEFAULT 'QUARTERLY',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Strict 1:1 account-to-IP assignment constraint.
-- Allows multiple accounts with empty string (NA), but guarantees any non-empty IP is unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_ip_address_unique
ON accounts (ip_address)
WHERE ip_address IS NOT NULL AND ip_address != '';

-- Seed initial proxy IPs (example structure; loaded in production via PROXY_IPS environment variable)
INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, valid_from, valid_until, plan)
VALUES
    ('148.113.41.41', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'aip_live_example', 'aip_sec_example', '2026-09-29 19:35:15+05:30', '2026-12-29 19:35:15+05:30', 'QUARTERLY')
ON CONFLICT (ip_address) DO NOTHING;
```

---

## 4. Backend Store & Domain Layer

1. **Domain Model (`internal/domain/proxy.go`)**:
   ```go
   type ProxyIP struct {
       IPAddress           string
       IPType              string // "ipv4" | "ipv6"
       Host                string
       Port                int
       Username            string
       Password            string
       ValidFrom           time.Time
       ValidUntil          time.Time
       Plan                string
       IsAssigned          bool
       AssignedAccountID   *uuid.UUID
       AssignedAccountName *string
   }
   ```
2. **Store Methods (`internal/store/postgres`)**:
   - `ListProxyIPs(ctx context.Context) ([]domain.ProxyIP, error)`
   - `AvailableProxyIPs(ctx context.Context, ipType string, excludeAccountID *uuid.UUID) ([]domain.ProxyIP, error)`
   - `ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error)`

---

## 5. Broker Egress Integration (`internal/kite`)

1. **Proxy Transport (`internal/kite/proxy.go`)**:
   - `RESTClientFor(cfg ProxyConfig) (*http.Client, error)` sets `TLSClientConfig: &tls.Config{InsecureSkipVerify: false}` on `http.Transport`.
2. **Live Broker Factory (`internal/kite/broker.go`)**:
   - `NewLiveBroker(apiKey, accessToken string, proxyCfg *ProxyConfig) (*Broker, error)`
   - When an account has live credentials and an assigned IP:
     1. Fetches `proxy_ips` matching `accounts.ip_address`.
     2. Calls `kite.RESTClientFor(proxyCfg)`.
     3. Injects the client into `kc.SetHTTPClient(client)`.
     4. Wraps in `kite.NewBroker(kc)`.
   - Unauthenticated/test accounts continue falling back to `fake.Broker` in development.
3. **Headless Login & Portfolio Syncer (`internal/kite/sync.go`)**:
   - When running headless login or sync, routes requests through the account's assigned proxy transport.

---

## 6. HTTP API (`internal/httpapi`)

Contract specified in [`docs/APIs/proxy_ips.md`](./APIs/proxy_ips.md):
- `GET /api/v1/proxy-ips`: Full inventory and assignment status.
- `GET /api/v1/proxy-ips/available?accountId=<uuid>`: Available IPs for allocation.
- `POST /api/v1/accounts`: Validates IP existence, requires IP for followers, enforces 1:1 uniqueness (`409 Conflict` if collision).
- `PATCH /api/v1/accounts/{id}`: Reallocates or updates IP assignment safely.

---

## 7. Frontend UI (`frontend/src`)

1. **API Client (`frontend/src/api.ts`)**:
   - Types `ProxyIP` and methods `fetchProxyIPs()`, `fetchAvailableProxyIPs(accountId?)`.
2. **Account Detail Drawer (`frontend/src/components/AccountDetailDrawer.tsx`)**:
   - IP Type Dropdown:
     - **Follower**: `IPv4`, `IPv6` (mandatory).
     - **Master**: `NA`, `IPv4`, `IPv6` (defaults to `NA`).
   - Auto-Assignment:
     - Selecting `IPv4` or `IPv6` automatically assigns the first available IP of that type.
     - If all IPs of that type are occupied, shows inline alert: `"All IPv4 addresses are currently assigned to other accounts."`
     - Selecting `NA` (master) clears the IP (`""`).
   - Visual Display:
     - Renders assigned IP in a clear badge / read-only display: `Assigned Static IP: <ipAddress>`.
