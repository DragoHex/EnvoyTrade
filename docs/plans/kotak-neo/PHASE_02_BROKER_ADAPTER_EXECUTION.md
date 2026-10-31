# Phase 2: Broker Adapter & Execution Seam

This document specifies Phase 2 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Implement the `broker.Broker` consumer interface (PLAN.md §1 seam rule) for Kotak Neo, mapping canonical orders to Kotak's native JSON/form schema, enforcing dedicated static proxy egress, and translating errors into standard domain sentinels.

---

## 2. Technical Design

### 2.1 Interface Implementation (`internal/kotak/broker.go`)
Implements `broker.Broker`:
```go
type Broker interface {
    PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error)
    GetPositions(ctx context.Context) ([]broker.Position, error)
    GetOpenOrders(ctx context.Context) ([]broker.Order, error)
    GetOrders(ctx context.Context) ([]broker.Order, error)
    GetOrderHistory(ctx context.Context, orderID string) ([]broker.Order, error)
    CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error)
}
```

### 2.2 Parameter Mapping
- **Exchange Segment**: Resolved via `instrument.Cache.ResolveToSecurityID(exchange, tradingsymbol)`.
  - Equity: `nse_cm`, `bse_cm`.
  - F&O: `nse_fo`, `bse_fo`, `mcx_fo`.
- **Product Type**:
  - `CNC` $\to$ `CNC`
  - `MIS` / `INTRADAY` $\to$ `MIS`
  - `NRML` / `MARGIN` $\to$ `NRML`
- **Order Type**:
  - `LIMIT` $\to$ `L` (Price formatted as decimal string)
  - `MARKET` $\to$ `MKT` (Price set to `"0"`)
  - `SL` $\to$ `SL` (Price and trigger price required)
  - `SL-M` $\to$ `SL-M` (Trigger price required, price `"0"`)
- **Transaction Type**:
  - `BUY` $\to$ `B`
  - `SELL` $\to$ `S`
- **Validity**: `"DAY"` (or `"IOC"` if specified)
- **Tagging**: Carries `IdempotencyTag` in order tracking fields.

### 2.3 Static Proxy Routing
- Wrapped via `gokotakneo.RESTClientFor(proxyCfg)`.
- Follower accounts must strictly egress through their assigned static proxy IP. If unproxied where required, returns `domain.ErrUnproxiedNotAllowed`.

### 2.4 Domain Sentinel Error Mapping
- HTTP 401, session expired, invalid token $\to$ `domain.ErrAuthExpired`
- HTTP 404, order not found $\to$ `domain.ErrNotFound`
- HTTP 502/503/504, connection timeout, network failure $\to$ `domain.ErrBrokerUnreachable`
- RMS margin rejection (Kotak `stCode` 1004 / 1006) $\to$ terminal non-retryable error

---

## 3. Test-Driven Development (TDD) Test Matrix

| Case Type | Test Name | Scenario / Setup | Expected Outcome |
|---|---|---|---|
| **Positive** | `TestPlaceOrder_LimitOrder` | Place canonical limit buy order (`CNC`, 10 qty @ 1450.50). | Correctly maps to Kotak Neo schema (`pt="L"`, `tt="B"`, `pc="CNC"`, `pr="1450.50"`), returns canonical `OrderResponse`. |
| **Positive** | `TestPlaceOrder_MarketOrder` | Place canonical market sell order (`MIS`, 5 qty). | Correctly maps `pt="MKT"`, `tt="S"`, `pr="0"`. |
| **Positive** | `TestGetPositions_NormalizesFields` | Kotak positions response with net/day quantities. | Normalized into `[]broker.Position` with decimal prices and standard product names. |
| **Positive** | `TestGetOrders_ParsesOrderBook` | Kotak order book response with terminal and open states. | Converted into `[]broker.Order` matching canonical domain fields. |
| **Positive** | `TestCancelOrder_Success` | Call `CancelOrder` for an open order number. | Dispatches cancel request to Kotak gateway, returns order response. |
| **Negative** | `TestPlaceOrder_MarginRejection` | Upstream returns Kotak error code `1004` (insufficient funds). | Fails immediately with terminal error, no retry attempt. |
| **Negative** | `TestPlaceOrder_SessionExpired` | Upstream returns HTTP 401. | Wraps `domain.ErrAuthExpired`. |
| **Negative** | `TestPlaceOrder_NetworkTimeout` | HTTP request times out. | Wraps `domain.ErrBrokerUnreachable`. |
| **Edge Case** | `TestBroker_FollowerUnproxied` | Follower broker invoked without proxy configuration. | Fails immediately with `domain.ErrUnproxiedNotAllowed`. |
| **Edge Case** | `TestPlaceOrder_FOSegmentResolution` | NIFTY Options contract placed. | Exchange segment correctly resolved to `nse_fo` via `instrument.Cache`. |
