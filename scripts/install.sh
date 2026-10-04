#!/usr/bin/env bash
set -e

# ==============================================================================
# install.sh - Smart installer for EnvoyTrade Single Binary (macOS & Linux)
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SOURCE="${SCRIPT_DIR}/envoytrade"

if [ ! -f "${BIN_SOURCE}" ]; then
    # Fallback if run from project root/scripts directory
    if [ -f "${SCRIPT_DIR}/../artifacts/envoytrade" ]; then
        BIN_SOURCE="${SCRIPT_DIR}/../artifacts/envoytrade"
    elif [ -f "./artifacts/envoytrade" ]; then
        BIN_SOURCE="./artifacts/envoytrade"
    elif [ -f "./bin/envoytrade" ]; then
        BIN_SOURCE="./bin/envoytrade"
    else
        echo "Error: envoytrade binary not found. Please run scripts/build_binary.sh first." >&2
        exit 1
    fi
fi

OS="$(uname -s)"
INSTALL_BIN="/usr/local/bin/envoytrade"

echo "=================================================================="
echo " EnvoyTrade Standalone Installer"
echo " Detected OS: ${OS}"
echo "=================================================================="

# Function for privileged command execution
run_privileged() {
    if [ "$(id -u)" -eq 0 ]; then
        "$@"
    elif command -v sudo >/dev/null 2>&1; then
        sudo "$@"
    else
        echo "Error: Root privileges required for: $*" >&2
        exit 1
    fi
}

# 1. Stop existing service if running (prevents text file busy / dirty binary overwrite)
echo "--> Checking for existing running service..."
if [ "${OS}" = "Darwin" ]; then
    TARGET_USER="${SUDO_USER:-}"
    if [ -z "${TARGET_USER}" ] || [ "${TARGET_USER}" = "root" ]; then
        TARGET_USER="$(stat -f '%Su' /dev/console 2>/dev/null || true)"
    fi
    if [ -z "${TARGET_USER}" ] || [ "${TARGET_USER}" = "root" ]; then
        TARGET_USER="$(logname 2>/dev/null || id -un)"
    fi
    TARGET_UID="$(id -u "${TARGET_USER}" 2>/dev/null || id -u)"

    if [ "$(id -u)" -eq 0 ]; then
        sudo -u "${TARGET_USER}" launchctl bootout "gui/${TARGET_UID}/com.envoytrade.server" 2>/dev/null || true
        launchctl bootout "gui/0/com.envoytrade.server" 2>/dev/null || true
    else
        launchctl bootout "gui/${TARGET_UID}/com.envoytrade.server" 2>/dev/null || true
    fi
elif [ "${OS}" = "Linux" ] && command -v systemctl >/dev/null 2>&1; then
    run_privileged systemctl stop envoytrade.service 2>/dev/null || true
fi

# 2. Install binary
echo "--> Installing binary to ${INSTALL_BIN}..."
run_privileged mkdir -p /usr/local/bin
run_privileged cp "${BIN_SOURCE}" "${INSTALL_BIN}"
run_privileged chmod 755 "${INSTALL_BIN}"

# Maintenance script source
MAINT_SRC="${SCRIPT_DIR}/db_maintenance.sh"
if [ ! -f "${MAINT_SRC}" ]; then
    MAINT_SRC="${SCRIPT_DIR}/../scripts/db_maintenance.sh"
fi
if [ -f "${MAINT_SRC}" ]; then
    echo "--> Installing database maintenance script to /usr/local/bin/envoytrade-db-maintenance..."
    run_privileged cp "${MAINT_SRC}" /usr/local/bin/envoytrade-db-maintenance
    run_privileged chmod 755 /usr/local/bin/envoytrade-db-maintenance
fi

