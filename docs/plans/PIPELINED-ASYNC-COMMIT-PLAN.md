# Implementation Plan: Single-Roundtrip Pipelined `pgx.Batch` for Order Hot Path

## Goal Description
In the previous optimization, wrapping `SET LOCAL synchronous_commit = 'off'` in standard `BeginOrderTx` introduced 3 sequential network roundtrips over the socket (`BEGIN`, `QUERY`, `COMMIT`), increasing P50 median latency from `0.95ms` to `1.41ms` (+0.46ms) even though P99 tail latency improved significantly (`11.28ms` down to `6.11ms`).

The goal is to bundle `BEGIN; SET LOCAL synchronous_commit = 'off'`, the order query, and `COMMIT` into a **single network roundtrip** using **`pgx.Batch`** (Postgres Extended Query protocol pipelining). This eliminates the 2 extra roundtrips, bringing P50 back to **sub-millisecond (`~0.8ms`)** while keeping P99 tail latency low (`~5ms`) and retaining complete isolation from account/auth tables.

---

## User Review Required
> [!IMPORTANT]
> **Pipelining Mechanics (`pgx.Batch`)**:
> `pgx.Batch` sends all queued commands over the TCP socket in a single network buffer frame without waiting for intermediate server responses:
> 1. `b.Queue("BEGIN; SET LOCAL synchronous_commit = 'off';")`
> 2. `b.Queue(batchQuery, args...)`
> 3. `b.Queue("COMMIT;")`
> 
> Executed via `s.pool.SendBatch(ctx, b)`.
> - **Total Network Roundtrips**: Exactly **1** (same as a naked single query).
> - **Postgres Execution**: Runs inside a transactional block with `synchronous_commit = 'off'`.
> - **State Isolation**: When `COMMIT` finishes, the connection returned to the pool immediately reverts to `synchronous_commit = 'on'`.

---

## Proposed Changes

### Store Layer (`internal/store/postgres`)

#### [MODIFY] [`internal/store/postgres/store.go`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/store/postgres/store.go)

1. **`InsertFollowerOrdersBatch`**:
   Replace the 3-step `BeginOrderTx` / `tx.Query` / `tx.Commit` sequence with pipelined `pgx.Batch`:
   ```go
   var b pgx.Batch
   b.Queue("BEGIN; SET LOCAL synchronous_commit = 'off';")
   b.Queue(batchQuery, masterFillIDs, followerIDs, tags, intendedQtys, lotSizes, sizingReasons, origins, tradingsymbols, exchanges, products, transactionTypes, orderTypes)
   b.Queue("COMMIT;")

   br := s.pool.SendBatch(ctx, &b)
   defer br.Close()

   if _, err := br.Exec(); err != nil {
       return nil, fmt.Errorf("begin batch order tx: %w", err)
   }

   rows, err := br.Query()
   if err != nil {
       return nil, fmt.Errorf("execute follower orders batch insert: %w", err)
   }
   defer rows.Close()

   res := make(map[string]int64, len(orders))
   for rows.Next() {
       var id int64
       var tag string
       if err := rows.Scan(&id, &tag); err != nil {
           return nil, fmt.Errorf("scan follower orders batch result: %w", err)
       }
       res[tag] = id
   }
   if err := rows.Err(); err != nil {
       return nil, fmt.Errorf("iterate follower orders batch result: %w", err)
   }
   rows.Close()

   if _, err := br.Exec(); err != nil {
       return nil, fmt.Errorf("commit batch order tx: %w", err)
   }
   ```

2. **Keep `BeginOrderTx` for Multi-Step Dependent Methods**:
   Methods like `UpdateFollowerOrderPlaced` perform multiple dependent queries (`UPDATE`, `DELETE RETURNING`, conditional `UPDATE`, `INSERT order_events`) based on runtime logic, so they continue using `BeginOrderTx(ctx)`.

---

## Verification Plan

### Automated Tests
1. **Integration Tests (`//go:build integration`)**:
   Run integration tests with Podman Testcontainers:
   ```sh
   export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
   export TESTCONTAINERS_RYUK_DISABLED=true
   go test -tags integration ./internal/store/postgres/... -v
   ```
   Verify:
   - `TestInsertFollowerOrdersBatch_AtomicityAndConflictHandling`: PASS
   - `TestStore_OrderTx_SynchronousCommitIsOff`: PASS

2. **Full Test Suites**:
   - `go test -tags integration ./...`
   - `go test -tags e2e -count=1 ./tests/e2e/ -v`

### Manual Verification
1. Re-build binary and start server on port 8080.
2. Verify latency metrics in stress test.
