#!/usr/bin/env bash
# scripts/expose.sh
# Exposes EnvoyTrade local service via DuckDNS with mandatory HTTPS.

set -euo pipefail

# Configuration defaults
DOMAIN="${DOMAIN:-envoytrade.duckdns.org}"
BACKEND_PORT="${BACKEND_PORT:-8080}"
HTTPS_PORT="${HTTPS_PORT:-443}"
HTTP_PORT="${HTTP_PORT:-80}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CADDYFILE="${CADDYFILE:-${SCRIPT_DIR}/Caddyfile}"
PID_FILE="${PID_FILE:-/tmp/caddy-envoytrade.pid}"
LOG_FILE="${LOG_FILE:-/tmp/caddy-envoytrade.log}"

# Formatting
BOLD='\033[1m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() {
    printf "${CYAN}==>${NC} %s\n" "$1"
}

log_ok() {
    printf "${GREEN}✔${NC} %s\n" "$1"
}

log_warn() {
    printf "${YELLOW}⚠${NC} %s\n" "$1"
}

log_err() {
    printf "${RED}✖${NC} %s\n" "$1"
}

# 1. Dependency checks
check_deps() {
    log_info "Checking dependencies..."
    if ! command -v curl >/dev/null 2>&1; then
        log_err "curl is required but not installed."
        exit 1
    fi

    if ! command -v caddy >/dev/null 2>&1; then
        log_err "caddy is required for automated TLS & reverse proxying."
        printf "Install it via Homebrew: brew install caddy\n"
        exit 1
    fi
    log_ok "Dependencies available (curl, caddy)."
}

# 2. Local Backend Check
check_backend() {
    log_info "Checking EnvoyTrade backend on port ${BACKEND_PORT}..."
    if curl -s -m 2 "http://localhost:${BACKEND_PORT}/api/v1/accounts" >/dev/null 2>&1; then
        log_ok "EnvoyTrade backend is responding on http://localhost:${BACKEND_PORT}"
        return 0
    elif curl -s -m 2 "http://localhost:${BACKEND_PORT}/" >/dev/null 2>&1; then
        log_ok "Service responding on http://localhost:${BACKEND_PORT}"
        return 0
    else
        log_warn "EnvoyTrade backend is NOT responding on http://localhost:${BACKEND_PORT}"
        log_warn "Start backend first: go run ./cmd/server"
        return 1
    fi
}

# 3. Detect LAN IP
get_lan_ip() {
    local lan_ip=""
    if command -v ipconfig >/dev/null 2>&1; then
        for iface in en0 en1 en2 en3; do
            lan_ip="$(ipconfig getifaddr "$iface" 2>/dev/null || true)"
            if [[ -n "$lan_ip" ]]; then
                break
            fi
        done
    fi

    if [[ -z "$lan_ip" ]] && command -v hostname >/dev/null 2>&1; then
        lan_ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    fi

    if [[ -z "$lan_ip" ]]; then
        lan_ip="127.0.0.1"
    fi
    echo "$lan_ip"
}

# 4. Detect WAN IP
get_wan_ip() {
    local wan_ip=""
    wan_ip="$(curl -s -m 4 https://api.ipify.org 2>/dev/null || true)"
    if [[ -z "$wan_ip" ]]; then
        wan_ip="$(curl -s -m 4 https://ifconfig.me 2>/dev/null || true)"
    fi
    echo "$wan_ip"
}

