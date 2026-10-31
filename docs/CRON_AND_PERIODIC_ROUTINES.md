# Scheduled Jobs and Periodic Routines

This document details all operating system cron jobs, in-process periodic routines, and background loops across EnvoyTrade.

> [!IMPORTANT]
> **Maintenance Rule**: Whenever a scheduled job, periodic ticker, or background worker loop is added, updated, or removed, this document MUST be updated in the same change.

---

## 1. Operating System Cron Jobs

Installed via `scripts/install.sh` from `packaging/cron/envoytrade-maintenance.cron`.

| Job Name | Schedule | Timezone | Command / Target | Description |
|---|---|---|---|---|
| **Nightly DB Maintenance & Archival** | `0 1 * * *` (01:00 AM daily) | `Asia/Kolkata` (IST) | `/usr/local/bin/envoytrade-db-maintenance` (`scripts/db_maintenance.sh`) | Runs after MCX commodity close (11:30/11:55 PM IST). Performs: 1) Parquet archival of old terminal orders to `/var/backups/envoytrade/archive`, 2) `VACUUM ANALYZE` on active tables, 3) Compressed `pg_dump` backup, 4) Retention cleanup (>14 days). Logs to `/var/log/envoytrade/maintenance.log`. |

---

## 2. In-Process Go Periodic Routines (`cmd/server/main.go`)

These routines run as background goroutines inside the EnvoyTrade server process.

| Routine Name | Frequency / Schedule | Code Location | Key Functionality |
|---|---|---|---|
| **Reconciliation Poller** | Every `10s` (`PollInterval`) | `internal/recon/reconciler.go` (`poller.Run`) | REST poller checking active masters and followers. Backfills missed fills (WS gap fallback), detects stuck follower orders (>15s), reconciles terminal states, and triggers drift alerts. |
| **Portfolio Syncer** | Every `10s` ticker | `cmd/server/main.go` | Loops through all active accounts and executes `syncer.SyncAccountPortfolio` to refresh broker positions and MTM in DB. |
| **Session Pruner** | Every `1h` ticker | `cmd/server/main.go` | Executes `store.DeleteExpiredSessions` to remove expired HTTP session tokens from the `user_sessions` table. |
| **Dhan Token Renewer** | `12:15 AM` & `06:00 AM IST` daily | `internal/dhan/renewer.go` (`renewer.Run`) | Scheduled JWT token renewal for active Dhan accounts via DhanHQ API before trading hours, triggering ticker/worker restarts on refresh. |
| **Kotak Neo Token Renewer** | `06:30:00 AM IST` daily | `internal/kotak/renewer.go` (`renewer.Start`) | Scheduled 2-step TOTP + MPIN authentication renewal for active Kotak Neo accounts before market open, updating session tokens and triggering ticker/worker restarts on refresh. |
| **Instrument Master Daily Ingestion & Pruning** | `05:00:00 AM IST` daily | `internal/instrument/syncer.go` (`syncer.StartDailyRoutine`) | Downloads and parses fresh instrument masters for Kite, Dhan, & Kotak Neo (skipping cash equities for Kite & Dhan to save F&O derivatives/commodities, while syncing Kotak nse_cm & F&O). Only upon 100% successful parse, transactionally prunes the broker's rows, batch inserts new records, runs `VACUUM ANALYZE instruments`, and re-loads current-month/next-3-weeks F&O into the pre-allocated 10,000-capacity cache. |
| **Engine Lot Size Cache Wipe** | `01:00:00 IST` daily | `cmd/server/main.go` (`eng.ClearLotSizeCache`) | Clears in-memory instrument lot size cache so contract/lot size changes take effect next market open. |

---

## 3. Continuous Background Workers & Ingestion Loops

While not timer/cron-based, these long-running goroutines execute throughout the server lifecycle:

| Component | Code Location | Description |
|---|---|---|
| **Master Fill Consumer** | `internal/listener/listener.go` | Continuously drains `masterFillQueue` (`domain.MasterFill`), inserts fills to DB, and triggers copy-trade engine. |
| **Follower Status Consumer** | `internal/listener/listener.go` | Continuously drains `orderUpdateQueue` (`domain.OrderUpdate`), updates follower order statuses in DB, and appends audit events. |
| **Follower Worker Pool** | `internal/worker/pool.go` | Dedicated goroutine + buffered queue per follower account with ~10 orders/s rate limiter and exponential retry. |
| **Master Ticker Streams** | `internal/ticker/manager.go` | WebSocket tickers streaming real-time master fills for active master accounts (Kite / Dhan / Kotak Neo / Testbroker). |
| **Instrument Unified Cache Pre-load** | `internal/instrument/cache.go` | On server startup and daily at 05:00 AM IST, auto-loads all stock/commodity F&O contracts for the current month and next 3 weeks into a pre-allocated 10,000 capacity map. |
