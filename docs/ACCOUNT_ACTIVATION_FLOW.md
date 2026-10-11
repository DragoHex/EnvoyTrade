# Account Activation & Copy-Trading Controls

This guide explains how account activation and copy-trading controls work in EnvoyTrade from a trader's perspective.

---

## Two Levels of Control

EnvoyTrade provides two distinct controls for managing your accounts:

1. **Account Active Switch (Accounts Page)**
   - Controls whether EnvoyTrade connects to the broker for this account.
   - When **Active**: EnvoyTrade manages daily broker logins and continuously syncs holdings, open positions, and margins.
   - When **Inactive**: EnvoyTrade disconnects the account. No broker logins, no portfolio sync, and no trading activity.

2. **Copy Switch (Dashboard & Group Management)**
   - Controls whether trades placed on a Master account should be copied to this Follower account.
   - When **Enabled**: Orders placed on the Master account are replicated to this Follower based on its clone ratio and lot size.
   - When **Disabled**: The Follower ignores Master trades, but you can still view its positions and margins.

---

## How the Switches Interact

```
Accounts Page: Deactivate Account
  └── Turns account Inactive AND immediately turns Copy OFF on the Dashboard.

Accounts Page: Activate Account (Safety First)
  └── Resumes daily login and portfolio sync, but Copy stays OFF until you turn it ON.

Dashboard: Turn Copy ON
  └── Starts copying trades AND automatically activates the account if it was inactive.

Dashboard: Turn Copy OFF
  └── Pauses copy trading. The account stays active so you can still track positions and PnL.
```

---

## Scenarios & Behaviors

### 1. Deactivating an Account from the Accounts Page
* **Action:** You toggle an account's switch to **Inactive** on the Accounts page.
* **What Happens:**
  * EnvoyTrade stops daily logins and portfolio sync for this account.
  * Copy-trading is immediately disabled across all groups.
  * On the Dashboard, the account's copy toggle flips to **OFF**.
* **Result:** No master trades will be executed on this account under any circumstances.

### 2. Activating an Account from the Accounts Page (Safety First)
* **Action:** You toggle an account's switch to **Active** on the Accounts page.
* **What Happens:**
  * EnvoyTrade resumes daily login and refreshes holdings, positions, and margins.
  * **Copy-trading remains OFF.**
* **Result:** You can safely inspect your balance, margin, and current positions on the Dashboard before deciding to turn trade copying on.

### 3. Turning Copy ON from the Dashboard
* **Action:** You toggle a follower's copy switch to **ON** on the Dashboard or Group page.
* **What Happens:**
  * Trade replication is enabled for this follower.
  * If the account was inactive, EnvoyTrade automatically activates the account so that broker sessions and order placement are ready.
* **Result:** Future master fills will be copied to this follower.

### 4. Turning Copy OFF from the Dashboard
* **Action:** You toggle a follower's copy switch to **OFF** on the Dashboard.
* **What Happens:**
  * Copy-trading is paused. Incoming master trades are ignored.
  * The account remains **Active** on the Accounts page.
* **Result:** You can pause copy-trading during volatile events or strategy changes without losing visibility into the account's live portfolio and margins.