# 3. OS-Specific Installation
if [ "${OS}" = "Darwin" ]; then
    # ==========================================
    # macOS (Darwin) Installation
    # ==========================================
    echo "--> Configuring for macOS (launchd)..."

    # Dynamically resolve the active macOS user and group (never hardcoded).
    # When run via `sudo ./install.sh`, SUDO_USER is set to the calling user.
    # stat /dev/console gives the console owner as a fallback.
    TARGET_USER="${SUDO_USER:-}"
    if [ -z "${TARGET_USER}" ] || [ "${TARGET_USER}" = "root" ]; then
        TARGET_USER="$(stat -f '%Su' /dev/console 2>/dev/null || true)"
    fi
    if [ -z "${TARGET_USER}" ] || [ "${TARGET_USER}" = "root" ]; then
        TARGET_USER="$(logname 2>/dev/null || id -un)"
    fi
    TARGET_GROUP="$(id -gn "${TARGET_USER}" 2>/dev/null || echo "staff")"
    TARGET_UID="$(id -u "${TARGET_USER}" 2>/dev/null || id -u)"

    # Handle systems where user shell $HOME differs from DirectoryServices OS home (e.g. /Users/msp/MSP vs /Users/msp)
    OS_HOME="$(eval echo ~${TARGET_USER})"
    SHELL_HOME="${HOME:-${OS_HOME}}"
    if [ "${SHELL_HOME}" = "/var/root" ] || [ "${SHELL_HOME}" = "/root" ]; then
        SHELL_HOME="${OS_HOME}"
    fi

    # Ensure config and launch agent directories exist across all potential user home locations
    for h in "${SHELL_HOME}" "${OS_HOME}"; do
        [ -n "$h" ] || continue
        mkdir -p "${h}/.envoytrade"
        mkdir -p "${h}/Library/LaunchAgents"
    done

    CONFIG_FILE="${SHELL_HOME}/.envoytrade/envoytrade.env"
    PLIST_DEST="${OS_HOME}/Library/LaunchAgents/com.envoytrade.server.plist"
    PLIST_SRC="${SCRIPT_DIR}/com.envoytrade.server.plist"

    if [ ! -f "${PLIST_SRC}" ]; then
        PLIST_SRC="${SCRIPT_DIR}/../packaging/launchd/com.envoytrade.server.plist"
    fi

    # Detect PostgreSQL port (5434 for dev container or 5432 for system postgres)
    LOCAL_DB_PORT="5434"
    if pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
        LOCAL_DB_PORT="5432"
    fi

    # Check if a configuration already exists in either location
    EXISTING_CONFIG=""
    if [ -f "${SHELL_HOME}/.envoytrade/envoytrade.env" ]; then
        EXISTING_CONFIG="${SHELL_HOME}/.envoytrade/envoytrade.env"
    elif [ -f "${OS_HOME}/.envoytrade/envoytrade.env" ]; then
        EXISTING_CONFIG="${OS_HOME}/.envoytrade/envoytrade.env"
    fi

    if [ -z "${EXISTING_CONFIG}" ]; then
        echo "--> Creating default configuration at ${CONFIG_FILE}..."
        cat <<EOF > "${CONFIG_FILE}"
