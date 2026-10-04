#!/usr/bin/env bash
set -e

# ==============================================================================
# uninstall.sh - Smart uninstaller / decommissioner for EnvoyTrade Single Binary
# ==============================================================================

OS="$(uname -s)"
PURGE=false

for arg in "$@"; do
    case $arg in
        --purge|-p)
            PURGE=true
            shift
            ;;
    esac
done

echo "=================================================================="
echo " EnvoyTrade Decommissioning / Uninstallation"
echo " Detected OS: ${OS}"
echo "=================================================================="

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

if [ "${OS}" = "Darwin" ]; then
    # Dynamically resolve the active macOS user (never hardcoded)
    TARGET_USER="${SUDO_USER:-}"
    if [ -z "${TARGET_USER}" ] || [ "${TARGET_USER}" = "root" ]; then
        TARGET_USER="$(stat -f '%Su' /dev/console 2>/dev/null || true)"
    fi
    if [ -z "${TARGET_USER}" ] || [ "${TARGET_USER}" = "root" ]; then
        TARGET_USER="$(logname 2>/dev/null || id -un)"
    fi
    TARGET_UID="$(id -u "${TARGET_USER}" 2>/dev/null || id -u)"

    # Handle split user home directories (e.g. /Users/msp/MSP vs /Users/msp)
    OS_HOME="$(eval echo ~${TARGET_USER})"
    SHELL_HOME="${HOME:-${OS_HOME}}"
    if [ "${SHELL_HOME}" = "/var/root" ] || [ "${SHELL_HOME}" = "/root" ]; then
        SHELL_HOME="${OS_HOME}"
    fi

    echo "--> Stopping macOS launchd service for user ${TARGET_USER} (uid ${TARGET_UID})..."

    # Use modern bootout API — deregisters from GUI session domain cleanly
    if [ "$(id -u)" -eq 0 ]; then
        sudo -u "${TARGET_USER}" launchctl bootout "gui/${TARGET_UID}/com.envoytrade.server" 2>/dev/null || true
        launchctl bootout "gui/0/com.envoytrade.server" 2>/dev/null || true
        rm -f /var/root/Library/LaunchAgents/com.envoytrade.server.plist 2>/dev/null || true
    else
        launchctl bootout "gui/${TARGET_UID}/com.envoytrade.server" 2>/dev/null || true
    fi

    echo "--> Removing LaunchAgent definitions..."
    for h in "${SHELL_HOME}" "${OS_HOME}"; do
        [ -n "$h" ] || continue
        if [ -f "${h}/Library/LaunchAgents/com.envoytrade.server.plist" ]; then
            rm -f "${h}/Library/LaunchAgents/com.envoytrade.server.plist"
            echo "    Removed ${h}/Library/LaunchAgents/com.envoytrade.server.plist"
        fi
    done

    echo "--> Removing binary and maintenance scripts..."
    if [ -f "/usr/local/bin/envoytrade" ]; then
        run_privileged rm -f "/usr/local/bin/envoytrade"
        echo "    Removed /usr/local/bin/envoytrade"
    fi
    if [ -f "/usr/local/bin/envoytrade-db-maintenance" ]; then
        run_privileged rm -f "/usr/local/bin/envoytrade-db-maintenance"
        echo "    Removed /usr/local/bin/envoytrade-db-maintenance"
    fi

    rm -f /tmp/envoytrade.stdout.log /tmp/envoytrade.stderr.log

    if [ "${PURGE}" = true ]; then
        echo "--> Purging configuration directories..."
        for h in "${SHELL_HOME}" "${OS_HOME}"; do
            [ -n "$h" ] || continue
            if [ -d "${h}/.envoytrade" ]; then
                rm -rf "${h}/.envoytrade"
                echo "    Purged ${h}/.envoytrade"
            fi
        done
    else
        echo "    Configuration preserved in ~/.envoytrade (use --purge to delete)."
    fi

    echo "=================================================================="
    echo " EnvoyTrade has been decommissioned from macOS."
    echo "=================================================================="

elif [ "${OS}" = "Linux" ]; then
    echo "--> Stopping and disabling Linux systemd service..."
    if command -v systemctl >/dev/null 2>&1; then
        run_privileged systemctl stop envoytrade.service 2>/dev/null || true
        run_privileged systemctl disable envoytrade.service 2>/dev/null || true
        if [ -f "/etc/systemd/system/envoytrade.service" ]; then
            run_privileged rm -f "/etc/systemd/system/envoytrade.service"
            run_privileged systemctl daemon-reload
            echo "    Removed /etc/systemd/system/envoytrade.service"
        fi
    fi

    echo "--> Removing binary and maintenance scripts..."
    if [ -f "/usr/local/bin/envoytrade" ]; then
        run_privileged rm -f "/usr/local/bin/envoytrade"
        echo "    Removed /usr/local/bin/envoytrade"
    fi
    if [ -f "/usr/local/bin/envoytrade-db-maintenance" ]; then
        run_privileged rm -f "/usr/local/bin/envoytrade-db-maintenance"
        echo "    Removed /usr/local/bin/envoytrade-db-maintenance"
    fi
    if [ -f "/etc/cron.d/envoytrade-maintenance" ]; then
        run_privileged rm -f "/etc/cron.d/envoytrade-maintenance"
        echo "    Removed /etc/cron.d/envoytrade-maintenance"
    fi

    if [ "${PURGE}" = true ]; then
        echo "--> Purging configuration, logs, backups, and data directories..."
        run_privileged rm -rf /etc/envoytrade /var/log/envoytrade /var/lib/envoytrade /var/backups/envoytrade
        echo "    Removed /etc/envoytrade, /var/log/envoytrade, /var/lib/envoytrade, and /var/backups/envoytrade"
    else
        echo "    Config preserved in /etc/envoytrade and logs in /var/log/envoytrade (use --purge to delete)."
    fi

    echo "=================================================================="
    echo " EnvoyTrade has been decommissioned from Linux."
    echo "=================================================================="
else
    echo "Unsupported OS: ${OS}."
fi
