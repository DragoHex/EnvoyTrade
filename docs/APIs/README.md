# EnvoyTrade Dashboard API — contract index

Contract only — nothing here is implemented yet (no `main.go`, no router, no `internal/admin` exists in
the Go backend as of this writing). This documents the API surface `docs/plans/UI-PLAN.md` needs, kept lean:
prefer expanding an existing endpoint over adding a new one.

## Conventions

- Base path: `/api/v1`.
- JSON request/response bodies.
- Errors: `{"error": "human readable message"}` with a non-2xx status.
- Auth: Database-backed session cookie (`envoytrade_session`), with fallback to `Authorization: Bearer <token>`. See [`auth.md`](./auth.md).
- IDs: account/master/follower IDs are the `accounts.id` UUID from the existing Go store.

## Endpoints by resource

| File | Endpoints | Backs |
|---|---|---|
| [`auth.md`](./auth.md) | `POST /auth/register`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` | Authentication wall and session management |
| [`groups.md`](./groups.md) | `GET /groups`, `GET /groups/{masterId}` | Dashboard page |
| [`accounts.md`](./accounts.md) | `GET /accounts`, `POST /accounts`, `PATCH /accounts/{id}` | Accounts page, and the Dashboard `CopyToggle`/Stop-Copy (via the same `PATCH`) |
| [`orders.md`](./orders.md) | `GET /accounts/{id}/orders` | Dashboard account expanded order, position, and holding drawer |
| [`actions.md`](./actions.md) | `POST /accounts/{id}/actions` | Dashboard Rebalance / Exit Open Orders buttons |
| [`positions.md`](./positions.md) | `POST /groups/{id}/positions/square-off`, `POST /accounts/{id}/positions/square-off` | Cluster and account portfolio square-off operations |
| [`rebalance.md`](./rebalance.md) | `GET /groups/{id}/positions/rebalance/diff`, `POST /groups/{id}/positions/rebalance`, `GET /accounts/{id}/positions/rebalance/diff`, `POST /accounts/{id}/positions/rebalance` | Cluster and account portfolio rebalance operations |
| [`analytics.md`](./analytics.md) | `GET /analytics/pnl`, `GET /analytics/trades` | Analytics page |
| [`proxy_ips.md`](./proxy_ips.md) | `GET /proxy-ips`, `GET /proxy-ips/available` | Proxy IP pool management and auto-assignment in Account drawer |

No endpoint is duplicated across files — e.g. the copy-enable toggle is **not** a separate
`/follow-links/{id}` resource, it's a field on the existing account `PATCH`; the three action buttons
share one endpoint distinguished by a `type` field rather than three near-identical routes.