DATABASE_URL=postgres://envoytrade:envoytrade@localhost:${LOCAL_DB_PORT}/envoytrade?sslmode=disable
PORT=8080
LOG_FILE=/var/log/envoytrade/app.log
LOG_LEVEL=info
LOG_TO_STDOUT=true
EOF
        chmod 600 "${CONFIG_FILE}"
    else
        CONFIG_FILE="${EXISTING_CONFIG}"
        # Guard: if existing config has stale linux socket default on macOS, warn or update
        if grep -q "host=/var/run/postgresql" "${CONFIG_FILE}" 2>/dev/null; then
            echo "--> Fixing legacy Linux Unix domain socket path in ${CONFIG_FILE} for macOS..."
            sed -i '' "s|host=/var/run/postgresql|host=localhost|g" "${CONFIG_FILE}" || true
        fi
    fi

    # Keep both SHELL_HOME and OS_HOME configurations in sync
    if [ "${SHELL_HOME}" != "${OS_HOME}" ]; then
        if [ "${CONFIG_FILE}" = "${SHELL_HOME}/.envoytrade/envoytrade.env" ]; then
            cp -f "${CONFIG_FILE}" "${OS_HOME}/.envoytrade/envoytrade.env" 2>/dev/null || true
        else
            cp -f "${CONFIG_FILE}" "${SHELL_HOME}/.envoytrade/envoytrade.env" 2>/dev/null || true
        fi
    fi

    # Fix ownership dynamically to the active user (matters when run via sudo)
    if [ "$(id -u)" -eq 0 ]; then
        for h in "${SHELL_HOME}" "${OS_HOME}"; do
            [ -n "$h" ] || continue
            chown -R "${TARGET_USER}:${TARGET_GROUP}" "${h}/.envoytrade" 2>/dev/null || true
            chown "${TARGET_USER}:${TARGET_GROUP}" "${h}/Library/LaunchAgents" 2>/dev/null || true
        done
        mkdir -p /var/log/envoytrade 2>/dev/null || true
        chown -R "${TARGET_USER}:${TARGET_GROUP}" /var/log/envoytrade 2>/dev/null || true
        rm -f /tmp/envoytrade.stdout.log /tmp/envoytrade.stderr.log
    fi

    # Check for local PostgreSQL
    echo "--> Checking PostgreSQL availability..."
    if pg_isready -h localhost -p 5432 >/dev/null 2>&1 || pg_isready -h localhost -p 5434 >/dev/null 2>&1; then
        echo "    PostgreSQL detected."
    else
        echo "    Note: PostgreSQL not detected on localhost:5432 or 5434."
        echo "    Ensure PostgreSQL is running before starting the service (e.g. make db-up)."
    fi

    # Install launchd plist and register the service
    if [ -f "${PLIST_SRC}" ]; then
        echo "--> Installing LaunchAgent to ${PLIST_DEST}..."
        cp "${PLIST_SRC}" "${PLIST_DEST}"
        if [ "${SHELL_HOME}" != "${OS_HOME}" ]; then
            cp "${PLIST_SRC}" "${SHELL_HOME}/Library/LaunchAgents/com.envoytrade.server.plist" 2>/dev/null || true
        fi

        if [ "$(id -u)" -eq 0 ]; then
            chown "${TARGET_USER}:${TARGET_GROUP}" "${PLIST_DEST}"
            if [ "${SHELL_HOME}" != "${OS_HOME}" ]; then
                chown "${TARGET_USER}:${TARGET_GROUP}" "${SHELL_HOME}/Library/LaunchAgents/com.envoytrade.server.plist" 2>/dev/null || true
            fi
            # Clean any old root-domain remnants
            rm -f /var/root/Library/LaunchAgents/com.envoytrade.server.plist 2>/dev/null || true
        fi

        echo "--> Registering LaunchAgent for user ${TARGET_USER} (uid ${TARGET_UID})..."
        sleep 1

        # Bootstrap the job into the user's GUI session domain using modern API
        if [ "$(id -u)" -eq 0 ]; then
            sudo -u "${TARGET_USER}" launchctl bootstrap "gui/${TARGET_UID}" "${PLIST_DEST}"
        else
            launchctl bootstrap "gui/${TARGET_UID}" "${PLIST_DEST}"
        fi

        echo "    Service registered with launchd (RunAtLoad=true, restarts on crash)."
        echo "    Stdout logs: /tmp/envoytrade.stdout.log"
        echo "    Stderr logs: /tmp/envoytrade.stderr.log"
    else
        echo "    Launchd plist not found; binary installed at ${INSTALL_BIN}."
    fi

    echo ""
    echo "=================================================================="
    echo " Installation complete on macOS!"
    echo " Binary:    ${INSTALL_BIN}"
    echo " Config:    ${CONFIG_FILE}"
    echo " Web UI:    http://localhost:8080"
    echo " Logs:      /tmp/envoytrade.stderr.log"
    echo " Status:    launchctl list | grep envoytrade"
    echo " Stop:      launchctl bootout gui/\$(id -u)/com.envoytrade.server"
    echo " Uninstall: bash uninstall.sh"
    echo "------------------------------------------------------------------"
    echo " [!] ACTION REQUIRED: Set up your configuration file:"
    echo "     --> nano ${CONFIG_FILE}"
    echo "     Ensure DATABASE_URL is set to your PostgreSQL database."
    echo "=================================================================="

elif [ "${OS}" = "Linux" ]; then
    # ==========================================
    # Linux (systemd) Installation
    # ==========================================
    echo "--> Configuring for Linux (systemd)..."

    CONFIG_DIR="/etc/envoytrade"
    CONFIG_FILE="${CONFIG_DIR}/envoytrade.env"
    LOG_DIR="/var/log/envoytrade"
    DATA_DIR="/var/lib/envoytrade"
    BACKUP_DIR="/var/backups/envoytrade"
    SERVICE_SRC="${SCRIPT_DIR}/envoytrade.service"

    if [ ! -f "${SERVICE_SRC}" ]; then
        SERVICE_SRC="${SCRIPT_DIR}/../packaging/systemd/envoytrade.service"
    fi

    # Create dedicated service user if not present
    if ! id -u envoytrade >/dev/null 2>&1; then
        echo "--> Creating system user 'envoytrade'..."
        run_privileged useradd --system --shell /usr/sbin/nologin --home-dir "${DATA_DIR}" --create-home envoytrade 2>/dev/null || true
    fi

    run_privileged mkdir -p "${CONFIG_DIR}" "${LOG_DIR}" "${DATA_DIR}" "${BACKUP_DIR}"
    run_privileged chown -R envoytrade:envoytrade "${LOG_DIR}" "${DATA_DIR}" "${BACKUP_DIR}"

    # Socket vs TCP determination for Linux
    DEFAULT_DB_URL="postgres://envoytrade:envoytrade@localhost:5432/envoytrade?sslmode=disable"
    if [ -d "/var/run/postgresql" ]; then
        echo "--> Detected Unix domain socket directory at /var/run/postgresql. Using high-speed IPC socket default."
        DEFAULT_DB_URL="postgres://envoytrade:envoytrade@/envoytrade?host=/var/run/postgresql&pool_min_conns=5&pool_max_conns=25"
    fi

    if [ ! -f "${CONFIG_FILE}" ]; then
        if [ -n "${ENV_EXAMPLE}" ] && [ -f "${ENV_EXAMPLE}" ]; then
            echo "--> Initializing configuration from packaged .env.example..."
            run_privileged cp "${ENV_EXAMPLE}" "${CONFIG_FILE}"
        else
            echo "--> Creating default configuration at ${CONFIG_FILE}..."
            run_privileged tee "${CONFIG_FILE}" > /dev/null <<EOF
