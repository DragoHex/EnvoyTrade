# EnvoyTrade V1 Release: Database Migration & Porting Guide

This document describes the schema architecture of the EnvoyTrade fresh release and provides end-to-end procedures for upgrading existing deployed environments, porting data across hosts, and validating data integrity.

---

## 1. Overview & Schema Aggregation

In this release, all database migrations and schema patches have been consolidated into a **unified single-pass baseline**:
- [`internal/store/postgres/migrations/0001_init.sql`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/store/postgres/migrations/0001_init.sql) defines the complete canonical schema in topological dependency order.
- Redundant `ALTER TABLE` statements and runtime `DO $$` rename blocks have been removed. Fresh deployments initialize in a single DDL round trip with zero startup lag.

### Canonical Schema Highlights
1. **`follow_links.clone_factor`**:
   The multiplier ratio is canonically named `clone_factor numeric(10,6) NOT NULL DEFAULT 1.0 CHECK (clone_factor > 0)`.
2. **`follower_orders` Direct Order Support**:
   - `master_fill_id` is nullable (`bigint REFERENCES master_fills(id)`), supporting direct manual, square-off, and rebalance orders alongside copy-trade orders.
   - Includes order metadata: `origin text NOT NULL DEFAULT 'copy_trade'`, `tradingsymbol`, `exchange`, `product`, `transaction_type`, `order_type`.
   - Unique partial index: `idx (follower_id, broker_order_id) WHERE broker_order_id IS NOT NULL`.
3. **`account_margins.product_mtm`**:
   Includes `product_mtm jsonb NOT NULL DEFAULT '{}'::jsonb` for product-level (e.g. MIS vs NRML) MTM breakdown.
4. **Tenant Scoping & Security**:
   `accounts` and `groups` are strictly scoped by `user_id uuid REFERENCES users(id) ON DELETE CASCADE`.
5. **Zero-Lag Postback Stash**:
   Includes `pending_order_updates` to decouple high-frequency Kite broker callbacks from worker dispatch.

---

## 2. Upgrade Scenarios

### Scenario A: Fresh Deployment
On a clean PostgreSQL database, start the new EnvoyTrade binary or run `go run cmd/server/main.go`.
`store.Migrate(ctx)` executes `0001_init.sql` directly:
```bash
./bin/envoytrade
```
All tables, constraints, enums, and indexes are created in one atomic pass.

---

