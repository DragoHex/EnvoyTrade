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

## `GET /api/v1/groups/{id}`

Full `AccountTable` data for one group — one response backs the whole `GroupCard`, returning account metadata and aggregated metrics (`netQty`, positions counts, open orders, total MTM, available cash, and available margin) for both the master account and its followers in a single database round-trip. Accepts either group ID or master account ID.

**Response** `200`:
```json
{
  "id": "uuid",
  "name": "Alpha Group",
  "masterId": "uuid",
  "masterAccountId": "ZX1234",
  "masterName": "Alice Trader",
  "masterActive": true,
  "masterNetQty": 150,
  "masterOpenPositionsCount": 2,
  "masterClosedPositionsCount": 5,
  "masterOpenOrdersCount": 1,
  "masterTotalMtm": "1240.50",
  "masterAvailableCash": "50000.00",
  "masterAvailableMargin": "120000.00",
  "followers": [
    {
      "accountId": "uuid",
      "name": "Bob Follower",
      "brokerAccountId": "ZY5678",
      "enabled": true,
      "status": "ok",
      "netQty": 150,
      "openPositionsCount": 2,
      "closedPositionsCount": 5,
      "openOrdersCount": 1,
      "totalMtm": "1240.50",
      "availableCash": "50000.00",
      "availableMargin": "120000.00"
    }
  ]
}
```

`404` if `id` doesn't exist or isn't a valid group or master account.

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
