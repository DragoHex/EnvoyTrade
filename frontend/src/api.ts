// Thin fetch wrapper over docs/APIs/{groups,accounts,actions,auth}.md — one
// function per endpoint the Dashboard page uses, nothing speculative.

export interface User {
  id: string
  email: string
  username: string
  name: string
  role: string
  active?: boolean
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
  hasImbalance?: boolean
}

export interface MarginBreakdown {
  net?: number | string
  cash?: number | string
  openingCash?: number | string
  collateral?: number | string
  utilised?: number | string
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
  mtmBreakdown?: Record<string, number | string>
  marginBreakdown?: MarginBreakdown
}

export interface GroupDetail {
  id: string
  name: string
  masterId: string
  masterAccountId: string
  masterName?: string
  masterActive: boolean
  hasImbalance?: boolean
  masterNetQty?: number
  masterOpenPositionsCount?: number
  masterClosedPositionsCount?: number
  masterOpenOrdersCount?: number
  masterTotalMtm?: number | string
  masterAvailableCash?: number | string
  masterAvailableMargin?: number | string
  masterMtmBreakdown?: Record<string, number | string>
  masterMarginBreakdown?: MarginBreakdown
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
  cloneFactor: string | null
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
  apiKey?: string
  apiSecret?: string
  accessToken?: string
  password?: string
  totpSecret?: string
  ip?: string
  cloneFactor?: string
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
  const urlStr = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
  return fetch(input, options).then(async (res) => {
    if (res.status === 401 && onUnauthorizedCallback) {
      // Upstream broker token errors on trading/rebalance operations must not log the trader out
      if (!urlStr.includes('/positions/rebalance') && !urlStr.includes('/positions/square-off') && !urlStr.includes('/actions')) {
        try {
          const clone = res.clone()
          const data = await clone.json().catch(() => null)
          const msg = data && typeof data === 'object' ? String(data.error || data.message || '') : ''
          if (!msg.toLowerCase().includes('broker')) {
            onUnauthorizedCallback()
          }
        } catch {
          onUnauthorizedCallback()
        }
      }
    }
    return res
  })
}

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    const errMsg =
      body && typeof body === 'object'
        ? (body.error || body.message || body.detail || res.statusText)
        : res.statusText
    throw new Error(errMsg || `request failed: ${res.status}`)
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
    | { cloneFactor?: string; maxQtyPerOrder?: number | null; groupId?: string | null }
    | { status: string }
    | { ip?: string }
    | { apiKey?: string; apiSecret?: string }
    | { password?: string; totpSecret?: string }
    | Record<string, unknown>,
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
  const payload = { ...body }
  if (payload.role === 'follower') {
    payload.cloneFactor = payload.cloneFactor ?? '1'
  }
  return apiFetch(`${BASE}/accounts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
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
  body: { accountId: string; cloneFactor: string; maxQtyPerOrder?: number },
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

export interface SquareOffOrder {
  orderId: string
  symbol: string
  side: string
  quantity: number
  status: string
  error?: string
}

export interface AccountSquareOffResult {
  accountId: string
  status: 'completed' | 'partial' | 'failed'
  orders: SquareOffOrder[]
  error?: string
}

export interface GroupSquareOffResult {
  groupId: string
  status: 'completed' | 'partial' | 'failed'
  account: AccountSquareOffResult
  followers: AccountSquareOffResult[]
  error?: string
}

export function squareOffGroup(
  groupId: string,
  body?: { symbols?: string[] }
): Promise<GroupSquareOffResult> {
  return apiFetch(`${BASE}/groups/${groupId}/positions/square-off`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  }).then((r) => json(r))
}

export function squareOffAccount(
  accountId: string,
  body?: { symbols?: string[] }
): Promise<AccountSquareOffResult> {
  return apiFetch(`${BASE}/accounts/${accountId}/positions/square-off`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  }).then((r) => json(r))
}

export interface SymbolDrift {
  exchange: string
  tradingsymbol: string
  product: string
  lot_size: number
  master_qty: number
  target_qty: number
  follower_qty: number
  drift_qty: number
  action: 'BUY' | 'SELL'
}

export interface FollowerDrift {
  account_id: string
  account_name: string
  broker_account_id: string
  enabled: boolean
  clone_factor: string
  symbols: SymbolDrift[]
}

export interface GroupRebalanceDiff {
  group_id: string
  master_id: string
  followers_evaluated: number
  followers_with_drift: number
  drifts: FollowerDrift[]
}

export interface RebalanceOrder {
  account_id: string
  role: string
  broker_order_id: string
  exchange: string
  tradingsymbol: string
  product: string
  side: string
  quantity: number
  status: string
}

export interface RebalanceResult {
  action: string
  status: 'completed' | 'partial' | 'empty'
  group_id?: string
  account_id?: string
  followers_affected: number
  cancelled_orders: number
  orders_placed: number
  orders: RebalanceOrder[]
  errors?: string[]
}

export function getGroupRebalanceDiff(groupId: string): Promise<GroupRebalanceDiff> {
  return apiFetch(`${BASE}/groups/${groupId}/positions/rebalance/diff`).then((r) => json(r))
}

export function rebalanceGroup(
  groupId: string,
  body?: { follower_ids?: string[] }
): Promise<RebalanceResult> {
  return apiFetch(`${BASE}/groups/${groupId}/positions/rebalance`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  }).then((r) => json(r))
}

export function getAccountRebalanceDiff(accountId: string): Promise<FollowerDrift> {
  return apiFetch(`${BASE}/accounts/${accountId}/positions/rebalance/diff`).then((r) => json(r))
}

export function rebalanceAccount(accountId: string): Promise<RebalanceResult> {
  return apiFetch(`${BASE}/accounts/${accountId}/positions/rebalance`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
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
  mtmBreakdown?: Record<string, number | string>
  marginBreakdown?: MarginBreakdown
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
  accountId?: string
  role?: string
  brokerAccountId?: string
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
  whitelistedBrokers?: string
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

export function fetchAvailableProxyIPs(accountId?: string, broker?: string): Promise<AvailableProxyIPsResponse> {
  const params = new URLSearchParams()
  if (accountId) params.set('accountId', accountId)
  if (broker) params.set('broker', broker)
  const qs = params.toString() ? `?${params.toString()}` : ''
  return apiFetch(`${BASE}/proxy-ips/available${qs}`).then((r) => json(r))
}

// Admin Portal API types and client functions

export interface KillSwitchStatus {
  engaged: boolean
  engagedAt?: string
  engagedBy?: string
  reason?: string
}

export interface TickerStatus {
  masterId: string
  active: boolean
}

export interface QueueStatus {
  masterFillDepth: number
  masterFillCap: number
  orderUpdateDepth: number
  orderUpdateCap: number
}

export interface DBPoolStatus {
  acquiredConns: number
  idleConns: number
  totalConns: number
  maxConns: number
}

export interface SystemResources {
  cpuPercent: number
  heapAllocMb: number
  sysMemoryMb: number
  goroutineCount: number
  gcCycles: number
}

export interface PipelineStatus {
  killSwitch: KillSwitchStatus
  masterTickers: TickerStatus[]
  queues: QueueStatus
  workerCount: number
  dbPool: DBPoolStatus
  systemResources?: SystemResources
}

export type ExpiryStatus = 'active' | 'expiring_soon' | 'expired'

export interface AdminProxyIP extends ProxyIP {
  daysRemaining: number
  expiryStatus: ExpiryStatus
  username?: string
  createdAt?: string
}

export interface ProxyIPStats {
  totalCount: number
  assignedCount: number
  availableCount: number
  ipv4Count: number
  ipv6Count: number
  expiringSoonCount: number
  expiredCount: number
  assignedExpiring: number
}

export interface AdminProxyIPsResponse {
  ips: AdminProxyIP[]
  stats: ProxyIPStats
}

export interface BulkUploadResult {
  total: number
  inserted: number
  updated: number
  errors: string[]
}

export interface ProxyTestResult {
  ipAddress: string
  success: boolean
  latencyMs: number
  egressIp?: string
  error?: string
}

export interface UserSummary {
  id: string
  email: string
  username: string
  name: string
  role: string
  active: boolean
  phone?: string
  accountCount: number
  sessionCount: number
  createdAt: string
  updatedAt: string
}

export interface AdminUsersResponse {
  users: UserSummary[]
  total: number
  limit: number
  offset: number
}

export interface Session {
  id: number
  tokenHash: string
  userId: string
  ipAddress?: string
  userAgent?: string
  expiresAt: string
  createdAt: string
  lastSeenAt: string
}

// Telemetry & KillSwitch
export function getAdminServiceStatus(): Promise<PipelineStatus> {
  return apiFetch(`${BASE}/admin/status`).then((r) => json(r))
}

export function postAdminKillSwitch(
  engaged: boolean,
  reason: string,
): Promise<{ status: string; killSwitch: KillSwitchStatus }> {
  return apiFetch(`${BASE}/admin/killswitch`, {
    method: 'POST',
    body: JSON.stringify({ engaged, reason }),
  }).then((r) => json(r))
}

// Admin Proxy IPs
export function getAdminProxyIPs(status?: string, type?: string): Promise<AdminProxyIPsResponse> {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (type) params.set('type', type)
  const qs = params.toString() ? `?${params.toString()}` : ''
  return apiFetch(`${BASE}/admin/proxy-ips${qs}`).then((r) => json(r))
}

export function postAdminProxyIPsBulk(items: any[]): Promise<BulkUploadResult> {
  return apiFetch(`${BASE}/admin/proxy-ips/bulk`, {
    method: 'POST',
    body: JSON.stringify(items),
  }).then((r) => json(r))
}

export function postAdminProxyIP(item: any): Promise<{ status: string; ip: AdminProxyIP }> {
  return apiFetch(`${BASE}/admin/proxy-ips`, {
    method: 'POST',
    body: JSON.stringify(item),
  }).then((r) => json(r))
}

export function patchAdminProxyIP(
  ip: string,
  update: { validUntil?: string; password?: string; plan?: string },
): Promise<{ status: string }> {
  return apiFetch(`${BASE}/admin/proxy-ips/${encodeURIComponent(ip)}`, {
    method: 'PATCH',
    body: JSON.stringify(update),
  }).then((r) => json(r))
}

export function putAdminProxyIP(ip: string, item: any): Promise<{ status: string; ip?: AdminProxyIP }> {
  return apiFetch(`${BASE}/admin/proxy-ips/${encodeURIComponent(ip)}`, {
    method: 'PUT',
    body: JSON.stringify(item),
  }).then((r) => json(r))
}

export function deleteAdminProxyIP(ip: string): Promise<{ status: string }> {
  return apiFetch(`${BASE}/admin/proxy-ips/${encodeURIComponent(ip)}`, {
    method: 'DELETE',
  }).then((r) => json(r))
}

export function testAdminProxyIP(ip: string): Promise<ProxyTestResult> {
  return apiFetch(`${BASE}/admin/proxy-ips/${encodeURIComponent(ip)}/test`, {
    method: 'POST',
  }).then((r) => json(r))
}

// Admin Users
export function getAdminUsers(limit = 20, offset = 0): Promise<AdminUsersResponse> {
  return apiFetch(`${BASE}/admin/users?limit=${limit}&offset=${offset}`).then((r) => json(r))
}

export function postAdminUser(user: {
  email: string
  username: string
  name?: string
  password: string
  role?: string
}): Promise<{ status: string; userId: string }> {
  return apiFetch(`${BASE}/admin/users`, {
    method: 'POST',
    body: JSON.stringify(user),
  }).then((r) => json(r))
}

export function patchAdminUser(
  id: string,
  update: { role?: string; active?: boolean },
): Promise<{ status: string }> {
  return apiFetch(`${BASE}/admin/users/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(update),
  }).then((r) => json(r))
}

