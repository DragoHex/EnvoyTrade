# Phase 1: Perpetual Authentication & Session Lifecycle

This document specifies Phase 1 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Establish perpetual, headless authentication and automated session maintenance for Angel One accounts. Once a user configures their Angel One credentials (Client Code, API Key, Password/MPIN, TOTP Secret) during account onboarding, EnvoyTrade maintains uninterrupted authenticated sessions autonomously across daily broker token expirations and server restarts.

---

## 2. Authentication Protocol & Credentials Model

### 2.1 Credential Mapping
No database migration is required. Credentials map directly to [`Account`](file:///Users/msp/MSP/Projects/EnvoyTrade/internal/domain/types.go):
- `broker`: `"angelone"`
- `broker_user_id`: Angel One Client Code (e.g. `A123456`)
- `api_key`: SmartAPI App API Key
- `api_secret`: SmartAPI App Secret (used for postback verification)
- `encrypted_password`: AES-GCM encrypted Password / MPIN
- `encrypted_totp_secret`: AES-GCM encrypted Base32 TOTP secret key
- `access_token`: JSON-serialized [`SessionJSON`](#22-session-storage-sessionjson)

### 2.2 Session Storage (`SessionJSON`)
Angel One returns a JWT token, Refresh token, and WebSocket Feed token upon authentication:
```go
type SessionJSON struct {
    JWTToken     string `json:"jwtToken"`
    RefreshToken string `json:"refreshToken"`
    FeedToken    string `json:"feedToken"`
    ClientCode   string `json:"clientCode"`
}
```

### 2.3 Headless Login Flow
`HeadlessLogin(ctx context.Context, creds HeadlessLoginCredentials) (*SessionJSON, error)`:
1. Validates non-empty `APIKey`, `ClientCode`, `Password`, and `TOTPSecret`.
2. Computes 6-digit TOTP code using `kite.GenerateTOTP(creds.TOTPSecret, time.Now())`.
3. Invokes `client.GenerateSession(ctx, creds.ClientCode, creds.Password, totpCode)`.
4. If authentication fails with invalid TOTP, retries with `time.Now().Add(-30*time.Second)` to handle clock drift.
5. On success, constructs and returns `SessionJSON`.

### 2.4 Token Renewal Flow
`RenewSession(ctx context.Context, apiKey string, sess *SessionJSON, proxyURL string) (*SessionJSON, error)`:
1. Calls `client.RenewAccessToken(ctx, sess.RefreshToken)`.
2. Updates `JWTToken`, `RefreshToken`, and `FeedToken`.
3. If `RenewAccessToken` fails with `401 Unauthorized` or token expired, triggers `HeadlessLogin` fallback using stored encrypted password and TOTP secret.

### 2.5 Expiration & Schedule Boundaries
- **Token Expiry**: Angel One sessions reset daily at `06:00:00 AM IST`.
  - `NextAngelExpiry(now time.Time) time.Time`: Computes the upcoming `06:00:00 AM IST` timestamp.
- **Renewal Daemon**:
  - `NextRenewerRunTime(now time.Time) time.Time`: Fires at `06:00:00 AM IST` daily (prior to market open).

---

## 3. Implementation Details

### 3.1 New Files
- `internal/angelone/auth.go`: `HeadlessLogin`, `RenewSession`, `SessionJSON` serialization/deserialization, `NextAngelExpiry`, error translators.
- `internal/angelone/renewer.go`: Background renewer daemon (`Renewer`) querying all active Angel One accounts and renewing sessions at `06:00 AM IST`.
- `internal/angelone/auth_test.go`: Pure TDD unit test suite.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestNormalizeClientCode_Valid` | Uppercase, whitespace trimming | Canonical client code (e.g. `"A123456"`). |
| `TestNormalizeClientCode_Empty` | Empty or whitespace client code | Returns `ErrInvalidInput`. |
| `TestHeadlessLogin_Success` | Valid credentials and TOTP | Returns valid `SessionJSON` with JWT, Refresh, and Feed tokens. |
| `TestHeadlessLogin_ClockDrift` | TOTP valid only at $t - 30s$ | First attempt fails, second attempt succeeds with drift code. |
| `TestHeadlessLogin_InvalidPassword` | Incorrect trading password | Fails with wrapped `domain.ErrAuthExpired`. |
| `TestHeadlessLogin_BadTOTPSecret` | Corrupted Base32 TOTP secret | Fails immediately during TOTP computation. |
| `TestHeadlessLogin_ProxyRouting` | Egress through forward proxy | Client sends HTTP request through designated proxy URL. |
| `TestRenewSession_Success` | Valid active refresh token | Returns renewed JWT access token. |
| `TestRenewSession_ExpiredFallback` | Expired refresh token | Automatically executes `HeadlessLogin` fallback. |
| `TestNextAngelExpiry_BeforeBoundary` | Evaluated at 04:00 AM IST | Returns 06:00 AM IST today. |
| `TestNextAngelExpiry_AfterBoundary` | Evaluated at 09:15 AM IST | Returns 06:00 AM IST tomorrow. |
| `TestSessionSerialization_Roundtrip` | JSON marshal & unmarshal | Clean byte-for-byte fidelity without data loss. |
