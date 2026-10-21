# Implementation Plan: EnvoyTrade Admin Portal, IP Expiry Tracking & Operational Controls

## Goal Description
Build an institutional-grade, low-overhead Admin Portal inside EnvoyTrade:
1. **IP Pool & Expiry Lifecycle Management**:
   - Bulk and single IP ingestion supporting the [`ips.json`](../../ips.json) schema, normalization, and non-destructive upserts.
   - **Real-time Expiry Tracking & Renewal**: Calculate days remaining, classify status (`active`, `expiring_soon`, `expired`), alert on assigned IPs nearing expiration, and support inline/bulk renewals.
   - On-demand single-IP proxy connectivity and latency testing (3s timeout).
2. **User & Tenant Management**: Decouple admin users from broker account mandates; support user listing with broker stats, role modification (`admin` vs `user`), active session revocation, and account activation/deactivation.
3. **Service Status & Emergency Controls**: 100% in-memory, lock-free pipeline telemetry (WebSocket tickers, `memchan` queue depths, worker counts, `pgxpool.Stat()`) fetched **strictly on-demand** (no periodic polling), and a zero-latency Global Emergency Kill Switch.
4. **Protected SolidJS UI**: Dedicated `/admin/*` views with role-guarded access, responsive design, dark/light theme support, manual Refresh button, and **zero background polling**.
5. **Rigorous TDD Test Suite**: Comprehensive unit and integration test coverage for all positive, negative, and boundary conditions.

---

## Performance & Starvation Guardrails

> [!IMPORTANT]
> **Zero Periodic Polling on Admin Portal (Strictly On-Demand)**:
> The admin portal has **no automatic polling or timers** (`setInterval` is completely banned). Data is fetched:
> - Once upon navigating to a page.
> - On explicit user interaction via a **manual "Refresh" button**.
> This guarantees **zero idle CPU, zero recurring network traffic, and zero DB contention** while an admin tab sits open in the background.

> [!IMPORTANT]
> **Zero Stop-The-World (STW) Pauses**:
> All Go runtime memory inspection (`runtime.ReadMemStats`) is **completely banned**. It triggers stop-the-world pauses that inject millisecond-level latency jitter into copy-trade order fan-out.

> [!IMPORTANT]
> **Zero DB Queries on Status Telemetry (`GET /api/v1/admin/status`)**:
> The status endpoint is **100% in-memory and lock-free**. It performs **zero database queries**:
> - Ticker status: in-memory state.
> - Queue depths: `len(chan)` atomic read.
> - Worker count: atomic integer.
> - DB pool health: `pgxpool.Stat()` (reads in-memory connection counters, 0 SQL queries).
> - Kill switch state: `atomic.Bool`.

> [!NOTE]
> **2-Core VPS Resource Guardrails**:
> - Proxy connectivity testing is strictly single-IP on-demand with a hard 3-second timeout.
> - User management queries are strictly paginated (`LIMIT 20`) to prevent table-scan spikes.

---