# 5. Resolve DuckDNS Subdomain IP
get_duckdns_ip() {
    local duck_ip=""
    # First attempt: Google DNS over HTTPS (reliable across firewalls)
    local doh_resp
    doh_resp="$(curl -s -m 4 "https://dns.google/resolve?name=${DOMAIN}&type=A" 2>/dev/null || true)"
    if [[ -n "$doh_resp" ]]; then
        duck_ip="$(printf "%s" "$doh_resp" | grep -o '"data":"[^"]*"' | head -n 1 | cut -d'"' -f4 || true)"
    fi

    # Fallback attempt: dig
    if [[ -z "$duck_ip" ]] && command -v dig >/dev/null 2>&1; then
        duck_ip="$(dig +short "${DOMAIN}" 2>/dev/null | tail -n 1 || true)"
    fi
    echo "$duck_ip"
}

# 6. Verify IP synchronization
verify_ip_sync() {
    local wan_ip
    wan_ip="$(get_wan_ip)"
    local duck_ip
    duck_ip="$(get_duckdns_ip)"

    log_info "Verifying IP alignment for ${DOMAIN}..."
    if [[ -z "$wan_ip" ]]; then
        log_warn "Unable to detect public WAN IP (check internet connection)."
        return 0
    fi

    if [[ -z "$duck_ip" ]]; then
        log_warn "Unable to resolve ${DOMAIN}. Verify DuckDNS registration."
        return 0
    fi

    printf "  • Current WAN IP:     %s\n" "$wan_ip"
    printf "  • DuckDNS Domain IP:  %s\n" "$duck_ip"

    if [[ "$wan_ip" == "$duck_ip" ]]; then
        log_ok "DuckDNS IP matches current public WAN IP (${wan_ip})."
    else
        log_warn "IP mismatch detected!"
        printf "    Your current WAN IP is:  %s\n" "$wan_ip"
        printf "    DuckDNS currently has:   %s\n" "$duck_ip"
        printf "    ${BOLD}Action required:${NC} Open duckdns.org in your browser and click 'update ip'.\n"
    fi
}

# 7. Router port forwarding helper & UPnP
check_port_forwarding() {
    local lan_ip
    lan_ip="$(get_lan_ip)"

    log_info "Checking router port forwarding..."
    local upnp_applied=false

    if command -v upnpc >/dev/null 2>&1; then
        # Try UPnP if client is present
        if upnpc -s >/dev/null 2>&1; then
            log_info "UPnP router found. Applying port mappings..."
            upnpc -a "$lan_ip" "$HTTPS_PORT" "$HTTPS_PORT" TCP 3600 >/dev/null 2>&1 && \
            upnpc -a "$lan_ip" "$HTTP_PORT" "$HTTP_PORT" TCP 3600 >/dev/null 2>&1 && \
            upnp_applied=true
        fi
    fi

    if [[ "$upnp_applied" == "true" ]]; then
        log_ok "UPnP port mappings configured (443 & 80 -> ${lan_ip})."
    else
        printf "  ${YELLOW}Notice:${NC} Ensure your home router forwards incoming traffic:\n"
        printf "    • External TCP ${HTTPS_PORT} -> ${lan_ip}:${HTTPS_PORT} (Mandatory HTTPS)\n"
        printf "    • External TCP ${HTTP_PORT}  -> ${lan_ip}:${HTTP_PORT}  (Let's Encrypt TLS challenge & HTTP->HTTPS redirect)\n"
    fi
}

# 8. Start Caddy reverse proxy
start_caddy() {
    log_info "Configuring and starting Caddy with mandatory HTTPS..."

    # Check if already running
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        log_ok "Caddy is already running (PID $(cat "$PID_FILE"))."
        return 0
    fi

    if ! [[ -f "$CADDYFILE" ]]; then
        log_err "Caddyfile not found at ${CADDYFILE}."
        exit 1
    fi

    # Validate configuration
    if ! caddy validate --config "$CADDYFILE" >/dev/null 2>&1; then
        log_err "Caddyfile validation failed."
        caddy validate --config "$CADDYFILE"
        exit 1
    fi

    export DOMAIN
    export BACKEND_PORT
    export LOG_FILE

    # Start caddy in background
    caddy start --config "$CADDYFILE" --pidfile "$PID_FILE"

    log_ok "Caddy background service started."
}

# 9. Verify HTTPS Enforcement
verify_https_enforcement() {
    log_info "Verifying mandatory HTTPS enforcement..."

    # Test HTTP -> HTTPS redirect
    local http_code
    http_code="$(curl -s -o /dev/null -w "%{http_code}" -m 2 "http://localhost:${HTTP_PORT}/" -H "Host: ${DOMAIN}" 2>/dev/null || true)"

    if [[ "$http_code" =~ ^(301|302|307|308)$ ]]; then
        log_ok "Mandatory HTTPS verified: HTTP (port 80) redirects with HTTP ${http_code} to HTTPS."
    else
        log_warn "HTTP redirect check returned status: ${http_code}."
    fi
}

# 10. Stop Caddy
stop_caddy() {
    log_info "Stopping Caddy reverse proxy..."
    caddy stop --config "$CADDYFILE" 2>/dev/null || true

    if [[ -f "$PID_FILE" ]]; then
        local pid
        pid="$(cat "$PID_FILE" 2>/dev/null || true)"
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            sleep 1
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
        rm -f "$PID_FILE"
    fi

    # Also clean up any orphan caddy processes referencing our Caddyfile
    local orphan_pids
    orphan_pids="$(pgrep -f "caddy.*scripts/Caddyfile" 2>/dev/null || true)"
    if [[ -n "$orphan_pids" ]]; then
        kill $orphan_pids 2>/dev/null || true
    fi
    log_ok "Caddy reverse proxy stopped."
}

# 11. Status command
status_caddy() {
    log_info "EnvoyTrade Exposure Status"
    printf "========================================\n"
    printf "Domain:         %s\n" "$DOMAIN"
    printf "Backend Port:   %s\n" "$BACKEND_PORT"
    printf "LAN IP:         %s\n" "$(get_lan_ip)"
    printf "WAN IP:         %s\n" "$(get_wan_ip)"
    printf "DuckDNS IP:     %s\n" "$(get_duckdns_ip)"
    printf "PID File:       %s\n" "$PID_FILE"
    printf "Log File:       %s\n" "$LOG_FILE"
    printf "%s\n" "----------------------------------------"

    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        log_ok "Caddy is RUNNING (PID $(cat "$PID_FILE"))."
    else
        log_warn "Caddy is STOPPED."
    fi

    if curl -s -m 2 "http://localhost:${BACKEND_PORT}/api/v1/accounts" >/dev/null 2>&1; then
        log_ok "Backend service is UP (localhost:${BACKEND_PORT})."
    else
        log_warn "Backend service is DOWN on localhost:${BACKEND_PORT}."
    fi
    printf "========================================\n"
}

# 12. Print accessible URLs
print_urls() {
    printf "\n"
    printf "${GREEN}${BOLD}================================================================${NC}\n"
    printf "${GREEN}${BOLD}  EnvoyTrade Service Exposed via Mandatory HTTPS               ${NC}\n"
    printf "${GREEN}${BOLD}================================================================${NC}\n"
    printf "  • Public Base URL:     ${BOLD}https://%s${NC}\n" "$DOMAIN"
    printf "  • API Accounts:        ${BOLD}https://%s/api/v1/accounts${NC}\n" "$DOMAIN"
    printf "  • Broker Callback URL: ${BOLD}https://%s/broker-callback${NC}\n" "$DOMAIN"
    printf "\n"
    printf "  ${BOLD}Kite Developer Portal Settings:${NC}\n"
    printf "    Postback URL: https://%s/broker-callback\n" "$DOMAIN"
    printf "    Redirect URL: https://%s/api/auth/callback\n" "$DOMAIN"
    printf "${GREEN}${BOLD}================================================================${NC}\n\n"
}

# Main dispatcher
main() {
    local cmd="${1:-start}"

    case "$cmd" in
        start)
            check_deps
            check_backend || true
            verify_ip_sync
            check_port_forwarding
            start_caddy
            verify_https_enforcement
            print_urls
            ;;
        run)
            check_deps
            check_backend || true
            verify_ip_sync
            check_port_forwarding
            print_urls
            log_info "Starting Caddy in foreground (Ctrl+C to stop)..."
            export DOMAIN
            export BACKEND_PORT
            export LOG_FILE
            exec caddy run --config "$CADDYFILE"
            ;;
        stop)
            stop_caddy
            ;;
        restart)
            stop_caddy
            sleep 1
            main start
            ;;
        status)
            status_caddy
            ;;
        check)
            check_deps
            check_backend || true
            verify_ip_sync
            check_port_forwarding
            ;;
        --help|-h|help)
            printf "Usage: %s [start|run|stop|restart|status|check]\n\n" "$0"
            printf "Commands:\n"
            printf "  start    Verify backend, sync IP, start Caddy in background (default)\n"
            printf "  run      Verify backend, sync IP, run Caddy in foreground (live logs)\n"
            printf "  stop     Stop Caddy reverse proxy\n"
            printf "  restart  Restart Caddy reverse proxy\n"
            printf "  status   Show current exposure, IP sync, and service status\n"
            printf "  check    Run IP and connectivity checks without starting Caddy\n"
            ;;
        *)
            log_err "Unknown command: $cmd"
            printf "Run '%s --help' for usage.\n" "$0"
            exit 1
            ;;
    esac
}

main "$@"
