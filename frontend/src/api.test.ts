import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  register,
  login,
  logout,
  getMe,
  updateProfile,
  updatePassword,
  getGroups,
  setOnUnauthorized,
  fetchProxyIPs,
  fetchAvailableProxyIPs,
  squareOffGroup,
  squareOffAccount,
  getGroupRebalanceDiff,
  getAdminServiceStatus,
  postAdminKillSwitch,
  getAdminProxyIPs,
  putAdminProxyIP,
  testAdminProxyIP,
  getAdminUsers,
  getKillSwitchStatus,
  getAdminUserAccounts,
  getAdminOrderIssues,
} from './api'

describe('api client', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    setOnUnauthorized(null)
  })

  it('includes credentials: include in requests', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify([]), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )

    await getGroups()

    expect(fetchSpy).toHaveBeenCalledTimes(1)
    const [url, init] = fetchSpy.mock.calls[0]
    expect(url).toBe('/api/v1/groups')
    expect(init?.credentials).toBe('include')
  })

  it('login sends POST /api/v1/auth/login and returns user', async () => {
    const fakeUser = {
      id: 'u-123',
      email: 'trader@example.com',
      username: 'trader1',
      name: 'Trader One',
      role: 'user',
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ user: fakeUser, token: 'tok_abc' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await login({ email: 'trader@example.com', password: 'password123' })

    expect(result.user).toEqual(fakeUser)
    expect(result.token).toBe('tok_abc')
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/auth/login')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body)).toEqual({
      email: 'trader@example.com',
      password: 'password123',
    })
  })

  it('register sends POST /api/v1/auth/register and returns user', async () => {
    const fakeUser = {
      id: 'u-456',
      email: 'new@example.com',
      username: 'newtrader',
      name: 'New Trader',
      role: 'user',
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ user: fakeUser, token: 'tok_def' }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await register({
      email: 'new@example.com',
      username: 'newtrader',
      name: 'New Trader',
      password: 'password123',
    })

    expect(result.user).toEqual(fakeUser)
    expect(result.token).toBe('tok_def')
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/auth/register')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('include')
  })

  it('logout sends POST /api/v1/auth/logout', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ status: 'ok' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await logout()
    expect(result.status).toBe('ok')
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/auth/logout')
    expect(init.method).toBe('POST')
  })

  it('getMe sends GET /api/v1/auth/me', async () => {
    const fakeUser = {
      id: 'u-123',
      email: 'trader@example.com',
      username: 'trader1',
      name: 'Trader One',
      role: 'user',
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ user: fakeUser }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await getMe()
    expect(result.user).toEqual(fakeUser)
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/auth/me')
    expect(init.credentials).toBe('include')
  })

  it('triggers onUnauthorized on 401 response from session auth endpoints', async () => {
    const unauthorizedCallback = vi.fn()
    setOnUnauthorized(unauthorizedCallback)

    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: 'unauthenticated' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(getMe()).rejects.toThrow('unauthenticated')
    expect(unauthorizedCallback).toHaveBeenCalledTimes(1)
  })

  it('does NOT trigger onUnauthorized when broker-level operations return 401 (e.g. rebalance diff broker token expiration)', async () => {
    const unauthorizedCallback = vi.fn()
    setOnUnauthorized(unauthorizedCallback)

    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: 'fetch follower ACTFLOW01 positions: auth expired (testbroker get positions: KiteError [403 TokenException]: Invalid or missing token)' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(getGroupRebalanceDiff('g-123')).rejects.toThrow('auth expired')
    expect(unauthorizedCallback).not.toHaveBeenCalled()
  })

  it('updateProfile sends PUT /api/v1/user/profile and returns user', async () => {
    const fakeUser = {
      id: 'u-123',
      email: 'updated@example.com',
      username: 'updated_user',
      name: 'Updated Name',
      role: 'user',
      phone: '+919876543210',
      address: '123 Test Street',
      gstNumber: '29ABCDE1234F1Z5',
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ user: fakeUser }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await updateProfile({
      email: 'updated@example.com',
      username: 'updated_user',
      name: 'Updated Name',
      phone: '9876543210',
      address: '123 Test Street',
      gstNumber: '29ABCDE1234F1Z5',
    })

    expect(result.user).toEqual(fakeUser)
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/user/profile')
    expect(init.method).toBe('PUT')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body)).toEqual({
      email: 'updated@example.com',
      username: 'updated_user',
      name: 'Updated Name',
      phone: '9876543210',
      address: '123 Test Street',
      gstNumber: '29ABCDE1234F1Z5',
    })
  })

  it('updatePassword sends POST /api/v1/user/password', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ status: 'ok' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await updatePassword({
      oldPassword: 'OldPassword123!',
      newPassword: 'NewPassword456!',
    })

    expect(result.status).toBe('ok')
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/user/password')
    expect(init.method).toBe('POST')
    expect(init.credentials).toBe('include')
    expect(JSON.parse(init.body)).toEqual({
      oldPassword: 'OldPassword123!',
      newPassword: 'NewPassword456!',
    })
  })

  it('throws descriptive error on API failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: 'invalid email or password' }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(
      login({ email: 'bad@example.com', password: 'wrong' }),
    ).rejects.toThrow('invalid email or password')
  })

  it('fetchProxyIPs sends GET /api/v1/proxy-ips', async () => {
    const fakeIPs = [
      { ipAddress: '148.113.41.41', ipType: 'ipv4', host: 'dc46-mum-01.algoip.in', port: 443 },
    ]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeIPs), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )

    const result = await fetchProxyIPs()
    expect(result).toEqual(fakeIPs)
    const [url] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/proxy-ips')
  })

  it('fetchAvailableProxyIPs sends GET /api/v1/proxy-ips/available with optional accountId', async () => {
    const fakeAvailable = {
      ipv4: [{ ipAddress: '148.113.41.42', ipType: 'ipv4' }],
      ipv6: [{ ipAddress: '2402:1f00::1', ipType: 'ipv6' }],
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeAvailable), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )

    const result = await fetchAvailableProxyIPs('acc-123')
    expect(result).toEqual(fakeAvailable)
    const [url] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/proxy-ips/available?accountId=acc-123')
  })

  it('squareOffGroup sends POST /api/v1/groups/:id/positions/square-off', async () => {
    const fakeResp = { groupId: 'g-1', status: 'completed', account: { accountId: 'm-1', status: 'completed', orders: [] }, followers: [] }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeResp), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )

    const result = await squareOffGroup('g-1', { symbols: ['NIFTY26OCTFUT'] })
    expect(result).toEqual(fakeResp)
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/groups/g-1/positions/square-off')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ symbols: ['NIFTY26OCTFUT'] })
  })

  it('squareOffAccount sends POST /api/v1/accounts/:id/positions/square-off', async () => {
    const fakeResp = { accountId: 'acc-1', status: 'completed', orders: [] }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeResp), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )

    const result = await squareOffAccount('acc-1')
    expect(result).toEqual(fakeResp)
    const [url, init] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/v1/accounts/acc-1/positions/square-off')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({})
  })

  it('admin status and killswitch APIs send correct requests', async () => {
    const fakeStatus = {
      killSwitch: { engaged: false },
      masterTickers: [],
      queues: { masterFillDepth: 0, masterFillCap: 1024, orderUpdateDepth: 0, orderUpdateCap: 512 },
      workerCount: 0,
      dbPool: { acquiredConns: 0, idleConns: 0, totalConns: 0, maxConns: 20 },
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeStatus), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )

    const status = await getAdminServiceStatus()
    expect(status).toEqual(fakeStatus)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/v1/admin/status')

    const ksResp = { status: 'ok', killSwitch: { engaged: true, reason: 'halt' } }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(ksResp), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const res = await postAdminKillSwitch(true, 'halt')
    expect(res).toEqual(ksResp)
    const [ksUrl, ksInit] = (globalThis.fetch as any).mock.calls[1]
    expect(ksUrl).toBe('/api/v1/admin/killswitch')
    expect(ksInit.method).toBe('POST')
    expect(JSON.parse(ksInit.body)).toEqual({ engaged: true, reason: 'halt' })
  })

  it('admin proxy IP APIs send correct requests', async () => {
    const fakeList = { ips: [], stats: { totalCount: 0 } }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeList), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const listRes = await getAdminProxyIPs('active', 'ipv4')
    expect(listRes).toEqual(fakeList)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/v1/admin/proxy-ips?status=active&type=ipv4')

    // test single IP
    const testResult = { ipAddress: '10.0.0.1', success: true, latencyMs: 50 }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(testResult), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const probe = await testAdminProxyIP('10.0.0.1')
    expect(probe).toEqual(testResult)
    expect((globalThis.fetch as any).mock.calls[1][0]).toBe('/api/v1/admin/proxy-ips/10.0.0.1/test')

    // putAdminProxyIP (single DB call individual IP JSON update)
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const putRes = await putAdminProxyIP('10.0.0.1', { ip: '10.0.0.1', host: 'dc1.algoip.in' })
    expect(putRes).toEqual({ status: 'ok' })
    expect((globalThis.fetch as any).mock.calls[2][0]).toBe('/api/v1/admin/proxy-ips/10.0.0.1')
    expect((globalThis.fetch as any).mock.calls[2][1].method).toBe('PUT')
    expect(JSON.parse((globalThis.fetch as any).mock.calls[2][1].body)).toEqual({ ip: '10.0.0.1', host: 'dc1.algoip.in' })
  })

  it('admin users APIs send correct requests', async () => {
    const fakeUsers = { users: [], total: 0, limit: 20, offset: 0 }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeUsers), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const res = await getAdminUsers(20, 0)
    expect(res).toEqual(fakeUsers)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/v1/admin/users?limit=20&offset=0')
  })

  it('getKillSwitchStatus sends GET /api/v1/killswitch', async () => {
    const fakeStatus = { engaged: true, reason: 'Market breaker' }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeStatus), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const res = await getKillSwitchStatus()
    expect(res).toEqual(fakeStatus)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/v1/killswitch')
  })

  it('getAdminUserAccounts sends GET /api/v1/admin/users/:id/accounts', async () => {
    const fakeData = { user: { id: 'u1', username: 'testuser' }, accounts: [] }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeData), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const res = await getAdminUserAccounts('u1')
    expect(res).toEqual(fakeData)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/v1/admin/users/u1/accounts')
  })

  it('getAdminOrderIssues handles both object and array responses', async () => {
    const fakeObj = { issues: [{ id: 1, tradingsymbol: 'INFY' }], total: 1, limit: 50, offset: 0 }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeObj), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const res1 = await getAdminOrderIssues(50, 0)
    expect(res1.issues).toHaveLength(1)
    expect(res1.total).toBe(1)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/v1/admin/order-issues?limit=50&offset=0')

    // Legacy array response
    const fakeArr = [{ id: 2, tradingsymbol: 'TCS' }]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(fakeArr), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    )
    const res2 = await getAdminOrderIssues(50, 0)
    expect(res2.issues).toHaveLength(1)
    expect(res2.total).toBe(1)
  })
})

