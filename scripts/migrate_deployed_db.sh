#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# migrate_deployed_db.sh - Automated Safe Migration for Deployed EnvoyTrade DB
#
# Performs:
#   1. Pre-migration inspection & validation
#   2. Compressed safety backup (pg_dump + gzip + sha256 checksum)
#   3. Transactional schema migration and data backfill (scripts/migrate_deployed_db.sql)
#   4. Post-migration data integrity verification & report
# ==============================================================================

export TZ="${TZ:-Asia/Kolkata}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SQL_FILE="${SCRIPT_DIR}/migrate_deployed_db.sql"

DRY_RUN=false
SKIP_BACKUP=false
BACKUP_DIR="${BACKUP_DIR:-/var/backups/envoytrade}"
CONTAINER_NAME="${CONTAINER_NAME:-}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --skip-backup)
            SKIP_BACKUP=true
            shift
            ;;
        --backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        --container)
            CONTAINER_NAME="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            echo "Usage: $0 [--dry-run] [--skip-backup] [--backup-dir DIR] [--container NAME]" >&2
            exit 1
            ;;
    esac
done

# Resolve DATABASE_URL
if [ -z "${DATABASE_URL:-}" ]; then
    for env_file in "${SCRIPT_DIR}/../.env" "${SCRIPT_DIR}/.env" "/etc/envoytrade/envoytrade.env" "./.env"; do
        if [ -f "${env_file}" ]; then
            DATABASE_URL=$(grep '^DATABASE_URL=' "${env_file}" 2>/dev/null | head -n 1 | cut -d= -f2- | tr -d '"' | tr -d "'" || true)
            if [ -n "${DATABASE_URL}" ]; then
                break
            fi
        fi
    done
fi

if [ -z "${DATABASE_URL:-}" ]; then
    DATABASE_URL="postgres://envoytrade:envoytrade@localhost:5434/envoytrade?sslmode=disable"
fi

# Detect execution mechanism (Docker/Podman container exec vs native psql/pg_dump)
run_psql() {
    local query="$1"
    if [ -n "${CONTAINER_NAME}" ]; then
        local cmd
        cmd=$(which podman 2>/dev/null || which docker 2>/dev/null || echo podman)
        $cmd exec -i "${CONTAINER_NAME}" psql -U envoytrade -d envoytrade -v ON_ERROR_STOP=1 -t -A -c "${query}"
    else
        psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -t -A -c "${query}"
    fi
}

run_psql_file() {
    local file="$1"
    if [ -n "${CONTAINER_NAME}" ]; then
        local cmd
        cmd=$(which podman 2>/dev/null || which docker 2>/dev/null || echo podman)
        $cmd exec -i "${CONTAINER_NAME}" psql -U envoytrade -d envoytrade -v ON_ERROR_STOP=1 < "${file}"
    else
        psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 < "${file}"
    fi
}

echo "=================================================================="
echo " EnvoyTrade Deployed Database Migration Tool"
echo " Time: $(date)"
echo " Target: ${DATABASE_URL}"
echo "=================================================================="

# Check database connectivity
echo "--> Testing database connection..."
if ! run_psql "SELECT 1;" >/dev/null 2>&1; then
    echo "ERROR: Failed to connect to PostgreSQL database via ${DATABASE_URL}" >&2
    exit 1
fi
echo "    Connection verified."

