import { createSignal, createEffect, Show } from 'solid-js'
import { useNavigate } from '@solidjs/router'
import { useAuth } from '../context/AuthContext'
import { SyncIcon } from '../components/icons'
import { EnvoyLogo } from '../components/EnvoyLogo'

export function LoginPage() {
  const { login, register, isAuthenticated } = useAuth()
  const navigate = useNavigate()
  const [mode, setMode] = createSignal<'login' | 'register'>('login')
  const [email, setEmail] = createSignal('')
  const [username, setUsername] = createSignal('')
  const [name, setName] = createSignal('')
  const [password, setPassword] = createSignal('')
  const [error, setError] = createSignal<string | null>(null)
  const [submitting, setSubmitting] = createSignal(false)

  createEffect(() => {
    if (isAuthenticated()) {
      navigate('/', { replace: true })
    }
  })

  const toggleMode = () => {
    setError(null)
    setMode((m) => (m === 'login' ? 'register' : 'login'))
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setError(null)
    setSubmitting(true)

    try {
      if (mode() === 'login') {
        await login({ email: email().trim(), password: password() })
      } else {
        await register({
          email: email().trim(),
          username: username().trim(),
          name: name().trim() || undefined,
          password: password(),
        })
      }
      navigate('/', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Authentication failed')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      style={{
        display: 'flex',
        'align-items': 'center',
        'justify-content': 'center',
        'min-height': 'calc(100vh - 120px)',
        padding: '2rem 1rem',
      }}
    >
      <div
        style={{
          width: '100%',
          'max-width': '420px',
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          'border-radius': '12px',
          padding: '2rem',
          'box-shadow': '0 8px 30px rgba(0, 0, 0, 0.2)',
        }}
      >
        <div style={{ 'text-align': 'center', 'margin-bottom': '1.5rem' }}>
          <div style={{ 'margin-bottom': '1rem', display: 'flex', 'justify-content': 'center' }}>
            <EnvoyLogo size={48} />
          </div>
          <h2 style={{ margin: '0 0 0.5rem 0', 'font-size': '1.75rem', color: 'var(--color-text)' }}>
            {mode() === 'login' ? 'Sign In' : 'Create Account'}
          </h2>
          <p style={{ margin: 0, 'font-size': '0.875rem', color: 'var(--color-text-secondary)' }}>
            {mode() === 'login'
              ? 'Enter your credentials to access EnvoyTrade'
              : 'Sign up to manage and copy-trade your accounts'}
          </p>
        </div>

        <Show when={error()}>
          {(msg) => (
            <div
              role="alert"
              style={{
                background: 'rgba(229, 72, 77, 0.12)',
                border: '1px solid rgba(229, 72, 77, 0.3)',
                color: '#e5484d',
                padding: '0.75rem 1rem',
                'border-radius': '6px',
                'margin-bottom': '1.25rem',
                'font-size': '0.875rem',
                'line-height': '1.4',
              }}
            >
              {msg()}
            </div>
          )}
        </Show>

        <form onSubmit={handleSubmit} style={{ display: 'flex', 'flex-direction': 'column', gap: '1rem' }}>
          <label style={{ display: 'flex', 'flex-direction': 'column', gap: '0.35rem', 'font-size': '0.875rem', 'font-weight': '500' }}>
            Email Address
            <input
              type="email"
              required
              value={email()}
              onInput={(e) => setEmail(e.currentTarget.value)}
              placeholder="you@example.com"
              style={{
                padding: '0.6rem 0.75rem',
                'border-radius': '6px',
                border: '1px solid var(--color-border)',
                background: 'var(--color-bg)',
                color: 'var(--color-text)',
                'font-size': '0.95rem',
              }}
            />
          </label>

          <Show when={mode() === 'register'}>
            <label style={{ display: 'flex', 'flex-direction': 'column', gap: '0.35rem', 'font-size': '0.875rem', 'font-weight': '500' }}>
              Username
              <input
                type="text"
                required
                value={username()}
                onInput={(e) => setUsername(e.currentTarget.value)}
                placeholder="trader1"
                style={{
                  padding: '0.6rem 0.75rem',
                  'border-radius': '6px',
                  border: '1px solid var(--color-border)',
                  background: 'var(--color-bg)',
                  color: 'var(--color-text)',
                  'font-size': '0.95rem',
                }}
              />
            </label>

            <label style={{ display: 'flex', 'flex-direction': 'column', gap: '0.35rem', 'font-size': '0.875rem', 'font-weight': '500' }}>
              Full Name (Optional)
              <input
                type="text"
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="John Doe"
                style={{
                  padding: '0.6rem 0.75rem',
                  'border-radius': '6px',
                  border: '1px solid var(--color-border)',
                  background: 'var(--color-bg)',
                  color: 'var(--color-text)',
                  'font-size': '0.95rem',
                }}
              />
            </label>
          </Show>

          <label style={{ display: 'flex', 'flex-direction': 'column', gap: '0.35rem', 'font-size': '0.875rem', 'font-weight': '500' }}>
            {mode() === 'register' ? 'Password (min. 8 characters)' : 'Password'}
            <input
              type="password"
              required
              minLength={mode() === 'register' ? 8 : undefined}
              value={password()}
              onInput={(e) => setPassword(e.currentTarget.value)}
              placeholder="••••••••"
              style={{
                padding: '0.6rem 0.75rem',
                'border-radius': '6px',
                border: '1px solid var(--color-border)',
                background: 'var(--color-bg)',
                color: 'var(--color-text)',
                'font-size': '0.95rem',
              }}
            />
          </label>

          <button
            type="submit"
            disabled={submitting()}
            style={{
              display: 'inline-flex',
              'align-items': 'center',
              'justify-content': 'center',
              gap: '0.5rem',
              'margin-top': '0.5rem',
              padding: '0.75rem 1rem',
              background: 'var(--color-accent)',
              color: '#04140f',
              border: 'none',
              'border-radius': '6px',
              'font-size': '0.95rem',
              'font-weight': '600',
              cursor: submitting() ? 'default' : 'pointer',
              opacity: submitting() ? 0.75 : 1,
              transition: 'opacity 0.15s ease',
            }}
          >
            <Show when={submitting()}>
              <span style={{ width: '18px', height: '18px', display: 'inline-flex' }}>
                <SyncIcon spinning />
              </span>
            </Show>
            {mode() === 'login' ? 'Sign In' : 'Create Account'}
          </button>
        </form>

        <div style={{ 'text-align': 'center', 'margin-top': '1.5rem', 'font-size': '0.875rem' }}>
          <button
            type="button"
            onClick={toggleMode}
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--color-accent)',
              cursor: 'pointer',
              'font-size': '0.875rem',
              'text-decoration': 'underline',
              padding: '0.25rem 0.5rem',
            }}
          >
            {mode() === 'login'
              ? "Don't have an account? Create Account"
              : 'Already have an account? Sign In'}
          </button>
        </div>
      </div>
    </div>
  )
}
