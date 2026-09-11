# Orders & Position Details API

Account-level trading detail view backing the Dashboard's expanded account drawer. Returns persistent
summary metrics along with six categorized tables: Open Positions, Closed Positions, Holdings, Open
Orders, Closed Orders, and Rejected Orders.

## `GET /api/v1/accounts/{id}/orders`

Fetch full order, position, holding, and summary metrics for a given account (master or follower).
Executes database-level pagination on the requested sub-tab.

### Query Parameters:
- `tab` (string, optional, default: `"open_positions"`): Active tab to fetch rows for:
  `"open_positions"`, `"closed_positions"`, `"holdings"`, `"open_orders"`, `"closed_orders"`, `"rejected_orders"`.
- `page` (integer, optional, default: `1`): 1-based page number.
- `limit` (integer, optional, default: `10`): Page size (default: 10 items).

### Data sources:
- **Master account orders**: Received from broker over WebSocket (persisted in `master_fills`).
- **Follower account orders**: Copied from master and sent to broker with updated status (persisted in `follower_orders` joined with `master_fills`).
- **Positions & Holdings & Margins**: Stored in `account_positions`, `account_holdings`, and `account_margins` (synced from broker or populated with seed state).

### Response `200 OK`:
```json
{
  "accountId": "a0000000-0000-0000-0000-000000000001",
  "role": "master",
  "brokerAccountId": "M-KU2675",
  "summary": {
    "netQty": -890,
    "openPositionsCount": 8,
    "closedPositionsCount": 2,
    "pendingOrdersCount": 0,
    "totalMtm": 380.00,
    "realizedPnl": 0.00,
    "accountValue": 2163520.84,
    "status": "online"
  },
  "counts": {
    "openPositions": 8,
    "closedPositions": 2,
    "holdings": 11,
    "openOrders": 0,
    "closedOrders": 7,
    "rejectedOrders": 0
  },
  "pagination": {
    "tab": "open_positions",
    "page": 1,
    "limit": 10,
    "totalCount": 8,
    "totalPages": 1
  },
  "openPositions": [
    {
      "product": "CNC",
      "instrument": "CRUDEOIL17SEP26C10600",
      "qty": -100,
      "avgPrice": "0.00/111.10",
      "ltp": 114.4,
      "mtm": -330.00,
      "action": "exit"
    }
  ],
  "closedPositions": [],
  "holdings": [],
  "openOrders": [],
  "closedOrders": [],
  "rejectedOrders": []
}
```

### Errors:
- `400 Bad Request`: `{"error": "invalid account id"}` if account id is not a valid UUID.
- `404 Not Found`: `{"error": "account not found"}` if account id does not exist.
