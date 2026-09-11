# Design Doc — Kite Copy-Trade Platform (V1)

**Status:** Final for V1 build
**Chosen design:** Master via WebSocket → API dispatch to followers → Follower status via Postback → Periodic reconciliation poll as backstop

---

## 1. Overview

Single master trader places orders manually on Kite (web/app). Platform detects the fill, replicates it proportionally to N follower accounts via API, tracks each follower's order outcome, and reconciles against source of truth periodically to catch anything the push channels miss. Each follower maps to exactly one master.

## 2. Why this design (decision record)

| Leg | Channel | Reason |
|---|---|---|
| Master order detection | WebSocket (`on_order_update`) | Postback only fires for orders placed via the app's own `api_key`. Master trades manually on Kite web/app — postback would never see it. WS is the only channel that captures manual trades regardless of origin. |
| Follower order status | Postback (HTTP webhook) | Follower orders ARE placed via the platform's own `api_key`, so postback legitimately fires. Stateless endpoint scales flat with follower count — no N persistent WS connections to babysit. |
| Backstop | Periodic reconciliation poll | Neither WS nor postback has a confirmed delivery guarantee from Zerodha. A money-moving system can't rely on push alone — poll closes the gap. |

## 3. Trade flow

```
Trader (Kite app/web)
   │ manual order
   ▼
Kite Broker — Master Session
   │ WS on_order_update (status: COMPLETE)
   ▼
WS Listener (Go, persistent conn, reconnect/backoff)
   │ durable write FIRST (Postgres), then push to fan-out
   ▼
Fan-out Engine
   │ read master↔follower map, compute scaled qty (capital ratio, lot-rounded)
   ▼
Order Worker Pool (1 per follower, throttled to Kite's ~10 orders/sec/account limit)
   │ egress via follower's own static IP/proxy
   ▼
Kite Broker — Follower Account
   │ Postback webhook (signed, checksum-verified) → platform's public HTTPS endpoint
   ▼
Postback Handler
   │ write status (success/failed/in-progress) to Postgres
   ▼
Reconciliation Poller (runs every N seconds, see §5)
   │ diffs master fills vs follower order status in DB
   ▼
Admin Dashboard (reads DB)
```

## 4. Components

- **WS Listener** — one persistent connection, authenticated as master. Handles reconnect with exponential backoff. On reconnect, triggers a catch-up order-history pull to cover any gap window before resuming live stream.
- **Fan-out Engine** — reads follower registry, computes per-follower quantity, dispatches to worker pool. Stateless beyond DB reads.
- **Order Worker Pool** — one goroutine per follower, isolated failure domain (one follower's error/timeout/rejection doesn't block others). Rate-limited per account.
- **Postback Handler** — single public HTTPS endpoint (`/webhooks/kite/postback`), verifies Zerodha's checksum signature (`order_id + order_timestamp + api_secret`, SHA256) before trusting payload, writes to DB.
- **Reconciliation Poller** — scheduled job, described in §5.
- **Postgres** — single instance for V1: `master_accounts`, `follower_accounts`, `follower_map` (enforces 1 master per follower), `orders`, `order_status_log`.
- **Admin Dashboard** — read-only view over DB for now.

## 5. Reconciliation poll — design

**Frequency:** every 30–60s during market hours (tune based on order volume; doesn't need to be sub-second, it's a safety net not the primary path).

**What it does each run:**
1. Pull master's recent order history via REST (`orders`/`trades` endpoint) for the polling window.
2. For each master fill in that window, verify a corresponding event was durably written by the WS listener. If missing → WS gap detected, backfill directly from this poll (acts as its own fallback trigger).
3. For each follower expected to have replicated that master fill, verify a terminal status (`COMPLETE`/`REJECTED`/`CANCELLED`) exists in `order_status_log`. If a follower order is stuck `PENDING`/missing past a timeout threshold (e.g. 2 min) → pull that follower's order status directly via REST as a postback-miss fallback.
4. Any unresolved drift (master filled, follower neither confirmed nor pollable-resolved) → raise an alert (log + notification hook), do not silently drop.

**Why this closes the gap:** WS reconnect windows and unconfirmed postback delivery are the two known blind spots in this design. The poll doesn't replace either channel (both are still faster in the common case) — it's the guaranteed-eventual-consistency layer underneath them.

## 6. Failure handling

- **Follower rejection (margin/RMS)** — logged as terminal state, no retry (re-placing could double-execute), surfaced to dashboard/alert.
- **Follower API timeout** — worker retries with backoff up to a cap, then marks failed; reconciliation poll independently verifies true state via REST regardless of what the worker believed.
- **WS disconnect** — reconnect + catch-up pull (see §5 step 2) before resuming live stream, so no silent gap.
- **Postback endpoint downtime** — reconciliation poll is the recovery path; no dependency on Zerodha retry behavior since that isn't confirmed/documented.
- **Partial fills** — copy trigger is on `status: COMPLETE` only (not `OPEN`), so partial fills on master don't cascade partial mismatched copies. If master itself partial-fills, wait for final terminal status before dispatching.

## 7. Security

- Postback payload checksum verified before any DB write — rejects spoofed calls to the public endpoint.
- Per-account API keys/secrets/access tokens encrypted at rest.
- Static IP enforced per account at the Kite Connect layer (existing constraint, unchanged by this design).

## 8. Compliance notes (carried from earlier scoping)

- Every follower order must carry the broker/exchange-assigned algo/tag ID.
- If followers are non-family third parties, platform likely needs registration as the broker's "agent" under SEBI's principal-agent framework — legal dependency, tracked separately from this design.
- `order_status_log` doubles as the audit trail — retain full history, not just latest state.

## 9. Explicit non-goals for V1

- No Redis/Kafka eve[118;1:3unt bus (in-memory channel + durable-write-first, per earlier decision).
- No OTel/Jaeger/Loki/Prometheus stack — basic logs + counters only.
- No multi-process split — single Go binary.
- No support for followers with multiple masters (1:1 enforced at schema level).

These remain the documented V2+ extension points from the earlier architecture doc.
