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

  it('triggers onUnauthorized on 401 response', async () => {
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
})
