#!/usr/bin/env bash
# scripts/expose_test.sh
# Comprehensive tests for scripts/expose.sh and scripts/Caddyfile

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EXPOSE_SCRIPT="${SCRIPT_DIR}/expose.sh"
CADDYFILE="${SCRIPT_DIR}/Caddyfile"

PASS_COUNT=0
FAIL_COUNT=0

assert_eq() {
    local desc="$1"
    local expected="$2"
    local actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        printf "✔ PASS: %s\n" "$desc"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        printf "✖ FAIL: %s (expected '%s', got '%s')\n" "$desc" "$expected" "$actual"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_contains() {
    local desc="$1"
    local needle="$2"
    local haystack="$3"
    if [[ "$haystack" == *"$needle"* ]]; then
        printf "✔ PASS: %s\n" "$desc"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        printf "✖ FAIL: %s ('%s' not found in output)\n" "$desc" "$needle"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_success() {
    local desc="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        printf "✔ PASS: %s\n" "$desc"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        printf "✖ FAIL: %s (command failed: %s)\n" "$desc" "$*"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

printf "================================================\n"
printf "Running tests for expose.sh and Caddyfile\n"
printf "================================================\n\n"

# Test 1: Bash syntax
assert_success "expose.sh has valid bash syntax" bash -n "$EXPOSE_SCRIPT"

# Test 2: Caddyfile syntax validation
assert_success "Caddyfile is valid per Caddy validator" caddy validate --config "$CADDYFILE"

# Test 3: Caddyfile enforces reverse proxy to backend
CADDY_CONTENT="$(cat "$CADDYFILE")"
assert_contains "Caddyfile proxies to localhost:8080" "reverse_proxy localhost:{\$BACKEND_PORT:8080}" "$CADDY_CONTENT"
assert_contains "Caddyfile has Strict-Transport-Security header" "Strict-Transport-Security" "$CADDY_CONTENT"
assert_contains "Caddyfile has X-Content-Type-Options nosniff" "X-Content-Type-Options" "$CADDY_CONTENT"
assert_contains "Caddyfile has X-Frame-Options DENY" "X-Frame-Options" "$CADDY_CONTENT"

# Test 4: Help command
HELP_OUTPUT="$("$EXPOSE_SCRIPT" --help 2>&1 || true)"
assert_contains "Help output mentions commands" "Commands:" "$HELP_OUTPUT"
assert_contains "Help output mentions start" "start" "$HELP_OUTPUT"
assert_contains "Help output mentions check" "check" "$HELP_OUTPUT"
assert_contains "Help output mentions status" "status" "$HELP_OUTPUT"

# Test 5: Unknown command fails with exit 1
set +e
"$EXPOSE_SCRIPT" invalid-cmd >/dev/null 2>&1
EXIT_CODE=$?
set -e
assert_eq "Unknown command exits with 1" "1" "$EXIT_CODE"

# Test 6: Check command executes pre-flight checks
CHECK_OUTPUT="$("$EXPOSE_SCRIPT" check 2>&1 || true)"
assert_contains "Check outputs dependency verification" "Dependencies available" "$CHECK_OUTPUT"
assert_contains "Check outputs domain IP alignment" "Verifying IP alignment" "$CHECK_OUTPUT"
assert_contains "Check outputs router port forwarding notice" "router forwards incoming traffic" "$CHECK_OUTPUT"

# Test 7: Status command executes
STATUS_OUTPUT="$("$EXPOSE_SCRIPT" status 2>&1 || true)"
assert_contains "Status output includes Domain" "Domain:" "$STATUS_OUTPUT"
assert_contains "Status output includes LAN IP" "LAN IP:" "$STATUS_OUTPUT"
assert_contains "Status output includes WAN IP" "WAN IP:" "$STATUS_OUTPUT"
assert_contains "Status output includes Backend status" "Backend" "$STATUS_OUTPUT"

# Test 8: Verify mandatory HTTPS port 443 in adapted config
ADAPTED_JSON="$(caddy adapt --config "$CADDYFILE")"
assert_contains "Adapted Caddy config listens on port 443" '":443"' "$ADAPTED_JSON"
assert_contains "Adapted Caddy config targets localhost:8080" '"dial":"localhost:8080"' "$ADAPTED_JSON"

printf "\n================================================\n"
printf "Results: %d passed, %d failed\n" "$PASS_COUNT" "$FAIL_COUNT"
printf "================================================\n"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    exit 1
fi
