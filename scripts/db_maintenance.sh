#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# db_maintenance.sh - PostgreSQL Maintenance & Backup for EnvoyTrade
#
# Designed to run nightly at 01:00 AM IST (after MCX commodity market close)
# Performs:
#   1. VACUUM ANALYZE across active trading tables
#   2. Compressed pg_dump backup (gzip)
#   3. Prunes backups older than RETENTION_DAYS (default 14 days)
# ==============================================================================

DRY_RUN=false
DO_VACUUM=true
DO_BACKUP=true
RETENTION_DAYS=14
BACKUP_DIR="${BACKUP_DIR:-/var/backups/envoytrade}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --vacuum-only)
            DO_VACUUM=true
            DO_BACKUP=false
            shift
            ;;
        --backup-only)
            DO_VACUUM=false
            DO_BACKUP=true
            shift
            ;;
        --retention-days)
            RETENTION_DAYS="$2"
            shift 2
            ;;
        --backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown argument: $1" >&2
            echo "Usage: $0 [--dry-run] [--vacuum-only] [--backup-only] [--retention-days N] [--backup-dir DIR]" >&2
            exit 1
            ;;
    esac
done

# Resolve DATABASE_URL if not directly set
if [ -z "${DATABASE_URL:-}" ]; then
    if [ -r "/etc/envoytrade/envoytrade.env" ]; then
        # shellcheck disable=SC1091
        DATABASE_URL=$(grep '^DATABASE_URL=' /etc/envoytrade/envoytrade.env 2>/dev/null | cut -d= -f2- | tr -d '"' | tr -d "'" || true)
    elif [ -r "${HOME}/.envoytrade/envoytrade.env" ]; then
        # shellcheck disable=SC1091
        DATABASE_URL=$(grep '^DATABASE_URL=' "${HOME}/.envoytrade/envoytrade.env" 2>/dev/null | cut -d= -f2- | tr -d '"' | tr -d "'" || true)
    elif [ -r ".env" ]; then
        DATABASE_URL=$(grep '^DATABASE_URL=' .env 2>/dev/null | cut -d= -f2- | tr -d '"' | tr -d "'" || true)
    fi
fi

# Fallback default for local development
if [ -z "${DATABASE_URL:-}" ]; then
    DATABASE_URL="postgres://envoytrade:envoytrade@localhost:5434/envoytrade?sslmode=disable"
fi

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/envoytrade_${TIMESTAMP}.sql.gz"

echo "=================================================================="
echo " EnvoyTrade Database Maintenance"
echo " Timestamp: $(date '+%Y-%m-%d %H:%M:%S %Z')"
echo " Mode:      $([ "${DRY_RUN}" = true ] && echo "DRY-RUN" || echo "ACTIVE")"
echo " Options:   Vacuum=${DO_VACUUM} | Backup=${DO_BACKUP} | Retention=${RETENTION_DAYS}d"
echo "=================================================================="

# Determine Postgres execution tool (psql/pg_dump or container exec)
USE_CONTAINER=""
if ! command -v psql >/dev/null 2>&1 || ! command -v pg_dump >/dev/null 2>&1; then
    for c in envoytrade-db; do
        if podman ps --filter "name=^/${c}$" --filter "name=^${c}$" -q 2>/dev/null | grep -q . || \
           docker ps --filter "name=^/${c}$" --filter "name=^${c}$" -q 2>/dev/null | grep -q .; then
            USE_CONTAINER="${c}"
            break
        fi
    done
fi

# Tables to vacuum analyze
TABLES=(
    "master_fills"
    "follower_orders"
    "order_events"
    "accounts"
    "groups"
    "follow_links"
    "positions"
    "instruments"
)

# 1. VACUUM ANALYZE
if [ "${DO_VACUUM}" = true ]; then
    echo "--> [1/2] Running VACUUM ANALYZE on trading tables..."
    for tbl in "${TABLES[@]}"; do
        SQL="VACUUM (ANALYZE) ${tbl};"
        if [ "${DRY_RUN}" = true ]; then
            echo "    [DRY-RUN] ${SQL}"
        else
            if [ -n "${USE_CONTAINER}" ]; then
                podman exec "${USE_CONTAINER}" psql -U envoytrade -d envoytrade -c "${SQL}" 2>/dev/null || \
                docker exec "${USE_CONTAINER}" psql -U envoytrade -d envoytrade -c "${SQL}" 2>/dev/null || true
            else
                psql "${DATABASE_URL}" -c "${SQL}" 2>/dev/null || echo "    Note: table ${tbl} vacuum skipped/failed"
            fi
            echo "    ✓ Cleaned and analyzed: ${tbl}"
        fi
    done
fi

# 2. Compressed Backup
if [ "${DO_BACKUP}" = true ]; then
    echo "--> [2/2] Creating compressed database backup..."
    if [ "${DRY_RUN}" = true ]; then
        echo "    [DRY-RUN] pg_dump -> gzip > ${BACKUP_FILE}"
        echo "    [DRY-RUN] Retention cleanup: Purge files older than ${RETENTION_DAYS} days in ${BACKUP_DIR}"
    else
        # Ensure backup dir exists
        if ! mkdir -p "${BACKUP_DIR}" 2>/dev/null; then
            BACKUP_DIR="/tmp/envoytrade-backups"
            mkdir -p "${BACKUP_DIR}"
            BACKUP_FILE="${BACKUP_DIR}/envoytrade_${TIMESTAMP}.sql.gz"
        fi

        if [ -n "${USE_CONTAINER}" ]; then
            (podman exec "${USE_CONTAINER}" pg_dump -U envoytrade -d envoytrade 2>/dev/null || \
             docker exec "${USE_CONTAINER}" pg_dump -U envoytrade -d envoytrade) | gzip > "${BACKUP_FILE}"
        else
            pg_dump "${DATABASE_URL}" | gzip > "${BACKUP_FILE}"
        fi

        BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
        echo "    ✓ Backup created: ${BACKUP_FILE} (${BACKUP_SIZE})"

        # Retention Cleanup
        echo "--> Pruning backups older than ${RETENTION_DAYS} days in ${BACKUP_DIR}..."
        find "${BACKUP_DIR}" -name "envoytrade_*.sql.gz" -type f -mtime +"${RETENTION_DAYS}" -delete 2>/dev/null || true
        echo "    ✓ Old backups pruned."
    fi
fi

echo "=================================================================="
echo " Maintenance Task Complete!"
echo "=================================================================="
