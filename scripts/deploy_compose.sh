#!/usr/bin/env bash
set -e

# ==============================================================================
# deploy_compose.sh - Deploys EnvoyTrade via Docker Compose (Headless API + Web)
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

# 1. Detect Docker Compose or Podman Compose
COMPOSE_CMD=""
if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
elif podman compose version >/dev/null 2>&1; then
    COMPOSE_CMD="podman compose"
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
else
    echo "Error: docker compose (or podman compose) is required." >&2
    exit 1
fi

echo "=================================================================="
echo " Deploying EnvoyTrade via ${COMPOSE_CMD}"
echo "=================================================================="

# 2. Generate .env file if missing
if [ ! -f .env ]; then
    echo "--> Creating .env with generated secure password..."
    PASSWORD=$(openssl rand -hex 16 2>/dev/null || date +%s | sha256sum | head -c 32)
    cat << EOF > .env
POSTGRES_DB=envoytrade
POSTGRES_USER=envoytrade
POSTGRES_PASSWORD=${PASSWORD}
PORT=8080
LOG_LEVEL=info
EOF
    echo "    Created .env file."
fi

# 3. Build and launch services
echo "--> Building and starting containers (db, api, web)..."
${COMPOSE_CMD} up -d --build

echo ""
echo "=================================================================="
echo " EnvoyTrade Compose Stack Deployed!"
echo " Web UI / API: http://localhost:${PORT:-8080}"
echo " Services:"
${COMPOSE_CMD} ps
echo "=================================================================="
