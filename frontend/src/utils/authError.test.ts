import { describe, it, expect } from 'vitest'
import { formatAuthError } from './authError'

describe('formatAuthError', () => {
  it('returns empty string for empty or null error', () => {
    expect(formatAuthError(null)).toBe('')
    expect(formatAuthError('')).toBe('')
    expect(formatAuthError('   ')).toBe('')
  })

  it('preserves clean user-facing error messages', () => {
    expect(formatAuthError('Token expired at 06:00 IST')).toBe('Token expired at 06:00 IST')
    expect(formatAuthError('Token expired / session inactive')).toBe('Token expired / session inactive')
    expect(formatAuthError('Invalid credentials')).toBe('Invalid credentials')
    expect(formatAuthError('Session expired')).toBe('Session expired')
    expect(formatAuthError('Invalid 2FA TOTP code')).toBe('Invalid 2FA TOTP code')
  })

  it('sanitizes backend API traces from error messages', () => {
    const rawErrors = [
      'dhan sync failed: get dhan holdings: dhan api error [DH-1111] (HTTP 500): No holdings available',
      'missing request_token from connect/login (HTTP 500, sess_id=abc, body: "error")',
      'connect login request: Post "https://kite.zerodha.com/connect/login": dial tcp 1.2.3.4: connect: connection refused',
      'session token request: Post "https://api.kite.trade/session/token": 401 Unauthorized',
      'kite login error: Incorrect password or user id',
      'kite 2fa error: Invalid TOTP code (External TOTP is not enabled)',
      'get dhan fund limits: 401 unauthorized token expired',
      'unknown backend failure in POST /api/v2/user/margins',
    ]

    for (const raw of rawErrors) {
      const sanitized = formatAuthError(raw)
      expect(sanitized.toLowerCase()).not.toContain('http://')
      expect(sanitized.toLowerCase()).not.toContain('https://')
      expect(sanitized.toLowerCase()).not.toContain('post ')
      expect(sanitized.toLowerCase()).not.toContain('get ')
      expect(sanitized.toLowerCase()).not.toContain('/api/')
      expect(sanitized.toLowerCase()).not.toContain('connect/login')
      expect(sanitized.toLowerCase()).not.toContain('sess_id')
      expect(sanitized.toLowerCase()).not.toContain('dial tcp')
    }
  })

  it('translates Kotak Neo errors into clear user-facing messages', () => {
    expect(
      formatAuthError(
        'kotak-neo api error [424]: HTTP 424: {"error":[{"code":"10564","message":"User not found for given ucc"}]}'
      )
    ).toBe('Invalid Kotak Neo Client ID (UCC) or Trade API not enabled for this user')

    expect(
      formatAuthError(
        'kotak-neo api error [400]: HTTP 400: {"error":[{"code":"400","message":"Invalid field \'MobileNumber\'"}]}'
      )
    ).toBe('Invalid Kotak Neo registered mobile number')
  })

  it('translates 401 and unauthorized API errors into clean session messages', () => {
    expect(
      formatAuthError('session token request: Post "https://api.kite.trade/session/token": 401 Unauthorized')
    ).toBe('Broker session expired or token invalid')
  })

  it('translates TOTP/2FA errors into clean 2FA messages', () => {
    expect(
      formatAuthError('kite 2fa error: invalid totp code (External TOTP is not enabled on this account)')
    ).toBe('Invalid 2FA TOTP code or external TOTP not enabled')
  })

  it('translates credential errors into clean credential messages', () => {
    expect(
      formatAuthError('kite login error: Incorrect password or user id')
    ).toBe('Invalid broker credentials or password')
  })

  it('translates 5xx and gateway timeout errors into server unavailable messages', () => {
    expect(
      formatAuthError('dhan sync failed: get dhan holdings: dhan api error [DH-1111] (HTTP 500): No holdings available')
    ).toBe('Broker server temporarily unavailable')
  })
})
