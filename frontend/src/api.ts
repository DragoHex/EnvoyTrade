// Thin fetch wrapper over docs/APIs/{groups,accounts,actions}.md — one
// function per endpoint the Dashboard page uses, nothing speculative.

export interface GroupSummary {
  id: string
  name: string
  masterId: string
  masterAccountId: string
  masterName?: string
  broker: string
  followerCount: number
  status: 'ok' | 'error'
  active?: boolean
}

export interface GroupFollower {
  accountId: string
  name: string
  brokerAccountId: string
  enabled: boolean
  status: 'ok' | 'error'
  // netQty/positions/openOrders/totalMtm/availableCash/availableMargin are
  // omitted by the backend until broker wiring exists (docs/APIs/groups.md
  // "Current implementation" note) — rendered as "—" by AccountTable.
}

export interface GroupDetail {
  id: string
  name: string
  masterId: string
  masterAccountId: string
  masterName?: string
  masterActive: boolean
  followers: GroupFollower[]
}

export type ActionType = 'rebalance' | 'square_off' | 'exit_open_orders'

export interface Account {
  id: string
  name: string
  role: 'master' | 'follower'
  broker: string
  brokerAccountId: string
  groupId?: string | null
  groupName?: string | null
  masterId: string | null
  capitalRatio: string | null
  maxQtyPerOrder: number | null
  enabled: boolean
  active: boolean
  status: 'ok' | 'error'
}

export interface CreateAccountRequest {
  name: string
  role: 'master' | 'follower'
  broker: string
  brokerAccountId: string
  apiKey: string
  apiSecret: string
  capitalRatio?: string
  maxQtyPerOrder?: number
  groupId?: string
  masterId?: string
}

export interface CreateGroupRequest {
  name: string
  masterId: string
}

export interface PatchGroupRequest {
  name?: string
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

export function createGroup(body: CreateGroupRequest): Promise<{ id: string; name: string; masterId: string }> {
  return fetch(`${BASE}/groups`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function patchGroup(id: string, body: PatchGroupRequest): Promise<{ status: string }> {
  return fetch(`${BASE}/groups/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function deleteGroup(id: string): Promise<void> {
  return fetch(`${BASE}/groups/${id}`, { method: 'DELETE' }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function getGroupDetail(id: string): Promise<GroupDetail> {
  return fetch(`${BASE}/groups/${id}`).then((r) => json(r))
}

export function patchAccount(
  id: string,
  body:
    | { name: string }
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

export function getAccount(id: string): Promise<Account> {
  return getAccounts([id]).then((accounts) => {
    if (!accounts[0]) throw new Error('account not found')
    return accounts[0]
  })
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
  groupId: string,
  body: { accountId: string; capitalRatio: string; maxQtyPerOrder?: number },
): Promise<void> {
  return fetch(`${BASE}/groups/${groupId}/followers`, {
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

export interface AccountSummaryMetrics {
  netQty: number
  openPositionsCount?: number
  openCount?: number
  closedPositionsCount?: number
  closedCount?: number
  pendingOrdersCount?: number
  pendingMetric?: number
  totalMtm: number | string
  realizedPnl: number | string
  accountValue: number | string
  status: string
}

export interface TabCounts {
  openPositions: number
  closedPositions: number
  holdings: number
  openOrders: number
  closedOrders: number
  rejectedOrders: number
}

export interface PaginationInfo {
  tab: string
  page: number
  limit: number
  totalCount: number
  totalPages: number
}

export interface PositionItem {
  product: string
  instrument: string
  qty: number
  avgPrice: string
  ltp: number | string
  mtm: number | string
  action?: string
}

export interface HoldingItem {
  instrument: string
  sellableQuantity: number
  buyAveragePrice: number | string
  ltp: number | string
  pnl: number | string
  action?: string
}

export interface OrderDetailItem {
  id?: string
  product?: string
  time?: string
  instrument: string
  quantity: number
  price?: number | string
  triggerPrice?: number | string
  limitPrice?: number | string
  type: string
  status?: string
  reason?: string
  action?: string
}

export interface AccountOrdersResponse {
  summary: AccountSummaryMetrics
  counts?: TabCounts
  pagination?: PaginationInfo
  openPositions: PositionItem[]
  closedPositions: PositionItem[]
  holdings: HoldingItem[]
  openOrders: OrderDetailItem[]
  closedOrders: OrderDetailItem[]
  rejectedOrders: OrderDetailItem[]
}

export function getAccountOrders(
  accountId: string,
  tab?: string,
  page?: number,
  limit?: number,
): Promise<AccountOrdersResponse> {
  const params = new URLSearchParams()
  if (tab) params.set('tab', tab)
  if (page !== undefined && page > 0) params.set('page', String(page))
  if (limit !== undefined && limit > 0) params.set('limit', String(limit))
  const qs = params.toString() ? `?${params.toString()}` : ''
  return fetch(`${BASE}/accounts/${accountId}/orders${qs}`).then((r) => json(r))
}
