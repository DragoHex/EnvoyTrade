import { createSignal, onMount, Show, For } from 'solid-js'
import { AdminLayout } from './AdminLayout'
import {
  getAdminBrokerReadiness,
  getAdminOrderIssues,
  getAdminExecutionFeed,
  type BrokerReadinessSummary,
  type OrderIssue,
  type ExecutionEvent,
} from '../../api'
import { SyncIcon } from '../../components/icons'
import { Skeleton } from '../../components/Skeleton'
import { RoleIcon } from '../../components/RoleIcon'
import { AuthStatusIcon } from '../../components/AuthStatusIcon'

type OperationsSubTab = 'readiness' | 'feed' | 'issues'

export function AdminOperationsPage() {
  const [activeTab, setActiveTab] = createSignal<OperationsSubTab>('readiness')
  const [readiness, setReadiness] = createSignal<BrokerReadinessSummary | null>(null)
  const [executionFeed, setExecutionFeed] = createSignal<ExecutionEvent[]>([])
  const [orderIssues, setOrderIssues] = createSignal<OrderIssue[]>([])
  const [loading, setLoading] = createSignal(true)
  const [refreshing, setRefreshing] = createSignal(false)
  const [lastUpdated, setLastUpdated] = createSignal<string>('')
  const [error, setError] = createSignal<string | null>(null)

  // Sub-tab 1: Broker readiness pagination (50/page)
  const [readinessPage, setReadinessPage] = createSignal(1)
  const readinessPageSize = 50
  const paginatedAttentionAccounts = () =>
    (readiness()?.attentionAccounts ?? []).slice(
      (readinessPage() - 1) * readinessPageSize,
      readinessPage() * readinessPageSize,
    )

  // Sub-tab 2: Execution feed pagination (50/page)
  const [feedPage, setFeedPage] = createSignal(1)
  const feedPageSize = 50
  const paginatedFeed = () =>
    executionFeed().slice((feedPage() - 1) * feedPageSize, feedPage() * feedPageSize)

  // Sub-tab 3: Order issues pagination (50/page, server-side)
  const [issuesOffset, setIssuesOffset] = createSignal(0)
  const issuesLimit = 50
  const [issuesTotal, setIssuesTotal] = createSignal(0)
  const [issuesLoading, setIssuesLoading] = createSignal(false)

  // Track loaded status to enable lazy-loading and in-memory tab caching
  const [readinessLoaded, setReadinessLoaded] = createSignal(false)
  const [feedLoaded, setFeedLoaded] = createSignal(false)
  const [issuesLoaded, setIssuesLoaded] = createSignal(false)

  const loadReadiness = async (isManual = false) => {
    if (isManual) setRefreshing(true)
    setError(null)
    try {
      const data = await getAdminBrokerReadiness()
      setReadiness(data)
      setReadinessLoaded(true)
      const now = new Date()
      setLastUpdated(now.toLocaleTimeString())
    } catch (err: any) {
      setError(err?.message || 'Failed to load broker readiness')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  const loadFeed = async (isManual = false) => {
    if (isManual) setRefreshing(true)
    setError(null)
    try {
      const data = await getAdminExecutionFeed()
      setExecutionFeed(data || [])
      setFeedLoaded(true)
      const now = new Date()
      setLastUpdated(now.toLocaleTimeString())
    } catch (err: any) {
      setError(err?.message || 'Failed to load execution feed')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  const loadIssues = async (offset: number, isManual = false) => {
    if (isManual) setRefreshing(true)
    setIssuesLoading(true)
    setError(null)
    try {
      const res = await getAdminOrderIssues(issuesLimit, offset)
      setOrderIssues(res.issues || [])
      setIssuesTotal(res.total || 0)
      setIssuesOffset(res.offset || 0)
      setIssuesLoaded(true)
      const now = new Date()
      setLastUpdated(now.toLocaleTimeString())
    } catch (err: any) {
      setError(err?.message || 'Failed to load order issues')
    } finally {
      setIssuesLoading(false)
      setRefreshing(false)
      setLoading(false)
    }
  }

  const handleTabChange = (tab: OperationsSubTab) => {
    setActiveTab(tab)
    if (tab === 'readiness' && !readinessLoaded()) {
      loadReadiness(false)
    } else if (tab === 'feed' && !feedLoaded()) {
      loadFeed(false)
    } else if (tab === 'issues' && !issuesLoaded()) {
      loadIssues(issuesOffset(), false)
    }
  }

  const handleRefresh = () => {
    const tab = activeTab()
    if (tab === 'readiness') {
      loadReadiness(true)
    } else if (tab === 'feed') {
      loadFeed(true)
    } else if (tab === 'issues') {
      loadIssues(issuesOffset(), true)
    }
  }

  onMount(() => {
    loadReadiness(false)
  })

  return (
    <AdminLayout activeTab="operations">
      <div class="admin-ops-header" style={{ 'margin-bottom': '1.25rem', display: 'flex', 'justify-content': 'space-between', 'align-items': 'center' }}>
        <div>
          <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
            {lastUpdated() ? `Last updated: ${lastUpdated()}` : 'Awaiting operations data...'}
          </span>
        </div>
        <button
          type="button"
          class="btn-secondary"
          onClick={handleRefresh}
          disabled={refreshing()}
          style={{ display: 'inline-flex', 'align-items': 'center', gap: '0.4rem' }}
        >
          <SyncIcon spinning={refreshing()} />
          <span>{refreshing() ? 'Refreshing...' : 'Refresh'}</span>
        </button>
      </div>

      <Show when={error()}>
        <div style={{ padding: '0.75rem 1rem', background: 'rgba(239, 68, 68, 0.1)', color: '#ef4444', 'border-radius': '6px', 'margin-bottom': '1rem' }}>
          {error()}
        </div>
      </Show>

      {/* Sub-Tabs Navigation */}
      <div class="admin-filter-bar" style={{ 'margin-bottom': '1.25rem' }}>
        <div class="filter-pills">
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeTab() === 'readiness' }}
            onClick={() => handleTabChange('readiness')}
          >
            Pre-Market Broker Readiness
            <Show when={(readiness()?.attentionAccounts?.length ?? 0) > 0}>
              <span class="badge badge-danger" style={{ 'margin-left': '0.4rem', 'font-size': '0.7rem' }}>
                {readiness()!.attentionAccounts.length}
              </span>
            </Show>
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeTab() === 'feed' }}
            onClick={() => handleTabChange('feed')}
          >
            Live Execution Feed
            <Show when={executionFeed().length > 0}>
              <span class="badge badge-success" style={{ 'margin-left': '0.4rem', 'font-size': '0.7rem' }}>
                {executionFeed().length}
              </span>
            </Show>
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeTab() === 'issues' }}
            onClick={() => handleTabChange('issues')}
          >
            Order Issues & Rejections Audit
            <Show when={(issuesTotal() || 0) > 0}>
              <span class="badge badge-warning" style={{ 'margin-left': '0.4rem', 'font-size': '0.7rem' }}>
                {issuesTotal()}
              </span>
            </Show>
          </button>
        </div>
      </div>

      <Show
        when={!loading()}
        fallback={<Skeleton height={280} radius={8} />}
      >
        {/* Sub-Tab 1: Pre-Market Broker Readiness */}
        <Show when={activeTab() === 'readiness'}>
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">Pre-Market Broker Readiness</h4>
              <span class={readiness()?.masterReady && (readiness()?.followersReady === readiness()?.totalFollowers || readiness()?.totalFollowers === 0) ? 'badge badge-success' : 'badge badge-warning'}>
                {readiness()?.masterReady && (readiness()?.followersReady === readiness()?.totalFollowers || readiness()?.totalFollowers === 0) ? 'All Accounts Ready' : 'Action Required'}
              </span>
            </div>

            <div style={{ display: 'grid', 'grid-template-columns': 'repeat(auto-fit, minmax(220px, 1fr))', gap: '1rem', 'margin-bottom': '1rem' }}>
              <div class="telemetry-stat-row">
                <span class="telemetry-stat-label">Master Zerodha Token</span>
                <span class="telemetry-stat-val">
                  <span class={readiness()?.masterReady ? 'badge badge-success' : 'badge badge-danger'}>
                    {readiness()?.masterReady ? 'Ready' : 'Expired / Action Needed'}
                  </span>
                </span>
              </div>
              <div class="telemetry-stat-row">
                <span class="telemetry-stat-label">Followers Authenticated</span>
                <span class="telemetry-stat-val">
                  {readiness() ? `${readiness()!.followersReady} / ${readiness()!.totalFollowers} Ready` : '—'}
                </span>
              </div>
              <div class="telemetry-stat-row">
                <span class="telemetry-stat-label">Attention Needed</span>
                <span class="telemetry-stat-val" style={{ color: (readiness()?.attentionAccounts?.length ?? 0) > 0 ? '#ef4444' : 'inherit' }}>
                  {readiness()?.attentionAccounts?.length ?? 0} Accounts
                </span>
              </div>
            </div>

            <Show
              when={readiness()?.attentionAccounts && readiness()!.attentionAccounts.length > 0}
              fallback={
                <div style={{ padding: '1.5rem', 'text-align': 'center', color: 'var(--color-accent)', 'font-size': '0.9rem', 'border-top': '1px solid var(--color-border)', 'margin-top': '1rem' }}>
                  ✓ All master and follower broker accounts have active sessions and are ready for market execution.
                </div>
              }
            >
              <div style={{ 'margin-top': '1rem', 'border-top': '1px solid var(--color-border)', 'padding-top': '0.75rem' }}>
                <div style={{ 'font-size': '0.85rem', 'font-weight': 600, 'margin-bottom': '0.5rem', color: '#ef4444' }}>
                  Accounts Requiring Morning Login / Refresh:
                </div>
                <div class="admin-table-container">
                  <table class="admin-table" style={{ 'table-layout': 'fixed' }}>
                    <thead>
                      <tr>
                        <th style={{ width: '38%' }}>Account</th>
                        <th style={{ width: '12%', 'text-align': 'center' }}>Role</th>
                        <th style={{ width: '16%', 'text-align': 'center' }}>Auth Status</th>
                        <th style={{ width: '34%' }}>Error / Expiry</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={paginatedAttentionAccounts()}>
                        {(acc) => (
                          <tr>
                            <td style={{ 'font-weight': 600 }}>
                              {acc.name} ({acc.brokerAccountId})
                            </td>
                            <td style={{ 'text-align': 'center' }}>
                              <span class="role-cell-centered">
                                <RoleIcon role={acc.role} />
                              </span>
                            </td>
                            <td style={{ 'text-align': 'center' }}>
                              <span class="auth-status-cell-centered">
                                <AuthStatusIcon
                                  status={acc.authStatus === 'authenticated' ? 'error' : (acc.authStatus || 'error')}
                                  error={acc.authError || 'Token expired / session inactive'}
                                />
                              </span>
                            </td>
                            <td style={{ 'font-size': '0.85rem', color: '#ef4444' }}>
                              {acc.authError || 'Token expired / session inactive'}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>

                <Show when={(readiness()?.attentionAccounts?.length ?? 0) > readinessPageSize}>
                  <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-top': '1rem' }}>
                    <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
                      Showing {(readinessPage() - 1) * readinessPageSize + 1} &ndash;{' '}
                      {Math.min(readinessPage() * readinessPageSize, readiness()!.attentionAccounts.length)} of{' '}
                      {readiness()!.attentionAccounts.length} accounts
                    </span>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      <button
                        type="button"
                        class="btn-secondary"
                        disabled={readinessPage() === 1}
                        onClick={() => setReadinessPage(readinessPage() - 1)}
                      >
                        Previous
                      </button>
                      <button
                        type="button"
                        class="btn-secondary"
                        disabled={readinessPage() * readinessPageSize >= readiness()!.attentionAccounts.length}
                        onClick={() => setReadinessPage(readinessPage() + 1)}
                      >
                        Next
                      </button>
                    </div>
                  </div>
                </Show>
              </div>
            </Show>
          </div>
        </Show>

        {/* Sub-Tab 2: Live Execution Feed */}
        <Show when={activeTab() === 'feed'}>
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">Live Execution Feed</h4>
              <span class="badge badge-neutral">In-Memory (0 DB writes)</span>
            </div>

            <Show
              when={executionFeed().length > 0}
              fallback={
                <div style={{ padding: '2rem', 'text-align': 'center', color: 'var(--color-text-secondary)', 'font-size': '0.85rem' }}>
                  No master fills received during this process run yet.
                </div>
              }
            >
              <div class="admin-table-container">
                <table class="admin-table" style={{ 'table-layout': 'fixed' }}>
                  <thead>
                    <tr>
                      <th style={{ width: '15%' }}>Time</th>
                      <th style={{ width: '20%' }}>Instrument</th>
                      <th style={{ width: '10%' }}>Side</th>
                      <th style={{ width: '12%' }}>Master Qty</th>
                      <th style={{ width: '13%' }}>Price</th>
                      <th style={{ width: '15%' }}>Fan-Out</th>
                      <th style={{ width: '15%' }}>Latency</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={paginatedFeed()}>
                      {(ev) => (
                        <tr>
                          <td style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
                            {new Date(ev.timestamp).toLocaleTimeString()}
                          </td>
                          <td style={{ 'font-weight': 600 }}>
                            {ev.tradingsymbol} <span style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>{ev.exchange}</span>
                          </td>
                          <td>
                            <span class={ev.transactionType === 'BUY' ? 'badge badge-success' : 'badge badge-danger'}>
                              {ev.transactionType}
                            </span>
                          </td>
                          <td>{ev.masterQty}</td>
                          <td>{ev.price > 0 ? ev.price.toFixed(2) : 'MKT'}</td>
                          <td>
                            <span class="badge badge-success">{ev.followersCount} placed</span>
                            <Show when={ev.skippedCount > 0}>
                              <span class="badge badge-warning" style={{ 'margin-left': '0.3rem' }}>{ev.skippedCount} skipped</span>
                            </Show>
                          </td>
                          <td style={{ 'font-weight': 600, color: ev.latencyMs > 50 ? '#f59e0b' : 'var(--color-accent)' }}>
                            {ev.latencyMs} ms
                          </td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </div>

              <Show when={executionFeed().length > feedPageSize}>
                <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-top': '1rem' }}>
                  <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
                    Showing {(feedPage() - 1) * feedPageSize + 1} &ndash;{' '}
                    {Math.min(feedPage() * feedPageSize, executionFeed().length)} of{' '}
                    {executionFeed().length} events
                  </span>
                  <div style={{ display: 'flex', gap: '0.5rem' }}>
                    <button
                      type="button"
                      class="btn-secondary"
                      disabled={feedPage() === 1}
                      onClick={() => setFeedPage(feedPage() - 1)}
                    >
                      Previous
                    </button>
                    <button
                      type="button"
                      class="btn-secondary"
                      disabled={feedPage() * feedPageSize >= executionFeed().length}
                      onClick={() => setFeedPage(feedPage() + 1)}
                    >
                      Next
                    </button>
                  </div>
                </div>
              </Show>
            </Show>
          </div>
        </Show>

        {/* Sub-Tab 3: Order Issues & Rejections Audit */}
        <Show when={activeTab() === 'issues'}>
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">Order Issues & Rejections Audit</h4>
              <span class={(issuesTotal() || 0) > 0 ? 'badge badge-warning' : 'badge badge-success'}>
                {(issuesTotal() || 0) > 0 ? `${issuesTotal()} Discrepancies` : 'Zero Issues'}
              </span>
            </div>

            <Show
              when={!issuesLoading()}
              fallback={<Skeleton height={200} radius={8} />}
            >
              <Show
                when={(orderIssues() ?? []).length > 0}
                fallback={
                  <div style={{ padding: '2rem', 'text-align': 'center', color: 'var(--color-text-secondary)', 'font-size': '0.85rem' }}>
                    Zero order issues recorded in the audit window.
                  </div>
                }
              >
                <div class="admin-table-container">
                  <table class="admin-table" style={{ 'table-layout': 'fixed' }}>
                    <thead>
                      <tr>
                        <th style={{ width: '14%' }}>Time</th>
                        <th style={{ width: '22%' }}>Follower</th>
                        <th style={{ width: '18%' }}>Instrument</th>
                        <th style={{ width: '12%' }}>Intended / Placed</th>
                        <th style={{ width: '14%' }}>Status</th>
                        <th style={{ width: '20%' }}>Reason / Error</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={orderIssues() || []}>
                        {(issue) => (
                          <tr>
                            <td style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
                              {issue.createdAt ? new Date(issue.createdAt).toLocaleTimeString() : '—'}
                            </td>
                            <td style={{ 'font-weight': 600 }}>
                              {issue.followerName || 'Follower'} ({issue.brokerAccountId || '—'})
                            </td>
                            <td>
                              {issue.tradingsymbol || '—'}{' '}
                              <span style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>
                                {issue.exchange || ''}
                              </span>
                            </td>
                            <td>
                              {issue.intendedQty} &rarr; {issue.placedQty}
                            </td>
                            <td>
                              <span class={issue.terminalStatus === 'SKIPPED' ? 'badge badge-warning' : 'badge badge-danger'}>
                                {issue.terminalStatus || 'FAILED'}
                              </span>
                            </td>
                            <td style={{ 'font-size': '0.85rem', color: issue.terminalStatus === 'SKIPPED' ? '#f59e0b' : '#ef4444' }}>
                              {issue.lastError || issue.sizingReason || 'Discrepancy'}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>

                <Show when={(issuesTotal() || 0) > issuesLimit}>
                  <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-top': '1rem' }}>
                    <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
                      Showing {issuesOffset() + 1} &ndash;{' '}
                      {Math.min(issuesOffset() + issuesLimit, issuesTotal() || 0)} of{' '}
                      {issuesTotal()} issues
                    </span>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      <button
                        type="button"
                        class="btn-secondary"
                        disabled={issuesOffset() === 0 || issuesLoading()}
                        onClick={() => loadIssues(Math.max(0, issuesOffset() - issuesLimit))}
                      >
                        Previous
                      </button>
                      <button
                        type="button"
                        class="btn-secondary"
                        disabled={issuesOffset() + issuesLimit >= (issuesTotal() || 0) || issuesLoading()}
                        onClick={() => loadIssues(issuesOffset() + issuesLimit)}
                      >
                        Next
                      </button>
                    </div>
                  </div>
                </Show>
              </Show>
            </Show>
          </div>
        </Show>
      </Show>
    </AdminLayout>
  )
}
