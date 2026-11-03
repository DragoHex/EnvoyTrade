# Phase 2: Broker Adapter & Execution Seam

This document specifies Phase 2 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Implement the `broker.Broker` seam adapter for Angel One in `internal/angelone/broker.go`. The adapter encapsulates all SmartAPI REST communication, parameter conversion, forward proxy egress, and domain sentinel error translation.

---

## 2. Broker Seam Implementation (`broker.Broker`)

The adapter satisfies `internal/broker.Broker`:
```go
type Broker interface {
    PlaceOrder(ctx context.Context, variety string, params OrderParams) (OrderResponse, error)
    GetPositions(ctx context.Context) ([]Position, error)
    GetOpenOrders(ctx context.Context) ([]Order, error)
    GetOrders(ctx context.Context) ([]Order, error)
    GetOrderHistory(ctx context.Context, orderID string) ([]Order, error)
    CancelOrder(ctx context.Context, variety, orderID string) (OrderResponse, error)
}
```

### 2.1 Parameter Mapping (`PlaceOrder`)
- **Symbol Translation**: Translates `(Exchange, Tradingsymbol)` to Angel One's `(Exchange, SymbolToken)` via `instrument.Cache.ResolveToSecurityID`.
- **Product Type**:
  - `CNC` &rarr; `"DELIVERY"`
  - `MIS` &rarr; `"INTRADAY"`
  - `NRML` &rarr; `"CARRYFORWARD"`
- **Order Type**:
  - `MARKET` &rarr; `"MARKET"`
  - `LIMIT` &rarr; `"LIMIT"`
  - `SL` &rarr; `"STOPLOSS_LIMIT"`
  - `SL-M` &rarr; `"STOPLOSS_MARKET"`
- **Variety**: Defaults to `"NORMAL"` (or `"AMO"` / `"STOPLOSS"`).
- **Price & Trigger Price**: Converted directly to `decimal.Decimal` (zero-loss precision).
- **Order Tag**: Carries EnvoyTrade's deterministic idempotency tag (`Tag`, max 20 chars).

### 2.2 Error Translation Taxonomy
SmartAPI errors wrap `goangelone.APIError` and map to domain sentinels:
- `ErrUnauthorized` / HTTP 401 / `"Invalid Token"` &rarr; `domain.ErrAuthExpired`
- `ErrOrderNotFound` / HTTP 404 / `"Order does not exist"` &rarr; `domain.ErrNotFound`
- Margin / RMS rejection (`"Insufficient Funds"`, `"RMS:Rule"`, `"AB03"`) &rarr; immediate terminal rejection (never retried).
- `ErrBrokerUnreachable` / HTTP 502 / 503 / 504 / connection timeout &rarr; `domain.ErrBrokerUnreachable`.
- `ErrRateLimit` / HTTP 429 &rarr; rate limit backoff.

### 2.3 Forward Proxy Egress
- Constructed via `goangelone.RESTClientFor(proxyCfg)` or `goangelone.WithProxy(proxyCfg)`.
- Enforces strict egress through follower-assigned static proxy IPs.

---

## 3. Implementation Details

### 3.1 New Files
- `internal/angelone/broker.go`: `Broker` struct, `PlaceOrder`, `CancelOrder`, `GetOrders`, `GetOrderHistory`, `GetPositions`, parameter/status normalizers, and error translators.
- `internal/angelone/broker_test.go`: Pure TDD unit test suite using mock SmartAPI HTTP transports.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestBroker_PlaceOrder_MarketBuy_Success` | Valid market order parameters | Translates symbol token, sends `"MARKET"`, returns valid `OrderID`. |
| `TestBroker_PlaceOrder_LimitSell_Success` | Valid limit order parameters | Sends exact `decimal.Decimal` price and quantity. |
| `TestBroker_PlaceOrder_ZeroQuantity` | Order quantity = 0 | Rejects with `ErrInvalidInput` before making network call. |
| `TestBroker_PlaceOrder_UnresolvedSymbol` | Symbol missing from instrument cache | Returns `domain.ErrNotFound`. |
| `TestBroker_PlaceOrder_RMSRejection` | Insufficient funds error from broker | Returns terminal RMS rejection (no retries). |
| `TestBroker_PlaceOrder_AuthExpired` | 401 Unauthorized from SmartAPI | Wraps `domain.ErrAuthExpired`. |
| `TestBroker_PlaceOrder_NetworkTimeout` | Network timeout / connection drop | Wraps `domain.ErrBrokerUnreachable`. |
| `TestBroker_CancelOrder_Success` | Cancel pending order | Dispatches `CancelOrder` and returns confirmation. |
| `TestBroker_GetOrders_Normalization` | Order book retrieval | Normalizes product (`DELIVERY` &rarr; `CNC`), order type, and timestamps. |
| `TestBroker_GetPositions_Normalization` | Positions retrieval | Maps net quantity, calculates average buy/sell prices, returns `[]broker.Position`. |
| `TestBroker_StaticProxyEgress` | Configured proxy URL | All HTTP transport calls execute through proxy gateway. |
