# Implementation Plan: Unified PostgreSQL Tuning, Single-Binary Unix Sockets, and 1:00 AM IST Maintenance

## Goal Description
EnvoyTrade needs a streamlined, production-grade persistence configuration across both deployment topologies:
1. **Single Binary Deployment**: Connects via high-speed Unix Domain Sockets (`host=/var/run/postgresql`) on Linux VPS to eliminate TCP stack latency, while preserving seamless fallback for macOS.
2. **Docker Compose Deployment**: Uses standard container networking (`db:5432` TCP) without forcing container mounts of host Unix sockets.
3. **Common PostgreSQL Tuning**: A single canonical PostgreSQL configuration file (`envoytrade-postgres.conf`) shared and applied identically across both Baremetal/Native PostgreSQL and Docker Compose PostgreSQL.
4. **Commodity Market (MCX) Safe Scheduling**: Indian commodity markets trade until 11:30 PM / 11:55 PM IST. All heavy maintenance tasks (database `VACUUM ANALYZE`, index optimization, and nightly backups) must strictly trigger at **1:00 AM IST**, safely outside both equity (NSE/BSE) and commodity (MCX) trading sessions.

---

## Architecture: Socket vs. TCP Isolation

```mermaid
flowchart TD
    subgraph Option1 [Option 1: Single Binary on Linux VPS]
        SB["EnvoyTrade Binary (/usr/local/bin/envoytrade)"]
        US["Unix Domain Socket (/var/run/postgresql/.s.PGSQL.5432)"]
        LP[("Native PostgreSQL 16")]
        SB -->|"Direct IPC (<0.15ms latency)"| US
        US --> LP
    end

    subgraph Option2 [Option 2: Docker Compose Stack]
        N["Nginx (web)"]
        API["Headless Go API (api)"]
        CP[("PostgreSQL 16 (db)")]
        N -->|"Reverse Proxy"| API
        API -->|"Linux Bridge Network TCP (db:5432)"| CP
    end

    subgraph SharedTuning [Common Configuration]
        CONF["envoytrade-postgres.conf<br>(shared_buffers=1GB, autovacuum_scale_factor=0.05, etc.)"]
        CONF -.->|"Applied in /etc/postgresql/16/main/conf.d/"| LP
        CONF -.->|"Mounted into postgres:16 container"| CP
    end
```

---

## Proposed Changes

### Component 1: Shared PostgreSQL Performance Tuning

#### `packaging/postgres/envoytrade-postgres.conf`
Canonical configuration holding performance and autovacuum tuning:
- Memory: `shared_buffers = 1GB`, `work_mem = 32MB`, `maintenance_work_mem = 128MB`, `effective_cache_size = 3GB`.
- Disk & WAL: `wal_buffers = 16MB`, `checkpoint_completion_target = 0.9`, `random_page_cost = 1.1`, `effective_io_concurrency = 200`.
- Trade Safety: `synchronous_commit = on`.
- Micro-Autovacuum: `autovacuum = on`, `autovacuum_vacuum_scale_factor = 0.05`, `autovacuum_analyze_scale_factor = 0.02`, `autovacuum_vacuum_cost_limit = 500`.

#### `docker-compose.yml`
- Mount `packaging/postgres/envoytrade-postgres.conf` into `/etc/postgresql/postgresql.conf` in the `db` service container.
- Command override: `command: ["postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"]`.

---

### Component 2: Single-Binary Unix Socket Isolation

#### `packaging/envoytrade.env.example`
Provide clear OS-specific configuration targets:
```ini
# Linux VPS (Native Unix Socket - Fastest, sub-0.15ms latency):
DATABASE_URL=postgres://envoytrade:envoytrade@/envoytrade?host=/var/run/postgresql&pool_min_conns=5&pool_max_conns=25

# Fallback / macOS TCP (Local container or Homebrew Postgres):
# DATABASE_URL=postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable&pool_min_conns=5&pool_max_conns=25
```

#### `scripts/install.sh`
- On **Linux**: Check if `/var/run/postgresql` exists. If present, initialize `CONFIG_FILE` using the Unix domain socket URL (`host=/var/run/postgresql`). If `packaging/postgres/envoytrade-postgres.conf` exists, offer to copy it to `/etc/postgresql/16/main/conf.d/envoytrade.conf`.
- On **macOS**: Maintain TCP loopback `localhost:5432` / `localhost:5434` as default since Homebrew/Podman macOS setups use TCP.

---

### Component 3: 1:00 AM IST Commodity-Safe Maintenance & Backup

#### `scripts/db_maintenance.sh`
Executes safe post-market database maintenance:
1. `VACUUM ANALYZE` across tables (`master_fills`, `follower_orders`, `order_events`, `accounts`, `positions`).
2. Generates an encrypted/compressed backup: `pg_dump ... | gzip > /var/backups/envoytrade/envoytrade_YYYYMMDD_010000.sql.gz`.
3. Purges local backup files older than 14 days.

#### `packaging/cron/envoytrade-maintenance.cron`
Cron definition:
```cron
# Run daily at 01:00 AM IST (after MCX commodity market settlement)
# If server timezone is Asia/Kolkata (IST):
0 1 * * * envoytrade /usr/local/bin/envoytrade-db-maintenance >> /var/log/envoytrade/maintenance.log 2>&1

# If server timezone is UTC (20:30 UTC = 01:00 AM IST next day):
# 30 20 * * * envoytrade /usr/local/bin/envoytrade-db-maintenance >> /var/log/envoytrade/maintenance.log 2>&1
```

#### `Makefile`
- Add `db-maintenance`: runs `scripts/db_maintenance.sh`.
- Update `package-binary`: bundles `packaging/postgres/envoytrade-postgres.conf` and `scripts/db_maintenance.sh` into the release tarball.

---

## Verification Plan

### Automated Tests
1. **Config Validation**:
   - Verify `docker-compose.yml` mounts and boots PostgreSQL with the tuned `envoytrade-postgres.conf`.
   - Run `SHOW shared_buffers;` and `SHOW autovacuum_vacuum_scale_factor;` inside the container to assert settings took effect.
2. **Benchmark & Sanity Suite**:
   - Run `make benchmark` to confirm both Single Binary and Docker Compose continue to pass 8/8 sanity tests.
3. **Maintenance Script Dry-Run**:
   - Execute `scripts/db_maintenance.sh` against the active test database; verify backup artifact is created and `VACUUM ANALYZE` completes without errors.

### Manual Verification
- Verify that `packaging/envoytrade.env.example` clearly explains socket vs. TCP usage.
- Confirm cron documentation notes both IST and UTC equivalencies for the 1:00 AM IST trigger.
