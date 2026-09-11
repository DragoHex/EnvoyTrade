# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

EnvoyTrade is a copy-trade platform on Zerodha Kite Connect: one master trading account, N follower accounts, strictly 1-master-per-follower. Single Go binary, single Postgres. `docs/PLAN.md` is the full V1 implementation plan (package layout, data model, Kite integration details, milestones); `docs/ARCHITECTURE.md` covers system/deployment diagrams. Read `docs/PLAN.md` before extending scope — it documents *why*, not just what.

The core V1 copy-trade pipeline is wired in `cmd/server/main.go`:
- **Fan-out mechanism** (`internal/engine`): master fill → lot-sized, idempotent follower orders.
- **Order-callback ingestion** (`internal/kite/callback`, `internal/listener`): Kite postback (`POST /broker-callback`) + WS ticker (`callback.MasterTicker`) → in-memory queue (`internal/queue/memchan`) → store/engine.
- **Broker adapter & worker pool** (`internal/kite`, `internal/worker`): real `kite.Broker` (`*kiteconnect.Client` wrapper) with per-account static proxy egress (`RESTClientFor`), per-account rate-limiting (~10 orders/s), and timeout retries (up to 3 attempts). Margin/RMS rejections fail immediately.
- **Reconciliation backstop** (`internal/recon`): periodic REST poller (30–60s) backfilling missed master fills, resolving stuck follower orders (>2 min), and raising drift alerts.
- **Admin HTTP API** (`internal/httpapi`): dashboard groups, accounts, and rebalance actions.
- **Structured logging** (`slog`): zero-dependency JSON/text logging across server lifecycle, HTTP middleware, engine, worker pool, listener, and recon. Defaults to `/var/log/envoytrade/app.log` (configurable via `LOG_FILE`, `LOG_FORMAT`, `LOG_LEVEL`, `LOG_TO_STDOUT`) with graceful fallback to `stdout` when unprivileged.

Remaining future scope per `docs/PLAN.md`: interactive daily auth web flow / refresh-token daemon, multi-account session management, kill switch panic button, and full metrics/tracing.

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

# Frontend tests & build
pnpm -C frontend test --run
pnpm -C frontend build
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
- `internal/queue.Publisher[T]`/`Consumer[T]` is the dispatch-transport seam — generic over event type (`domain.MasterFill`, `domain.OrderUpdate`), so `memchan.Queue[T]` serves both without a second implementation. Swapping `memchan` for Redis Streams/Kafka later should be a drop-in.
- `internal/kite/callback.AccountLookup` and `internal/listener.MasterFillStore`/`FollowerStatusStore`/`MasterFillEngine` are each declared by their consumer, satisfied structurally by `internal/store/postgres.Store` / `internal/engine.Engine` — same pattern as the fan-out seams, no shared "Store" interface.

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
- **Worker retry and rate-limiting**: `internal/worker.Pool` limits dispatch (~10 orders/s per account) and retries network timeouts with exponential backoff up to 3 attempts. Margin/RMS/input rejections fail terminally on first attempt (no retry to prevent duplicate executions). Circuit breaker and global kill switch remain later milestones.
- **Money/ratio math is decimal-only**, via `shopspring/decimal`, never `float64` — `SizeOrder` is called out in comments as the highest-risk function in the system.

### Order-callback ingestion (postback + WS)

```
Kite postback (HTTP)                    Kite WS ticker (master only)
  → callback.Handler                       → callback.MasterTicker
     verify checksum (per-account            (wraps *kiteticker.Ticker;
     api_secret) → drop non-terminal          PLAN.md §3.4: shared global
     → role==master: ToMasterFill             dialer means at most one
       role==follower: ToOrderUpdate          Ticker per process)
     → queue.Publisher[T]                  → same MasterFill queue
              ↓                                       ↓
listener.MasterFillConsumer.Handle    (drains queue.Consumer[domain.MasterFill])
  → store.InsertMasterFill (ErrDuplicate → no-op) → engine.HandleMasterFill

listener.FollowerStatusConsumer.Handle (drains queue.Consumer[domain.OrderUpdate])
  → store.UpdateFollowerOrderStatus by broker_order_id (ErrNotFound → drop, not fatal)
  → store.AppendOrderEvent (audit trail)
```

