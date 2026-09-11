# Analytics API

Backs the Analytics page's per-group `GroupAnalyticsPage`: `OrdersPane`, `PnLPane`, `MtmPane`.

## `GET /api/v1/analytics/pnl`

**Query params**: `groupId` (optional), `accountId` (optional, overrides `groupId`), `from`, `to` (ISO
date). If `from`/`to` are omitted, defaults to the **current calendar month** (`PnLPane`'s default view).

**Response** `200`:
```json
{
  "points": [
    { "t": "2026-09-01T00:00:00Z", "pnl": "1200.50" }
  ],
  "streaks": {
    "current": { "type": "win", "count": 3 },
    "bestWin": 5,
    "bestLoss": 2
  }
}
```

**Data source note**: cumulative P&L requires broker margin/position snapshots not yet wired
(`gokiteconnect`'s `GetMargins`/`GetPositions`) — contract shape is final, data source is a follow-up.
`streaks` additionally requires trade-pairing/realized-P&L logic that doesn't exist yet anywhere in the
codebase (no such concept in `internal/domain` today) — shape only, not computed.

## `GET /api/v1/analytics/mtm`

**Query params**: `groupId` (optional), `accountId` (optional, overrides `groupId`), `from`, `to`.

**Response** `200`:
```json
{
  "points": [
    { "t": "2026-09-01T00:00:00Z", "mtm": "340.00" }
  ]
}
```

**Data source note**: same broker gap as `/pnl` (`GetMargins`/`GetPositions`, see `groups.md`) — no MTM
field exists anywhere in the store today; contract shape only.

## `GET /api/v1/analytics/trades`

Backs `OrdersPane` too — pass the master account's `accountId` to get its order trace, same shape as any
other account's trade history (no separate "orders" endpoint).

**Query params**: `accountId` (optional), `from`, `to`.

**Response** `200`:
```json
{
  "trades": [
    {
      "time": "2026-09-01T09:15:00Z",
      "accountId": "uuid",
      "brokerAccountId": "ZY5678",
      "symbol": "RELIANCE",
      "side": "BUY",
      "qty": 10,
      "price": "2900.00",
      "status": "COMPLETE"
    }
  ]
}
```

Backed by existing `follower_orders`/`master_fills` rows — no new data source needed, unlike `pnl`.