DATABASE_URL=${DEFAULT_DB_URL}
PORT=8080
LOG_FILE=/var/log/envoytrade/app.log
LOG_LEVEL=info
LOG_FORMAT=json
LOG_TO_STDOUT=false
EOF
        fi
        run_privileged chmod 600 "${CONFIG_FILE}"
        run_privileged chown envoytrade:envoytrade "${CONFIG_FILE}"
    fi

    # Check for PostgreSQL canonical tuning configuration
    PG_CONF_SRC="${SCRIPT_DIR}/envoytrade-postgres.conf"
    if [ ! -f "${PG_CONF_SRC}" ]; then
        PG_CONF_SRC="${SCRIPT_DIR}/../packaging/postgres/envoytrade-postgres.conf"
    fi
    if [ -f "${PG_CONF_SRC}" ]; then
        for conf_d in /etc/postgresql/16/main/conf.d /etc/postgresql/conf.d; do
            if [ -d "${conf_d}" ]; then
                echo "--> Installing canonical PostgreSQL tuning configuration to ${conf_d}/envoytrade.conf..."
                run_privileged cp "${PG_CONF_SRC}" "${conf_d}/envoytrade.conf"
                run_privileged chmod 644 "${conf_d}/envoytrade.conf"
                echo "    Note: Reload postgres to apply settings (e.g. sudo systemctl reload postgresql)"
                break
            fi
        done
    fi

    # Install 01:00 AM IST maintenance cron job
    CRON_SRC="${SCRIPT_DIR}/envoytrade-maintenance.cron"
    if [ ! -f "${CRON_SRC}" ]; then
        CRON_SRC="${SCRIPT_DIR}/../packaging/cron/envoytrade-maintenance.cron"
    fi
    if [ -f "${CRON_SRC}" ] && [ -d "/etc/cron.d" ]; then
        echo "--> Installing nightly 1:00 AM IST maintenance cron to /etc/cron.d/envoytrade-maintenance..."
        run_privileged cp "${CRON_SRC}" /etc/cron.d/envoytrade-maintenance
        run_privileged chmod 644 /etc/cron.d/envoytrade-maintenance
    fi

    # Install systemd service
    if [ -f "${SERVICE_SRC}" ] && command -v systemctl >/dev/null 2>&1; then
        echo "--> Installing systemd service..."
        run_privileged cp "${SERVICE_SRC}" /etc/systemd/system/envoytrade.service
        run_privileged systemctl daemon-reload
        echo "--> Enabling and starting envoytrade.service..."
        run_privileged systemctl enable --now envoytrade.service
        echo "    Service status:"
        run_privileged systemctl status envoytrade.service --no-pager || true
    fi

    echo ""
    echo "=================================================================="
    echo " Installation complete on Linux!"
    echo " Binary:    ${INSTALL_BIN}"
    echo " Config:    ${CONFIG_FILE}"
    echo " Logs:      ${LOG_DIR}/app.log"
    echo " Status:    sudo systemctl status envoytrade"
    echo " Restart:   sudo systemctl restart envoytrade"
    echo " Stop:      sudo systemctl stop envoytrade"
    echo " Uninstall: sudo bash uninstall.sh"
    echo "------------------------------------------------------------------"
    echo " [!] ACTION REQUIRED: Set up your configuration file:"
    echo "     --> sudo nano ${CONFIG_FILE}"
    echo "     Ensure DATABASE_URL is configured for your PostgreSQL host."
    echo "     After editing, apply changes: sudo systemctl restart envoytrade"
    echo "=================================================================="

else
    echo "Unsupported OS: ${OS}. Binary placed at ${INSTALL_BIN}."
fi
