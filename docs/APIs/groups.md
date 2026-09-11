# Groups API

A "group" is one master account plus its follower accounts (reconstructed from `accounts` +
`follow_links` — there's no dedicated group table in the Go store).

## `GET /api/v1/groups`

List all groups, for the Dashboard's `GroupList`.

**Response** `200`:
```json
[
  {
    "masterId": "uuid",
    "masterAccountId": "ZX1234",
    "broker": "kite",
    "followerCount": 3,
    "status": "ok",         // "ok" | "error" — rollup: "error" if any follower is "error"
    "active": true          // master-level Stop/Start switch (docs/APIs/accounts.md's "active")
  }
]
```

## `GET /api/v1/groups/{masterId}`

Full `AccountTable` data for one group — one response backs the whole `GroupCard`, no per-column
endpoint.

**Response** `200`:
```json
{
  "masterId": "uuid",
  "masterAccountId": "ZX1234",
  "masterActive": true,
  "followers": [
    {
      "accountId": "uuid",
      "brokerAccountId": "ZY5678",
      "enabled": true,
      "netQty": 150,
      "positions": { "open": 2, "closed": 5 },
      "openOrders": 1,
      "totalMtm": "1240.50",
      "availableCash": "50000.00",
      "availableMargin": "120000.00",
      "status": "ok"       // "ok" | "error"
    }
  ]
}
```

**Data source note**: `netQty`/`positions`/`openOrders` are derivable from `follower_orders` (not
currently aggregated by any store method); `totalMtm`/`availableCash`/`availableMargin` require broker
data not yet wired (`gokiteconnect`'s `GetMargins`/`GetPositions` — see `internal/kite`, not yet built).
This response shape is final; the data-source wiring is a separate follow-up.

**Current implementation** (`internal/httpapi`): returns only `accountId`, `brokerAccountId`, `enabled`,
`status` per follower — the six fields above are omitted from the response entirely (not `null`/`0`,
absent) until the aggregation/broker wiring above exists. Frontend renders missing columns as `—`.

`404` if `masterId` doesn't exist or isn't a master account.

## `POST /api/v1/groups/{masterId}/followers`

Attach an existing follower-role account to a group — the group-management page's "Add account to
group" action (`docs/UI-PLAN.md`'s Accounts page).

**Request**:
```json
{ "accountId": "uuid", "capitalRatio": "0.5", "maxQtyPerOrder": 100 }
```

**Response** `201`, empty body. `400` if `masterId` isn't a master account, `accountId` isn't a
follower-role account, or `capitalRatio`/`accountId` is malformed. `409` if `accountId` is already
attached to a group (a follower can only belong to one group at a time — see
`docs/APIs/accounts.md`'s `DELETE /accounts/{id}/group` to detach it first).
