# Phase 6: Postback Webhook Ingestion

This document specifies Phase 6 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Ingest asynchronous HTTP order execution postbacks sent by Angel One webhooks, parse payloads, filter terminal status transitions, translate tokens to canonical symbols, and route events into EnvoyTrade's copy-trade queues.

---

## 2. Webhook Ingestion Pipeline (`callback/handler.go`)

### 2.1 Adaptive Router Integration
The postback handler registers with `unifiedcallback.AdaptiveRouter`:
- **Subpath Route**: `POST /broker-callback/angelone`
- **Query Parameter**: `POST /broker-callback?broker=angelone`
- **Zero-Allocation Byte Fingerprint**: Already implemented in `internal/callback/router.go`:
  ```go
  if bytes.Contains(body, []byte(`"uniqueorderid"`)) && bytes.Contains(body, []byte(`"filledshares"`)) {
      return "angelone"
  }
  ```

### 2.2 Processing & Canonical Event Mapping
```
Incoming HTTP POST /broker-callback
  ├── Parse: goangelone.ParsePostback(body) -> PostbackPayload
  ├── Account Lookup: AccountLookup.AccountByBrokerUserID(clientCode)
  ├── Filter: IsTerminalStatus(payload.OrderStatus) (COMPLETE, REJECTED, CANCELLED)
  ├── Symbol Translation: instrument.Cache.ResolveToSymbol(exchange, symboltoken)
  ├── Dispatch:
  │     ├── Master Account  -> queue.Publisher[domain.MasterFill]
  │     └── Follower Account -> queue.Publisher[domain.OrderUpdate]
  └── Response: HTTP 200 OK (Always acknowledge to prevent infinite webhook retries)
```

### 2.3 HTTP 200 OK Guarantee
In accordance with `PLAN.md §3.4`, the postback endpoint always responds with HTTP 200 OK except when the request body itself is malformed JSON (HTTP 400). Unknown accounts, non-terminal updates, or queue overflows are acknowledged and dropped to prevent broker retries.

---

## 3. Implementation Details

### 3.1 New Files
- `internal/angelone/callback/handler.go`: Webhook `Handler` struct, JSON decoding, account lookup, event dispatch.
- `internal/angelone/callback/handler_test.go`: Pure TDD unit test suite.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestCallback_MasterExecution_Success` | Terminal execution for master account | Resolves symbol, builds `MasterFill`, publishes to queue, responds HTTP 200. |
| `TestCallback_FollowerExecution_Success` | Terminal execution for follower account | Builds `OrderUpdate`, publishes to follower queue, responds HTTP 200. |
| `TestCallback_NonTerminal_Dropped` | Status is `"OPEN"` or `"TRIGGER PENDING"` | Acknowledged with HTTP 200; 0 messages published. |
| `TestCallback_UnknownAccount_Dropped` | Account not found in local database | Acknowledged with HTTP 200; drops event gracefully without crashing. |
| `TestCallback_MalformedJSON_BadRequest` | Corrupted JSON payload | Responds with HTTP 400 Bad Request. |
| `TestCallback_ReverseSymbolResolution` | Payload has `symboltoken: "3045"` | Resolves to canonical `NSE:SBIN-EQ`. |