## Proposed Changes

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                 ROUTING & SECURITY                               │
│  authMiddleware → RequireAdminMiddleware → /api/v1/admin/*                       │
├────────────────────────┬────────────────────────┬────────────────────────────────┤
│ 1. IP & Expiry Mgmt    │ 2. User Management     │ 3. Service Status & Controls   │
│  - GET /admin/proxy-ips│  - GET /admin/users    │  - GET /admin/status           │
│  - POST /admin/proxy-  │    (paginated, 20 max) │    (100% RAM, on-demand only,  │
│    ips/bulk (ips.json) │  - POST /admin/users   │     NO background polling)     │
│  - POST /admin/proxy-  │  - PATCH /admin/users/ │  - POST /admin/killswitch      │
│    ips (single)        │    {id} (role/active)  │    (atomic pause + reason)     │
│  - PATCH /admin/proxy- │  - GET /admin/users/   │                                │
│    ips/{ip} (edit)     │    {id}/sessions       │                                │
│  - DELETE /admin/proxy-│  - DELETE /admin/      │                                │
│    ips/{ip} (409-check)│    users/{id}/sessions │                                │
│  - POST /admin/proxy-  │                        │                                │
│    ips/{ip}/test       │                        │                                │
└────────────────────────┴────────────────────────┴────────────────────────────────┘
```

---

### Component 1: Domain & Store Layer (IP Expiry & Management)

#### [MODIFY] `internal/domain/proxy.go`
- Add normalization and validation methods for proxy IPs.
- Add helper to parse IST timestamps (`"2006-01-02 15:04:05 (MST)"`) alongside RFC3339.
- Add dynamic expiry status and days remaining calculations:
  ```go
  type ExpiryStatus string

  const (
      ExpiryActive       ExpiryStatus = "active"        // > 15 days remaining
      ExpiryExpiringSoon ExpiryStatus = "expiring_soon" // <= 15 days and >= 0
      ExpiryExpired      ExpiryStatus = "expired"       // < 0 days remaining
  )

  func ComputeExpiry(validUntil time.Time, now time.Time) (daysRemaining int, status ExpiryStatus) {
      diff := validUntil.Sub(now)
      days := int(diff.Hours() / 24)
      if diff < 0 {
          return days, ExpiryExpired
      }
      if days <= 15 {
          return days, ExpiryExpiringSoon
      }
      return days, ExpiryActive
  }
  ```
- Enrich `domain.ProxyIP` with computed fields:
  ```go
  type ProxyIP struct {
      // ... existing fields ...
      DaysRemaining int          `json:"daysRemaining"`
      ExpiryStatus  ExpiryStatus `json:"expiryStatus"`
  }

  type ProxyIPStats struct {
      TotalCount        int `json:"totalCount"`
      AssignedCount     int `json:"assignedCount"`
      AvailableCount    int `json:"availableCount"`
      IPv4Count         int `json:"ipv4Count"`
      IPv6Count         int `json:"ipv6Count"`
      ExpiringSoonCount int `json:"expiringSoonCount"`
      ExpiredCount      int `json:"expiredCount"`
      AssignedExpiring  int `json:"assignedExpiring"`
  }
  ```

#### [MODIFY] `internal/store/postgres/proxy_ips.go`
- `ListProxyIPs` supports sorting by `valid_until ASC` (earliest expiring first) and status filtering.
- Add `ProxyIPStats(ctx context.Context) (domain.ProxyIPStats, error)`: executed strictly on-demand on IP tab view.
- Add batch upsert method `UpsertProxyIPsBatch(ctx context.Context, ips []domain.ProxyIP) (inserted int, updated int, err error)` with `ON CONFLICT (ip_address) DO UPDATE SET valid_until = EXCLUDED.valid_until, valid_from = EXCLUDED.valid_from, password = EXCLUDED.password, plan = EXCLUDED.plan` (does NOT touch `accounts` table).
- Add `UpdateProxyIP(ctx context.Context, ipAddress string, validUntil time.Time, password, plan string) error`.
- Add `DeleteProxyIP(ctx context.Context, ipAddress string) error`: checks `accounts(ip_address)` and returns `domain.ErrConflict` if assigned to an active account.

#### [MODIFY] `internal/domain/auth.go`
- Add `UserSummary` with counts of owned accounts, groups, and active sessions.
- Add `RoleAdmin = "admin"` and `RoleUser = "user"` constants.

#### [MODIFY] `internal/store/postgres/auth.go`
- Add `ListUsers(ctx context.Context, limit, offset int) ([]domain.UserSummary, int, error)` (strictly paginated, max limit 50).
- Add `UpdateUserRoleAndStatus(ctx context.Context, userID uuid.UUID, role string, active *bool) error`.
- Add `ListUserSessions(ctx context.Context, userID uuid.UUID) ([]domain.Session, error)`.
- Add `RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) (int64, error)`.

---

### Component 2: Core Copy Engine & Telemetry Layer

#### [MODIFY] `internal/engine/engine.go`
- Add `killSwitchEngaged atomic.Bool` and `killSwitchReason atomic.Pointer[string]` in `Engine`.
- In `HandleMasterFill`, check `if e.killSwitchEngaged.Load() { return nil }` (zero latency drop on emergency stop).
- Add `SetKillSwitch(engaged bool, reason string, engagedBy string)` and `GetKillSwitchStatus() KillSwitchStatus`.

#### [NEW] `internal/telemetry/telemetry.go`
- Pure in-memory telemetry collector (0 SQL queries, 0 STW pauses, sub-microsecond response):
  ```go
  type PipelineStatus struct {
      KillSwitch        KillSwitchStatus `json:"killSwitch"`
      MasterTickers     []TickerStatus   `json:"masterTickers"`
      Queues            QueueStatus      `json:"queues"`
      WorkerCount       int              `json:"workerCount"`
      DBPool            DBPoolStatus     `json:"dbPool"`
  }
  ```
  - `Queues.MasterFillDepth = masterFillQueue.Len()` (lock-free)
  - `Queues.OrderUpdateDepth = orderUpdateQueue.Len()` (lock-free)
  - `WorkerCount = workerPool.Count()` (atomic read)
  - `DBPool = pool.Stat()` (`AcquiredConns()`, `IdleConns()`, `TotalConns()`, `MaxConns()` from in-memory pgx counters)

---

### Component 3: HTTP API & Middleware Layer

#### [NEW] `internal/httpapi/admin_proxy_ips.go`
- Shared helper `parseAndValidateProxyIP(raw domain.ProxyIPInput) (domain.ProxyIP, error)`:
  - Normalizes IPv4 / IPv6 addresses.
  - Parses dates (`2026-10-06 08:57:22 (IST)` and ISO8601).
  - Calculates `DaysRemaining` and `ExpiryStatus`.
- `getAdminProxyIPs`: Handler for `GET /api/v1/admin/proxy-ips` with query parameters `status` and `type`, plus stats payload.
- `postAdminProxyIPsBulk`: Handler for `POST /api/v1/admin/proxy-ips/bulk` (accepts array matching `ips.json`, returns `{"total": 16, "inserted": 14, "updated": 2, "errors": []}`).
- `postAdminProxyIP`: Handler for `POST /api/v1/admin/proxy-ips` (single IP upsert).
- `patchAdminProxyIP`: Handler for `PATCH /api/v1/admin/proxy-ips/{ip}` (`{"validUntil": "...", "password": "...", "plan": "..."}`).
- `deleteAdminProxyIP`: Handler for `DELETE /api/v1/admin/proxy-ips/{ip}` (returns 409 Conflict if assigned to an active account).
- `postAdminProxyIPTest`: Handler for `POST /api/v1/admin/proxy-ips/{ip}/test` with **hard 3-second timeout**:
  - Dials proxy via `CONNECT` tunnel.
  - Issues test HTTPS probe to `https://api.kite.trade`.

#### [NEW] `internal/httpapi/admin_users.go`
- `getAdminUsers`: Handler for `GET /api/v1/admin/users` (strictly paginated).
- `postAdminUser`: Handler for `POST /api/v1/admin/users` (create platform user directly with role).
- `patchAdminUser`: Handler for `PATCH /api/v1/admin/users/{id}` (change role and active status).
- `getAdminUserSessions`: Handler for `GET /api/v1/admin/users/{id}/sessions` (list active devices, client IPs, last seen).
- `deleteAdminUserSessions`: Handler for `DELETE /api/v1/admin/users/{id}/sessions` (force logout).

#### [NEW] `internal/httpapi/admin_service.go`
- `getAdminServiceStatus`: Handler for `GET /api/v1/admin/status` (100% in-memory, instant response, called only on user navigation or refresh).
- `postAdminKillSwitch`: Handler for `POST /api/v1/admin/killswitch` (`{"engaged": true/false, "reason": "..."}`).

#### [MODIFY] `internal/httpapi/middleware_auth.go`
- Add `requireAdminMiddleware(next http.Handler) http.Handler`:
  - Enforces `user.Role == domain.RoleAdmin`, returns `403 Forbidden` otherwise.

#### [MODIFY] `internal/httpapi/router.go`
- Mount admin endpoints under `requireAdminMiddleware`.

---

### Component 4: Frontend SolidJS Admin Portal

#### [MODIFY] `frontend/src/api.ts`
- Extend `ProxyIP` interface with `daysRemaining` and `expiryStatus`.
- Add Admin API methods:
  - `fetchAdminProxyIPs(filter?: { status?: string; type?: string })`
  - `bulkUploadProxyIPs(ips: ProxyIPInput[]): Promise<BulkUploadResult>`
  - `createProxyIP(ip: ProxyIPInput)`
  - `updateProxyIP(ip: string, data: Partial<ProxyIPInput>)`
  - `deleteProxyIP(ip: string)`
  - `testProxyIP(ipAddress: string)`
  - `fetchAdminUsers(page?: number, limit?: number)`
  - `createAdminUser(data: CreateUserRequest)`
  - `updateAdminUser(id: string, data: { role?: string; active?: boolean })`
  - `fetchAdminUserSessions(userId: string)`
  - `revokeUserSessions(userId: string)`
  - `fetchServiceStatus()`
  - `toggleKillSwitch(engaged: boolean, reason?: string)`

#### [MODIFY] `frontend/src/App.tsx`
- TopNav renders `<A href="/admin">Admin</A>` tab when `user()?.role === 'admin'`.
- Register protected routes: `/admin/status`, `/admin/ips`, `/admin/users`.

#### [NEW] `frontend/src/pages/admin/AdminIPsPage.tsx`
- Top metric gauges (fetched on-demand when page is mounted).
- Manual **"Refresh" button** with loading spinner.
- Filter tabs: `All`, `Active`, `Expiring Soon (<=15d)`, `Expired`, `Assigned`.
- Table Badges: 🟢 `Expires in 85 days` | 🟡 `Expires in 6 days` | 🔴 `Expired`.
- Actions per row:
  - "Test" (single IP probe, 3s timeout)
  - "Edit / Renew" modal (update validity date, password, plan)
  - "Delete" button (trash icon with confirmation; disabled with hover tooltip if IP is assigned)
- "Bulk Import" modal: Upload or paste `ips.json`, displays pre-commit diff, and returns structured summary (`inserted`, `updated`, `errors`).

#### [NEW] `frontend/src/pages/admin/AdminUsersPage.tsx`
- Paginated user list with role badge, account count, session count.
- Status toggle: Active / Suspended switch.
- Actions:
  - "Edit Role" modal (`user` ↔ `admin`)
  - "View Sessions" drawer/modal (shows active client IPs, User Agent, Last Seen)
  - "Force Logout" button (revokes sessions with confirmation modal)

#### [NEW] `frontend/src/pages/admin/AdminStatusPage.tsx`
- **Global Kill Switch** toggle button with modal prompting for optional audit reason.
  - When engaged: displays prominent red banner with engagement timestamp and reason.
- Manual **"Refresh" button** with last-checked timestamp (NO background polling).
- Status cards:
  - Master Tickers (Connected / Idle / Reconnecting)
  - In-memory Queues (`MasterFill` & `OrderUpdate` depths)
  - Active Worker Count
  - Postgres DB Connection Pool (`Acquired` / `Idle` / `Max`)

---

## Verification Plan

### Automated Test-Driven Development (TDD)

#### 1. Unit Tests (`go test ./internal/...`)
- **IP Expiry Calculation Unit Tests** (`internal/domain/proxy_test.go`):
  - ✅ Positive: Date 60 days in future returns `daysRemaining=60, status=ExpiryActive`.
  - ⚠️ Boundary: Exactly 15 days in future returns `daysRemaining=15, status=ExpiryExpiringSoon`.
  - ⚠️ Boundary: 0 days (today) returns `daysRemaining=0, status=ExpiryExpiringSoon`.
  - 🔴 Boundary: Negative duration returns `status=ExpiryExpired`.
  - ⚖️ Edge Case: Date parsing with `(IST)` offset correctly aligns to UTC without drift.
- **Admin HTTP API Unit Tests** (`internal/httpapi/admin_proxy_ips_test.go`, `admin_users_test.go`, `admin_service_test.go`):
  - ✅ Positive: Bulk upload of `ips.json` returns structured summary `{"total": 16, "inserted": 14, "updated": 2}`.
  - ✅ Positive: `PATCH /api/v1/admin/proxy-ips/{ip}` updates validity date and credentials.
  - ✅ Positive: `DELETE /api/v1/admin/proxy-ips/{ip}` deletes an unassigned IP (204 No Content).
  - ❌ Negative: `DELETE /api/v1/admin/proxy-ips/{ip}` on an assigned IP returns `409 Conflict`.
  - ✅ Positive: `GET /api/v1/admin/users/{id}/sessions` returns active user sessions.
  - ✅ Positive: `PATCH /api/v1/admin/users/{id}` toggles active state `{"active": false}`.
  - ✅ Positive: `POST /api/v1/admin/killswitch` records reason and `GET /api/v1/admin/status` reflects it in <1ms (0 DB queries).
  - ❌ Negative: Non-admin gets `403 Forbidden` across all admin routes.
  - ❌ Negative: Invalid date string or malformed IP returns `400 Bad Request`.
- **Engine Kill Switch Unit Tests** (`internal/engine/engine_test.go`):
  - ✅ Positive: When engaged, `HandleMasterFill` exits immediately with 0 dispatched follower orders.
  - ✅ Positive: When cleared, fan-out resumes normally.

#### 2. Integration Tests (`go test -tags integration ./...`)
- Real PostgreSQL testcontainer:
  - Bulk upsert of 16 IPs preserves existing `assigned_account_id`.
  - Renewal of an assigned IP updates `valid_until` while keeping account binding intact.
  - Deleting an unassigned IP removes it from `proxy_ips`.
  - Deleting an assigned IP returns conflict error without modifying the database.
  - Deactivating a user (`active = false`) blocks login and auth middleware checks.

#### 3. Frontend Tests (`pnpm -C frontend test --run`)
- Expiry badge renders correct color (green >15d, amber <=15d, red expired).
- Delete IP button disabled with tooltip when IP is assigned.
- Kill switch modal captures reason string before sending request.
- Admin pages contain NO `setInterval` calls; manual refresh re-fetches data.
- Bulk import modal parses `ips.json` and renders summary.
