#!/usr/bin/env bash
set -e

# ==============================================================================
# build_binary.sh - Packages EnvoyTrade into a Single Standalone Binary
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ARTIFACTS_DIR="${ROOT_DIR}/artifacts"

OS="${GOOS:-$(go env GOOS)}"
ARCH="${GOARCH:-$(go env GOARCH)}"
TARGET_NAME="envoytrade"
TARBALL_NAME="envoytrade-${OS}-${ARCH}.tar.gz"

echo "=================================================================="
echo " Packaging EnvoyTrade Single Embedded Binary"
echo " OS: ${OS} | Arch: ${ARCH}"
echo "=================================================================="

# 1. Ensure artifacts directory exists
mkdir -p "${ARTIFACTS_DIR}"

# 2. Build frontend assets
echo "--> Building frontend with pnpm..."
cd "${ROOT_DIR}/frontend"
pnpm install --frozen-lockfile
pnpm build

# 3. Build Go binary with embedded UI build tag
echo "--> Compiling standalone binary (${OS}/${ARCH}) with -tags embed_ui..."
cd "${ROOT_DIR}"
CGO_ENABLED=0 GOOS="${OS}" GOARCH="${ARCH}" go build -tags embed_ui -ldflags="-s -w" -o "${ARTIFACTS_DIR}/${TARGET_NAME}" ./cmd/server

# Verify the binary was created and is executable
chmod +x "${ARTIFACTS_DIR}/${TARGET_NAME}"
echo "--> Binary compiled successfully: ${ARTIFACTS_DIR}/${TARGET_NAME} ($(du -h "${ARTIFACTS_DIR}/${TARGET_NAME}" | cut -f1))"

# 4. Assemble release tarball
STAGE_DIR="$(mktemp -d)"
trap 'rm -rf "${STAGE_DIR}"' EXIT

cp "${ARTIFACTS_DIR}/${TARGET_NAME}" "${STAGE_DIR}/"
cp "${ROOT_DIR}/scripts/install.sh" "${STAGE_DIR}/"
cp "${ROOT_DIR}/scripts/uninstall.sh" "${STAGE_DIR}/"
cp "${ROOT_DIR}/scripts/db_maintenance.sh" "${STAGE_DIR}/"
cp "${ROOT_DIR}/packaging/systemd/envoytrade.service" "${STAGE_DIR}/"
cp "${ROOT_DIR}/packaging/launchd/com.envoytrade.server.plist" "${STAGE_DIR}/"
cp "${ROOT_DIR}/packaging/postgres/envoytrade-postgres.conf" "${STAGE_DIR}/"
cp "${ROOT_DIR}/packaging/cron/envoytrade-maintenance.cron" "${STAGE_DIR}/"
cp "${ROOT_DIR}/packaging/envoytrade.env.example" "${STAGE_DIR}/"
cp "${ROOT_DIR}/packaging/envoytrade.env.example" "${STAGE_DIR}/.env.example"

chmod +x "${STAGE_DIR}/install.sh" "${STAGE_DIR}/uninstall.sh" "${STAGE_DIR}/db_maintenance.sh"

echo "--> Packaging release archive: ${ARTIFACTS_DIR}/${TARBALL_NAME}"
tar -czf "${ARTIFACTS_DIR}/${TARBALL_NAME}" -C "${STAGE_DIR}" .

echo "=================================================================="
echo " Build Complete!"
echo " Artifact: ${ARTIFACTS_DIR}/${TARBALL_NAME}"
echo " Binary:   ${ARTIFACTS_DIR}/${TARGET_NAME}"
echo "=================================================================="
