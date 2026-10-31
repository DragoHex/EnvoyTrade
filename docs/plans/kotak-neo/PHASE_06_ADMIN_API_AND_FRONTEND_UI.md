# Phase 6: HTTP Admin API, Quotas & Frontend UI

This document specifies Phase 6 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Enable multi-tenant users to configure and manage Kotak Neo accounts via the EnvoyTrade web dashboard and Admin HTTP API. Enforce user plan quotas and proxy IP whitelist matching. Implement a dedicated UI input in `AccountDetailDrawer` with a preceding `+91` country prefix addon component and strict 10-digit mobile number validation.

---

## 2. Technical Design

### 2.1 Admin HTTP API Updates (`internal/httpapi/accounts.go`)
- **Broker Whitelist**: Whitelist `"kotakneo"` and `"kotak"` in `postAccount` and `patchAccount`.
- **Quota Enforcement**: Calls `store.CheckAccountQuota(ctx, userID, "kotakneo")` before account creation.
- **Proxy Matching**: Validates `proxyIP.MatchesBroker("kotakneo")`.
- **First-Class Mobile Number & Credential Mapping**:
  - `accounts.api_key`: Stores the normalized Registered Mobile Number (`+91XXXXXXXXXX`).
  - `accounts.api_secret`: Stores the Neo Developer Portal Consumer Key (Bearer token).
  - `postAccountRequest` and `patchAccountRequest` accept `mobileNumber *string`.
  - Normalizes via `NormalizeMobileNumber` into `+91XXXXXXXXXX` and stores in `accounts.api_key`.
  - `toAccountResponse` returns `mobileNumber: a.ApiKey` and `apiSecret: a.ApiSecret` when `a.Broker == "kotakneo"`.
- **Credential Encryption**: AES-GCM encryption on MPIN (`password`) and TOTP secret before storage.
- **Initial Portfolio Sync Trigger**: Launches background `SyncAccountPortfolio` on creation.

### 2.2 Frontend UI Component (`AccountDetailDrawer.tsx`)
When `broker() === 'kotakneo'`:
- Displays standard vertical form matching other brokers (no extraneous external links or tooltips):
  - **Kotak Neo Client ID / UCC** (`brokerAccountId`, placeholder `e.g. UCC12345`).
  - **Mobile Number with Preceding `+91` Component**:
    ```tsx
    <label>
      Mobile Number
      <div style={{ display: 'flex', 'align-items': 'center', gap: '0.5rem' }}>
        <span class="country-prefix" style={{
          padding: '0.5rem 0.75rem',
          background: 'var(--color-surface, #f5f5f7)',
          border: '1px solid var(--color-border, #e2e8f0)',
          'border-radius': '6px',
          color: 'var(--color-text, #44475b)',
          'font-weight': 600,
          'font-size': '0.9rem',
          'user-select': 'none',
        }}>+91</span>
        <input
          type="tel"
          pattern="[0-9]{10}"
          maxLength={10}
          value={kotakMobile()}
          onInput={(e) => setKotakMobile(e.currentTarget.value.replace(/\D/g, '').slice(0, 10))}
          placeholder="9876543210"
          required
          style={{ flex: 1 }}
        />
      </div>
      <Show when={kotakMobile().length > 0 && kotakMobile().length !== 10}>
        <span style={{ 'font-size': '0.75rem', color: '#e5484d', 'margin-top': '0.25rem', display: 'block' }}>
          {kotakMobile().length}/10 digits (must be exactly 10 digits)
        </span>
      </Show>
    </label>
    ```
  - **Neo Consumer Key**: Labeled "Neo Consumer Key" (`api_secret`), placeholder `Neo Consumer Key`.
  - **Kotak Neo MPIN (6-digit)**: Password input with `maxLength={6}`, placeholder `6-digit MPIN`.
  - **TOTP Secret Key**: Text input for Base32 secret key, placeholder `Base32 2FA secret (e.g. JBSWY3DPEHPK3PXP)`.
- In Edit mode: Strips `+91` on load into `kotakMobile()`; displays the 10 digits next to the `+91` prefix; prepends `+91` on save.
- Submit button disabled if `kotakMobile().length !== 10`.
- **Auth Error Banner**:
  When `account.authStatus === 'error'`, prompts:
  `"Please re-enter your Kotak Neo MPIN and TOTP secret below to reconnect."`

### 2.3 Broker Logo Component (`BrokerLogo.tsx`)
- Add `kotakneo` to `BROKER_REGISTRY` and `SUPPORTED_BROKERS` with official vector logo SVG (`KotakLogo`) and tooltip.

### 2.4 Quota and Limits Display (`AccountQuotaPopover.tsx`, `AdminUsersPage.tsx`)
- `AccountQuotaPopover.tsx`: Add `'kotakneo'` to `coreKeys` and sort `order`.
- `AdminUsersPage.tsx`: Add "Kotak Neo Allowed IPs" number input in the user limits drawer.

---

## 3. Test-Driven Development (TDD) Test Matrix

| Case Type | Test Name | Scenario / Setup | Expected Outcome |
|---|---|---|---|
| **Positive** | `TestCreateAccount_KotakNeo_Success` | POST `/api/v1/accounts` with `broker="kotakneo"`, 10-digit mobile, MPIN, TOTP, proxy IP. | Encrypts credentials, stores `+91XXXXXXXXXX`, sets status pending, triggers portfolio sync. |
| **Positive** | `TestPatchAccount_KotakNeo_UpdateMobile` | PATCH `/api/v1/accounts/{id}` with new mobile number. | Validates, normalizes, updates `accounts.api_secret`. |
| **Positive** | `TestFrontend_Drawer_ShowsPrecedingPrefix` | Render `AccountDetailDrawer` with `broker="kotakneo"`. | Renders `+91` addon component and 10-digit mobile input field. |
| **Negative** | `TestCreateAccount_KotakNeo_QuotaExceeded` | User has reached allowed account quota for `kotakneo`. | Returns HTTP 403 Forbidden. |
| **Negative** | `TestCreateAccount_KotakNeo_IncompatibleProxy` | Assigned proxy IP does not whitelist `kotakneo`. | Returns HTTP 400 Bad Request. |
| **Negative** | `TestFrontend_Drawer_ValidationWarning` | User types 9 digits into mobile number input. | Shows `9/10 digits (must be exactly 10 digits)` warning; submit is prevented. |
| **Edge Case** | `TestFrontend_Drawer_StripsCountryCodeOnLoad` | Existing account loaded with `+919876543210`. | Input box cleanly populated with `9876543210` without duplication. |
