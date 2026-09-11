# EnvoyTrade Dashboard API — contract index

Contract only — nothing here is implemented yet (no `main.go`, no router, no `internal/admin` exists in
the Go backend as of this writing). This documents the API surface `docs/UI-PLAN.md` needs, kept lean:
prefer expanding an existing endpoint over adding a new one.

## Conventions

- Base path: `/api/v1`.
- JSON request/response bodies.
- Errors: `{"error": "human readable message"}` with a non-2xx status.
- Auth: not designed yet, out of scope for this contract pass.
- IDs: account/master/follower IDs are the `accounts.id` UUID from the existing Go store.

## Endpoints by resource

| File | Endpoints | Backs |
|---|---|---|
| [`groups.md`](./groups.md) | `GET /groups`, `GET /groups/{masterId}` | Dashboard page |
| [`accounts.md`](./accounts.md) | `GET /accounts`, `POST /accounts`, `PATCH /accounts/{id}` | Accounts page, and the Dashboard `CopyToggle`/Stop-Copy (via the same `PATCH`) |
| [`actions.md`](./actions.md) | `POST /accounts/{id}/actions` | Dashboard Rebalance / Square Off / Exit Open Orders buttons |
| [`analytics.md`](./analytics.md) | `GET /analytics/pnl`, `GET /analytics/trades` | Analytics page |

No endpoint is duplicated across files — e.g. the copy-enable toggle is **not** a separate
`/follow-links/{id}` resource, it's a field on the existing account `PATCH`; the three action buttons
share one endpoint distinguished by a `type` field rather than three near-identical routes.