export function getAdminUserSessions(id: string): Promise<{ sessions: Session[] }> {
  return apiFetch(`${BASE}/admin/users/${id}/sessions`).then((r) => json(r))
}

export function deleteAdminUserSessions(
  id: string,
): Promise<{ status: string; revokedCount: number }> {
  return apiFetch(`${BASE}/admin/users/${id}/sessions`, {
    method: 'DELETE',
  }).then((r) => json(r))
}

export function getKillSwitchStatus(): Promise<KillSwitchStatus> {
  return apiFetch(`${BASE}/killswitch`).then((r) => json(r))
}

export function getAdminUserAccounts(
  id: string,
): Promise<{ user: User; accounts: Account[] }> {
  return apiFetch(`${BASE}/admin/users/${id}/accounts`).then((r) => json(r))
}

// Operational Monitoring Types & Functions
export interface AttentionAccount {
  id: string
  name: string
  brokerAccountId: string
  role: string
  authStatus: string
  authError: string
  tokenExpiresAt?: string
}

export interface BrokerReadinessSummary {
  masterReady: boolean
  totalFollowers: number
  followersReady: number
  attentionAccounts: AttentionAccount[]
}

export interface OrderIssue {
  id: number
  createdAt: string
  followerName: string
  brokerAccountId: string
  tradingsymbol: string
  exchange: string
  intendedQty: number
  placedQty: number
  sizingReason: string
  terminalStatus: string
  lastError: string
}

