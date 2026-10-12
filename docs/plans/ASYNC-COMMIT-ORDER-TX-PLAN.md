# Implementation Plan: Transaction-Scoped `synchronous_commit = off` for Order Hot Path

## Goal Description
Enable PostgreSQL `SET LOCAL synchronous_commit = 'off'` strictly for trade and order write transactions (`master_fills`, `follower_orders`, `order_events`, `pending_order_updates`), while keeping account management, user authentication, sessions, groups, and migrations on full durable `synchronous_commit = 'on'`.

This cuts order database write latency from **2–5ms down to ~0.2ms** by eliminating disk fsync blocking on trade execution, while guaranteeing that account credentials, password changes, token updates, and tenant boundaries are never susceptible to WAL lag.

---

## User Review Required
> [!IMPORTANT]
> `SET LOCAL synchronous_commit = 'off'` applies **only to the active transaction block**. When the transaction commits, PostgreSQL buffers the WAL in RAM and flushes it asynchronously (within default `wal_writer_delay` = 200ms).
> 
> - **In-memory ACID preserved**: Constraints (`UNIQUE`, foreign keys), rollbacks, and atomic multi-row updates remain 100% consistent.
> - **Zero impact on account safety**: `CreateAccount`, `DeleteAccount`, `SetAccountAccessToken`, `SetAccountEncryptedCredentials`, `CreateGroup`, and auth/session tables will continue using default `synchronous_commit = 'on'`.

---

## Proposed Changes

### 1. Store Layer (`internal/store/postgres`)

#### [MODIFY] [`internal/store/postgres/store.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/store/postgres/store.go)

Introduce a private transaction runner `beginOrderTx(ctx)` that starts a transaction with `SET LOCAL synchronous_commit = 'off'`:

```go
func (s *Store) beginOrderTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin order tx: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = 'off'"); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("set local synchronous_commit off: %w", err)
	}
	return tx, nil
}
```

Apply `beginOrderTx` strictly to the order/trade write path:

1. **`InsertFollowerOrdersBatch`**:
   - Wrap the `UNNEST` query inside `beginOrderTx(ctx)` so bulk follower order preparation writes to RAM buffer without fsync blocking.
2. **`InsertMasterFill`**:
   - Wrap `InsertMasterFill` in `beginOrderTx(ctx)`.
3. **`UpdateFollowerOrderPlaced`**:
   - Replace raw `s.pool.Begin(ctx)` with `beginOrderTx(ctx)` so updating `broker_order_id`, consuming `pending_order_updates`, and inserting status events commit in ~0.2ms.
4. **`InsertFollowerOrder` & `InsertDirectFollowerOrder`**:
   - Wrap in `beginOrderTx(ctx)`.
5. **`UpdateFollowerOrderStatus` & `AppendOrderEvent`**:
   - Wrap status transitions in `beginOrderTx(ctx)`.

All other methods (`CreateAccount`, `DeleteAccount`, `SetAccountAccessToken`, `SetAccountEncryptedCredentials`, `CreateGroup`, `Migrate`, etc.) continue using regular pool queries or standard `pool.Begin(ctx)` with full `synchronous_commit = 'on'`.

---

## Verification Plan

### Automated Tests
1. **Unit & Integration Tests**:
   - Run integration tests with Podman Testcontainers:
     ```sh
     export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
     export TESTCONTAINERS_RYUK_DISABLED=true
     go test -tags integration ./internal/store/postgres/... -v
     ```
   - Assert in a dedicated test `TestStore_OrderTx_SynchronousCommitIsOff` that:
     - `SHOW synchronous_commit` inside `beginOrderTx` returns `'off'`.
     - Outside `beginOrderTx` (in standard connection pool queries), `SHOW synchronous_commit` returns `'on'`.
2. **Full Test Suites**:
   - Race detector: `go test -race ./...`
   - E2E Test Suite: `go test -tags e2e ./tests/e2e/... -v`

### Manual Benchmark Verification
1. Rebuild and run benchmark with `python3 scripts/stress_test_local.py`.
2. Analyze with `python3 scripts/analyze_copy_latency.py` to confirm further reduction in DB insertion time.
