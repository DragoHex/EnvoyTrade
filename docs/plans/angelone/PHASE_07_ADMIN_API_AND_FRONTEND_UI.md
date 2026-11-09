# Phase 7: Admin HTTP API, Quotas & Frontend UI

This document specifies Phase 7 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Enable Angel One account provisioning across the Admin HTTP API and the EnvoyTrade frontend dashboard, incorporating credential encryption, quota validation, dedicated proxy compatibility, and UI form controls.

---

## 2. Admin HTTP API (`internal/httpapi`)

### 2.1 Broker Enum Whitelisting
Update `internal/httpapi/accounts.go` to accept `"angelone"`:
- In `postAccount`:
  ```go
  if broker != "kite" && broker != "testbroker" && broker != "dhan" && broker != "kotakneo" && broker != "angelone" {
      writeError(w, http.StatusBadRequest, `broker must be "kite", "dhan", "kotakneo", "angelone", or "testbroker"`)
      return
  }
  ```
- In `patchAccount`: Accept updates to Angel One account attributes, password, or TOTP secret.

### 2.2 Quota & Proxy Validation
- Calls `store.CheckAccountQuota(ctx, userID, "angelone")` to prevent plan limit overages.
- Validates assigned proxy IP matches Angel One broker routing requirements via `proxyIP.MatchesBroker("angelone")`.
- Encrypts `password` and `totpSecret` with AES-GCM (`internal/crypto`) before persistence.

---

## 3. Frontend Dashboard (`frontend/src/`)

### 3.1 Broker Branding (`BrokerLogo.tsx`)
- Implement `AngelOneLogo` SVG matching official branding.
- Add `ANGELONE_CONFIG` to `SUPPORTED_BROKERS` and `BROKER_REGISTRY`.

### 3.2 Account Provisioning Drawer (`AccountDetailDrawer.tsx`)
- **Broker Selection**: Includes `"Angel One"` in the broker selector.
- **Form Inputs**:
  - `brokerAccountId`: Label dynamically updates to `"Angel One Client Code"` (placeholder `e.g. A123456`).
  - `apiKey`: Label dynamically updates to `"SmartAPI API Key"`.
  - `password`: Password / MPIN field.
  - `totpSecret`: Base32 Authenticator Secret Key field.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestPostAccount_AngelOne_Success` | Valid Angel One master/follower creation | Verifies quota, encrypts credentials, returns HTTP 201 Created. |
| `TestPostAccount_AngelOne_QuotaExceeded` | User plan quota limit reached | Rejects with HTTP 403 / 400 quota exceeded. |
| `TestPostAccount_AngelOne_ProxyMismatch` | Incompatible static proxy IP | Rejects with HTTP 400 proxy validation error. |
| `TestPatchAccount_AngelOne_UpdateCreds` | Update trading password and TOTP secret | Re-encrypts credentials, persists to DB, triggers account refresh. |
| `TestFrontend_BrokerLogo_AngelOne` | Render `BrokerLogo` with `broker="angelone"` | Renders official SVG with proper dimensions and accessibility label. |
| `TestFrontend_AccountDrawer_AngelOneFields` | Select Angel One in drawer | Displays Client Code, API Key, Password, and TOTP Secret inputs. |
