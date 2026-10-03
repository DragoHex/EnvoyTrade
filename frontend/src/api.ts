// Thin fetch wrapper over docs/APIs/{groups,accounts,actions,auth}.md — one
// function per endpoint the Dashboard page uses, nothing speculative.

export interface User {
  id: string
  email: string
  username: string
  name: string
  role: string
  phone?: string
  address?: string
  gstNumber?: string
}

export interface AuthResponse {
  user: User
  token?: string
}

export interface RegisterRequest {
  email: string
  username: string
  password: string
  name?: string
}

export interface LoginRequest {
  email: string
  password: string
}

export interface UpdateProfileRequest {
  email: string
  username: string
  name?: string
  phone?: string
  address?: string
  gstNumber?: string
}

export interface UpdatePasswordRequest {
  oldPassword: string
  newPassword: string
}

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
  netQty?: number
  openPositionsCount?: number
  closedPositionsCount?: number
  openOrdersCount?: number
  totalMtm?: number | string
  availableCash?: number | string
  availableMargin?: number | string
}

export interface GroupDetail {
  id: string
  name: string
  masterId: string
  masterAccountId: string
  masterName?: string
  masterActive: boolean
  masterNetQty?: number
  masterOpenPositionsCount?: number
  masterClosedPositionsCount?: number
  masterOpenOrdersCount?: number
  masterTotalMtm?: number | string
  masterAvailableCash?: number | string
  masterAvailableMargin?: number | string
  followers: GroupFollower[]
}

export type ActionType = 'rebalance' | 'square_off' | 'exit_open_orders' | 'sync_positions'

export interface Account {
  id: string
  name: string
  role: 'master' | 'follower'
  broker: string
  brokerAccountId: string
  apiKey?: string | null
  apiSecret?: string | null
  ip?: string | null
  authStatus?: 'unauthenticated' | 'authenticated' | 'error' | null
  authError?: string | null
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
  password?: string
  totpSecret?: string
  ip?: string
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

let onUnauthorizedCallback: (() => void) | null = null

export function setOnUnauthorized(cb: (() => void) | null) {
  onUnauthorizedCallback = cb
}

function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const options: RequestInit = {
    ...init,
    credentials: 'include',
  }
  return fetch(input, options).then((res) => {
    if (res.status === 401 && onUnauthorizedCallback) {
      onUnauthorizedCallback()
    }
    return res
  })
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `request failed: ${res.status}`)
  }
  return res.json()
}

// Auth endpoints
export function register(body: RegisterRequest): Promise<AuthResponse> {
  return apiFetch(`${BASE}/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function login(body: LoginRequest): Promise<AuthResponse> {
  return apiFetch(`${BASE}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function logout(): Promise<{ status: string }> {
  return apiFetch(`${BASE}/auth/logout`, {
    method: 'POST',
  }).then((r) => json(r))
}

export function getMe(): Promise<AuthResponse> {
  return apiFetch(`${BASE}/auth/me`).then((r) => json(r))
}

export function updateProfile(body: UpdateProfileRequest): Promise<{ user: User }> {
  return apiFetch(`${BASE}/user/profile`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function updatePassword(body: UpdatePasswordRequest): Promise<{ status: string }> {
  return apiFetch(`${BASE}/user/password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

// Group & Account endpoints
export function getGroups(): Promise<GroupSummary[]> {
  return apiFetch(`${BASE}/groups`).then((r) => json(r))
}

export function createGroup(body: CreateGroupRequest): Promise<{ id: string; name: string; masterId: string }> {
  return apiFetch(`${BASE}/groups`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function patchGroup(id: string, body: PatchGroupRequest): Promise<{ status: string }> {
  return apiFetch(`${BASE}/groups/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function deleteGroup(id: string): Promise<void> {
  return apiFetch(`${BASE}/groups/${id}`, { method: 'DELETE' }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function getGroupDetail(id: string): Promise<GroupDetail> {
  return apiFetch(`${BASE}/groups/${id}`).then((r) => json(r))
}

export function patchAccount(
  id: string,
  body:
    | { name: string }
    | { enabled: boolean }
    | { active: boolean }
    | { capitalRatio?: string; maxQtyPerOrder?: number }
    | { status: string }
    | { ip?: string }
    | { apiKey?: string; apiSecret?: string }
    | { password?: string; totpSecret?: string },
): Promise<Record<string, unknown>> {
  return apiFetch(`${BASE}/accounts/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function getAccounts(ids?: string[]): Promise<Account[]> {
  const query = ids && ids.length > 0 ? `?ids=${ids.join(',')}` : ''
  return apiFetch(`${BASE}/accounts${query}`).then((r) => json(r))
}

export function getAccount(id: string): Promise<Account> {
  return getAccounts([id]).then((accounts) => {
    if (!accounts[0]) throw new Error('account not found')
    return accounts[0]
  })
}

export function createAccount(body: CreateAccountRequest): Promise<Account> {
  return apiFetch(`${BASE}/accounts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => json(r))
}

export function deleteAccount(id: string): Promise<void> {
  return apiFetch(`${BASE}/accounts/${id}`, { method: 'DELETE' }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function removeAccountFromGroup(id: string): Promise<void> {
  return apiFetch(`${BASE}/accounts/${id}/group`, { method: 'DELETE' }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function addAccountToGroup(
  groupId: string,
  body: { accountId: string; capitalRatio: string; maxQtyPerOrder?: number },
): Promise<void> {
  return apiFetch(`${BASE}/groups/${groupId}/followers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then((r) => {
    if (!r.ok) return json(r)
  })
}

export function postAction(id: string, type: ActionType): Promise<{ type: string; status: string }> {
  return apiFetch(`${BASE}/accounts/${id}/actions`, {
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
  availableCash?: number | string
  availableMargin?: number | string
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
  pnl?: number | string
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
  return apiFetch(`${BASE}/accounts/${accountId}/orders${qs}`).then((r) => json(r))
}

export interface ProxyIP {
  ipAddress: string
  ipType: 'ipv4' | 'ipv6'
  host: string
  port: number
  validFrom: string
  validUntil: string
  plan: string
  isAssigned?: boolean
  assignedAccountId?: string | null
  assignedAccountName?: string | null
}

export interface AvailableProxyIPsResponse {
  ipv4: ProxyIP[]
  ipv6: ProxyIP[]
}

export function fetchProxyIPs(): Promise<ProxyIP[]> {
  return apiFetch(`${BASE}/proxy-ips`).then((r) => json(r))
}

export function fetchAvailableProxyIPs(accountId?: string): Promise<AvailableProxyIPsResponse> {
  const qs = accountId ? `?accountId=${encodeURIComponent(accountId)}` : ''
  return apiFetch(`${BASE}/proxy-ips/available${qs}`).then((r) => json(r))
}

