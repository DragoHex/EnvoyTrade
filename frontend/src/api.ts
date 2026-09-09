// Thin fetch wrapper over docs/APIs/{groups,accounts,actions}.md — one
// function per endpoint the Dashboard page uses, nothing speculative.

export interface GroupSummary {
  masterId: string
  masterAccountId: string
  broker: string
  followerCount: number
  status: 'ok' | 'error'
}

export interface GroupFollower {
  accountId: string
  brokerAccountId: string
  enabled: boolean
  status: 'ok' | 'error'
  // netQty/positions/openOrders/totalMtm/availableCash/availableMargin are
  // omitted by the backend until broker wiring exists (docs/APIs/groups.md
  // "Current implementation" note) — rendered as "—" by AccountTable.
}

export interface GroupDetail {
  masterId: string
  masterAccountId: string
  masterActive: boolean
  followers: GroupFollower[]
}

export type ActionType = 'rebalance' | 'square_off' | 'exit_open_orders'

export interface Account {
  id: string
  role: 'master' | 'follower'
  broker: string
  brokerAccountId: string
  masterId: string | null
  capitalRatio: string | null
  maxQtyPerOrder: number | null
  enabled: boolean
  active: boolean
  status: 'ok' | 'error'
}

export interface CreateAccountRequest {
  role: 'master' | 'follower'
  broker: string
  brokerAccountId: string
  apiKey: string
  apiSecret: string
  capitalRatio?: string
  maxQtyPerOrder?: number
  masterId?: string
}

const BASE = '/api/v1'

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `request failed: ${res.status}`)
  }
  return res.json()
}

export function getGroups(): Promise<GroupSummary[]> {
  return fetch(`${BASE}/groups`).then((r) => json(r))
}

export function getGroupDetail(masterId: string): Promise<GroupDetail> {
  return fetch(`${BASE}/groups/${masterId}`).then((r) => json(r))
}

export function patchAccount(
  id: string,
  body:
    | { enabled: boolean }
    | { active: boolean }
    | { capitalRatio?: string; maxQtyPerOrder?: number }
    | { status: string },
): Promise<Record<string, unknown>> {
  return fetch(`${BASE}/accounts/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function getAccounts(ids?: string[]): Promise<Account[]> {
  const query = ids && ids.length > 0 ? `?ids=${ids.join(',')}` : ''
  return fetch(`${BASE}/accounts${query}`).then((r) => json(r))
}

export function createAccount(body: CreateAccountRequest): Promise<Account> {
  return fetch(`${BASE}/accounts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function deleteAccount(id: string): Promise<void> {
  return fetch(`${BASE}/accounts/${id}`, { method: 'DELETE' }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function removeAccountFromGroup(id: string): Promise<void> {
  return fetch(`${BASE}/accounts/${id}/group`, { method: 'DELETE' }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function addAccountToGroup(
  masterId: string,
  body: { accountId: string; capitalRatio: string; maxQtyPerOrder?: number },
): Promise<void> {
  return fetch(`${BASE}/groups/${masterId}/followers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function postAction(id: string, type: ActionType): Promise<{ type: string; status: string }> {
  return fetch(`${BASE}/accounts/${id}/actions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type }),
  }).then((r) => json(r))
}
