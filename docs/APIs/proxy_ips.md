# Proxy IPs API

Covers the proxy IP management and availability endpoints used by the Account onboarding / edit drawer (`AccountDetailDrawer`) to allocate dedicated static IPs for Kite Connect broker egress.

## Design Rules
1. **Strict 1:1 Account-to-IP Assignment**:
   - An IP address can be assigned to at most one account at any given time.
   - Enforced in the database via partial unique index `idx_accounts_ip_address_unique` on `accounts(ip_address) WHERE ip_address IS NOT NULL AND ip_address != ''`.
2. **Follower vs Master Constraints**:
   - **Follower accounts**: Static IP assignment is mandatory. Must choose between `IPv4` and `IPv6`.
   - **Master accounts**: Static IP assignment is optional (`NA` / direct, `IPv4`, or `IPv6`).
3. **Automatic Allocation**:
   - Selecting an IP type (`IPv4` or `IPv6`) assigns an available, unassigned IP of that type.
   - When an account is deleted or its IP changed, its previous IP is immediately returned to the unassigned pool.

---

## `GET /api/v1/proxy-ips`

Lists all proxy IPs registered in the system along with their assignment status.

**Auth**: Session cookie or Bearer token required.

**Response** `200`:
```json
[
  {
    "ipAddress": "148.113.41.41",
    "ipType": "ipv4",
    "host": "dc46-mum-01.algoip.in",
    "port": 443,
    "validFrom": "2026-09-29T14:05:15Z",
    "validUntil": "2026-12-29T14:05:15Z",
    "plan": "QUARTERLY",
    "isAssigned": true,
    "assignedAccountId": "a5183e89-6cb2-4d32-91a2-6dc525570185",
    "assignedAccountName": "Amit Verma"
  },
  {
    "ipAddress": "148.113.41.42",
    "ipType": "ipv4",
    "host": "dc46-mum-01.algoip.in",
    "port": 443,
    "validFrom": "2026-09-29T14:05:15Z",
    "validUntil": "2026-12-29T14:05:15Z",
    "plan": "QUARTERLY",
    "isAssigned": false,
    "assignedAccountId": null,
    "assignedAccountName": null
  },
  {
    "ipAddress": "2402:1f00:8302:91e6:6d08:9249:eca8:8252",
    "ipType": "ipv6",
    "host": "dc46-mum-01.algoip.in",
    "port": 443,
    "validFrom": "2026-09-29T14:05:15Z",
    "validUntil": "2026-12-29T14:05:15Z",
    "plan": "QUARTERLY",
    "isAssigned": false,
    "assignedAccountId": null,
    "assignedAccountName": null
  }
]
```

---

## `GET /api/v1/proxy-ips/available`

Returns currently unassigned proxy IPs grouped by IP type (`ipv4` and `ipv6`).

### Query Parameters
- `accountId` *(optional, UUID)*: If supplied (e.g. during account edit), the account's own currently assigned IP will be included in the results so it is not reported as unavailable to itself.

**Response** `200`:
```json
{
  "ipv4": [
    {
      "ipAddress": "148.113.41.42",
      "ipType": "ipv4",
      "host": "dc46-mum-01.algoip.in",
      "port": 443,
      "validFrom": "2026-09-29T14:05:15Z",
      "validUntil": "2026-12-29T14:05:15Z",
      "plan": "QUARTERLY"
    }
  ],
  "ipv6": [
    {
      "ipAddress": "2402:1f00:8302:91e6:6d08:9249:eca8:8252",
      "ipType": "ipv6",
      "host": "dc46-mum-01.algoip.in",
      "port": 443,
      "validFrom": "2026-09-29T14:05:15Z",
      "validUntil": "2026-12-29T14:05:15Z",
      "plan": "QUARTERLY"
    },
    {
      "ipAddress": "2402:1f00:8302:9102:794b:a90f:c2bb:9293",
      "ipType": "ipv6",
      "host": "dc46-mum-01.algoip.in",
      "port": 443,
      "validFrom": "2026-09-29T14:05:15Z",
      "validUntil": "2026-12-29T14:05:15Z",
      "plan": "QUARTERLY"
    }
  ]
}
```

---

## Account Integration (`POST /api/v1/accounts`, `PATCH /api/v1/accounts/{id}`)

Proxy IP assignment is managed directly through the existing `POST /api/v1/accounts` and `PATCH /api/v1/accounts/{id}` endpoints via the `ip` field:

### Validation Rules
1. **Follower**: `ip` is required. The IP must exist in `proxy_ips` and must not be assigned to any other account. If all IPs of the chosen type are exhausted, `400 Bad Request` or `409 Conflict` is returned.
2. **Master**: `ip` is optional. Empty string (`""`) denotes `NA` (no proxy). If specified, must exist in `proxy_ips` and not be assigned to another account.
3. **Collision Protection**: If an attempt is made to assign an IP already bound to an active account, returns `409 Conflict`:
   ```json
   {"error": "IP address 148.113.41.41 is already assigned to another account"}
   ```
