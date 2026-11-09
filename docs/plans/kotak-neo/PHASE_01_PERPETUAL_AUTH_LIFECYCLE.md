# Phase 1: Perpetual Authentication & Session Lifecycle

This document specifies Phase 1 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Establish perpetual, headless authentication for Kotak Neo accounts so that after the user enters their credentials once during account creation, EnvoyTrade maintains uninterrupted authenticated sessions autonomously across daily broker token expirations and server restarts.

---

## 2. Authentication Protocol & Credentials Model

### 2.1 Credential Mapping
No database schema migration is required. Credentials map directly to [`Account`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/domain/types.go#L159-L179):
- `broker`: `"kotakneo"`
- `broker_user_id`: Kotak Neo Client ID / UCC (e.g. `X12345`)
- `api_key`: Registered Mobile Number in canonical `+91XXXXXXXXXX` format
- `api_secret`: Neo Developer Portal Consumer Key (Bearer Access Token)
- `encrypted_password`: AES-GCM encrypted 6-digit MPIN
- `encrypted_totp_secret`: AES-GCM encrypted Base32 TOTP secret key
- `access_token`: Active `TradeSessionJSON` containing discovered dynamic cluster endpoints and tokens

### 2.2 Dynamic Session Storage (`TradeSessionJSON`)
Kotak Neo dynamically returns cluster URLs upon successful Step 2 MPIN validation:
```go
type TradeSessionJSON struct {
    Token        string `json:"token"`
    Sid          string `json:"sid"`
    BaseURL      string `json:"baseUrl"`
    FeedURL      string `json:"feedUrl"`
    RTURL        string `json:"rtUrl"`
    GreetingName string `json:"greetingName,omitempty"`
    UCC          string `json:"ucc,omitempty"`
}
```

### 2.3 Mobile Number Normalization
Upstream Kotak Neo strictly requires `+91` followed by 10 digits (`+919876543210`).
The backend provides `NormalizeMobileNumber(raw string) (string, error)`:
- Strips whitespace, dashes, and parens.
- 10 digits (`9876543210`) -> prepends `+91`.
- 12 digits starting with `91` (`919876543210`) -> prepends `+`.
- 13 characters starting with `+91` (`+919876543210`) -> valid as-is.
- Any other length or non-Indian format fails validation.

### 2.4 Token Expiry Calculation (`NextKotakExpiry`)
Kotak Neo sessions expire overnight.
`NextKotakExpiry(now time.Time) time.Time`:
- Evaluated against `Asia/Kolkata` (IST).
- Calculates the next `06:00:00 AM IST`.
- If called between midnight and 06:00 AM IST $\to$ expires today at 06:00 AM IST.
- If called after 06:00 AM IST $\to$ expires tomorrow at 06:00 AM IST.

---

## 3. Automated Morning Renewer Daemon (`internal/kotak/renewer.go`)

### 3.1 Execution Schedule
- Runs as an in-process background goroutine in `cmd/server/main.go`.
- Scheduled at **`06:30:00 AM IST` daily**, before market open (09:00 / 09:15 AM IST).

### 3.2 Renewer Algorithm
1. Queries active `kotakneo` accounts via store.
2. For each account:
   - Resolves assigned static proxy IP.
   - Decrypts MPIN and TOTP secret with AES-GCM (`internal/crypto`).
   - Generates fresh TOTP using RFC 6238 HMAC-SHA1.
   - Dispatches Step 1 (`tradeApiLogin`) and Step 2 (`tradeApiValidate`) through the static proxy client.
   - Serializes `TradeSessionJSON` and persists via `store.SetAccountAccessToken` with `token_expires_at = NextKotakExpiry(now)`.
   - Triggers `OnTokenRefreshed` callback:
     - `AccountPool.Evict(accountID)`
     - If master: `tickerManager.RestartMaster(ctx, masterID)`
     - If follower: `followerRegistrar.RegisterFollower(ctx, followerID)`
3. Concurrency Protection:
   - Utilizes `singleflight.Group` per account to prevent redundant parallel logins.

---

## 4. Test-Driven Development (TDD) Test Matrix

| Case Type | Test Name | Scenario / Setup | Expected Outcome |
|---|---|---|---|
| **Positive** | `TestHeadlessLogin_Success` | Valid consumer key, mobile (+91), UCC, MPIN, and TOTP secret against mock HTTP server. | Successfully executes Step 1 & Step 2; returns valid `TradeSessionJSON` with dynamic `BaseURL`, `FeedURL`, `RTURL`. |
| **Positive** | `TestHeadlessLogin_StaticProxy` | Configured with static forward proxy settings. | All login requests egress strictly through the proxy transport. |
| **Positive** | `TestNextKotakExpiry_Before0600` | Current time is `03:30 AM IST`. | Expiry returned is `06:00 AM IST` today. |
| **Positive** | `TestNextKotakExpiry_After0600` | Current time is `09:30 AM IST`. | Expiry returned is `06:00 AM IST` tomorrow. |
| **Positive** | `TestNormalizeMobileNumber_TenDigits` | Input `9876543210`. | Returns `+919876543210`. |
| **Positive** | `TestNormalizeMobileNumber_FullPrefix` | Input `+919876543210`. | Returns `+919876543210`. |
| **Positive** | `TestRenewer_ScheduledMorningRun` | Clock reaches 06:30 AM IST with active accounts. | Tokens refreshed, new expiry saved, `OnTokenRefreshed` invoked. |
| **Negative** | `TestHeadlessLogin_InvalidTOTP` | Step 1 receives HTTP 401 with error message. | Fails fast, wraps `domain.ErrAuthExpired`. Client unauthenticated. |
| **Negative** | `TestHeadlessLogin_InvalidMPIN` | Step 2 receives HTTP 401 with error message. | Fails fast, wraps `domain.ErrAuthExpired`. |
| **Negative** | `TestHeadlessLogin_Gateway502` | Upstream returns HTTP 502 / HTML error page. | Returns `domain.ErrBrokerUnreachable`. |
| **Negative** | `TestNormalizeMobileNumber_Invalid` | Input `12345` or non-numeric characters. | Returns clear validation error. |
| **Negative** | `TestRenewer_PermanentAuthFailure` | Revoked credentials return 401 on renew. | Sets `auth_status = "unauthenticated"`, `status = "error"`. |
| **Edge Case** | `TestHeadlessLogin_ClockDriftRetry` | Initial TOTP fails; succeeds on `t - 30s` window retry. | Successfully authenticates without user intervention. |
| **Edge Case** | `TestHeadlessLogin_WhitespaceSanitization` | Inputs contain leading/trailing whitespace. | Strings trimmed cleanly before dispatch. |
| **Edge Case** | `TestRenewer_SingleflightDedup` | Multiple simultaneous renewal triggers for same account. | Coalesces into exactly 1 network handshake. |
