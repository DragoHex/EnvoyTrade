# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

EnvoyTrade is a copy-trade platform on Zerodha Kite Connect: one master trading account, N follower accounts, strictly 1-master-per-follower. Single Go binary, single Postgres. `docs/PLAN.md` is the full V1 implementation plan (package layout, data model, Kite integration details, milestones); `docs/ARCHITECTURE.md` covers system/deployment diagrams. Read `docs/PLAN.md` before extending scope — it documents *why*, not just what.

Only the **fan-out mechanism** (the core copy-trade path: master fill → sized, idempotent, isolated follower orders) is built so far. Everything else `docs/PLAN.md` describes — the WS ticker listener, real Kite REST wiring, session/auth lifecycle, reconciliation, retry/circuit-breaker, kill switch, admin dashboard, metrics/tracing — is not implemented yet.

## Commands

```sh
go build ./...
go vet ./...

# Fast unit tests (domain, queue/memchan, worker, kite/fake) — no external deps
go test ./...

# Integration tests (store/postgres, engine) require Docker/Podman for testcontainers-go
go test -tags integration ./...

# Single test
go test -tags integration ./internal/engine/... -run TestHandleMasterFill_RedeliveryIsANoOp -v
```

Docker isn't available in this environment — use **Podman** instead. Testcontainers needs `DOCKER_HOST` pointed at the Podman socket:

```sh
podman machine start   # if not already running
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test -tags integration ./...
```

`./gokiteconnect` is a **vendored third-party SDK** (`github.com/zerodha/gokiteconnect/v4`, aliased via a `replace` in `go.mod`). Never edit it — it's consumed as-is for reference and by `internal/kite`. If its shape is a problem, work around it from the service side.

## Architecture

### The seam rule

Interfaces are defined in the **consuming** package, not the implementing one — idiomatic Go, and explicit in comments throughout as "PLAN.md §1's seam rule". Concretely:
- `internal/broker` defines `Broker` (`PlaceOrder`) and broker-agnostic `OrderParams`/`OrderResponse` types. `internal/kite/fake` implements it for tests; a real Kite-backed implementation would live in `internal/kite` and adapt to/from `kiteconnect.OrderParams`. Adding a second broker means writing a new adapter package, never touching `internal/broker` or the consumers.
- `internal/engine.Store` and `internal/worker.Store` each declare only the store methods *they* need — `internal/store/postgres.Store` satisfies both structurally, with no shared "Store" interface anywhere.
- `internal/engine.Dispatcher` is declared by engine; `internal/worker.Pool` satisfies it structurally. Engine never imports worker.
- `internal/queue.Publisher`/`Consumer` is the dispatch-transport seam — swapping `memchan` for Redis Streams/Kafka later should be a drop-in.

`internal/domain` has zero dependencies beyond `shopspring/decimal` — no store, no broker, no queue. It holds pure types (`MasterFill`, `FollowLink`, `FollowerOrder`, `Job`, `Instrument`, `OrderEvent`) and pure logic (`SizeOrder`, `IdempotencyTag`), and the two sentinel errors (`ErrDuplicate`, `ErrNotFound`) other packages recognize without importing a concrete store.

### Fan-out data flow

```
MasterFill (signal) → engine.HandleMasterFill
  → store.EnabledFollowLinks(masterID)         one call resolves every follower to fan out to
  → per follower:
      store.InstrumentLotSize(exchange, symbol)  canonical lot size, NEVER trusted from the signal
      domain.SizeOrder(...)                      pure, decimal-only, floors to lot multiples
      store.InsertFollowerOrder(...)             idempotency tag persisted BEFORE any broker call
      dispatcher.Dispatch(followerID, job)        non-blocking; false → dead-lettered, not retried
  → store.SetMasterFillDispatchState(dispatched)
```

- **Idempotency is DB-enforced, not just application logic.** `IdempotencyTag(masterFillID, followerID)` is deterministic; `follower_orders` has unique constraints on the tag and on `(master_fill_id, follower_id)`; `master_fills` is unique on `(master_id, broker_order_id, filled_quantity, status)`. A duplicate insert returns `domain.ErrDuplicate`, and `engine.fanOutToFollower` treats that as a silent no-op (redelivery), not an error.
- **Lot size always comes from the `instruments` table**, resolved by `(exchange, tradingsymbol)` — never from a value carried on the fill payload. This matters most for F&O: lot sizes vary per contract and change over a contract's life, so trusting the signal risks silently wrong quantities. A symbol missing from `instruments` (`domain.ErrNotFound`) resolves to lot size 0, which `SizeOrder` already reports as `ReasonBadInstrument` — visible on the `follower_order` row, not silently dropped.
- **Every sizing outcome is visible, never silent.** `SizingReason` (`ReasonOK`, `ReasonBelowOneLot`, `ReasonCapped`, `ReasonBadInstrument`, `ReasonInvalidRatio`) is persisted alongside every `follower_order`, including zero-quantity ones — a skipped follower must be auditable, not invisible.
- **Worker isolation is structural, not best-effort.** `internal/worker.Pool` runs one goroutine + one buffered channel per follower. `Dispatch` never blocks — a full channel returns `false` immediately so one slow/backed-up follower can never stall fan-out to others. A panic inside one follower's placement goroutine is recovered and recorded as a failure; it cannot take down another follower's goroutine. `Pool.Shutdown()` closes an internal `done` channel (not just `ctx`) so workers terminate deterministically even if the caller passed `context.Background()`.
- **No retry, no circuit breaker, no kill switch in this slice** — a broker error is recorded as a terminal failure on first attempt. These are later milestones per `docs/PLAN.md` §4.4 and explicitly out of scope for the current code.
- **Money/ratio math is decimal-only**, via `shopspring/decimal`, never `float64` — `SizeOrder` is called out in comments as the highest-risk function in the system.

### Postgres specifics

- `internal/store/postgres.NewPool` must be used instead of raw `pgxpool.New` — it registers the `shopspring/decimal` pgx codec (`decimalpgx.Register`) on every connection via `AfterConnect`. Without it, `numeric` columns won't scan into `decimal.Decimal` fields.
- Nullable `numeric` columns (e.g. `average_price`) must be scanned into a `*decimal.Decimal` and copied out — scanning `NULL` directly into `decimal.Decimal` panics.
- Migrations are plain SQL files under `internal/store/postgres/migrations/`, embedded via `//go:embed` and applied in order by `Store.Migrate` (not idempotent — run once against a fresh database, as tests do via testcontainers).

### Test conventions

- Integration tests (`store/postgres`, `engine`) are gated behind `//go:build integration` and spin up real Postgres via testcontainers-go — no mocked DB layer.
- Worker isolation/leak tests use `go.uber.org/goleak` to assert no goroutines survive `Pool.Shutdown()`.
- `internal/kite/fake.Broker` is a programmable fake (injectable `Latency`, `Err`, `OrderID`, and call recording) used across worker and kite/fake tests — not a mock framework.
