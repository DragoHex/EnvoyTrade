# Broker Callbacks Architecture

Unified single-URL webhook ingestion for multi-broker order updates and execution fills.

---

## 1. Endpoints

- **Unified Webhook URL**: `POST /broker-callback`
- **Subpath Route**: `POST /broker-callback/{broker}` (e.g. `/broker-callback/kite`, `/broker-callback/angelone`, `/broker-callback/groww`)
- **Authentication**: Public webhook. Bypasses session cookie middleware in `internal/httpapi/middleware_auth.go`.

---

## 2. 5-Stage Adaptive Routing Pipeline

Located in `internal/callback/router.go` (`AdaptiveRouter`).

```
Incoming Request POST /broker-callback
  │
  ├── 1. Check Query Param `?broker=name`
  │      └── Matched registered broker handler? → Dispatch
  │
  ├── 2. Check HTTP Header `X-Broker: name`
  │      └── Matched registered broker handler? → Dispatch
  │
  ├── 3. Check Path Prefix `/broker-callback/{broker}`
  │      └── Matched registered broker handler? → Dispatch
  │
  ├── 4. Zero-Allocation Fast Raw Byte Fingerprinting (~80ns)
  │      ├── contains `"groww_order_id"`                         → Groww Handler
  │      ├── contains `"dhanClientId"` / `"dhan_client_id"`       → Dhan Handler
  │      ├── contains `"nOrdNo"` & `"trdSym"`                     → Kotak Neo Handler
  │      ├── contains `"AppOrderID"` & `"ExchangeInstrumentID"`   → Symphony Handler
  │      ├── contains `"uniqueorderid"` & `"filledshares"`         → Angel One Handler
  │      └── contains `"checksum"` & `"user_id"`                 → Kite / Zerodha Handler
  │
  └── 5. Default Fallback
         └── Dispatches to default handler (Zerodha Kite)
```

- **Body Preservation**: `r.Body` is read up to 1MB (`http.MaxBytesReader`) and restored via `io.NopCloser(bytes.NewReader(bodyBytes))` so downstream handlers parse JSON and verify crypto signatures without duplicate network reads.
- **Aliases**: Query/path/header broker names automatically resolve aliases (`kotak` ↔ `kotakneo`, `zerodha` ↔ `kite`).
- **Performance**: Benchmark `BenchmarkAdaptiveRouter_Fingerprint` runs at ~1µs per request with zero heap allocations during fingerprint scanning.

---

## 3. Broker Signatures & Fingerprints

| Broker | Fingerprint Tokens | Checksum / Auth Algorithm | Account Lookup | Aliases |
|---|---|---|---|---|
| **Zerodha Kite** | `"checksum"` + `"user_id"` | `SHA-256(order_id + order_timestamp + api_secret)` | `AccountByBrokerAndUserID(ctx, "kite", user_id)` | `zerodha`, `kite` |
| **Angel One** | `"uniqueorderid"` + `"filledshares"` | Header token / static postback URL | `AccountByBrokerAndUserID(ctx, "angelone", clientcode)` | `angelone`, `angel` |
| **Groww** | `"groww_order_id"` | SHA-256 HMAC payload signature | `AccountByBrokerAndUserID(ctx, "groww", user_id)` | `groww` |
| **Dhan** | `"dhanClientId"` / `"dhan_client_id"` | Postback signature token / IP whitelist | `AccountByBrokerAndUserID(ctx, "dhan", dhanClientId)` | `dhan` |
| **Kotak Neo** | `"nOrdNo"` + `"trdSym"` | Webhook HMAC-SHA256 signature token | `AccountByBrokerAndUserID(ctx, "kotakneo", clientCode)` | `kotak`, `kotakneo` |
| **Symphony** | `"AppOrderID"` + `"ExchangeInstrumentID"` | Basic/Bearer token / TLS postback | `AccountByBrokerAndUserID(ctx, "symphony", ClientID)` | `symphony`, `sym` |
| **TestBroker** | `"checksum"` + `"user_id"` | `SHA-256(order_id + order_timestamp + api_secret)` | `AccountByBrokerAndUserID(ctx, "testbroker", user_id)` | `testbroker` |

---

## 4. Response Protocol (Acknowledge & Drop)

Per `PLAN.md §3.4`:
Brokers resend webhooks indefinitely if non-200 HTTP statuses are returned. To prevent retry storms, `/broker-callback` adheres to:

- **200 OK**:
  - Valid JSON terminal order processed successfully.
  - Non-terminal status (e.g. `OPEN`, `TRIGGER PENDING`) dropped.
  - Checksum verification failure (invalid signature).
  - Unknown account or inactive tenant.
  - Unknown broker requested explicitly.
  - Full internal queue (dropped and flagged for reconciler pickup).
- **400 Bad Request**:
  - Malformed non-JSON payload.
  - Request body exceeds 1MB limit.

---

## 5. Ingestion & Fan-out Flow

```
Kite / Angel / Groww Postback
  │
  ▼
`POST /broker-callback` (`callback.AdaptiveRouter`)
  │
  ▼
Broker Adapter Handler (`internal/kite/callback.Handler`, etc.)
  │ Verify signature using account's `api_secret`
  │ Filter for terminal status: domain.IsTerminal(status)
  │
  ├── Role: Master → queue.Publisher[domain.MasterFill]
  │                     │
  │                     ▼
  │               listener.MasterFillConsumer
  │                     │ store.InsertMasterFill (idempotent, ErrDuplicate -> no-op)
  │                     ▼
  │               engine.HandleMasterFill (sizes & fans out to followers)
  │
  └── Role: Follower → queue.Publisher[domain.OrderUpdate]
                        │
                        ▼
                  listener.FollowerStatusConsumer
                        │ store.UpdateFollowerOrderStatus
                        ▼
                  store.AppendOrderEvent (audit log)
```

---

## 6. Registering a New Broker

In `cmd/server/main.go`:

```go
// 1. Create broker postback handler
brokerHandler := newBrokerPostbackHandler(store, masterFillQueue, orderUpdateQueue, logger)

// 2. Register with postback router
postbackRouter.Register("broker_name", brokerHandler)

// 3. (Optional) If payload has unique tokens, register fingerprint in internal/callback/router.go
```
