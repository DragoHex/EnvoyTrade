#!/usr/bin/env bash
# scripts/seed_test_data.sh
# Seeds test data (masters, followers, groups, follow links) into EnvoyTrade database.
#
# Usage:
#   ./scripts/seed_test_data.sh              # Idempotent upsert of test data
#   ./scripts/seed_test_data.sh --clean      # Truncate tables before seeding
#   ./scripts/seed_test_data.sh --migrate    # Force apply migrations before seeding
#
# Environment variables:
#   DATABASE_URL   - Postgres connection string (default: postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable)
#   CONTAINER_NAME - Container name if running in Podman/Docker (default: envoytrade-db)
#   DB_USER        - Container postgres user (default: envoytrade)
#   DB_NAME        - Container postgres database (default: envoytrade)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
SEED_SQL="${SCRIPT_DIR}/seed_test_data.sql"
FLUSH_SQL="${SCRIPT_DIR}/flush_data.sql"
MIGRATIONS_DIR="${ROOT_DIR}/internal/store/postgres/migrations"

DATABASE_URL="${DATABASE_URL:-postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable}"
CONTAINER_NAME="${CONTAINER_NAME:-envoytrade-db}"
DB_USER="${DB_USER:-envoytrade}"
DB_NAME="${DB_NAME:-envoytrade}"

# Formatting
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

CLEAN=0
FORCE_MIGRATE=0

for arg in "$@"; do
  case "$arg" in
    --clean|-c)
      CLEAN=1
      ;;
    --migrate|-m)
      FORCE_MIGRATE=1
      ;;
    --help|-h)
      echo -e "${BOLD}Usage:${NC} $0 [--clean|-c] [--migrate|-m] [--help|-h]"
      echo ""
      echo "Options:"
      echo "  --clean, -c     Truncate all tables before seeding"
      echo "  --migrate, -m   Run all schema migrations before seeding"
      echo "  --help, -h      Show this help message"
      exit 0
      ;;
    *)
      echo -e "${RED}Unknown option:${NC} $arg"
      echo "Use --help for usage."
      exit 1
      ;;
  esac
done

# Backend execution helper
EXEC_MODE=""
if command -v psql >/dev/null 2>&1; then
  EXEC_MODE="psql"
elif command -v podman >/dev/null 2>&1 && podman inspect -f '{{.State.Running}}' "${CONTAINER_NAME}" 2>/dev/null | grep -q "true"; then
  EXEC_MODE="podman"
elif command -v docker >/dev/null 2>&1 && docker inspect -f '{{.State.Running}}' "${CONTAINER_NAME}" 2>/dev/null | grep -q "true"; then
  EXEC_MODE="docker"
else
  echo -e "${RED}Error:${NC} Neither 'psql' CLI nor a running '${CONTAINER_NAME}' container (podman/docker) was found."
  echo "Ensure PostgreSQL is accessible or start the container: podman start ${CONTAINER_NAME}"
  exit 1
fi

run_sql_file() {
  local file="$1"
  if [[ "$EXEC_MODE" == "psql" ]]; then
    psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$file"
  elif [[ "$EXEC_MODE" == "podman" ]]; then
    podman exec -i "${CONTAINER_NAME}" psql -U "${DB_USER}" -d "${DB_NAME}" -v ON_ERROR_STOP=1 < "$file"
  elif [[ "$EXEC_MODE" == "docker" ]]; then
    docker exec -i "${CONTAINER_NAME}" psql -U "${DB_USER}" -d "${DB_NAME}" -v ON_ERROR_STOP=1 < "$file"
  fi
}

run_sql_query_scalar() {
  local sql="$1"
  if [[ "$EXEC_MODE" == "psql" ]]; then
    psql "$DATABASE_URL" -t -A -c "$sql"
  elif [[ "$EXEC_MODE" == "podman" ]]; then
    podman exec -i "${CONTAINER_NAME}" psql -U "${DB_USER}" -d "${DB_NAME}" -t -A -c "$sql"
  elif [[ "$EXEC_MODE" == "docker" ]]; then
    docker exec -i "${CONTAINER_NAME}" psql -U "${DB_USER}" -d "${DB_NAME}" -t -A -c "$sql"
  fi
}

echo -e "${BOLD}${CYAN}==> EnvoyTrade Test Data Seeder${NC}"
echo -e "Backend:  ${GREEN}${EXEC_MODE}${NC} (target: ${CONTAINER_NAME:-DATABASE_URL})"

# Check if tables exist
ACCOUNTS_EXISTS="$(run_sql_query_scalar "SELECT to_regclass('public.accounts');" || true)"

if [[ "$FORCE_MIGRATE" -eq 1 ]] || [[ -z "$ACCOUNTS_EXISTS" ]]; then
  echo -e "${YELLOW}==> Applying schema migrations...${NC}"
  for mig in "${MIGRATIONS_DIR}"/*.sql; do
    echo "  Applying $(basename "$mig")..."
    run_sql_file "$mig" >/dev/null
  done
  echo -e "${GREEN}✓ Migrations applied successfully.${NC}"
fi

# Clean if requested
if [[ "$CLEAN" -eq 1 ]]; then
  echo -e "${YELLOW}==> Cleaning existing data (flush all tables)...${NC}"
  run_sql_file "$FLUSH_SQL" >/dev/null
  echo -e "${GREEN}✓ Tables truncated.${NC}"
fi

# Inject test data
echo -e "${CYAN}==> Injecting test data from ${SEED_SQL}...${NC}"
run_sql_file "$SEED_SQL" >/dev/null
echo -e "${GREEN}✓ Test data injected successfully.${NC}"

# Verification summary
ACCOUNTS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM accounts;")"
GROUPS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM groups;")"
FOLLOW_LINKS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM follow_links;")"
POSITIONS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM account_positions;")"
HOLDINGS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM account_holdings;")"
MARGINS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM account_margins;")"
MASTER_FILLS_COUNT="$(run_sql_query_scalar "SELECT count(*) FROM master_fills;")"

echo ""
echo -e "${BOLD}Current Database Summary:${NC}"
echo -e "  Accounts:     ${GREEN}${ACCOUNTS_COUNT}${NC}"
echo -e "  Groups:       ${GREEN}${GROUPS_COUNT}${NC}"
echo -e "  Follow Links: ${GREEN}${FOLLOW_LINKS_COUNT}${NC}"
echo -e "  Positions:    ${GREEN}${POSITIONS_COUNT}${NC}"
echo -e "  Holdings:     ${GREEN}${HOLDINGS_COUNT}${NC}"
echo -e "  Margins:      ${GREEN}${MARGINS_COUNT}${NC}"
echo -e "  Master Fills: ${GREEN}${MASTER_FILLS_COUNT}${NC}"
echo ""
echo -e "${GREEN}${BOLD}Done!${NC}"
