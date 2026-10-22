# Broker Authentication Lifecycle & Token Renewal

This document details the authentication lifecycle for broker accounts (primarily Zerodha Kite Connect), headless auto-renewal, fail-safe mechanisms for missed schedules, and pre-market readiness procedures in EnvoyTrade.

---

## 1. Core Principles & Expiration Rules

1. **Daily Expiration Cutoff**:
   - Zerodha Kite Connect access tokens **strictly expire at 06:00 AM IST every single day**, regardless of when they were issued.
   - Any token with an expiration timestamp before `now()` or with `NULL` expiration is considered inactive.
2. **Entitlement & Refresh Tokens**:
   - Standard Kite Connect apps do **not** provide OAuth refresh tokens (refresh token entitlement is a separate paid add-on from Zerodha).
   - EnvoyTrade bridges this gap using an automated **Headless 2FA Login Client** (`internal/kite/login.go`) for accounts configured with encrypted credentials.
3. **Fail-Fast Safety (No Silent RMS Failures)**:
   - When an account's token is expired or unauthenticated, the engine refuses order placement (`domain.ErrAuthExpired`).
   - Copy-trading orders are **never** dispatched with unauthenticated or unverified sessions, avoiding cascading RMS rejections and broker rate penalties.

---

## 2. Authentication & Self-Healing Decision Flow

EnvoyTrade evaluates token validity **dynamically in Go memory on every operation** rather than relying solely on a fixed scheduled cron. If a 6:00 AM job is missed (e.g. server reboot, network blip), the system self-heals on-demand.

```mermaid
flowchart TD
    Start["Request / System Boot / Background Syncer"] --> CheckToken{"Is Token Active?<br/>(token_expires_at > now)"}
    
    CheckToken -- "Yes" --> ActiveSession["Active Broker Session<br/>(status: 'authenticated')"]
    ActiveSession --> MarketReady["Ready for Live Market Execution<br/>(Master Ticker / Follower Workers Active)"]
    
    CheckToken -- "No" --> CheckCreds{"Stored Encrypted<br/>Password & TOTP Seed?"}
    
    CheckCreds -- "Yes" --> HeadlessAuth["Trigger Headless Login<br/>(internal/kite/login.go)"]
    
    HeadlessAuth --> GenTOTP["Decrypt Credentials (AES-256-GCM)<br/>Generate RFC 6238 TOTP Code"]
    GenTOTP --> ProxyEgress["Route via Account's Assigned<br/>Static Proxy IP"]
    ProxyEgress --> PostKite["Submit Credentials to Zerodha Kite<br/>Exchange Request Token for Access Token"]
    
    PostKite -- "Success" --> PersistToken["Persist New Access Token<br/>Set token_expires_at = Next 06:00 AM IST<br/>auth_status = 'authenticated'"]
    PersistToken --> RehookServices["Notify OnTokenRefreshed:<br/>1. Restart Master Ticker Manager<br/>2. Re-register Follower Worker Broker"]
    RehookServices --> MarketReady
    
    PostKite -- "Failure (Captcha / Bad Password / 2FA Mismatch)" --> RecordError["Record Error in DB:<br/>auth_status = 'error'<br/>auth_error = 'Kite Login: ...'"]
    CheckCreds -- "No" --> RecordInactive["Record Inactive Status:<br/>auth_status = 'error'<br/>auth_error = 'Token expired / session inactive'"]
    
    RecordError --> AttentionBoard["Pre-Market Broker Readiness Attention Board<br/>(Trade Operations: 07:00 - 09:00 AM IST)"]
    RecordInactive --> AttentionBoard
    
    AttentionBoard --> HumanIntervention["Admin / Trader Morning Checklist:<br/>1. Update Credentials in UI Drawer, or<br/>2. Complete Interactive Login"]
    AttentionBoard --> DispatchRefusal["Safe Fail-Fast:<br/>Order Dispatch Blocked for Account<br/>(No Cascade RMS Rejections)"]
```

---

## 3. Headless Auto-Login Sequence

When token renewal triggers, the following 5-step sequence runs through the account's assigned static proxy IP:

