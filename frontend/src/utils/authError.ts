/**
 * Sanitizes and formats broker authentication error messages for user-facing UI.
 * Strictly prevents leaking backend API paths, URLs, HTTP verbs, method traces, or payloads.
 */
export function formatAuthError(error?: string | null): string {
  if (!error) return ''
  const trimmed = error.trim()
  if (!trimmed) return ''

  const lower = trimmed.toLowerCase()

  // Detect technical or backend API artifacts (URLs, HTTP methods, API paths, etc.)
  const hasBackendAPITraces =
    lower.includes('http://') ||
    lower.includes('https://') ||
    lower.includes('post ') ||
    lower.includes('get ') ||
    lower.includes('/api/') ||
    lower.includes('connect/login') ||
    lower.includes('session/token') ||
    lower.includes('sess_id') ||
    lower.includes('dial tcp') ||
    lower.includes('get dhan') ||
    lower.includes('dhan api error') ||
    lower.includes('kite web login') ||
    lower.includes('dhan sync failed') ||
    lower.includes('connect login') ||
    lower.includes('kite login error') ||
    lower.includes('kite 2fa error')

  if (!hasBackendAPITraces) {
    return trimmed
  }

  if (
    lower.includes('401') ||
    lower.includes('unauthorized') ||
    lower.includes('token expired') ||
    lower.includes('invalid token') ||
    lower.includes('token revoked')
  ) {
    return 'Broker session expired or token invalid'
  }

  if (
    lower.includes('totp') ||
    lower.includes('2fa') ||
    lower.includes('twofa') ||
    lower.includes('external totp')
  ) {
    return 'Invalid 2FA TOTP code or external TOTP not enabled'
  }

  if (
    lower.includes('password') ||
    lower.includes('credentials') ||
    lower.includes('user id') ||
    lower.includes('user_id') ||
    lower.includes('login error')
  ) {
    return 'Invalid broker credentials or password'
  }

  if (
    lower.includes('500') ||
    lower.includes('502') ||
    lower.includes('503') ||
    lower.includes('504') ||
    lower.includes('gateway') ||
    lower.includes('timeout') ||
    lower.includes('dial') ||
    lower.includes('connection refused') ||
    lower.includes('econnrefused') ||
    lower.includes('dh-1111') ||
    lower.includes('server error') ||
    lower.includes('service unavailable')
  ) {
    return 'Broker server temporarily unavailable'
  }

  if (
    lower.includes('429') ||
    lower.includes('rate limit') ||
    lower.includes('too many requests')
  ) {
    return 'Broker rate limit exceeded'
  }

  if (lower.includes('missing') || lower.includes('required')) {
    return 'Missing broker API key or credentials'
  }

  return 'Authentication failed with broker'
}
