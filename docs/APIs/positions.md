# Positions & Square-Off API

Provides cluster-level and account-level portfolio square-off operations.

---

## 1. Group Square-Off

Flattens open positions across the entire trading cluster (the group's Master account and all linked Follower accounts).

### `POST /api/v1/groups/{group_id}/positions/square-off`

**Parameters**:
- `group_id` (path, UUID, required): The ID of the trading group.

**Request Body** (JSON, optional):
```json
{
  "symbols": ["CRUDEOIL17SEP26C10600", "CRUDEOIL17SEP26C10700"]
}
```
*Note: If `symbols` is omitted or empty, all open positions in the cluster are squared off.*

**Behavior**:
1. **Safety Pre-Check**: Cancels all open/pending limit orders across the master and all followers in the group to prevent post-liquidation fills.
2. **Flattening**: Reads live broker positions and places opposite `MARKET` counter-orders for each open contract.
   - Long positions (`qty > 0`) $\rightarrow$ `SELL`
   - Short positions (`qty < 0`) $\rightarrow$ `BUY`
   - Tag: `sqoff-m-...` (Master) or `sqoff-f-...` (Follower).
3. **Engine Decoupling**: Master square-off orders tagged with `sqoff-` are recognized by `engine.HandleMasterFill` and skipped from copy fan-out, avoiding double-execution.
4. **Portfolio Cache**: Re-syncs local database position cache for master and followers.

**Response** (`200 OK`):
```json
{
  "action": "square_off",
  "status": "completed",
  "group_id": "b0000000-0000-0000-0000-000000000001",
  "master_id": "f6e70723-b904-4427-83ee-a85771dee2e4",
  "followers_affected": 4,
  "cancelled_orders": 2,
  "positions_squared_off": 8,
  "orders": [
    {
      "account_id": "f6e70723-b904-4427-83ee-a85771dee2e4",
      "role": "master",
      "broker_order_id": "TB_ORD_101",
      "exchange": "MCX",
      "tradingsymbol": "CRUDEOIL17SEP26C10700",
      "product": "CNC",
      "side": "BUY",
      "quantity": 100,
      "status": "placed"
    }
  ],
  "errors": []
}
```

**Status Values**:
- `completed`: All positions and orders placed successfully.
- `partial`: At least one account experienced a broker error, but other accounts were processed (errors reported in `errors: [...]`).
- `empty`: No open positions found (clean no-op).

---

## 2. Single Account Square-Off

Flattens open positions strictly for one designated account. Master and peer followers are completely unaffected.

### `POST /api/v1/accounts/{account_id}/positions/square-off`

**Parameters**:
- `account_id` (path, UUID, required): The ID of the follower or master account.

**Request Body** (JSON, optional):
```json
{
  "symbols": ["NIFTY26OCTFUT"]
}
```

**Response** (`200 OK`):
```json
{
  "action": "square_off",
  "status": "completed",
  "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
  "role": "follower",
  "followers_affected": 1,
  "cancelled_orders": 0,
  "positions_squared_off": 1,
  "orders": [
    {
      "account_id": "a5183e89-6cb2-4d32-91a2-6dc525570185",
      "role": "follower",
      "broker_order_id": "TB_ORD_102",
      "exchange": "NFO",
      "tradingsymbol": "NIFTY26OCTFUT",
      "product": "NRML",
      "side": "SELL",
      "quantity": 75,
      "status": "placed"
    }
  ],
  "errors": []
}
```

---

## 3. Error Responses

- `400 Bad Request`: Invalid UUID format or malformed JSON body.
- `401 Unauthorized`: Missing or invalid session cookie / token.
- `404 Not Found`: Group or account ID does not exist.
