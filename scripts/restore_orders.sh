#!/usr/bin/env bash
set -euo pipefail

export TZ="Asia/Kolkata"

# ==============================================================================
# restore_orders.sh - Restore historical orders from Parquet to PostgreSQL
#
# Supports selective date range filtering:
#   --from YYYY-MM-DD : Start date (inclusive)
#   --upto YYYY-MM-DD : End date (inclusive)
#   --archive-dir DIR : Base archive directory (default: /var/backups/envoytrade/archive)
# ==============================================================================

FROM_DATE=""
UPTO_DATE=""
ARCHIVE_DIR="${ARCHIVE_DIR:-/var/backups/envoytrade/archive}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --from)
            FROM_DATE="$2"
            shift 2
            ;;
        --upto)
            UPTO_DATE="$2"
            shift 2
            ;;
        --archive-dir)
            ARCHIVE_DIR="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--from YYYY-MM-DD] [--upto YYYY-MM-DD] [--archive-dir DIR]"
            echo ""
            echo "Options:"
            echo "  --from YYYY-MM-DD   Start date of order history to restore (inclusive)"
            echo "  --upto YYYY-MM-DD   End date of order history to restore (inclusive)"
            echo "  --archive-dir DIR   Directory containing Hive Parquet files (default: /var/backups/envoytrade/archive)"
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            echo "Usage: $0 [--from YYYY-MM-DD] [--upto YYYY-MM-DD] [--archive-dir DIR]" >&2
            exit 1
            ;;
    esac
done

# Resolve DATABASE_URL if not set
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

if [ -z "${DATABASE_URL:-}" ]; then
    DATABASE_URL="postgres://envoytrade:envoytrade@localhost:5434/envoytrade?sslmode=disable"
fi

# Locate envoytrade binary or container fallback
RESTORE_CMD=""
if [ -f "./cmd/server/main.go" ] && command -v go >/dev/null 2>&1; then
    RESTORE_CMD="go run ./cmd/server"
elif command -v envoytrade >/dev/null 2>&1; then
    RESTORE_CMD="envoytrade"
elif [ -x "/usr/local/bin/envoytrade" ]; then
    RESTORE_CMD="/usr/local/bin/envoytrade"
else
    for c in envoytrade-api; do
        if podman ps --filter "name=^/${c}$" --filter "name=^${c}$" -q 2>/dev/null | grep -q . || \
           docker ps --filter "name=^/${c}$" --filter "name=^${c}$" -q 2>/dev/null | grep -q .; then
            if command -v podman >/dev/null 2>&1 && podman ps -q --filter "name=^/${c}$" --filter "name=^${c}$" 2>/dev/null | grep -q .; then
                RESTORE_CMD="podman exec -i ${c} envoytrade"
            else
                RESTORE_CMD="docker exec -i ${c} envoytrade"
            fi
            break
        fi
    done
fi

if [ -z "${RESTORE_CMD}" ]; then
    echo "Error: Neither envoytrade binary, go, nor running envoytrade-api container was found." >&2
    exit 1
fi

CMD_ARGS=("restore" "--dir" "${ARCHIVE_DIR}")
if [ -n "${FROM_DATE}" ]; then
    CMD_ARGS+=("--from" "${FROM_DATE}")
fi
if [ -n "${UPTO_DATE}" ]; then
    CMD_ARGS+=("--upto" "${UPTO_DATE}")
fi

echo "=================================================================="
echo " EnvoyTrade Order Parquet Restore"
echo " Timestamp: $(date '+%Y-%m-%d %H:%M:%S %Z')"
echo " Archive:   ${ARCHIVE_DIR}"
echo " Range:     From=[${FROM_DATE:-ALL}] Upto=[${UPTO_DATE:-ALL}]"
echo "=================================================================="

DATABASE_URL="${DATABASE_URL}" ${RESTORE_CMD} "${CMD_ARGS[@]}"

echo "=================================================================="
echo " Restore Complete!"
echo "=================================================================="