export interface ExecutionEvent {
  timestamp: string
  tradingsymbol: string
  exchange: string
  transactionType: string
  masterQty: number
  price: number
  followersCount: number
  skippedCount: number
  latencyMs: number
}

export interface BulkProxyTestSummary {
  totalTested: number
  successCount: number
  failureCount: number
  avgLatencyMs: number
  results: ProxyTestResult[]
}

export function getAdminBrokerReadiness(): Promise<BrokerReadinessSummary> {
  return apiFetch(`${BASE}/admin/broker-readiness`).then((r) => json(r))
}

export interface AdminOrderIssuesResponse {
  issues: OrderIssue[]
  total: number
  limit: number
  offset: number
}

export function getAdminOrderIssues(limit = 50, offset = 0): Promise<AdminOrderIssuesResponse> {
  return apiFetch(`${BASE}/admin/order-issues?limit=${limit}&offset=${offset}`)
    .then((r) => json<any>(r))
    .then((data) => {
      if (Array.isArray(data)) {
        return {
          issues: data,
          total: data.length,
          limit,
          offset,
        }
      }
      return {
        issues: Array.isArray(data?.issues) ? data.issues : [],
        total: typeof data?.total === 'number' ? data.total : (Array.isArray(data?.issues) ? data.issues.length : 0),
        limit: typeof data?.limit === 'number' ? data.limit : limit,
        offset: typeof data?.offset === 'number' ? data.offset : offset,
      }
    })
}

export function getAdminExecutionFeed(): Promise<ExecutionEvent[]> {
  return apiFetch(`${BASE}/admin/execution-feed`).then((r) => json(r))
}

export function testAllAdminProxyIPs(): Promise<BulkProxyTestSummary> {
  return apiFetch(`${BASE}/admin/proxy-ips/test-all`, {
    method: 'POST',
  }).then((r) => json(r))
}



