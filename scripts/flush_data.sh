#!/usr/bin/env bash
# scripts/flush_data.sh
# Clears all data from EnvoyTrade database tables.
#
# Usage:
#   ./scripts/flush_data.sh
#
# Environment variables:
#   DATABASE_URL   - Postgres connection string (default: postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable)
#   CONTAINER_NAME - Container name if running in Podman/Docker (default: envoytrade-db)
#   DB_USER        - Container postgres user (default: envoytrade)
#   DB_NAME        - Container postgres database (default: envoytrade)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLUSH_SQL="${SCRIPT_DIR}/flush_data.sql"

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

echo -e "${BOLD}${CYAN}==> EnvoyTrade Data Flush${NC}"
echo -e "Backend:  ${GREEN}${EXEC_MODE}${NC} (target: ${CONTAINER_NAME:-DATABASE_URL})"
echo -e "${YELLOW}==> Truncating all tables...${NC}"

run_sql_file "$FLUSH_SQL" >/dev/null

echo -e "${GREEN}✓ All tables flushed successfully.${NC}"
