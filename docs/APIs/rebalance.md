# Positions & Rebalance API

Provides cluster-level and account-level portfolio rebalance operations to reconcile follower position drifts against the canonical master account.

---

## 1. Group Rebalance Diff (Preview)

Computes live position drifts across all linked followers in a group relative to the master account's open positions.

### `GET /api/v1/groups/{group_id}/positions/rebalance/diff`

**Parameters**:
- `group_id` (path, UUID, required): The ID of the trading group.

**Response** (`200 OK`):
```json
{
  "group_id": "f6e70723-b904-4427-83ee-a85771dee2e4",
  "master_id": "f6e70723-b904-4427-83ee-a85771dee2e4",
  "followers_evaluated": 3,
  "followers_with_drift": 2,
  "drifts": [
    {
      "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
      "account_name": "Follower Alpha",
      "broker_account_id": "KITE_FA01",
      "enabled": true,
      "clone_factor": "1.0",
      "symbols": [
        {
          "exchange": "NFO",
          "tradingsymbol": "NIFTY26OCTFUT",
          "product": "NRML",
          "lot_size": 75,
          "master_qty": 75,
          "target_qty": 75,
          "follower_qty": 0,
          "drift_qty": 75,
          "action": "BUY"
        }
      ]
    },
    {
      "account_id": "b6294f90-7dc3-5e43-a2b3-7ed636681296",
      "account_name": "Follower Beta",
      "broker_account_id": "KITE_FB02",
      "enabled": true,
      "clone_factor": "0.5",
      "symbols": [
        {
          "exchange": "NSE",
          "tradingsymbol": "RELIANCE",
          "product": "CNC",
          "lot_size": 1,
          "master_qty": 0,
          "target_qty": 0,
          "follower_qty": 50,
          "drift_qty": -50,
          "action": "SELL"
        }
      ]
    }
  ]
}
```

---

## 2. Group Rebalance Execution

Brings selected followers (or all drifting followers) into alignment with the master account's open positions.

### `POST /api/v1/groups/{group_id}/positions/rebalance`

**Parameters**:
- `group_id` (path, UUID, required): The ID of the trading group.

**Request Body** (JSON, optional):
```json
{
  "follower_ids": ["a5183e89-6cb2-4d32-91a2-6dc525570185"]
}
```
*Note: If `follower_ids` is omitted or empty, all drifting followers in the group are rebalanced.*

**Behavior**:
1. **Safety Pre-Check**: Cancels open/pending limit orders on drifting contracts for target followers to prevent conflict fills.
2. **Order Generation**:
   - Computes target quantities using canonical `lot_size` and `SizeOrder(masterQty, cloneFactor, lotSize, maxQtyPerOrder)`.
   - Rogue/leftover positions where Master has 0 quantity target 0 (closed via opposite counter-orders).
   - Places `MARKET` counter-orders with tag `rebal-f-{shortID}-{ts}`.
3. **Portfolio Sync**: Re-syncs local database position cache for rebalanced followers.

**Response** (`200 OK`):
```json
{
  "action": "rebalance",
  "status": "completed",
  "group_id": "f6e70723-b904-4427-83ee-a85771dee2e4",
  "followers_affected": 1,
  "orders_placed": 1,
  "orders": [
    {
      "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
      "broker_order_id": "TB_ORD_201",
      "exchange": "NFO",
      "tradingsymbol": "NIFTY26OCTFUT",
      "product": "NRML",
      "side": "BUY",
      "quantity": 75,
      "status": "placed"
    }
  ],
  "errors": []
}
```

---

## 3. Single Account Rebalance Diff

Computes position drift for one specific follower relative to its group's master.

### `GET /api/v1/accounts/{account_id}/positions/rebalance/diff`

**Parameters**:
- `account_id` (path, UUID, required): The ID of the follower account.

**Response** (`200 OK`):
```json
{
  "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
  "account_name": "Follower Alpha",
  "broker_account_id": "KITE_FA01",
  "enabled": true,
  "clone_factor": "1.0",
  "symbols": [
    {
      "exchange": "NFO",
      "tradingsymbol": "NIFTY26OCTFUT",
      "product": "NRML",
      "lot_size": 75,
      "master_qty": 75,
      "target_qty": 75,
      "follower_qty": 0,
      "drift_qty": 75,
      "action": "BUY"
    }
  ]
}
```

---

## 4. Single Account Rebalance Execution

Rebalances a single follower account to match master positions.

### `POST /api/v1/accounts/{account_id}/positions/rebalance`

**Parameters**:
- `account_id` (path, UUID, required): The ID of the follower account.

**Request Body**: Empty or `{}`.

**Response** (`200 OK`):
```json
{
  "action": "rebalance",
  "status": "completed",
  "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
  "followers_affected": 1,
  "orders_placed": 1,
  "orders": [
    {
      "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
      "broker_order_id": "TB_ORD_201",
      "exchange": "NFO",
      "tradingsymbol": "NIFTY26OCTFUT",
      "product": "NRML",
      "side": "BUY",
      "quantity": 75,
      "status": "placed"
    }
  ],
  "errors": []
}
```

---

## 5. Error Responses

- `400 Bad Request`: Invalid UUID format or malformed JSON body.
- `401 Unauthorized`: Missing or invalid session cookie.
- `404 Not Found`: Group or account ID does not exist or account is not a linked follower.
- `500 Internal Server Error`: Internal failure or unhandled broker communication error.
