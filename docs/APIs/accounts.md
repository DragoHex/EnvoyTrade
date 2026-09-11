# Accounts API

Covers the Accounts page (`AccountsTable` + `AccountDetailDrawer`) **and** the Dashboard's
`CopyToggle`/Stop-Copy control — both go through the same account resource rather than a separate
follow-link endpoint.

Two independent on/off concepts exist, at different scope:
- `enabled` (`follow_links.enabled`) — per-follower copy-trading switch. Dashboard follower row's
  toggle.
- `active` (`accounts.active`) — master-level global switch. Dashboard master row's Stop/Start
  button; when `false` the fan-out engine skips that master's fills entirely (no follower is
  affected individually, nothing dispatches for anyone under that master).

## `GET /api/v1/accounts`

List every account (master + follower), for the Accounts page's flat list. An optional
`?ids=uuid,uuid` filters to a specific set — how the group-management page
(`/accounts/groups/{masterId}`) hydrates full fields for a group's members, whose IDs it already
has from `GET /groups/{masterId}`.

**Response** `200`:
```json
[
  {
    "id": "uuid",
    "role": "master",             // "master" | "follower"
    "broker": "kite",              // dropdown value; only "kite" supported today
    "brokerAccountId": "ZX1234",
    "masterId": null,              // follower's group; null for master or a detached follower
    "capitalRatio": null,         // null for master; decimal string for follower
    "maxQtyPerOrder": null,       // null for master; int for follower
    "enabled": true,              // follow-link enabled state; always true for master
    "active": true,                // accounts.active — master Stop/Start state
    "status": "ok"                // "ok" | "error"
  }
]
```
`400` if `ids` contains a malformed UUID.

## `POST /api/v1/accounts`

Create a master or follower account.

**Request**:
```json
{
  "role": "follower",
  "broker": "kite",              // only "kite" accepted for now; reserved for future brokers
  "brokerAccountId": "ZY5678",
  "apiKey": "...",
  "apiSecret": "...",
  "capitalRatio": "0.5",         // required if role=follower
  "maxQtyPerOrder": 100,         // required if role=follower
  "masterId": "uuid"             // required if role=follower — the group to attach to
}
```

**Response** `201` — same shape as one item in `GET /accounts`. `409` if `brokerAccountId` already exists.
`400` if `broker` isn't `"kite"`, `role` isn't `master`/`follower`, or (for a follower)
`capitalRatio`/`maxQtyPerOrder`/`masterId` is missing or `masterId` doesn't resolve to a master
account.

**Data source note**: `apiKey` has no backing column yet (`accounts` table currently stores only
`api_secret`, used for postback checksum verification) — request/response shape is final, storing
`apiKey` is a follow-up backend change, not built now.

## `PATCH /api/v1/accounts/{id}`

Update an existing account. Partial body — only send fields being changed.

**Request** (follower example — toggling copy off, same call the Dashboard's `CopyToggle`/Stop-Copy
button makes):
```json
{ "enabled": false }
```
**Response** `200`: `{ "enabled": false }`.

**Request** (master example — the Dashboard master row's Stop/Start button):
```json
{ "active": false }
```
**Response** `200`: `{ "active": false }`.

**Request** (Accounts page edit example — `capitalRatio`/`maxQtyPerOrder` may be sent individually
or together; omitting one leaves it unchanged):
```json
{ "capitalRatio": "0.4", "maxQtyPerOrder": 50 }
```
**Response** `200`: echoes back just the field(s) sent, e.g. `{ "capitalRatio": "0.4", "maxQtyPerOrder": 50 }`.

**Request** (status, either role):
```json
{ "status": "ok" }
```
**Response** `200`: `{ "status": "ok" }`.

`404` if `id` doesn't exist. `400` if trying to set `capitalRatio`/`maxQtyPerOrder` on a master
account (those fields only apply to follow-links), or if no recognized field is sent.

## `DELETE /api/v1/accounts/{id}`

Delete an account — the Accounts page's "Delete" action. `204` on success. `409` if the account is
still referenced (attached to a group via a follow_link, or has fill/order history) — the UI
surfaces this as "remove it from its group first" (see the next endpoint). `404` if `id` doesn't
exist.

## `DELETE /api/v1/accounts/{id}/group`

Detach a follower from its group (deletes its follow_link) without deleting the account —
available from both the Accounts page (per-row action) and the group-management page. `204` on
success. `404` if the account has no follow_link to remove.
