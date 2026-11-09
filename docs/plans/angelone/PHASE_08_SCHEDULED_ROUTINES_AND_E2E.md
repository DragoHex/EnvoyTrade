# Phase 8: Scheduled Routines & E2E Verification

This document specifies Phase 8 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Wire all Angel One components into `cmd/server/main.go`, register scheduled routines, update `docs/CRON_AND_PERIODIC_ROUTINES.md`, and execute complete end-to-end integration testing.

---

## 2. Server Wiring (`cmd/server/main.go`)

### 2.1 Broker Adapter Factory
```go
angeloneBrokerFactory := broker.FactoryFunc(func(ctx context.Context, acc broker.BrokerAccount) (broker.Broker, error) {
    proxyCfg := acc.ToProxyConfig()
    var session *angelone.SessionJSON
    if acc.AccessToken != "" {
        session, _ = angelone.DeserializeSession(acc.AccessToken)
    }
    return angelone.NewLiveBroker(acc.ApiKey, session, proxyCfg, instrumentCache)
})
brokerRegistry.Register("angelone", angeloneBrokerFactory)
```

### 2.2 Ticker & Renewer Lifecycle
- **WebSocket Master Ticker**: Register `angeloneTickerFactory` with `tickerManager.RegisterFactory("angelone", angeloneTickerFactory)`.
- **Token Renewer (06:00 AM IST)**: Initialize `angeloneRenewer.Start(ctx)` with token eviction callbacks:
  ```go
  angeloneRenewer.OnTokenRefreshed = func(refreshCtx context.Context, accountID uuid.UUID) error {
      evicter.Evict(accountID)
      return handleTokenRefreshed(refreshCtx, accountID, store, tickerManager, followerRegistrar, logger)
  }
  ```
- **Portfolio Syncer**: Register `rawAngelOneSyncer` within `multiBrokerSyncer`.
- **Postback Ingestion**: Register `angelonePostback` in `postbackRouter.Register("angelone", angelonePostback)`.
- **Instrument Syncer**: Add `--broker angelone` support to `runSyncInstruments`.

---

## 3. Scheduled Routines Documentation Update

Update [`docs/CRON_AND_PERIODIC_ROUTINES.md`](../../CRON_AND_PERIODIC_ROUTINES.md) to record:
- **Angel One Token Renewer**: In-process background goroutine executing at `06:00:00 AM IST` daily.
- **Instrument Sync**: Daily streaming JSON ingestion at `05:00:00 AM IST`.

---

## 4. End-to-End Verification Plan

### 4.1 Automated Tests
```sh
# Unit Tests (Fast, zero dependencies)
go test -v ./internal/angelone/...
go test -v ./internal/instrument/... -run TestSyncAngelOne
go test -v ./internal/httpapi/... -run TestPostAccount_AngelOne

# Frontend Tests
pnpm -C frontend test --run
pnpm -C frontend build

# Integration Tests (Requires Podman / Testcontainers)
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test -tags integration -v ./internal/store/postgres/... -run TestAngelOne
```

### 4.2 Manual Verification Checklist
1. Create Angel One master and follower accounts via dashboard drawer with static proxy IPs.
2. Confirm password and TOTP secrets are encrypted in Postgres (`accounts` table).
3. Verify portfolio sync populates funds, margins, and positions on the dashboard.
4. Execute test order on master; verify fan-out to follower with `ET-[hash]` tag.
5. Trigger intentional disconnect on master WebSocket; confirm auto-reconnect and REST catch-up reconciliation.
