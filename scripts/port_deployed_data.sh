#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# port_deployed_data.sh - Cross-Host Data Porting Tool for EnvoyTrade
#
# Ports complete data from an existing source PostgreSQL database to a fresh
# target PostgreSQL database running the clean aggregated schema.
#
# USAGE:
#   ./scripts/port_deployed_data.sh \
#     --source-db "postgres://user:pass@old-host:5432/envoytrade" \
#     --target-db "postgres://user:pass@new-host:5432/envoytrade"
# ==============================================================================

SOURCE_DB=""
TARGET_DB=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INIT_SQL="${SCRIPT_DIR}/../internal/store/postgres/migrations/0001_init.sql"
MIGRATE_SQL="${SCRIPT_DIR}/migrate_deployed_db.sql"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --source-db)
            SOURCE_DB="$2"
            shift 2
            ;;
        --target-db)
            TARGET_DB="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            echo "Usage: $0 --source-db <SOURCE_URL> --target-db <TARGET_URL>" >&2
            exit 1
            ;;
    esac
done

if [ -z "${SOURCE_DB}" ] || [ -z "${TARGET_DB}" ]; then
    echo "ERROR: Both --source-db and --target-db are required." >&2
    echo "Usage: $0 --source-db <SOURCE_URL> --target-db <TARGET_URL>" >&2
    exit 1
fi

echo "=================================================================="
echo " EnvoyTrade Cross-Host Data Porting Utility"
echo " Source: ${SOURCE_DB}"
echo " Target: ${TARGET_DB}"
echo "=================================================================="

echo "--> Checking connectivity to source database..."
psql "${SOURCE_DB}" -v ON_ERROR_STOP=1 -c "SELECT 1;" >/dev/null
echo "    Source connected."

echo "--> Checking connectivity to target database..."
psql "${TARGET_DB}" -v ON_ERROR_STOP=1 -c "SELECT 1;" >/dev/null
echo "    Target connected."

# Step 1: Ensure Target has the Fresh Unified Baseline Schema
echo "--> Initializing fresh canonical schema on target database..."
psql "${TARGET_DB}" -v ON_ERROR_STOP=1 -f "${INIT_SQL}" >/dev/null
echo "    Target baseline schema initialized."

# Step 2: Ensure Source has aligned schema if older (run idempotent alignment on source)
echo "--> Aligning source schema in-memory/transactionally to prevent dump errors..."
psql "${SOURCE_DB}" -v ON_ERROR_STOP=1 -f "${MIGRATE_SQL}" >/dev/null

# Step 3: Stream Data in Dependency Order
TABLES=(
    "users"
    "proxy_ips"
    "accounts"
    "groups"
    "follow_links"
    "master_fills"
    "follower_orders"
    "order_events"
    "instruments"
    "account_positions"
    "account_holdings"
    "account_margins"
    "pending_order_updates"
)

echo "--> Streaming table data from source to target..."
for tbl in "${TABLES[@]}"; do
    echo "    Porting table: ${tbl}..."
    pg_dump "${SOURCE_DB}" --data-only --table="public.${tbl}" --no-owner --no-privileges | \
        psql "${TARGET_DB}" -v ON_ERROR_STOP=1 -q
done
echo "    All tables ported successfully."

# Step 4: Reset Sequences on Target Database
echo "--> Synchronizing identity sequences on target database..."
psql "${TARGET_DB}" -v ON_ERROR_STOP=1 -q -c "
DO \$\$
DECLARE
    seq RECORD;
BEGIN
    FOR seq IN (
        SELECT sequence_name, table_name, column_name
        FROM information_schema.sequences s
        JOIN information_schema.columns c
          ON c.column_default LIKE '%' || s.sequence_name || '%'
        WHERE s.sequence_schema = 'public'
    ) LOOP
        EXECUTE 'SELECT setval(''' || quote_ident(seq.sequence_name) || ''', COALESCE((SELECT MAX(' || quote_ident(seq.column_name) || ') FROM ' || quote_ident(seq.table_name) || '), 1))';
    END LOOP;
END \$\$;"

# Step 5: Verification of Row Count Parity
echo "=================================================================="
echo " Verification Report (Source vs Target Record Counts):"
echo "------------------------------------------------------------------"
printf "%-25s %-15s %-15s\n" "Table" "Source Count" "Target Count"
printf "%-25s %-15s %-15s\n" "-------------------------" "---------------" "---------------"

ALL_MATCH=true
for tbl in "${TABLES[@]}"; do
    SRC_CNT=$(psql "${SOURCE_DB}" -t -A -c "SELECT COUNT(*) FROM public.${tbl};")
    TGT_CNT=$(psql "${TARGET_DB}" -t -A -c "SELECT COUNT(*) FROM public.${tbl};")
    printf "%-25s %-15s %-15s\n" "${tbl}" "${SRC_CNT}" "${TGT_CNT}"
    if [ "${SRC_CNT}" != "${TGT_CNT}" ]; then
        ALL_MATCH=false
    fi
done

echo "------------------------------------------------------------------"
if [ "${ALL_MATCH}" = true ]; then
    echo " SUCCESS: All table row counts match perfectly!"
    echo " Target database is fully ported and ready for EnvoyTrade release."
else
    echo " WARNING: Some row counts differed. Please review above."
    exit 1
fi
