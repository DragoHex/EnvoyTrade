import { createSignal, onMount, Show, For } from 'solid-js'
import { useParams, A } from '@solidjs/router'
import { AdminLayout } from './AdminLayout'
import {
  getAdminUserAccounts,
  patchAccount,
  testAdminProxyIP,
  getAdminUserSessions,
  deleteAdminUserSessions,
  type User,
  type Account,
  type Session,
  type ProxyTestResult,
} from '../../api'
import { BrokerLogo } from '../../components/BrokerLogo'
import { Skeleton } from '../../components/Skeleton'
import { CopyToggle } from '../../components/CopyToggle'
import { RoleIcon } from '../../components/RoleIcon'
import { AuthStatusIcon } from '../../components/AuthStatusIcon'

export function AdminUserAccountsPage() {
  const params = useParams()
  const [user, setUser] = createSignal<User | null>(null)
  const [accounts, setAccounts] = createSignal<Account[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)

  // Sub-tabs: 'accounts' | 'sessions'
  const [activeTab, setActiveTab] = createSignal<'accounts' | 'sessions'>('accounts')

  // Copy toggle in-flight
  const [togglingAccountId, setTogglingAccountId] = createSignal<string | null>(null)

  // Proxy test state
  const [testingIp, setTestingIp] = createSignal<string | null>(null)
  const [ipTestResults, setIpTestResults] = createSignal<Record<string, ProxyTestResult>>({})

  // Sessions state (lazy loaded)
  const [sessions, setSessions] = createSignal<Session[]>([])
  const [sessionsLoaded, setSessionsLoaded] = createSignal(false)
  const [sessionsLoading, setSessionsLoading] = createSignal(false)
  const [revokingSessions, setRevokingSessions] = createSignal(false)

  const loadAccounts = async () => {
    if (!params.id) return
    setLoading(true)
    setError(null)
    try {
      const res = await getAdminUserAccounts(params.id)
      setUser(res.user)
      setAccounts(res.accounts || [])
    } catch (err: any) {
      setError(err?.message || 'Failed to load user accounts')
    } finally {
      setLoading(false)
    }
  }

  const loadSessions = async () => {
    if (!params.id) return
    setSessionsLoading(true)
    try {
      const res = await getAdminUserSessions(params.id)
      setSessions(res.sessions || [])
      setSessionsLoaded(true)
    } catch (err: any) {
      setError(err?.message || 'Failed to load active sessions')
    } finally {
      setSessionsLoading(false)
    }
  }

  const handleRevokeSessions = async () => {
    if (!params.id) return
    setRevokingSessions(true)
    try {
      await deleteAdminUserSessions(params.id)
      setSessions([])
    } catch (err: any) {
      setError(err?.message || 'Failed to revoke sessions')
    } finally {
      setRevokingSessions(false)
    }
  }

  const handleToggleCopy = async (acct: Account) => {
    setTogglingAccountId(acct.id)
    try {
      const nextEnabled = !acct.enabled
      await patchAccount(acct.id, { enabled: nextEnabled })
      setAccounts((prev) =>
        prev.map((a) => (a.id === acct.id ? { ...a, enabled: nextEnabled } : a)),
      )
    } catch (err: any) {
      setError(err?.message || 'Failed to toggle copy status')
    } finally {
      setTogglingAccountId(null)
    }
  }

  const handleTestProxy = async (ip: string) => {
    setTestingIp(ip)
    try {
      const res = await testAdminProxyIP(ip)
      setIpTestResults((prev) => ({ ...prev, [ip]: res }))
    } catch (err: any) {
      setIpTestResults((prev) => ({
        ...prev,
        [ip]: { ipAddress: ip, success: false, latencyMs: 0, error: err?.message },
      }))
    } finally {
      setTestingIp(null)
    }
  }

  onMount(() => {
    loadAccounts()
  })

  return (
    <AdminLayout activeTab="users">
      <div style={{ 'margin-bottom': '1.25rem', display: 'flex', 'align-items': 'center', 'justify-content': 'space-between' }}>
        <div style={{ display: 'flex', 'align-items': 'center', gap: '0.75rem' }}>
          <A
            href="/admin/users"
            class="btn-secondary"
            style={{
              'text-decoration': 'none',
              display: 'inline-flex',
              'align-items': 'center',
              gap: '0.4rem',
              'font-size': '0.85rem',
              padding: '0.4rem 0.75rem',
            }}
          >
            &larr; Back to Platform Users
          </A>
          <span style={{ color: 'var(--color-text-secondary)', 'font-size': '0.85rem' }}>/</span>
          <span style={{ 'font-weight': 600, color: 'var(--color-text)', 'font-size': '0.95rem' }}>
            {user()?.username ? `${user()!.username}'s Profile & Accounts` : 'User Profile & Accounts'}
          </span>
        </div>
      </div>

      <Show when={error()}>
        <div style={{ padding: '0.75rem 1rem', background: 'rgba(239, 68, 68, 0.1)', color: '#ef4444', 'border-radius': '6px', 'margin-bottom': '1rem' }}>
          {error()}
        </div>
      </Show>

      {/* User Identity Card */}
      <Show
        when={!loading() && user()}
        fallback={<Skeleton height={110} radius={8} />}
      >
        <div
          style={{
            background: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            'border-radius': '8px',
            padding: '1.25rem',
            'margin-bottom': '1.5rem',
            display: 'flex',
            'flex-direction': 'column',
            gap: '0.75rem',
          }}
        >
          <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'flex-wrap': 'wrap', gap: '1rem' }}>
            <div style={{ display: 'flex', 'flex-direction': 'column', gap: '0.2rem' }}>
              <div style={{ display: 'flex', 'align-items': 'center', gap: '0.5rem' }}>
                <span style={{ 'font-size': '1.15rem', 'font-weight': 700, color: 'var(--color-text)' }}>
                  {user()!.name || user()!.username}
                </span>
                <span class={user()!.role === 'admin' ? 'badge badge-warning' : 'badge badge-neutral'}>
                  {user()!.role.toUpperCase()}
                </span>
                <span class={user()!.active ? 'badge badge-success' : 'badge badge-danger'}>
                  {user()!.active ? 'Active' : 'Suspended'}
                </span>
              </div>
              <div style={{ 'font-size': '0.8rem', color: 'var(--color-text-secondary)' }}>
                @{user()!.username} &bull; {user()!.email}
              </div>
            </div>

            <div style={{ display: 'flex', gap: '2rem', 'align-items': 'center' }}>
              <div style={{ 'text-align': 'right' }}>
                <div style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)', 'text-transform': 'uppercase', 'font-weight': 600 }}>
                  Trading Accounts
                </div>
                <div style={{ 'font-size': '1.25rem', 'font-weight': 700, color: 'var(--color-accent)' }}>
                  {accounts().length}
                </div>
              </div>
            </div>
          </div>

          {/* Enriched KYC and Metadata Details */}
          <div
            style={{
              display: 'flex',
              'flex-wrap': 'wrap',
              gap: '1.5rem',
              'padding-top': '0.75rem',
              'border-top': '1px solid var(--color-border)',
              'font-size': '0.85rem',
              color: 'var(--color-text-secondary)',
            }}
          >
            <div>
              <span style={{ 'font-weight': 600, color: 'var(--color-text)' }}>Phone: </span>
              <span>{user()!.phone || '—'}</span>
            </div>
            <div>
              <span style={{ 'font-weight': 600, color: 'var(--color-text)' }}>Address: </span>
              <span>{user()!.address || '—'}</span>
            </div>
            <div>
              <span style={{ 'font-weight': 600, color: 'var(--color-text)' }}>GSTIN: </span>
              <span style={{ 'font-family': 'monospace' }}>{user()!.gstNumber || '—'}</span>
            </div>
          </div>
        </div>
      </Show>

      {/* Subpage Tabs Switcher */}
      <div class="tab-group" style={{ 'margin-bottom': '1rem' }} role="tablist">
        <button
          type="button"
          role="tab"
          class="tab-btn"
          classList={{ active: activeTab() === 'accounts' }}
          aria-selected={activeTab() === 'accounts'}
          onClick={() => setActiveTab('accounts')}
        >
          Trading Accounts ({accounts().length})
        </button>
        <button
          type="button"
          role="tab"
          class="tab-btn"
          classList={{ active: activeTab() === 'sessions' }}
          aria-selected={activeTab() === 'sessions'}
          onClick={() => {
            setActiveTab('sessions')
            if (!sessionsLoaded()) loadSessions()
          }}
        >
          Active Sessions
        </button>
      </div>

      {/* Tab 1: Accounts List Table */}
      <Show when={activeTab() === 'accounts'}>
        <Show
          when={!loading()}
          fallback={<Skeleton height={240} radius={8} />}
        >
          <div class="admin-table-container">
            <table class="admin-table" style={{ 'table-layout': 'fixed' }}>
              <thead>
                <tr>
                  <th style={{ width: '18%' }}>Account Name</th>
                  <th style={{ width: '12%' }}>Broker ID</th>
                  <th style={{ width: '11%' }}>Broker</th>
                  <th style={{ width: '7%', 'text-align': 'center' }}>Role</th>
                  <th style={{ width: '9%', 'text-align': 'center' }}>Auth Status</th>
                  <th style={{ width: '7%', 'text-align': 'center' }}>Copy</th>
                  <th style={{ width: '11%' }}>Sizing</th>
                  <th style={{ width: '25%' }}>Proxy IP</th>
                </tr>
              </thead>
              <tbody>
                <Show
                  when={accounts().length > 0}
                  fallback={
                    <tr>
                      <td colspan={8} style={{ 'text-align': 'center', padding: '2.5rem', color: 'var(--color-text-secondary)' }}>
                        No trading accounts associated with this user.
                      </td>
                    </tr>
                  }
                >
                  <For each={accounts()}>
                    {(acct) => (
                      <tr>
                        <td>
                          <div style={{ 'font-weight': 600, color: 'var(--color-text)' }}>
                            {acct.name}
                          </div>
                          <Show when={acct.groupName}>
                            <div style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)', 'margin-top': '0.1rem' }}>
                              Group: {acct.groupName}
                            </div>
                          </Show>
                        </td>
                        <td>
                          <span style={{ 'font-family': 'monospace', 'font-size': '0.85rem' }}>
                            {acct.brokerAccountId}
                          </span>
                        </td>
                        <td>
                          <div style={{ display: 'flex', 'align-items': 'center', gap: '0.4rem' }}>
                            <BrokerLogo broker={acct.broker} size={20} />
                            <span style={{ 'font-size': '0.85rem', 'text-transform': 'capitalize' }}>
                              {acct.broker}
                            </span>
                          </div>
                        </td>
                        <td style={{ 'text-align': 'center' }}>
                          <span class="role-cell-centered">
                            <RoleIcon role={acct.role} />
                          </span>
                        </td>
                        <td style={{ 'text-align': 'center' }}>
                          <span class="auth-status-cell-centered">
                            <AuthStatusIcon status={acct.authStatus} error={acct.authError} />
                          </span>
                        </td>
                        <td style={{ 'text-align': 'center' }}>
                          <Show
                            when={acct.role === 'follower'}
                            fallback={<span style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>Master</span>}
                          >
                            <CopyToggle
                              enabled={acct.enabled}
                              disabled={togglingAccountId() === acct.id}
                              ariaLabel={`Toggle copy trading for ${acct.name}`}
                              title={`Toggle copy trading for ${acct.name}`}
                              onToggle={() => handleToggleCopy(acct)}
                            />
                          </Show>
                        </td>
                        <td>
                          <Show
                            when={acct.role === 'follower'}
                            fallback={<span style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>—</span>}
                          >
                            <div style={{ 'font-size': '0.8rem' }}>
                              <span style={{ 'font-weight': 600, color: 'var(--color-text)' }}>
                                {acct.cloneFactor ? `${acct.cloneFactor}x` : '1.0x'}
                              </span>
                              <Show when={acct.maxQtyPerOrder}>
                                <div style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>
                                  {acct.maxQtyPerOrder} qty
                                </div>
                              </Show>
                            </div>
                          </Show>
                        </td>
                        <td class="col-nowrap" style={{ 'white-space': 'nowrap', 'word-break': 'normal' }}>
                          <div
                            style={{
                              display: 'inline-flex',
                              'align-items': 'center',
                              gap: '0.45rem',
                              'white-space': 'nowrap',
                              'flex-wrap': 'nowrap',
                            }}
                          >
                            <span
                              style={{
                                'font-family': 'monospace',
                                'font-size': '0.8rem',
                                'white-space': 'nowrap',
                                'word-break': 'normal',
                                color: acct.ip ? 'var(--color-accent)' : 'var(--color-text-secondary)',
                              }}
                            >
                              {acct.ip || 'Direct'}
                            </span>
                            <Show when={acct.ip}>
                              <button
                                type="button"
                                class="btn-secondary"
                                style={{
                                  padding: '0.2rem 0.45rem',
                                  'font-size': '0.75rem',
                                  'white-space': 'nowrap',
                                  'word-break': 'normal',
                                  'flex-shrink': 0,
                                }}
                                aria-label={`Test Proxy ${acct.ip}`}
                                onClick={() => handleTestProxy(acct.ip!)}
                                disabled={testingIp() === acct.ip}
                              >
                                {testingIp() === acct.ip ? '...' : 'Test'}
                              </button>
                              <Show when={ipTestResults()[acct.ip!] !== undefined}>
                                <span
                                  style={{
                                    'font-size': '0.75rem',
                                    'font-weight': 600,
                                    'white-space': 'nowrap',
                                    'word-break': 'normal',
                                    'flex-shrink': 0,
                                    color: ipTestResults()[acct.ip!].success ? 'var(--color-accent)' : '#ef4444',
                                  }}
                                  title={
                                    ipTestResults()[acct.ip!].error ||
                                    (ipTestResults()[acct.ip!].success ? 'Probe successful' : 'Probe failed')
                                  }
                                >
                                  {ipTestResults()[acct.ip!].success
                                    ? `${ipTestResults()[acct.ip!].latencyMs}ms`
                                    : 'Failed'}
                                </span>
                              </Show>
                            </Show>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </Show>
              </tbody>
            </table>
          </div>
        </Show>
      </Show>

      {/* Tab 2: Active Sessions Tab */}
      <Show when={activeTab() === 'sessions'}>
        <Show
          when={!sessionsLoading()}
          fallback={<Skeleton height={200} radius={8} />}
        >
          <div
            style={{
              background: 'var(--color-surface)',
              border: '1px solid var(--color-border)',
              'border-radius': '8px',
              padding: '1.25rem',
            }}
          >
            <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-bottom': '1rem' }}>
              <div style={{ 'font-weight': 600, 'font-size': '0.95rem', color: 'var(--color-text)' }}>
                Active Devices & Sessions ({sessions().length})
              </div>
              <button
                type="button"
                class="btn-danger"
                style={{ 'font-size': '0.8rem', padding: '0.35rem 0.75rem' }}
                onClick={handleRevokeSessions}
                disabled={revokingSessions() || sessions().length === 0}
              >
                {revokingSessions() ? 'Revoking...' : 'Revoke All Sessions'}
              </button>
            </div>

            <Show
              when={sessions().length > 0}
              fallback={
                <div style={{ 'text-align': 'center', padding: '2rem', color: 'var(--color-text-secondary)' }}>
                  No active sessions found for this user.
                </div>
              }
            >
              <div style={{ display: 'flex', 'flex-direction': 'column', gap: '0.75rem' }}>
                <For each={sessions()}>
                  {(sess) => (
                    <div
                      style={{
                        padding: '0.85rem',
                        border: '1px solid var(--color-border)',
                        'border-radius': '6px',
                        background: 'var(--color-bg)',
                        'font-size': '0.85rem',
                      }}
                    >
                      <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-bottom': '0.3rem' }}>
                        <span style={{ 'font-weight': 600, color: 'var(--color-text)', 'font-family': 'monospace' }}>
                          {sess.ipAddress || 'Unknown IP'}
                        </span>
                        <span class="badge badge-success">ACTIVE</span>
                      </div>
                      <div style={{ color: 'var(--color-text-secondary)', 'margin-bottom': '0.3rem', 'font-size': '0.8rem' }}>
                        {sess.userAgent || 'Unknown Browser / Platform'}
                      </div>
                      <div style={{ color: 'var(--color-text-secondary)', 'font-size': '0.75rem' }}>
                        Last seen: {new Date(sess.lastSeenAt).toLocaleString()} &bull; Expires: {new Date(sess.expiresAt).toLocaleString()}
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </Show>
      </Show>
    </AdminLayout>
  )
}
