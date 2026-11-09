# Phase 7: Scheduled Routines & E2E Verification

This document specifies Phase 7 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Formally catalog all new in-process periodic routines and tickers for Kotak Neo in `docs/CRON_AND_PERIODIC_ROUTINES.md` per mandatory repository rules, execute full unit and integration test suites, and perform end-to-end regression testing.

---

## 2. Mandatory Documentation Updates (`docs/CRON_AND_PERIODIC_ROUTINES.md`)

Add the following entries to **Section 2 (In-Process Go Periodic Routines)**:
- **Kotak Neo Token Renewer**:
  - Frequency / Schedule: `06:30:00 AM IST` daily
  - Code Location: `internal/kotak/renewer.go` (`renewer.Run`)
  - Description: Automated 2-step TOTP/MPIN login for active Kotak Neo accounts through assigned static proxies before trading hours, refreshing session tokens and dynamic gateways, and restarting master tickers / follower workers.
- **Kotak Neo Instrument Master Sync**:
  - Frequency / Schedule: `05:00:00 AM IST` daily
  - Code Location: `internal/instrument/syncer.go` (`syncer.StartDailyRoutine`)
  - Description: Downloads and parses streaming Kotak Neo scrip master segment CSVs. Transactionally prunes existing records only upon 100% success, batch inserts in chunks of 5,000, runs `VACUUM ANALYZE instruments`, and refreshes active F&O in `instrument.Cache`.

---

## 3. End-to-End Test & Verification Matrix

### 3.1 Automated Verification Commands
```sh
# 1. Run unit tests across all Kotak and related packages
go test -v ./internal/kotak/...
go test -v ./internal/kotak/ws/...
go test -v ./internal/kotak/callback/...
go test -v ./internal/instrument/...
go test -v ./internal/httpapi/... -run TestAccount

# 2. Run frontend unit tests
pnpm -C frontend test --run

# 3. Build Go backend and frontend
go build ./...
go vet ./...
pnpm -C frontend build

# 4. Run integration tests (Postgres via Podman testcontainers)
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test -tags integration ./internal/store/postgres/... -v
```

### 3.2 End-to-End Workflow Verification
1. **First-Time Account Addition**: Add a Kotak Neo master and follower account via UI with 10-digit mobile number, MPIN, TOTP, and assigned proxy IP. Verify status transitions to `active`.
2. **Scheduled Morning Renewal**: Run renewal loop and verify `accounts.access_token` and `token_expires_at` are updated, and master ticker restarts with new session.
3. **Copy-Trade Execution**: Execute an order on master account; verify real-time fan-out placement to Kotak Neo follower account.
4. **WebSocket Reconnect**: Simulate temporary network drop on master WebSocket; verify clean reconnect and catch-up reconciliation.