# Pre-migration Inspection
echo "--> Inspecting database state..."
HAS_CAPITAL_RATIO=$(run_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'follow_links' AND column_name = 'capital_ratio';")
HAS_CLONE_FACTOR=$(run_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'follow_links' AND column_name = 'clone_factor';")
HAS_PRODUCT_MTM=$(run_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'account_margins' AND column_name = 'product_mtm';")
FOLLOWER_ORDERS_COUNT=$(run_psql "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'follower_orders';")
if [ "${FOLLOWER_ORDERS_COUNT}" -gt 0 ]; then
    TOTAL_FOLLOWER_ORDERS=$(run_psql "SELECT COUNT(*) FROM follower_orders;")
else
    TOTAL_FOLLOWER_ORDERS=0
fi

echo "    follow_links column capital_ratio present: ${HAS_CAPITAL_RATIO}"
echo "    follow_links column clone_factor present:  ${HAS_CLONE_FACTOR}"
echo "    account_margins column product_mtm present: ${HAS_PRODUCT_MTM}"
echo "    Total follower_orders in database:         ${TOTAL_FOLLOWER_ORDERS}"

if [ "${DRY_RUN}" = true ]; then
    echo ""
    echo "=================================================================="
    echo " [DRY RUN] Inspection complete. Actions that would be performed:"
    echo "   - Create timestamped compressed backup in ${BACKUP_DIR}"
    if [ "${HAS_CAPITAL_RATIO}" -gt 0 ]; then
        echo "   - Rename follow_links.capital_ratio to clone_factor (preserving all multipliers)"
    fi
    echo "   - Relax follower_orders.master_fill_id NOT NULL constraint"
    echo "   - Add order metadata columns (origin, tradingsymbol, exchange, product, etc.)"
    echo "   - Backfill metadata on existing follower_orders from linked master_fills"
    echo "   - Ensure unique index on (follower_id, broker_order_id)"
    if [ "${HAS_PRODUCT_MTM}" -eq 0 ]; then
        echo "   - Add product_mtm column to account_margins"
    fi
    echo "   - Ensure users, sessions, proxy_ips, and pending_order_updates exist"
    echo "   - Auto-link orphaned accounts/groups to primary tenant user"
    echo "=================================================================="
    exit 0
fi

# Step 1: Safety Backup
if [ "${SKIP_BACKUP}" = false ]; then
    mkdir -p "${BACKUP_DIR}" 2>/dev/null || BACKUP_DIR="./backups"
    mkdir -p "${BACKUP_DIR}"
    TIMESTAMP=$(date +%Y%m%d_%H%M%S)
    BACKUP_FILE="${BACKUP_DIR}/envoytrade_pre_migration_${TIMESTAMP}.sql.gz"
    echo "--> Creating pre-migration safety backup: ${BACKUP_FILE}..."
    
    if [ -n "${CONTAINER_NAME}" ]; then
        cmd=$(which podman 2>/dev/null || which docker 2>/dev/null || echo podman)
        $cmd exec -i "${CONTAINER_NAME}" pg_dump -U envoytrade -d envoytrade | gzip > "${BACKUP_FILE}"
    else
        pg_dump "${DATABASE_URL}" | gzip > "${BACKUP_FILE}"
    fi
    
    # Generate SHA-256 Checksum
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${BACKUP_FILE}" > "${BACKUP_FILE}.sha256"
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${BACKUP_FILE}" > "${BACKUP_FILE}.sha256"
    fi
    echo "    Backup created successfully ($(du -h "${BACKUP_FILE}" | cut -f1))."
else
    echo "--> Backup skipped via --skip-backup flag."
fi

# Step 2: Run Transactional Migration
echo "--> Executing transactional schema migration (${SQL_FILE})..."
if ! run_psql_file "${SQL_FILE}"; then
    echo "ERROR: Migration failed! The transaction was automatically rolled back." >&2
    if [ "${SKIP_BACKUP}" = false ]; then
        echo "To restore pre-migration state: gunzip -c ${BACKUP_FILE} | psql ${DATABASE_URL}" >&2
    fi
    exit 1
fi
echo "    Migration SQL applied successfully."

# Step 3: Verification & Integrity Report
echo "--> Running post-migration integrity verification..."

USERS_COUNT=$(run_psql "SELECT COUNT(*) FROM users;")
ACCOUNTS_COUNT=$(run_psql "SELECT COUNT(*) FROM accounts;")
GROUPS_COUNT=$(run_psql "SELECT COUNT(*) FROM groups;")
LINKS_COUNT=$(run_psql "SELECT COUNT(*) FROM follow_links;")
FILLS_COUNT=$(run_psql "SELECT COUNT(*) FROM master_fills;")
ORDERS_COUNT=$(run_psql "SELECT COUNT(*) FROM follower_orders;")
POSITIONS_COUNT=$(run_psql "SELECT COUNT(*) FROM account_positions;")
MARGINS_COUNT=$(run_psql "SELECT COUNT(*) FROM account_margins;")

# Verify critical column presence
FINAL_CLONE_FACTOR=$(run_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'follow_links' AND column_name = 'clone_factor';")
FINAL_PRODUCT_MTM=$(run_psql "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'account_margins' AND column_name = 'product_mtm';")
FINAL_NULL_FILL_ID=$(run_psql "SELECT is_nullable FROM information_schema.columns WHERE table_name = 'follower_orders' AND column_name = 'master_fill_id';")

if [ "${FINAL_CLONE_FACTOR}" -ne 1 ] || [ "${FINAL_PRODUCT_MTM}" -ne 1 ] || [ "${FINAL_NULL_FILL_ID}" != "YES" ]; then
    echo "ERROR: Verification failed! Invariants not met (clone_factor: ${FINAL_CLONE_FACTOR}, product_mtm: ${FINAL_PRODUCT_MTM}, nullable: ${FINAL_NULL_FILL_ID})" >&2
    exit 1
fi

echo "=================================================================="
echo " MIGRATION SUCCESSFUL! Verified Database Invariants:"
echo "   - users:             ${USERS_COUNT} records"
echo "   - accounts:          ${ACCOUNTS_COUNT} records"
echo "   - groups:            ${GROUPS_COUNT} records"
echo "   - follow_links:      ${LINKS_COUNT} records (clone_factor verified)"
echo "   - master_fills:      ${FILLS_COUNT} records"
echo "   - follower_orders:   ${ORDERS_COUNT} records (nullable master_fill_id verified)"
echo "   - account_positions: ${POSITIONS_COUNT} records"
echo "   - account_margins:   ${MARGINS_COUNT} records (product_mtm verified)"
echo "=================================================================="
echo " EnvoyTrade is ready for the fresh release."
exit 0