### Scenario B: In-Place Upgrade of an Existing Deployed Database
If you have an existing EnvoyTrade instance running in production or staging, use [`scripts/migrate_deployed_db.sh`](file:///Users/msp/MSP/Projects/EnvoyTrade/scripts/migrate_deployed_db.sh).

#### Pre-Upgrade Checklist
1. **Schedule Maintenance Window**: Run outside market hours (or when copy-trade activity is idle).
2. **Stop the EnvoyTrade Daemon**:
   ```bash
   # Systemd (Linux)
   sudo systemctl stop envoytrade

   # Launchd (macOS)
   launchctl unload ~/Library/LaunchAgents/com.envoytrade.server.plist
   ```

#### 1. Dry-Run Inspection
Verify current schema state without applying any changes:
```bash
./scripts/migrate_deployed_db.sh --dry-run
```
Expected output displays column presence (`capital_ratio` vs `clone_factor`, `product_mtm`, order count) and the planned operations.

#### 2. Execute Migration
Run the automated migration tool:
```bash
# If using native Postgres
./scripts/migrate_deployed_db.sh

# Or via Makefile
make db-migrate

# If running within a container (e.g. Podman/Docker)
./scripts/migrate_deployed_db.sh --container envoytrade-db
```

The script automatically:
- Creates a timestamped, gzip-compressed safety backup with SHA-256 checksum in `/var/backups/envoytrade/` (or `./backups/`).
- Transactionally executes [`scripts/migrate_deployed_db.sql`](file:///Users/msp/MSP/Projects/EnvoyTrade/scripts/migrate_deployed_db.sql).
- Renames `capital_ratio` &rarr; `clone_factor` (preserving exact values).
- Relaxes `follower_orders.master_fill_id` `NOT NULL` constraint.
- Adds metadata columns (`origin`, `tradingsymbol`, `exchange`, etc.).
- **Enriches historical data**: Backfills trade symbols and product types from linked `master_fills` onto historical `follower_orders`.
- Assigns any orphaned accounts/groups to the primary tenant user.
- Verifies post-migration row counts and invariants.

#### 3. Start New Release Binary
Deploy the updated `envoytrade` binary and restart the service:
```bash
# Systemd
sudo systemctl start envoytrade

# Verify service logs
journalctl -u envoytrade -f
```

---

### Scenario C: Cross-Host Data Porting (ETL to New Database)
When migrating to a new PostgreSQL server (e.g. staging to production or cloud migration), use [`scripts/port_deployed_data.sh`](file:///Users/msp/MSP/Projects/EnvoyTrade/scripts/port_deployed_data.sh):

```bash
./scripts/port_deployed_data.sh \
  --source-db "postgres://envoytrade:password@old-host:5432/envoytrade" \
  --target-db "postgres://envoytrade:password@new-host:5432/envoytrade"
```

The porting script:
1. Applies the fresh canonical `0001_init.sql` schema to the target database.
2. Aligns the source schema transactionally to avoid dump errors.
3. Streams data table by table in topological dependency order (`users` &rarr; `accounts` &rarr; `groups` &rarr; `follow_links` &rarr; `master_fills` &rarr; `follower_orders`, etc.).
4. Resets identity sequences (`setval`) on the target database.
5. Prints a side-by-side row count comparison table asserting 100% parity.

---

### Scenario D: Encryption Key Migration & Rotation
If the target host uses a different `ENCRYPTION_KEY` from the source database, account credentials (`encrypted_password`, `encrypted_totp_secret`) must be re-encrypted before starting the application:

```bash
# Test with dry-run
go run ./scripts/reencrypt_credentials.go \
  -old-key "old-key-from-source" \
  -new-key "new-key-on-target" \
  -db "postgres://envoytrade:password@new-host:5432/envoytrade" \
  -dry-run

# Apply re-encryption transactionally
go run ./scripts/reencrypt_credentials.go \
  -old-key "old-key-from-source" \
  -new-key "new-key-on-target" \
  -db "postgres://envoytrade:password@new-host:5432/envoytrade"
```

---

## 3. Post-Migration Verification Checklist

After upgrading or porting, verify the following:

| Check | Method | Expected Result |
|---|---|---|
| **Health API** | `curl -f http://localhost:8080/healthz` | HTTP 200 `{"status":"ok"}` |
| **Authentication** | Log in via Web UI or `POST /api/v1/auth/login` | Session cookie set, dashboard loads |
| **Accounts** | Open `/accounts` page | All master & follower accounts visible with broker status |
| **Clone Factors** | Check follower multipliers in Groups | Multipliers match historical capital ratios |
| **Historical Orders** | Inspect follower order drawer | Symbols and order types displayed (not blank) |
| **Positions & MTM** | Check Account / Group details | Positions list and product MTM breakdown displayed |
| **Log Inspection** | `tail -f /var/log/envoytrade/app.log` | Zero SQL errors, clean fanout/poller loops |

---

## 4. Rollback Runbook

If any critical issue arises during or immediately after cutover:

1. **Stop the new binary**:
   ```bash
   sudo systemctl stop envoytrade
   ```

2. **Restore pre-migration database snapshot**:
   The migration tool automatically saves a backup with a `.sha256` checksum file in the backup directory:
   ```bash
   gunzip -c /var/backups/envoytrade/envoytrade_pre_migration_<timestamp>.sql.gz | psql "$DATABASE_URL"
   ```

3. **Revert application binary**:
   Replace `/usr/local/bin/envoytrade` with the previous binary release.

4. **Restart service**:
   ```bash
   sudo systemctl start envoytrade
   ```
