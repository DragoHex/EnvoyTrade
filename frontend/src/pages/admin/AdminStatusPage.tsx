import { createSignal, onMount, Show, For } from 'solid-js'
import { AdminLayout } from './AdminLayout'
import {
  getAdminServiceStatus,
  postAdminKillSwitch,
  getAdminInstrumentStatus,
  postAdminInstrumentSync,
  type PipelineStatus,
  type ScripMasterSummary,
} from '../../api'
import { SyncIcon } from '../../components/icons'
import { BrokerLogo } from '../../components/BrokerLogo'
import { Skeleton } from '../../components/Skeleton'
import { useEscapeKey } from '../../utils/useEscapeKey'

export function AdminStatusPage() {
  const [status, setStatus] = createSignal<PipelineStatus | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [refreshing, setRefreshing] = createSignal(false)
  const [lastUpdated, setLastUpdated] = createSignal<string>('')
  const [error, setError] = createSignal<string | null>(null)

  // On-demand Scrip Master state (strictly user-triggered / on-mount, NO periodic interval)
  const [scripStatus, setScripStatus] = createSignal<ScripMasterSummary | null>(null)
  const [scripLoading, setScripLoading] = createSignal(true)
  const [scripRefreshing, setScripRefreshing] = createSignal(false)
  const [scripSyncing, setScripSyncing] = createSignal<string | null>(null)
  const [scripNotice, setScripNotice] = createSignal<string | null>(null)
  const [scripError, setScripError] = createSignal<string | null>(null)

  // Kill Switch Modal state
  const [killModalOpen, setKillModalOpen] = createSignal(false)
  const [killActionEngage, setKillActionEngage] = createSignal(true)
  const [killReason, setKillReason] = createSignal('')
  const [killSubmitting, setKillSubmitting] = createSignal(false)

  useEscapeKey({
    enabled: killModalOpen,
    canDismiss: () => !killSubmitting(),
    onEscape: () => setKillModalOpen(false),
  })

  const loadStatus = async (isManual = false) => {
    if (isManual) {
      setRefreshing(true)
    }
    setError(null)
    try {
      const data = await getAdminServiceStatus()
      setStatus(data)
      const now = new Date()
      setLastUpdated(now.toLocaleTimeString())
    } catch (err: any) {
      setError(err?.message || 'Failed to load telemetry status')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  const loadScripStatus = async (isManual = false) => {
    if (isManual) {
      setScripRefreshing(true)
    }
    setScripError(null)
    try {
      const data = await getAdminInstrumentStatus()
      setScripStatus(data)
    } catch (err: any) {
      setScripError(err?.message || 'Failed to load instrument master status')
    } finally {
      setScripLoading(false)
      setScripRefreshing(false)
    }
  }

  const handleSyncScrips = async (broker = 'all') => {
    setScripSyncing(broker)
    setScripNotice(null)
    setScripError(null)
    try {
      await postAdminInstrumentSync(broker)
      setScripNotice(
        `Sync for ${broker.toUpperCase()} initiated in background. Ingestion takes ~15–30s. Click "Refresh" to inspect updated counts.`
      )
    } catch (err: any) {
      setScripError(err?.message || `Failed to trigger sync for ${broker}`)
    } finally {
      setScripSyncing(null)
    }
  }

  onMount(() => {
    loadStatus(false)
    loadScripStatus(false)
  })

  const openKillModal = (engage: boolean) => {
    setKillActionEngage(engage)
    setKillReason('')
    setKillModalOpen(true)
  }

  const handleKillSwitchSubmit = async (e: Event) => {
    e.preventDefault()
    setKillSubmitting(true)
    try {
      const res = await postAdminKillSwitch(killActionEngage(), killReason().trim())
      if (status()) {
        setStatus({
          ...status()!,
          killSwitch: res.killSwitch,
        })
      }
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('killswitch:change', { detail: res.killSwitch }))
      }
      setKillModalOpen(false)
      await loadStatus(false)
    } catch (err: any) {
      setError(err?.message || 'Failed to toggle kill switch')
    } finally {
      setKillSubmitting(false)
    }
  }

  return (
    <AdminLayout activeTab="status">
      <div class="admin-status-header" style={{ 'margin-bottom': '1rem', display: 'flex', 'justify-content': 'space-between', 'align-items': 'center' }}>
        <div>
          <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
            {lastUpdated() ? `Last updated: ${lastUpdated()}` : 'Awaiting status...'}
          </span>
        </div>
        <button
          type="button"
          class="btn-secondary"
          onClick={() => loadStatus(true)}
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

      <Show
        when={!loading() && status()}
        fallback={
          <div>
            <Skeleton height={90} radius={8} />
            <div style={{ height: '1.5rem' }} />
            <div class="telemetry-grid">
              <Skeleton height={140} radius={8} />
              <Skeleton height={140} radius={8} />
              <Skeleton height={140} radius={8} />
              <Skeleton height={140} radius={8} />
              <Skeleton height={140} radius={8} />
            </div>
          </div>
        }
      >
        {/* Emergency Kill Switch Banner / Card */}
        <div classList={{ 'killswitch-card': true, engaged: status()!.killSwitch.engaged }}>
          <div classList={{ 'killswitch-info': true, engaged: status()!.killSwitch.engaged }}>
            <h3>
              <span style={{ display: 'inline-block', width: '10px', height: '10px', 'border-radius': '50%', background: status()!.killSwitch.engaged ? '#ef4444' : 'var(--color-accent)' }} />
              {status()!.killSwitch.engaged ? 'EMERGENCY KILL SWITCH ENGAGED' : 'Copy Engine Operational'}
            </h3>
            <Show when={status()!.killSwitch.engaged}>
              <div class="killswitch-reason-box">
                <div><strong>Reason:</strong> {status()!.killSwitch.reason || 'None provided'}</div>
                <div style={{ 'margin-top': '0.2rem', 'font-size': '0.75rem' }}>
                  Triggered by: {status()!.killSwitch.engagedBy || 'Unknown'} at{' '}
                  {status()!.killSwitch.engagedAt ? new Date(status()!.killSwitch.engagedAt!).toLocaleString() : 'N/A'}
                </div>
              </div>
            </Show>
          </div>
          <div>
            <Show
              when={status()!.killSwitch.engaged}
              fallback={
                <button
                  type="button"
                  class="btn-danger"
                  onClick={() => openKillModal(true)}
                >
                  Engage Kill Switch
                </button>
              }
            >
              <button
                type="button"
                class="btn-secondary"
                onClick={() => openKillModal(false)}
                style={{ 'border-color': '#ef4444', color: '#ef4444' }}
              >
                Disengage Kill Switch
              </button>
            </Show>
          </div>
        </div>

        {/* Telemetry Grid */}
        <div class="telemetry-grid">
          {/* Card 1: Master Tickers */}
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">Master WebSocket Tickers</h4>
              <span class="badge badge-success">
                {status()!.masterTickers.length} Active
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Active Connections</span>
              <span class="telemetry-stat-val">{status()!.masterTickers.length}</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Stream Status</span>
              <span class="telemetry-stat-val">
                {status()!.masterTickers.some((t) => t.active) ? (
                  <span style={{ color: 'var(--color-accent)' }}>Connected</span>
                ) : (
                  <span style={{ color: 'var(--color-text-secondary)' }}>Idle</span>
                )}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Fallback Mode</span>
              <span class="telemetry-stat-val">Polling Reconciler</span>
            </div>
          </div>

          {/* Card 2: Queues */}
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">In-Memory Queues</h4>
              <span class="badge badge-success">Healthy</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Master Fill Queue</span>
              <span class="telemetry-stat-val">
                {status()!.queues.masterFillDepth} / {status()!.queues.masterFillCap}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Order Update Queue</span>
              <span class="telemetry-stat-val">
                {status()!.queues.orderUpdateDepth} / {status()!.queues.orderUpdateCap}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Drop Policy</span>
              <span class="telemetry-stat-val">Outbox Redelivery</span>
            </div>
          </div>

          {/* Card 3: Worker Pool */}
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">Follower Worker Pool</h4>
              <span class="badge badge-success">
                {status()!.workerCount} Workers
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Dedicated Goroutines</span>
              <span class="telemetry-stat-val">{status()!.workerCount}</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Egress Isolation</span>
              <span class="telemetry-stat-val">Static Proxy 1:1</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Rate Limiter</span>
              <span class="telemetry-stat-val">~10 orders/s per acct</span>
            </div>
          </div>

          {/* Card 4: Postgres Pool */}
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">PostgreSQL Connection Pool</h4>
              <span class="badge badge-neutral">pgxpool</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Acquired (Active)</span>
              <span class="telemetry-stat-val">{status()!.dbPool.acquiredConns}</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Idle Connections</span>
              <span class="telemetry-stat-val">{status()!.dbPool.idleConns}</span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Total / Max Conns</span>
              <span class="telemetry-stat-val">
                {status()!.dbPool.totalConns} / {status()!.dbPool.maxConns}
              </span>
            </div>
          </div>

          {/* Card 5: System Resources */}
          <div class="telemetry-card">
            <div class="telemetry-card-header">
              <h4 class="telemetry-card-title">System Resources (Process)</h4>
              <span class={(status()!.systemResources?.cpuPercent ?? 0) > 80 ? 'badge badge-warning' : 'badge badge-success'}>
                {(status()!.systemResources?.cpuPercent ?? 0) > 80 ? 'High Load' : 'Healthy'}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">CPU Utilization</span>
              <span class="telemetry-stat-val">
                {status()!.systemResources?.cpuPercent != null ? `${status()!.systemResources!.cpuPercent.toFixed(1)}%` : '—'}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Heap / Virtual Memory</span>
              <span class="telemetry-stat-val">
                {status()!.systemResources?.heapAllocMb != null
                  ? `${status()!.systemResources!.heapAllocMb.toFixed(1)} MB / ${(status()!.systemResources!.sysMemoryMb ?? 0).toFixed(1)} MB`
                  : '—'}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">Active Goroutines</span>
              <span class="telemetry-stat-val">
                {status()!.systemResources ? status()!.systemResources!.goroutineCount : '—'}
              </span>
            </div>
            <div class="telemetry-stat-row">
              <span class="telemetry-stat-label">GC Cycles</span>
              <span class="telemetry-stat-val">
                {status()!.systemResources ? status()!.systemResources!.gcCycles : '—'}
              </span>
            </div>
          </div>
        </div>

        {/* Scrip Master & In-Memory Cache Card */}
        <div style={{ 'margin-top': '1.5rem' }}>
          <div class="telemetry-card" style={{ padding: '1.25rem' }}>
            <div class="telemetry-card-header" style={{ 'margin-bottom': '1rem' }}>
              <div>
                <h4 class="telemetry-card-title" style={{ 'font-size': '1.05rem', display: 'flex', 'align-items': 'center', gap: '0.6rem' }}>
                  <span>Scrip Master & Instrument Cache</span>
                  <Show when={scripStatus()}>
                    <span
                      class={
                        scripStatus()!.status === 'healthy'
                          ? 'badge badge-success'
                          : scripStatus()!.status === 'warning'
                          ? 'badge badge-warning'
                          : 'badge badge-danger'
                      }
                    >
                      {scripStatus()!.status === 'healthy'
                        ? 'Healthy'
                        : scripStatus()!.status === 'warning'
                        ? 'Warning'
                        : 'Degraded / Empty'}
                    </span>
                  </Show>
                </h4>
                <div style={{ 'font-size': '0.8rem', color: 'var(--color-text-secondary)', 'margin-top': '0.2rem' }}>
                  Unified scrip master feeds & 10,000 capacity pre-allocated F&O cache for cross-broker symbol translation (On-Demand).
                </div>
              </div>
              <div style={{ display: 'flex', gap: '0.4rem', 'align-items': 'center' }}>
                <button
                  type="button"
                  class="btn-secondary"
                  onClick={() => loadScripStatus(true)}
                  disabled={scripRefreshing()}
                  style={{
                    display: 'inline-flex',
                    'align-items': 'center',
                    gap: '0.35rem',
                    'font-size': '0.75rem',
                    padding: '0.25rem 0.55rem',
                    'line-height': '1.2',
                  }}
                >
                  <span style={{ width: '13px', height: '13px', display: 'inline-flex', 'align-items': 'center' }}>
                    <SyncIcon spinning={scripRefreshing()} />
                  </span>
                  <span>{scripRefreshing() ? 'Refreshing...' : 'Refresh'}</span>
                </button>
                <button
                  type="button"
                  class="btn-secondary"
                  onClick={() => handleSyncScrips('all')}
                  disabled={scripSyncing() !== null}
                  style={{
                    display: 'inline-flex',
                    'align-items': 'center',
                    gap: '0.35rem',
                    'font-size': '0.75rem',
                    padding: '0.25rem 0.55rem',
                    'line-height': '1.2',
                  }}
                  title="Trigger on-demand background sync for all broker scrip masters"
                >
                  <span style={{ width: '13px', height: '13px', display: 'inline-flex', 'align-items': 'center' }}>
                    <SyncIcon spinning={scripSyncing() === 'all'} />
                  </span>
                  <span>{scripSyncing() === 'all' ? 'Syncing...' : 'Sync All'}</span>
                </button>
              </div>
            </div>

            <Show when={scripNotice()}>
              <div
                style={{
                  padding: '0.65rem 0.9rem',
                  background: 'rgba(4, 180, 136, 0.1)',
                  color: 'var(--color-accent)',
                  'border-radius': '6px',
                  'margin-bottom': '1rem',
                  'font-size': '0.825rem',
                }}
              >
                {scripNotice()}
              </div>
            </Show>

            <Show when={scripError()}>
              <div
                style={{
                  padding: '0.65rem 0.9rem',
                  background: 'rgba(239, 68, 68, 0.1)',
                  color: '#ef4444',
                  'border-radius': '6px',
                  'margin-bottom': '1rem',
                  'font-size': '0.825rem',
                }}
              >
                {scripError()}
              </div>
            </Show>

            <Show when={scripStatus()?.alerts && scripStatus()!.alerts.length > 0}>
              <div
                style={{
                  padding: '0.65rem 0.9rem',
                  background: 'rgba(245, 158, 11, 0.12)',
                  color: '#f59e0b',
                  'border-radius': '6px',
                  'margin-bottom': '1rem',
                  'font-size': '0.825rem',
                }}
              >
                <strong>Scrip Master Alerts:</strong>
                <ul style={{ margin: '0.3rem 0 0 1.2rem', padding: 0 }}>
                  <For each={scripStatus()!.alerts}>
                    {(alert) => <li>{alert}</li>}
                  </For>
                </ul>
              </div>
            </Show>

            <Show
              when={!scripLoading() && scripStatus()}
              fallback={
                <div style={{ padding: '0.5rem 0' }}>
                  <Skeleton height={35} radius={6} />
                  <div style={{ height: '0.75rem' }} />
                  <Skeleton height={110} radius={6} />
                </div>
              }
            >
              {/* In-memory Cache Telemetry */}
              <div
                style={{
                  background: 'var(--color-bg, #0f1117)',
                  border: '1px solid var(--color-border)',
                  'border-radius': '6px',
                  padding: '0.75rem 1rem',
                  'margin-bottom': '1rem',
                  display: 'flex',
                  'flex-wrap': 'wrap',
                  gap: '1.5rem',
                  'align-items': 'center',
                }}
              >
                <div>
                  <div style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)', 'margin-bottom': '0.2rem' }}>
                    Active In-Memory F&O Cache
                  </div>
                  <div style={{ 'font-weight': 700, 'font-size': '1.05rem', color: 'var(--color-text)' }}>
                    {scripStatus()!.cache.activeCount.toLocaleString()} / {scripStatus()!.cache.capacity.toLocaleString()}
                    <span style={{ 'font-size': '0.8rem', 'font-weight': 500, color: 'var(--color-text-secondary)', 'margin-left': '0.4rem' }}>
                      ({Math.round((scripStatus()!.cache.activeCount / (scripStatus()!.cache.capacity || 10000)) * 100)}% capacity)
                    </span>
                  </div>
                </div>
                <div>
                  <div style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)', 'margin-bottom': '0.2rem' }}>
                    Last Boot/Routine Auto-Load
                  </div>
                  <div style={{ 'font-weight': 600, 'font-size': '0.9rem', color: 'var(--color-text)' }}>
                    {scripStatus()!.cache.lastAutoLoad
                      ? new Date(scripStatus()!.cache.lastAutoLoad!).toLocaleString()
                      : 'Pending morning routine (05:00 AM IST)'}
                  </div>
                </div>
              </div>

              {/* Broker Scrip Masters Table */}
              <div class="admin-table-container" style={{ border: '1px solid var(--color-border)', 'border-radius': '6px', overflow: 'hidden' }}>
                <table class="admin-table" style={{ width: '100%', 'border-collapse': 'collapse', 'table-layout': 'fixed' }}>
                  <thead>
                    <tr style={{ background: 'var(--color-bg, #0f1117)', 'border-bottom': '1px solid var(--color-border)' }}>
                      <th style={{ 'text-align': 'left', padding: '0.6rem 0.75rem', width: '25%', 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>Broker Feed</th>
                      <th style={{ 'text-align': 'left', padding: '0.6rem 0.75rem', width: '22%', 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>Total Scrips</th>
                      <th style={{ 'text-align': 'left', padding: '0.6rem 0.75rem', width: '28%', 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>Last Ingestion</th>
                      <th style={{ 'text-align': 'left', padding: '0.6rem 0.75rem', width: '13%', 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>Health</th>
                      <th style={{ 'text-align': 'right', padding: '0.6rem 0.75rem', width: '12%', 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={['kite', 'dhan', 'kotakneo']}>
                      {(brokerKey) => {
                        const b = () => scripStatus()!.brokers[brokerKey] || { broker: brokerKey, count: 0, status: 'empty' }
                        return (
                          <tr style={{ 'border-bottom': '1px solid var(--color-border)' }}>
                            <td style={{ padding: '0.65rem 0.75rem', 'vertical-align': 'middle' }}>
                              <BrokerLogo broker={brokerKey} showName={true} size={16} />
                            </td>
                            <td style={{ padding: '0.65rem 0.75rem', 'font-weight': 600, 'vertical-align': 'middle', 'font-size': '0.85rem' }}>
                              {b().count.toLocaleString()} contracts
                            </td>
                            <td style={{ padding: '0.65rem 0.75rem', color: 'var(--color-text-secondary)', 'vertical-align': 'middle', 'font-size': '0.8rem' }}>
                              {b().lastRefreshed ? new Date(b().lastRefreshed!).toLocaleString() : 'Never'}
                            </td>
                            <td style={{ padding: '0.65rem 0.75rem', 'vertical-align': 'middle' }}>
                              <span
                                class={
                                  b().status === 'healthy'
                                    ? 'badge badge-success'
                                    : b().status === 'stale'
                                    ? 'badge badge-warning'
                                    : 'badge badge-danger'
                                }
                              >
                                {b().status === 'healthy'
                                  ? 'Healthy'
                                  : b().status === 'stale'
                                  ? 'Stale (>24h)'
                                  : 'Empty'}
                              </span>
                            </td>
                            <td style={{ padding: '0.65rem 0.75rem', 'text-align': 'right', 'vertical-align': 'middle' }}>
                              <button
                                type="button"
                                class="btn-secondary"
                                onClick={() => handleSyncScrips(brokerKey)}
                                disabled={scripSyncing() !== null}
                                style={{ 'font-size': '0.75rem', padding: '0.2rem 0.55rem' }}
                                title={`Download and ingest ${brokerKey.toUpperCase()} scrip master`}
                              >
                                {scripSyncing() === brokerKey ? 'Syncing...' : 'Sync'}
                              </button>
                            </td>
                          </tr>
                        )
                      }}
                    </For>
                  </tbody>
                </table>
              </div>
            </Show>
          </div>
        </div>
      </Show>

      {/* Kill Switch Toggle Modal */}
      <Show when={killModalOpen()}>
        <div class="modal-overlay" role="dialog" aria-modal="true" aria-label="Toggle Kill Switch">
          <div class="modal-card">
            <div class="modal-header-row">
              <h3>{killActionEngage() ? 'Engage Emergency Kill Switch' : 'Disengage Kill Switch'}</h3>
              <button
                type="button"
                onClick={() => setKillModalOpen(false)}
                style={{ background: 'transparent', border: 'none', cursor: 'pointer', 'font-size': '1.25rem', color: 'var(--color-text-secondary)' }}
              >
                &times;
              </button>
            </div>
            <form onSubmit={handleKillSwitchSubmit}>
              <div class="modal-body">
                <Show when={killActionEngage()}>
                  <div style={{ 'margin-top': '1rem' }}>
                    <label style={{ display: 'block', 'font-size': '0.85rem', 'font-weight': 600, 'margin-bottom': '0.4rem', color: 'var(--color-text)' }}>
                      Audit Reason
                    </label>
                    <input
                      type="text"
                      required
                      placeholder="e.g. Market volatility breaker, Broker maintenance, Emergency pause"
                      value={killReason()}
                      onInput={(e) => setKillReason(e.currentTarget.value)}
                      style={{
                        width: '100%',
                        padding: '0.5rem 0.75rem',
                        border: '1px solid var(--color-border)',
                        'border-radius': '6px',
                        background: 'var(--color-surface)',
                        color: 'var(--color-text)',
                        'box-sizing': 'border-box',
                      }}
                    />
                  </div>
                </Show>
              </div>
              <div class="modal-footer">
                <button
                  type="button"
                  class="btn-secondary"
                  onClick={() => setKillModalOpen(false)}
                  disabled={killSubmitting()}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  class={killActionEngage() ? 'btn-danger' : 'btn-primary'}
                  disabled={killSubmitting()}
                >
                  {killSubmitting()
                    ? 'Processing...'
                    : killActionEngage()
                    ? 'Engage Kill Switch'
                    : 'Disengage Kill Switch'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Show>
    </AdminLayout>
  )
}