```mermaid
sequenceDiagram
    autonumber
    participant Syncer as Syncer / Dispatcher
    participant Store as PostgreSQL Store
    participant Engine as Headless Client (kite/login.go)
    participant Proxy as Static Egress Proxy
    participant Kite as Zerodha Kite Connect
    participant Ticker as Ticker / Worker Pool

    Syncer->>Store: AccountAuthInfo(accountID)
    Store-->>Syncer: Return encrypted credentials, proxy IP, token
    
    Note over Syncer: Detected token_expires_at <= now()
    
    Syncer->>Engine: HeadlessLogin(brokerUserID, encPass, encTOTP, apiKey, apiSecret)
    Engine->>Engine: Decrypt via AES-256-GCM
    Engine->>Engine: Generate 6-digit TOTP code
    
    Engine->>Proxy: POST /api/login (User ID + Password)
    Proxy->>Kite: Forward via Static IP
    Kite-->>Proxy: 200 OK (request_id)
    Proxy-->>Engine: request_id
    
    Engine->>Proxy: POST /api/twofa (request_id + TOTP)
    Proxy->>Kite: Forward via Static IP
    Kite-->>Proxy: 200 OK (login session cookie)
    Proxy-->>Engine: Session cookie
    
    Engine->>Proxy: GET /connect/login?api_key=...
    Proxy->>Kite: Follow OAuth redirect
    Kite-->>Proxy: 302 Redirect (request_token)
    Proxy-->>Engine: request_token
    
    Engine->>Proxy: POST /session/token (api_key + request_token + checksum)
    Proxy->>Kite: Exchange for access token
    Kite-->>Proxy: 200 OK (access_token, public_token)
    Proxy-->>Engine: access_token
    
    Engine-->>Syncer: New Access Token
    
    Syncer->>Store: SetAccountAccessToken(id, token, nextExpiryAt, "authenticated", "")
    Syncer->>Ticker: OnTokenRefreshed(accountID)
    Ticker->>Ticker: Evict stale client & reconnect ticker/worker
```

---

## 4. Handling Missed 6:00 AM Schedules

If the scheduled 6:00 AM trigger does not run (for instance, the server was offline overnight and started at 07:30 AM), EnvoyTrade handles it through multiple defensive layers:

| Layer | Mechanism | Impact |
|---|---|---|
| **1. Lazy In-Memory Evaluation** | Go evaluates `token_expires_at.Before(now)` dynamically inside `accountFromRow` and `BrokerReadiness`. | No stale "authenticated" state is ever returned to clients or execution pipelines, even if the DB row wasn't updated by a cron. |
| **2. Self-Healing on Access** | Any portfolio synchronization, manual trigger, or administrative check invokes `SyncAccountPortfolio`. | Tokens are re-authenticated headlessly on-demand in <1.5s as soon as the service boots. |
| **3. Mid-Day Invalidation Catch** | If an active session is revoked mid-day (e.g. mobile app login conflict), `IsAuthError(err)` intercepts Kite `TokenException` or HTTP 403. | Immediately attempts re-login, evicts the stale cached broker, and resumes without process restarts. |
| **4. Pre-Market Visual Checklist** | The **Pre-Market Broker Readiness** dashboard (`/admin/operations`) aggregates all accounts between 07:00 AM and 09:00 AM IST. | Operators have visual confirmation that all masters and followers are ready before market open at 09:15 AM IST. |
| **5. Strict SEBI / Risk Refusal** | If an account remains in an error state at market open, `engine.HandleMasterFill` refuses order placement for that follower (`ErrAuthExpired`). | Zero orphaned orders or broker account locking. |

---

## 5. Next Cutoff Calculation

The expiration boundary is determined by `NextKiteExpiry`:

```go
func NextKiteExpiry(now time.Time) time.Time {
    nowIST := now.In(domain.IST)
    target := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 6, 0, 0, 0, domain.IST)
    if !nowIST.Before(target) {
        target = target.AddDate(0, 0, 1)
    }
    return target
}
```

- Any token renewed at 06:15 AM IST on day $D$ is valid until 06:00 AM IST on day $D+1$.
- Any token renewed at 05:45 AM IST on day $D$ is valid until 06:00 AM IST on day $D$.

---

## 6. Related Documentation

- For web dashboard and platform user authentication lifecycle, idle sliding timeout, and maximum ceiling rules, see [Platform User Session Lifecycle](USER_SESSION_LIFECYCLE.md).

