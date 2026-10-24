# EOD Data Archival & DuckDB Analytics

## 1. Why Caveman Do This?
Trading day make Postgres heavy. Old orders cause bloat, slow indexes, eat RAM.
Nightly job pull finished orders from Postgres, pack into small Parquet files, delete old rows from Postgres.
Next morning, main DB light and fast like cheetah.

---

## 2. When Job Run?
- **Schedule:** Nightly at **01:00 AM IST** (Every single day).
- **Why 1 AM:** MCX commodities close around 11:55 PM IST. 1 AM is safe after-market time.
- **Cron File:** `/etc/cron.d/envoytrade-maintenance`
- **Runner Script:** `/usr/local/bin/envoytrade-db-maintenance` (or `./scripts/db_maintenance.sh`)

---

## 3. Where Files Live & Structure
Hive partition layout. Standard format for DuckDB, Spark, and Pandas:

```
/var/backups/envoytrade/archive/    (or ./backups/archive/ in Docker)
├── follower_orders/
│   ├── date=2026-10-21/data.parquet
│   ├── date=2026-10-22/data.parquet
│   └── date=2026-10-23/data.parquet
├── master_fills/
│   ├── date=2026-10-21/data.parquet
│   └── date=2026-10-22/data.parquet
└── order_events/
    ├── date=2026-10-21/data.parquet
    └── date=2026-10-22/data.parquet
```

- Compression: **ZSTD** (small size, fast read).
- Safety rule: Only terminal orders (`COMPLETE`, `REJECTED`, `CANCELLED`, etc.) archived. Open orders stay in Postgres.

---

## 4. How Trigger Manually?

### A. Manual Archive
```bash
# Full maintenance (Archive + Vacuum + DB Backup)
./scripts/db_maintenance.sh

# Archive ONLY (skips vacuum & pg_dump)
./scripts/db_maintenance.sh --archive-only

# Archive up to specific date
./scripts/db_maintenance.sh --archive-only --cutoff 2026-10-23

# Direct binary command
envoytrade archive --cutoff 2026-10-23 --dir /var/backups/envoytrade/archive
```

### B. Manual Restore Back to Postgres
```bash
# Restore specific date range
./scripts/restore_orders.sh --from 2026-10-01 --upto 2026-10-15

# Restore all history
./scripts/restore_orders.sh

# Direct binary command
envoytrade restore --from 2026-10-01 --upto 2026-10-15 --dir /var/backups/envoytrade/archive
```
*Note:* Restore uses `ON CONFLICT DO NOTHING` (idempotent, safe to rerun) and automatically updates PostgreSQL ID sequence.

---

## 5. How Read with DuckDB?

DuckDB reads Hive Parquet folder directly. No need running Postgres. Zero database load.

### Start DuckDB
```bash
duckdb
```

### Query 1: Read all follower orders
```sql
SELECT * FROM '/var/backups/envoytrade/archive/follower_orders/**/*.parquet' LIMIT 10;
```

### Query 2: Filter by Date (Fast partition pruning)
DuckDB sees `date=` in folder path and skips other days completely:
```sql
SELECT 
    tradingsymbol,
    count(*) AS total_trades,
    sum(filled_qty) AS total_quantity
FROM '/var/backups/envoytrade/archive/follower_orders/**/*.parquet'
WHERE date BETWEEN '2026-10-01' AND '2026-10-23'
GROUP BY tradingsymbol
ORDER BY total_quantity DESC;
```

### Query 3: Join Master Fills with Follower Orders for 1-Year Analysis
```sql
SELECT 
    fo.date,
    fo.tradingsymbol,
    fo.filled_qty,
    fo.average_price AS follower_price,
    mf.average_price AS master_price
FROM '/var/backups/envoytrade/archive/follower_orders/**/*.parquet' fo
JOIN '/var/backups/envoytrade/archive/master_fills/**/*.parquet' mf 
  ON fo.master_fill_id = mf.id
WHERE fo.terminal_status = 'COMPLETE';
```

### Query 4: Export DuckDB result to CSV
```sql
COPY (
    SELECT * 
    FROM '/var/backups/envoytrade/archive/follower_orders/**/*.parquet'
    WHERE date = '2026-10-23'
) TO 'trades_2026_10_23.csv' (HEADER, DELIMITER ',');
```
