# Actions API

One lean endpoint for all three Dashboard row action buttons (Rebalance / Square Off / Exit Open
Orders), distinguished by `type` — avoids three near-identical routes.

## `POST /api/v1/accounts/{id}/actions`

**Request**:
```json
{ "type": "rebalance" }
```
`type` is one of:

| type | Meaning | Backend status |
|---|---|---|
| `rebalance` | Re-run fan-out for the account's most recent master fill (idempotent — safe to click again). Callable from a follower row or the master row itself — both resolve to the same master and re-fan-out to every enabled follower. | Not stubbed — reuses existing `internal/engine` fan-out path. |
| `square_off` | Exit all open positions for this account. | **Stubbed for this pass** — returns `501`. |
| `exit_open_orders` | Cancel all open orders for this account. | **Stubbed for this pass** — returns `501`. |

**Response**:
- `202 Accepted` — `{ "type": "rebalance", "status": "accepted" }` (async; no request body result to
  poll yet — future addition, not designed here).
- `501 Not Implemented` — `{ "error": "square_off is not implemented yet" }` for `square_off` /
  `exit_open_orders` until broker order-cancel/exit wiring exists.
- `404` if `id` doesn't exist. `400` for an unknown `type`.

No confirmation step is server-side — the UI's `ConfirmActionModal` gates `square_off`/`exit_open_orders`
before this call is made at all; the endpoint itself just executes.