- **Postback is the general path for both master and followers** — every account (master and each follower) has its own Kite Connect app/secret (`accounts.api_secret`), so postback works uniformly. WS ticker is a **master-only fast path**; delivering the same master fill via both is safe (`InsertMasterFill`'s unique constraint no-ops the second delivery).
- **Checksum uses the resolved account's own secret**, looked up by the postback's `user_id` via `AccountLookup.AccountByBrokerUserID` — never a single process-wide secret.
- **The postback HTTP handler always responds 200** except when the body itself doesn't parse as JSON (400) — an unknown account, bad checksum, non-terminal status, or a full queue are all "acknowledge and drop," since Kite retries on non-200 and none of those are worth a retry.
- **Only terminal order statuses (`COMPLETE`/`REJECTED`/`CANCELLED`, `callback.IsTerminal`) are forwarded** by either delivery path — intermediate states like `OPEN`/`TRIGGER PENDING` are dropped before ever reaching a queue.
- `gokiteconnect` has no postback support at all (checked before building this — the only checksum code in the SDK is the unrelated login-session checksum) and no payload struct for the postback body; `internal/kite/callback` decodes postback JSON directly into `kiteconnect.Order` (reused, not copied) since Kite's WS and postback payloads share that shape.
- `internal/kite/callback.Conn` narrows `*kiteticker.Ticker` to what `MasterTicker` needs, purely so tests can fake it — never edit the vendored SDK itself.

### Postgres specifics

- `internal/store/postgres.NewPool` must be used instead of raw `pgxpool.New` — it registers the `shopspring/decimal` pgx codec (`decimalpgx.Register`) on every connection via `AfterConnect`. Without it, `numeric` columns won't scan into `decimal.Decimal` fields.
- Nullable `numeric` columns (e.g. `average_price`) must be scanned into a `*decimal.Decimal` and copied out — scanning `NULL` directly into `decimal.Decimal` panics.
- Migrations are plain SQL files under `internal/store/postgres/migrations/`, embedded via `//go:embed` and applied in order by `Store.Migrate` — every statement is `IF NOT EXISTS`/`duplicate_object`-guarded, so it's safe to call on every `cmd/server` startup as well as against a fresh database (tests, via testcontainers). `0003_account_api_secret.sql` adds `accounts.api_secret` — every account has its own Kite Connect app, needed to verify that account's postback checksums. `0005_groups_and_names.sql` splits groups into a dedicated `groups` table (`id`, `name`, `master_id` with `ON DELETE CASCADE`), links `follow_links` directly via `group_id`, and adds `name` to `accounts`.

### Test conventions

- Integration tests (`store/postgres`, `engine`) are gated behind `//go:build integration` and spin up real Postgres via testcontainers-go — no mocked DB layer.
- Worker isolation/leak tests use `go.uber.org/goleak` to assert no goroutines survive `Pool.Shutdown()`.
- `internal/kite/fake.Broker` is a programmable fake (injectable `Latency`, `Err`, `OrderID`, and call recording) used across worker and kite/fake tests — not a mock framework.
- `internal/kite/callback` tests use small hand-written fakes (`fakeAccounts`, generic `fakePublisher[T]`, `fakeConn`) rather than a mock framework, same style as the rest of the repo.

## Frontend design principles

- **Never reload/remount a whole component for a small state change.** Only the piece that actually changed should update. A list refreshing after one row's action shouldn't blank or remount the other rows.
- **Loading state gets a skeleton of the real layout, not a "Loading…" text line.** Cap it with a timeout — stop showing the skeleton and say something's wrong if data never arrives.
- **Icon-only buttons get a hover tooltip with the action's name.** Never ship an icon-only control with no text anywhere.
- **Async actions (anything that hits the network on click) give immediate feedback**: disable the control and show a busy/spin state while pending, then confirm the outcome (success or failure) after — never leave a clicked button looking inert while work happens.
- **Toggles that represent on/off state look like switches, not checkboxes.**
- **Buttons and icons must stay visible and legible in both light and dark mode** — check contrast in both themes, not just one.
- **Destructive/irreversible actions are gated behind a confirmation step.**
- **A modal's dimmed backdrop and its content panel are visually and structurally distinct** — the backdrop dims the page, the panel stands out from it.
- **Verify UI changes by actually looking at the rendered result**, not just by reading the code — a change that looks right in source can still render wrong.
- **Show broker logo instead of plain text.** Use `<BrokerLogo broker={...} />` (`frontend/src/components/BrokerLogo.tsx`). An extensible `BROKER_REGISTRY` maps broker IDs (`kite`, `zerodha`) to official vector logos with accessible hover tooltip (`data-tooltip`). Unknown brokers fall back gracefully to a text badge.
- **Never display UUIDs on the UI.** Display human-readable `name` when present, falling back to `brokerAccountId`, never raw database UUID.
- **Distinct iconography for actions.** Use dedicated SVG icons with tooltips: pencil (`EditIcon`) for edit, person-with-minus (`UserMinusIcon`) for remove from group, trash bin (`TrashIcon`) for delete.
- **Fixed table layout prevents shifts on state toggle.** Use `table-layout: fixed` and explicit column widths/classes so buttons toggling text (e.g. "Enable"/"Disable") or badge status never cause table column jitter.

## UI color palette

Source of truth for `docs/UI-PLAN.md` — reuse for any future UI work, don't re-derive.

| Role | Light | Dark | Hex meaning |
|---|---|---|---|
| Primary/Brand | `#44475b` | `#1a1c24` | Dark slate |
| Accent (Action) | `#04b488` | `#06d896` | Teal/Green |
| Border/Divider | `#f0f0f2` | `#2a2c34` | Light gray / Charcoal |
| Text Primary | `#44475b` | `#f0f0f2` | Slate / Off-white |
| Background | `#ffffff` | `#0f1117` | White / Near-black |
| Surface | `#f5f5f7` | `#1a1c24` | Light gray / Dark gray |
