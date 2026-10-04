#!/usr/bin/env bash
set -e

# ==============================================================================
# decommission_compose.sh - Tears down EnvoyTrade Docker Compose Stack
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

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

DOWN_ARGS=()
REMOVE_VOLUMES=false
REMOVE_IMAGES=false

for arg in "$@"; do
    case $arg in
        --volumes|-v)
            REMOVE_VOLUMES=true
            ;;
        --all|-a)
            REMOVE_VOLUMES=true
            REMOVE_IMAGES=true
            ;;
    esac
done

if [ "${REMOVE_VOLUMES}" = true ]; then
    DOWN_ARGS+=("-v")
fi

if [ "${REMOVE_IMAGES}" = true ]; then
    DOWN_ARGS+=("--rmi" "all")
fi

echo "=================================================================="
echo " Decommissioning EnvoyTrade Compose Stack via ${COMPOSE_CMD}"
if [ "${REMOVE_VOLUMES}" = true ]; then
    echo " WARNING: Database volumes will be destroyed!"
fi
if [ "${REMOVE_IMAGES}" = true ]; then
    echo " WARNING: Container images will be deleted!"
fi
echo "=================================================================="

${COMPOSE_CMD} down "${DOWN_ARGS[@]}"

echo "=================================================================="
echo " Compose stack decommissioned successfully."
echo "=================================================================="
