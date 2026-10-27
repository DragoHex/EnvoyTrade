import { createSignal, onMount, Show, For } from 'solid-js'
import { AdminLayout } from './AdminLayout'
import {
  getAdminProxyIPs,
  postAdminProxyIPsBulk,
  postAdminProxyIP,
  putAdminProxyIP,
  deleteAdminProxyIP,
  testAdminProxyIP,
  testAllAdminProxyIPs,
  type AdminProxyIP,
  type ProxyIPStats,
  type BulkUploadResult,
  type ProxyTestResult,
  type BulkProxyTestSummary,
} from '../../api'
import { SyncIcon, EditIcon, TrashIcon } from '../../components/icons'
import { Skeleton } from '../../components/Skeleton'
import { BrokerLogo } from '../../components/BrokerLogo'
import { ThemedSelect } from '../../components/ThemedSelect'
import { ThemedNumberInput } from '../../components/ThemedNumberInput'
import { useEscapeKey } from '../../utils/useEscapeKey'


export const WHITELISTED_BROKER_OPTIONS = [
  { value: 'Zerodha', label: 'Zerodha' },
  { value: 'Dhan', label: 'Dhan' },
  { value: 'TestBroker', label: 'TestBroker' },
  { value: '', label: 'All Brokers (Unrestricted)' },
]

export const PLAN_OPTIONS = [
  { value: 'MONTHLY', label: 'MONTHLY' },
  { value: 'QUARTERLY', label: 'QUARTERLY' },
  { value: 'HALF_YEARLY', label: 'HALF_YEARLY' },
  { value: 'YEARLY', label: 'YEARLY' },
]

export const IP_TYPE_OPTIONS = [
  { value: 'IPV4', label: 'IPv4' },
  { value: 'IPV6', label: 'IPv6' },
]

export function getWhitelistedBrokerSelectOptions(currentVal?: string) {
  const options = [
    { value: 'Zerodha', label: 'Zerodha', badge: { text: 'Specialized', variant: 'emerald' as const } },
    { value: 'Dhan', label: 'Dhan', badge: { text: 'Specialized', variant: 'emerald' as const } },
    { value: 'TestBroker', label: 'TestBroker', badge: { text: 'Specialized', variant: 'emerald' as const } },
    { value: '', label: 'All Brokers (Unrestricted)', badge: { text: 'Unrestricted', variant: 'slate' as const } },
  ]
  if (currentVal && !options.some((o) => o.value === currentVal)) {
    options.unshift({
      value: currentVal,
      label: currentVal,
      badge: { text: 'Specialized', variant: 'emerald' as const },
    })
  }
  return options
}

export function AdminIPsPage() {
  const [allIps, setAllIps] = createSignal<AdminProxyIP[]>([])
  const [stats, setStats] = createSignal<ProxyIPStats | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [refreshing, setRefreshing] = createSignal(false)
  const [activeFilter, setActiveFilter] = createSignal<string>('all')
  const [error, setError] = createSignal<string | null>(null)

  // Derived in-memory filtered list (zero network latency, zero DB queries)
  const filteredIps = () => {
    const f = activeFilter()
    const list = allIps()
    if (f === 'all') return list
    if (f === 'active') return list.filter((ip) => ip.expiryStatus === 'active')
    if (f === 'expiring_soon') return list.filter((ip) => ip.expiryStatus === 'expiring_soon')
    if (f === 'expired') return list.filter((ip) => ip.expiryStatus === 'expired')
    if (f === 'ipv4') return list.filter((ip) => ip.ipType.toLowerCase() === 'ipv4')
    if (f === 'ipv6') return list.filter((ip) => ip.ipType.toLowerCase() === 'ipv6')
    return list
  }

  // Pagination state (20 IPs / page)
  const [page, setPage] = createSignal(1)
  const pageSize = 20
  const paginatedIps = () => filteredIps().slice((page() - 1) * pageSize, page() * pageSize)

  // Testing probe state
  const [testingIp, setTestingIp] = createSignal<string | null>(null)
  const [testResults, setTestResults] = createSignal<Record<string, ProxyTestResult>>({})
  const [testingAll, setTestingAll] = createSignal(false)
  const [bulkProbeSummary, setBulkProbeSummary] = createSignal<BulkProxyTestSummary | null>(null)

  // Bulk Ingest Modal state
  const [bulkModalOpen, setBulkModalOpen] = createSignal(false)
  const [bulkText, setBulkText] = createSignal('')
  const [bulkSubmitting, setBulkSubmitting] = createSignal(false)
  const [bulkResult, setBulkResult] = createSignal<BulkUploadResult | null>(null)
  const [bulkParseError, setBulkParseError] = createSignal<string | null>(null)

  // Edit / Renew Modal state
  const [editModalOpen, setEditModalOpen] = createSignal(false)
  const [editingIp, setEditingIp] = createSignal<AdminProxyIP | null>(null)
  const [editMode, setEditMode] = createSignal<'form' | 'json'>('form')
  const [editHost, setEditHost] = createSignal('')
  const [editPort, setEditPort] = createSignal(443)
  const [editUsername, setEditUsername] = createSignal('')
  const [editType, setEditType] = createSignal('IPV4')
  const [editWhitelistedBrokers, setEditWhitelistedBrokers] = createSignal('')
  const [editValidUntil, setEditValidUntil] = createSignal('')
  const [editPlan, setEditPlan] = createSignal('QUARTERLY')
  const [editPassword, setEditPassword] = createSignal('')
  const [editJsonText, setEditJsonText] = createSignal('')
  const [editJsonError, setEditJsonError] = createSignal<string | null>(null)
  const [editSubmitting, setEditSubmitting] = createSignal(false)

  // Delete Modal state
  const [deleteModalOpen, setDeleteModalOpen] = createSignal(false)
  const [deletingIp, setDeletingIp] = createSignal<AdminProxyIP | null>(null)
  const [deleteSubmitting, setDeleteSubmitting] = createSignal(false)

  // Single Add Modal state
  const [addModalOpen, setAddModalOpen] = createSignal(false)
  const [addMode, setAddMode] = createSignal<'form' | 'json'>('form')
  const [addJsonText, setAddJsonText] = createSignal('')
  const [addJsonError, setAddJsonError] = createSignal<string | null>(null)
  const [addForm, setAddForm] = createSignal({
    ip: '',
    host: 'dc6-mum-01.algoip.in',
    port: 443,
    user_id: '',
    password: '',
    valid_until: '',
    plan: 'QUARTERLY',
    type: 'IPV4',
    whitelisted_brokers: 'Zerodha',
  })
  const [addSubmitting, setAddSubmitting] = createSignal(false)

  // Escape key handlers for admin IP modals
  useEscapeKey({
    enabled: bulkModalOpen,
    canDismiss: () => !bulkSubmitting(),
    onEscape: () => setBulkModalOpen(false),
  })

  useEscapeKey({
    enabled: () => editModalOpen() && !!editingIp(),
    canDismiss: () => !editSubmitting(),
    onEscape: () => setEditModalOpen(false),
  })

  useEscapeKey({
    enabled: () => deleteModalOpen() && !!deletingIp(),
    canDismiss: () => !deleteSubmitting(),
    onEscape: () => setDeleteModalOpen(false),
  })

  useEscapeKey({
    enabled: addModalOpen,
    canDismiss: () => !addSubmitting(),
    onEscape: () => setAddModalOpen(false),
  })

  const [lastUpdated, setLastUpdated] = createSignal<string>('')

  const loadData = async (isManual = false) => {
    if (isManual) {
      setRefreshing(true)
    }
    setError(null)
    try {
      const res = await getAdminProxyIPs()
      setAllIps(res.ips || [])
      setStats(res.stats)
      const now = new Date()
      setLastUpdated(now.toLocaleTimeString())
    } catch (err: any) {
      setError(err?.message || 'Failed to load proxy IPs')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  onMount(() => {
    loadData(false)
  })

  const setFilter = (f: string) => {
    setActiveFilter(f)
    setPage(1)
  }

  // Probe single IP
  const handleTestProbe = async (ip: string) => {
    setTestingIp(ip)
    try {
      const res = await testAdminProxyIP(ip)
      setTestResults((prev) => ({ ...prev, [ip]: res }))
    } catch (err: any) {
      setTestResults((prev) => ({
        ...prev,
        [ip]: {
          ipAddress: ip,
          success: false,
          latencyMs: 0,
          error: err?.message || 'Probe failed',
        },
      }))
    } finally {
      setTestingIp(null)
    }
  }

  // Pre-flight probe all assigned proxies
  const handleTestAllProxies = async () => {
    setTestingAll(true)
    setError(null)
    try {
      const summary = await testAllAdminProxyIPs()
      setBulkProbeSummary(summary)
      if (summary.results) {
        const map = { ...testResults() }
        for (const res of summary.results) {
          map[res.ipAddress] = res
        }
        setTestResults(map)
      }
    } catch (err: any) {
      setError(err?.message || 'Failed to test all proxies')
    } finally {
      setTestingAll(false)
    }
  }

  // Edit / Renew
  const syncEditFormToJson = () => {
    if (!editingIp()) return
    const obj = {
      ip: editingIp()!.ipAddress,
      host: editHost().trim(),
      port: editPort(),
      user_id: editUsername().trim(),
      password: editPassword(),
      valid_until: editValidUntil() ? `${editValidUntil()} 23:59:59 (IST)` : '',
      plan: editPlan(),
      type: editType(),
      whitelisted_brokers: editWhitelistedBrokers().trim(),
    }
    setEditJsonText(JSON.stringify(obj, null, 2))
  }

  const syncJsonToEditForm = () => {
    const v = editJsonValidation()
    if (v.valid && v.parsed) {
      const p = v.parsed
      if (p.host !== undefined) setEditHost(String(p.host).trim())
      if (p.port !== undefined) setEditPort(Number(p.port) || 443)
      if (p.user_id !== undefined || p.username !== undefined) {
        setEditUsername(String(p.user_id || p.username || '').trim())
      }
      if (p.password !== undefined) setEditPassword(String(p.password))
      if (p.plan !== undefined) setEditPlan(String(p.plan))
      if (p.type !== undefined) setEditType(String(p.type).toUpperCase())
      if (p.whitelisted_brokers !== undefined || p.whitelistedBrokers !== undefined) {
        setEditWhitelistedBrokers(String(p.whitelisted_brokers ?? p.whitelistedBrokers ?? '').trim())
      }
      if (p.valid_until) {
        const d = new Date(p.valid_until)
        if (!isNaN(d.getTime())) {
          setEditValidUntil(d.toISOString().slice(0, 10))
        }
      }
    }
  }

  const openEditModal = (ip: AdminProxyIP) => {
    setEditingIp(ip)
    setEditJsonError(null)
    // Convert validUntil to input format YYYY-MM-DD
    if (ip.validUntil) {
      const d = new Date(ip.validUntil)
      setEditValidUntil(d.toISOString().slice(0, 10))
    } else {
      setEditValidUntil('')
    }
    setEditPlan(ip.plan || 'QUARTERLY')
    setEditHost(ip.host || '')
    setEditPort(ip.port || 443)
    setEditUsername(ip.username || '')
    setEditType((ip.ipType || 'ipv4').toUpperCase())
    setEditWhitelistedBrokers(ip.whitelistedBrokers || '')
    setEditPassword('')
    const sampleObj = {
      ip: ip.ipAddress,
      host: ip.host || '',
      port: ip.port || 443,
      user_id: ip.username || '',
      password: '',
      valid_until: ip.validUntil || '',
      plan: ip.plan || 'QUARTERLY',
      type: ip.ipType ? ip.ipType.toUpperCase() : 'IPV4',
      whitelisted_brokers: ip.whitelistedBrokers || '',
    }
    setEditJsonText(JSON.stringify(sampleObj, null, 2))
    setEditModalOpen(true)
  }

  const editJsonValidation = () => {
    const raw = editJsonText().trim()
    if (!raw) {
      return { valid: false, error: 'JSON payload is empty', parsed: null }
    }
    let parsed: any
    try {
      parsed = JSON.parse(raw)
    } catch (e: any) {
      return { valid: false, error: 'Invalid JSON syntax: ' + e.message, parsed: null }
    }
    if (Array.isArray(parsed) || typeof parsed !== 'object' || parsed === null) {
      return { valid: false, error: 'JSON must be an individual proxy IP object, not an array', parsed: null }
    }
    const jsonIP = String(parsed.ip || '').trim()
    if (!jsonIP) {
      return { valid: false, error: 'JSON must contain an "ip" field', parsed: null }
    }
    const currentIP = editingIp()?.ipAddress.trim() || ''
    if (jsonIP.toLowerCase() !== currentIP.toLowerCase()) {
      return {
        valid: false,
        error: `IP in JSON (${jsonIP}) does not match editing IP (${currentIP})`,
        parsed: null,
      }
    }
    return { valid: true, error: null, parsed }
  }

  const handleEditSubmit = async (e: Event) => {
    e.preventDefault()
    if (!editingIp()) return

    if (editMode() === 'json') {
      const v = editJsonValidation()
      if (!v.valid || !v.parsed) {
        setEditJsonError(v.error)
        return
      }
      setEditSubmitting(true)
      setEditJsonError(null)
      try {
        await putAdminProxyIP(editingIp()!.ipAddress, v.parsed)
        setEditModalOpen(false)
        await loadData(false)
      } catch (err: any) {
        setEditJsonError(err?.message || 'Failed to update proxy IP from JSON')
      } finally {
        setEditSubmitting(false)
      }
      return
    }

    setEditSubmitting(true)
    try {
      await putAdminProxyIP(editingIp()!.ipAddress, {
        ip: editingIp()!.ipAddress,
        host: editHost().trim(),
        port: editPort(),
        user_id: editUsername().trim(),
        password: editPassword() || undefined,
        valid_until: editValidUntil() ? `${editValidUntil()} 23:59:59 (IST)` : undefined,
        plan: editPlan(),
        type: editType(),
        whitelisted_brokers: editWhitelistedBrokers().trim(),
      })
      setEditModalOpen(false)
      await loadData(false)
    } catch (err: any) {
      setError(err?.message || 'Failed to update proxy IP')
    } finally {
      setEditSubmitting(false)
    }
  }

  // Delete
  const openDeleteModal = (ip: AdminProxyIP) => {
    setDeletingIp(ip)
    setDeleteModalOpen(true)
  }

  const handleDeleteSubmit = async () => {
    if (!deletingIp()) return
    setDeleteSubmitting(true)
    try {
      await deleteAdminProxyIP(deletingIp()!.ipAddress)
      setDeleteModalOpen(false)
      await loadData(false)
    } catch (err: any) {
      setError(err?.message || 'Failed to delete proxy IP')
    } finally {
      setDeleteSubmitting(false)
    }
  }

  // Bulk Import
  const handleBulkSubmit = async (e: Event) => {
    e.preventDefault()
    setBulkParseError(null)
    setBulkResult(null)

    let parsed: any[]
    try {
      parsed = JSON.parse(bulkText())
      if (!Array.isArray(parsed)) {
        setBulkParseError('JSON must be an array of proxy IP objects')
        return
      }
    } catch (err: any) {
      setBulkParseError('Invalid JSON: ' + err.message)
      return
    }

    setBulkSubmitting(true)
    try {
      const res = await postAdminProxyIPsBulk(parsed)
      setBulkResult(res)
      await loadData(false)
    } catch (err: any) {
      setBulkParseError(err?.message || 'Bulk upload failed')
    } finally {
      setBulkSubmitting(false)
    }
  }

  const handleFileUpload = (e: Event) => {
    const input = e.target as HTMLInputElement
    if (input.files && input.files[0]) {
      const reader = new FileReader()
      reader.onload = (ev) => {
        setBulkText(ev.target?.result as string)
      }
      reader.readAsText(input.files[0])
    }
  }

  // Add Single IP
  const syncAddFormToJson = () => {
    const f = addForm()
    const obj = {
      ip: f.ip.trim(),
      host: f.host.trim(),
      port: f.port,
      user_id: f.user_id.trim(),
      password: f.password,
      valid_until: f.valid_until ? `${f.valid_until} 23:59:59 (IST)` : '',
      plan: f.plan,
      type: f.type,
      whitelisted_brokers: (f.whitelisted_brokers || '').trim(),
    }
    setAddJsonText(JSON.stringify(obj, null, 2))
  }

  const syncJsonToAddForm = () => {
    const v = addJsonValidation()
    if (v.valid && v.parsed) {
      const p = v.parsed
      setAddForm({
        ip: p.ip ? String(p.ip).trim() : addForm().ip,
        host: p.host ? String(p.host).trim() : addForm().host,
        port: Number(p.port) || addForm().port,
        user_id: p.user_id !== undefined ? String(p.user_id).trim() : (p.username !== undefined ? String(p.username).trim() : addForm().user_id),
        password: p.password !== undefined ? String(p.password) : addForm().password,
        valid_until: p.valid_until ? String(p.valid_until).slice(0, 10) : addForm().valid_until,
        plan: p.plan ? String(p.plan) : addForm().plan,
        type: p.type ? String(p.type).toUpperCase() : addForm().type,
        whitelisted_brokers: p.whitelisted_brokers !== undefined ? String(p.whitelisted_brokers).trim() : (p.whitelistedBrokers !== undefined ? String(p.whitelistedBrokers).trim() : addForm().whitelisted_brokers),
      })
    }
  }

  const addJsonValidation = () => {
    const raw = addJsonText().trim()
    if (!raw) {
      return { valid: false, error: 'JSON payload is empty', parsed: null }
    }
    let parsed: any
    try {
      parsed = JSON.parse(raw)
    } catch (e: any) {
      return { valid: false, error: 'Invalid JSON syntax: ' + e.message, parsed: null }
    }
    if (Array.isArray(parsed) || typeof parsed !== 'object' || parsed === null) {
      return { valid: false, error: 'JSON must be an individual proxy IP object, not an array', parsed: null }
    }
    const jsonIP = String(parsed.ip || '').trim()
    if (!jsonIP) {
      return { valid: false, error: 'JSON must contain an "ip" field', parsed: null }
    }
    const jsonHost = String(parsed.host || '').trim()
    if (!jsonHost) {
      return { valid: false, error: 'JSON must contain a "host" field', parsed: null }
    }
    return { valid: true, error: null, parsed }
  }

  const openAddModal = () => {
    setAddJsonError(null)
    setAddForm({
      ip: '',
      host: 'dc6-mum-01.algoip.in',
      port: 443,
      user_id: '',
      password: '',
      valid_until: '',
      plan: 'QUARTERLY',
      type: 'IPV4',
      whitelisted_brokers: 'Zerodha',
    })
    syncAddFormToJson()
    setAddModalOpen(true)
  }

  const handleAddSubmit = async (e: Event) => {
    e.preventDefault()
    if (addMode() === 'json') {
      const v = addJsonValidation()
      if (!v.valid || !v.parsed) {
        setAddJsonError(v.error)
        return
      }
      setAddSubmitting(true)
      setAddJsonError(null)
      try {
        await postAdminProxyIP(v.parsed)
        setAddModalOpen(false)
        await loadData(false)
      } catch (err: any) {
        setAddJsonError(err?.message || 'Failed to add proxy IP from JSON')
      } finally {
        setAddSubmitting(false)
      }
      return
    }

    setAddSubmitting(true)
    try {
      const form = addForm()
      await postAdminProxyIP({
        ...form,
        valid_until: form.valid_until ? `${form.valid_until} 23:59:59 (IST)` : undefined,
      })
      setAddModalOpen(false)
      await loadData(false)
    } catch (err: any) {
      setError(err?.message || 'Failed to add proxy IP')
    } finally {
      setAddSubmitting(false)
    }
  }

  return (
    <AdminLayout activeTab="ips">
      <div class="admin-status-header" style={{ 'margin-bottom': '1rem', display: 'flex', 'justify-content': 'space-between', 'align-items': 'center' }}>
        <div>
          <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
            {lastUpdated() ? `Last updated: ${lastUpdated()}` : 'Awaiting IP data...'}
          </span>
        </div>
        <button
          type="button"
          class="btn-secondary"
          onClick={() => loadData(true)}
          disabled={refreshing()}
          style={{ display: 'inline-flex', 'align-items': 'center', gap: '0.4rem' }}
        >
          <SyncIcon spinning={refreshing()} />
          <span>{refreshing() ? 'Refreshing...' : 'Refresh'}</span>
        </button>
      </div>

      {/* Metric Cards */}
      <Show when={stats()}>
        <div class="admin-metrics-grid">
          <div class="metric-card">
            <span class="metric-title">Total IPs</span>
            <span class="metric-value">{stats()!.totalCount}</span>
            <span class="metric-sub">{stats()!.ipv4Count} IPv4 / {stats()!.ipv6Count} IPv6</span>
          </div>
          <div class="metric-card">
            <span class="metric-title">Available IPs</span>
            <span class="metric-value" style={{ color: 'var(--color-accent)' }}>{stats()!.availableCount}</span>
            <span class="metric-sub">Ready for assignment</span>
          </div>
          <div class="metric-card">
            <span class="metric-title">Assigned Accounts</span>
            <span class="metric-value">{stats()!.assignedCount}</span>
            <span class="metric-sub">Dedicated 1:1 bound</span>
          </div>
          <div class="metric-card">
            <span class="metric-title">Expiring Soon (≤15d)</span>
            <span class="metric-value" style={{ color: '#f59e0b' }}>{stats()!.expiringSoonCount}</span>
            <span class="metric-sub">{stats()!.assignedExpiring} assigned</span>
          </div>
          <div class="metric-card">
            <span class="metric-title">Expired</span>
            <span class="metric-value" style={{ color: '#ef4444' }}>{stats()!.expiredCount}</span>
            <span class="metric-sub">Require renewal</span>
          </div>
        </div>
      </Show>

      {/* Filter and Action Bar */}
      <div class="admin-filter-bar">
        <div class="filter-pills">
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeFilter() === 'all' }}
            onClick={() => setFilter('all')}
          >
            All IPs
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeFilter() === 'active' }}
            onClick={() => setFilter('active')}
          >
            Active
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeFilter() === 'expiring_soon' }}
            onClick={() => setFilter('expiring_soon')}
          >
            Expiring Soon
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeFilter() === 'expired' }}
            onClick={() => setFilter('expired')}
          >
            Expired
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeFilter() === 'ipv4' }}
            onClick={() => setFilter('ipv4')}
          >
            IPv4
          </button>
          <button
            type="button"
            classList={{ 'filter-pill': true, active: activeFilter() === 'ipv6' }}
            onClick={() => setFilter('ipv6')}
          >
            IPv6
          </button>
        </div>

        <div class="admin-ips-filter-actions">
          <button
            type="button"
            class="btn-secondary"
            onClick={handleTestAllProxies}
            disabled={testingAll() || refreshing()}
            style={{ display: 'inline-flex', 'align-items': 'center', gap: '0.4rem' }}
          >
            <span>{testingAll() ? 'Testing Proxies...' : '⚡ Test All Proxies'}</span>
          </button>
          <button
            type="button"
            class="btn-secondary"
            onClick={() => {
              setBulkModalOpen(true)
              setBulkResult(null)
              setBulkParseError(null)
            }}
          >
            Bulk Ingest
          </button>
          <button
            type="button"
            class="btn-primary"
            onClick={() => openAddModal()}
          >
            + Add IP
          </button>
        </div>
      </div>

      <Show when={error()}>
        <div style={{ padding: '0.75rem 1rem', background: 'rgba(239, 68, 68, 0.1)', color: '#ef4444', 'border-radius': '6px', 'margin-bottom': '1rem' }}>
          {error()}
        </div>
      </Show>

      {/* Bulk Proxy Test Summary Alert */}
      <Show when={bulkProbeSummary()}>
        <div style={{
          padding: '0.75rem 1rem',
          background: bulkProbeSummary()!.failureCount === 0 ? 'rgba(4, 180, 136, 0.1)' : 'rgba(245, 158, 11, 0.1)',
          color: bulkProbeSummary()!.failureCount === 0 ? 'var(--color-accent)' : '#f59e0b',
          'border-radius': '6px',
          'margin-bottom': '1rem',
          display: 'flex',
          'justify-content': 'space-between',
          'align-items': 'center',
        }}>
          <div>
            <strong>Proxy Pre-Flight Probe:</strong> Tested {bulkProbeSummary()!.totalTested} proxies &mdash;{' '}
            {bulkProbeSummary()!.successCount} succeeded, {bulkProbeSummary()!.failureCount} failed{' '}
            (Avg Latency: {bulkProbeSummary()!.avgLatencyMs} ms)
          </div>
          <button
            type="button"
            onClick={() => setBulkProbeSummary(null)}
            style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'inherit', 'font-size': '1.1rem' }}
          >
            &times;
          </button>
        </div>
      </Show>

      {/* IP Inventory Table */}
      <Show
        when={!loading()}
        fallback={<Skeleton height={280} radius={8} />}
      >
        <div class="admin-table-container">
          <table class="admin-table">
            <thead>
              <tr>
                <th style={{ width: '24%' }}>IP Address</th>
                <th style={{ width: '18%' }}>Gateway Host</th>
                <th style={{ width: '18%' }}>Whitelisted Brokers</th>
                <th style={{ width: '14%' }}>Assignment</th>
                <th style={{ width: '14%' }}>Validity & Expiry</th>
                <th style={{ width: '12%' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={filteredIps().length > 0}
                fallback={
                  <tr>
                    <td colspan={6} style={{ 'text-align': 'center', padding: '2rem', color: 'var(--color-text-secondary)' }}>
                      No proxy IPs found matching filter.
                    </td>
                  </tr>
                }
              >
                <For each={paginatedIps()}>
                  {(ip) => {
                    const probe = () => testResults()[ip.ipAddress]
                    const isProbing = () => testingIp() === ip.ipAddress

                    return (
                      <tr>
                        <td>
                          <div style={{ display: 'flex', 'align-items': 'center', gap: '0.5rem' }}>
                            <span class="badge badge-success">
                              {ip.ipType.toUpperCase()}
                            </span>
                            <span style={{ 'font-family': 'monospace', 'font-weight': 600 }}>{ip.ipAddress}</span>
                          </div>
                        </td>
                        <td>
                          <span style={{ color: 'var(--color-text-secondary)' }}>
                            {ip.host}:{ip.port}
                          </span>
                        </td>
                        <td>
                          <Show
                            when={ip.whitelistedBrokers && ip.whitelistedBrokers.trim()}
                            fallback={<span class="badge badge-neutral" style={{ 'font-size': '0.75rem' }}>All Brokers</span>}
                          >
                            <div style={{ display: 'flex', 'align-items': 'center', gap: '0.35rem', 'flex-wrap': 'wrap' }}>
                              <For each={ip.whitelistedBrokers!.split(',').map((s) => s.trim()).filter(Boolean)}>
                                {(b) => (
                                  <div style={{ display: 'inline-flex', 'align-items': 'center', gap: '0.35rem', background: 'var(--color-surface, rgba(0,0,0,0.05))', padding: '2px 6px', 'border-radius': '4px', border: '1px solid var(--color-border)' }}>
                                    <BrokerLogo broker={b} size={14} />
                                    <span style={{ 'font-size': '0.75rem', 'font-weight': 500 }}>{b}</span>
                                  </div>
                                )}
                              </For>
                            </div>
                          </Show>
                        </td>
                        <td>
                          <Show
                            when={ip.isAssigned}
                            fallback={<span class="badge badge-neutral">Available</span>}
                          >
                            <span class="badge badge-success">
                              {ip.assignedAccountName || 'Assigned'}
                            </span>
                          </Show>
                        </td>
                        <td>
                          <Show when={ip.expiryStatus === 'active'}>
                            <span class="badge badge-success">
                              Expires in {ip.daysRemaining}d
                            </span>
                          </Show>
                          <Show when={ip.expiryStatus === 'expiring_soon'}>
                            <span class="badge badge-warning">
                              Expires in {ip.daysRemaining}d
                            </span>
                          </Show>
                          <Show when={ip.expiryStatus === 'expired'}>
                            <span class="badge badge-danger">
                              Expired ({Math.abs(ip.daysRemaining)}d ago)
                            </span>
                          </Show>
                        </td>
                        <td>
                          <div class="table-actions">
                            <button
                              type="button"
                              class="icon-button"
                              onClick={() => handleTestProbe(ip.ipAddress)}
                              disabled={isProbing()}
                              aria-label="Test connectivity"
                              style={{ color: probe() ? (probe()!.success ? 'var(--color-accent)' : '#ef4444') : 'var(--color-text)' }}
                            >
                              <SyncIcon spinning={isProbing()} />
                            </button>
                            <button
                              type="button"
                              class="icon-button"
                              onClick={() => openEditModal(ip)}
                              aria-label="Edit / Renew IP"
                            >
                              <EditIcon />
                            </button>
                            <button
                              type="button"
                              class="icon-button"
                              onClick={() => openDeleteModal(ip)}
                              disabled={ip.isAssigned}
                              aria-label={ip.isAssigned ? 'Cannot delete: assigned to active account' : 'Delete IP'}
                              style={{ color: ip.isAssigned ? 'var(--color-text-secondary)' : '#ef4444', opacity: ip.isAssigned ? 0.4 : 1 }}
                            >
                              <TrashIcon />
                            </button>
                          </div>
                        </td>
                      </tr>
                    )
                  }}
                </For>
              </Show>
            </tbody>
          </table>
        </div>

        <Show when={filteredIps().length > pageSize}>
          <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-top': '1rem' }}>
            <span style={{ 'font-size': '0.85rem', color: 'var(--color-text-secondary)' }}>
              Showing {(page() - 1) * pageSize + 1} &ndash;{' '}
              {Math.min(page() * pageSize, filteredIps().length)} of{' '}
              {filteredIps().length} IPs
            </span>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <button
                type="button"
                class="btn-secondary"
                disabled={page() === 1}
                onClick={() => setPage(page() - 1)}
              >
                Previous
              </button>
              <button
                type="button"
                class="btn-secondary"
                disabled={page() * pageSize >= filteredIps().length}
                onClick={() => setPage(page() + 1)}
              >
                Next
              </button>
            </div>
          </div>
        </Show>
      </Show>

      {/* Bulk Import Modal */}
      <Show when={bulkModalOpen()}>
        <div class="modal-overlay" role="dialog" aria-modal="true" aria-label="Bulk Ingest Proxy IPs">
          <div class="modal-card" style={{ 'max-width': '650px' }}>
            <div class="modal-header-row">
              <h3>Bulk Ingest Proxy IPs</h3>
              <button
                type="button"
                onClick={() => setBulkModalOpen(false)}
                style={{ background: 'transparent', border: 'none', cursor: 'pointer', 'font-size': '1.25rem', color: 'var(--color-text-secondary)' }}
              >
                &times;
              </button>
            </div>
            <form onSubmit={handleBulkSubmit}>
              <div class="modal-body">
                <div style={{ 'margin-bottom': '0.75rem', display: 'flex', 'justify-content': 'flex-end', 'align-items': 'center' }}>
                  <label class="btn-secondary" style={{ 'font-size': '0.75rem', padding: '0.25rem 0.6rem', cursor: 'pointer' }}>
                    Upload File
                    <input type="file" accept=".json" onChange={handleFileUpload} style={{ display: 'none' }} />
                  </label>
                </div>
                <textarea
                  rows={10}
                  required
                  placeholder='[ { "ip": "2401:...", "host": "dc6-mum-01.algoip.in", "port": 443, ... } ]'
                  value={bulkText()}
                  onInput={(e) => setBulkText(e.currentTarget.value)}
                  style={{
                    width: '100%',
                    'font-family': 'monospace',
                    'font-size': '0.8rem',
                    padding: '0.75rem',
                    border: '1px solid var(--color-border)',
                    'border-radius': '6px',
                    background: 'var(--color-surface)',
                    color: 'var(--color-text)',
                    'box-sizing': 'border-box',
                  }}
                />

                <Show when={bulkParseError()}>
                  <div style={{ 'margin-top': '0.5rem', color: '#ef4444', 'font-size': '0.85rem' }}>
                    {bulkParseError()}
                  </div>
                </Show>

                <Show when={bulkResult()}>
                  <div style={{ 'margin-top': '0.75rem', padding: '0.75rem', background: 'rgba(4, 180, 136, 0.1)', 'border-radius': '6px', 'font-size': '0.85rem' }}>
                    <div><strong>Batch Results:</strong> Total: {bulkResult()!.total} | Inserted: {bulkResult()!.inserted} | Updated: {bulkResult()!.updated}</div>
                    <Show when={bulkResult()!.errors && bulkResult()!.errors.length > 0}>
                      <div style={{ 'margin-top': '0.5rem', color: '#ef4444' }}>
                        Errors ({bulkResult()!.errors.length}):
                        <ul style={{ margin: '0.25rem 0 0', 'padding-left': '1.25rem' }}>
                          <For each={bulkResult()!.errors.slice(0, 5)}>
                            {(err) => <li>{err}</li>}
                          </For>
                        </ul>
                      </div>
                    </Show>
                  </div>
                </Show>
              </div>
              <div class="modal-footer">
                <button
                  type="button"
                  class="btn-secondary"
                  onClick={() => setBulkModalOpen(false)}
                  disabled={bulkSubmitting()}
                >
                  Close
                </button>
                <button
                  type="submit"
                  class="btn-primary"
                  disabled={bulkSubmitting() || !bulkText().trim()}
                >
                  {bulkSubmitting() ? 'Ingesting...' : 'Ingest Batch'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Show>

      {/* Edit / Renew Modal */}
      <Show when={editModalOpen() && editingIp()}>
        <div
          class="modal-overlay"
          role="dialog"
          aria-modal="true"
          aria-label="Edit Proxy IP"
          onClick={(e) => {
            if (e.target === e.currentTarget && !editSubmitting()) setEditModalOpen(false)
          }}
        >
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <div class="modal-header-row">
              <h3 style={{ margin: 0, 'font-size': '1.15rem', 'font-weight': 600 }}>
                Edit Proxy IP: {editingIp()!.ipAddress}
              </h3>
              <button
                type="button"
                class="drawer-close"
                onClick={() => setEditModalOpen(false)}
                aria-label="Close"
              >
                &times;
              </button>
            </div>

            {/* Mode Selector Tabs using dashboard .tab-group & .tab-btn */}
            <div style={{ padding: '0.85rem 1.5rem 0' }}>
              <div role="tablist" class="tab-group" style={{ width: '100%', display: 'flex' }}>
                <button
                  type="button"
                  class="tab-btn"
                  classList={{ active: editMode() === 'form' }}
                  aria-pressed={editMode() === 'form'}
                  onClick={() => {
                    syncJsonToEditForm()
                    setEditMode('form')
                    setEditJsonError(null)
                  }}
                  style={{ flex: 1 }}
                >
                  Form Fields
                </button>
                <button
                  type="button"
                  class="tab-btn"
                  classList={{ active: editMode() === 'json' }}
                  aria-pressed={editMode() === 'json'}
                  onClick={() => {
                    syncEditFormToJson()
                    setEditMode('json')
                  }}
                  aria-label="Paste JSON / JSON Text"
                  style={{ flex: 1 }}
                >
                  JSON Text
                </button>
              </div>
            </div>

            <form onSubmit={handleEditSubmit}>
              <div class="modal-body">
                <Show when={editMode() === 'form'}>
                  <div style={{ display: 'grid', 'grid-template-columns': '2fr 1fr', gap: '0.85rem', 'margin-bottom': '0.85rem' }}>
                    <div>
                      <label for="edit-ip-host" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Gateway Host
                      </label>
                      <input
                        id="edit-ip-host"
                        type="text"
                        required
                        value={editHost()}
                        onInput={(e) => setEditHost(e.currentTarget.value)}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                    <div>
                      <label for="edit-ip-port" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Port
                      </label>
                      <ThemedNumberInput
                        id="edit-ip-port"
                        aria-label="Port"
                        required
                        value={editPort()}
                        onInput={(e) => setEditPort(parseInt(e.currentTarget.value) || 443)}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                  </div>

                  <div class="admin-form-grid-2col" style={{ 'margin-bottom': '0.85rem' }}>
                    <div>
                      <label for="edit-ip-user" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Username
                      </label>
                      <input
                        id="edit-ip-user"
                        type="text"
                        required
                        value={editUsername()}
                        onInput={(e) => setEditUsername(e.currentTarget.value)}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                    <div>
                      <label for="edit-ip-pass" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Password (Optional)
                      </label>
                      <input
                        id="edit-ip-pass"
                        type="password"
                        placeholder="Leave blank to keep existing password"
                        value={editPassword()}
                        onInput={(e) => setEditPassword(e.currentTarget.value)}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                  </div>

                  <div class="admin-form-grid-2col" style={{ 'margin-bottom': '0.85rem' }}>
                    <div>
                      <label for="edit-ip-valid-until" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Valid Until
                      </label>
                      <input
                        id="edit-ip-valid-until"
                        type="date"
                        required
                        value={editValidUntil()}
                        onInput={(e) => setEditValidUntil(e.currentTarget.value)}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                    <div>
                      <label for="edit-ip-plan" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Subscription Plan
                      </label>
                      <ThemedSelect
                        id="edit-ip-plan"
                        aria-label="Subscription Plan"
                        value={editPlan()}
                        onChange={(val) => setEditPlan(val)}
                        options={PLAN_OPTIONS}
                      />
                    </div>
                  </div>

                  <div class="admin-form-grid-2col">
                    <div>
                      <label for="edit-ip-type" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        IP Type
                      </label>
                      <ThemedSelect
                        id="edit-ip-type"
                        aria-label="IP Type"
                        value={editType()}
                        onChange={(val) => setEditType(val)}
                        options={IP_TYPE_OPTIONS}
                      />
                    </div>
                    <div>
                      <label for="edit-ip-whitelisted-brokers" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Whitelisted Brokers
                      </label>
                      <ThemedSelect
                        id="edit-ip-whitelisted-brokers"
                        aria-label="Whitelisted Brokers"
                        value={editWhitelistedBrokers()}
                        onChange={(val) => setEditWhitelistedBrokers(val)}
                        options={getWhitelistedBrokerSelectOptions(editWhitelistedBrokers())}
                      />
                    </div>
                  </div>
                </Show>

                <Show when={editMode() === 'json'}>
                  <div style={{ 'margin-bottom': '0.85rem' }}>
                    <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-bottom': '0.35rem' }}>
                      <label
                        for="edit-json-textarea"
                        style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, color: 'var(--color-text)' }}
                      >
                        Individual IP JSON
                      </label>
                      <span style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>
                        Single proxy IP object
                      </span>
                    </div>
                    <textarea
                      id="edit-json-textarea"
                      aria-label="Individual IP JSON"
                      rows={11}
                      value={editJsonText()}
                      onInput={(e) => {
                        setEditJsonText(e.currentTarget.value)
                        setEditJsonError(null)
                      }}
                      style={{
                        width: '100%',
                        padding: '0.75rem 0.85rem',
                        border: '1px solid var(--color-border)',
                        'border-radius': '8px',
                        background: 'var(--color-bg)',
                        color: 'var(--color-text)',
                        'font-family': 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        'font-size': '0.825rem',
                        'line-height': 1.5,
                        'box-sizing': 'border-box',
                        resize: 'vertical',
                      }}
                    />
                  </div>

                  <Show when={editJsonText().trim().length > 0}>
                    <div style={{ 'margin-bottom': '0.75rem' }}>
                      <Show when={editJsonValidation().valid}>
                        <div style={{
                          display: 'inline-flex',
                          'align-items': 'center',
                          gap: '0.4rem',
                          padding: '0.35rem 0.65rem',
                          'border-radius': '6px',
                          background: 'rgba(4, 180, 136, 0.12)',
                          border: '1px solid rgba(4, 180, 136, 0.25)',
                          color: 'var(--color-accent)',
                          'font-size': '0.78rem',
                          'font-weight': 500,
                        }}>
                          <span>✓</span> IP matches ({editingIp()!.ipAddress})
                        </div>
                      </Show>
                      <Show when={!editJsonValidation().valid}>
                        <div style={{
                          display: 'inline-flex',
                          'align-items': 'center',
                          gap: '0.4rem',
                          padding: '0.35rem 0.65rem',
                          'border-radius': '6px',
                          background: 'rgba(239, 68, 68, 0.1)',
                          border: '1px solid rgba(239, 68, 68, 0.2)',
                          color: '#ef4444',
                          'font-size': '0.78rem',
                          'font-weight': 500,
                        }}>
                          <span>⚠</span> {editJsonValidation().error}
                        </div>
                      </Show>
                    </div>
                  </Show>

                  <Show when={editJsonError()}>
                    <div style={{
                      padding: '0.6rem 0.8rem',
                      'border-radius': '6px',
                      background: 'rgba(239, 68, 68, 0.1)',
                      border: '1px solid rgba(239, 68, 68, 0.2)',
                      color: '#ef4444',
                      'font-size': '0.8rem',
                      'margin-bottom': '0.75rem',
                    }}>
                      {editJsonError()}
                    </div>
                  </Show>
                </Show>
              </div>
              <div class="modal-footer">
                <button
                  type="button"
                  class="btn-secondary"
                  onClick={() => setEditModalOpen(false)}
                  disabled={editSubmitting()}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  class="btn-primary"
                  disabled={editSubmitting() || (editMode() === 'json' && (!editJsonText().trim() || !editJsonValidation().valid))}
                >
                  {editSubmitting() ? 'Saving...' : 'Save Changes'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Show>

      {/* Delete Confirm Modal */}
      <Show when={deleteModalOpen() && deletingIp()}>
        <div class="modal-overlay" role="dialog" aria-modal="true" aria-label="Confirm Delete">
          <div class="modal-card">
            <div class="modal-header-row">
              <h3>Delete Proxy IP</h3>
              <button
                type="button"
                onClick={() => setDeleteModalOpen(false)}
                style={{ background: 'transparent', border: 'none', cursor: 'pointer', 'font-size': '1.25rem', color: 'var(--color-text-secondary)' }}
              >
                &times;
              </button>
            </div>
            <div class="modal-body">
              <p style={{ margin: 0, color: 'var(--color-text)' }}>
                Are you sure you want to delete proxy IP <strong>{deletingIp()!.ipAddress}</strong> from the inventory? This action cannot be undone.
              </p>
            </div>
            <div class="modal-footer">
              <button
                type="button"
                class="btn-secondary"
                onClick={() => setDeleteModalOpen(false)}
                disabled={deleteSubmitting()}
              >
                Cancel
              </button>
              <button
                type="button"
                class="btn-danger"
                onClick={handleDeleteSubmit}
                disabled={deleteSubmitting()}
              >
                {deleteSubmitting() ? 'Deleting...' : 'Delete IP'}
              </button>
            </div>
          </div>
        </div>
      </Show>

      {/* Add Single IP Modal */}
      <Show when={addModalOpen()}>
        <div
          class="modal-overlay"
          role="dialog"
          aria-modal="true"
          aria-label="Add Proxy IP"
          onClick={(e) => {
            if (e.target === e.currentTarget && !addSubmitting()) setAddModalOpen(false)
          }}
        >
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <div class="modal-header-row">
              <h3 style={{ margin: 0, 'font-size': '1.15rem', 'font-weight': 600 }}>Add Proxy IP</h3>
              <button
                type="button"
                class="drawer-close"
                onClick={() => setAddModalOpen(false)}
                aria-label="Close"
              >
                &times;
              </button>
            </div>

            {/* Mode Selector Tabs using dashboard .tab-group & .tab-btn */}
            <div style={{ padding: '0.85rem 1.5rem 0' }}>
              <div role="tablist" class="tab-group" style={{ width: '100%', display: 'flex' }}>
                <button
                  type="button"
                  class="tab-btn"
                  classList={{ active: addMode() === 'form' }}
                  aria-pressed={addMode() === 'form'}
                  onClick={() => {
                    syncJsonToAddForm()
                    setAddMode('form')
                    setAddJsonError(null)
                  }}
                  style={{ flex: 1 }}
                >
                  Form Fields
                </button>
                <button
                  type="button"
                  class="tab-btn"
                  classList={{ active: addMode() === 'json' }}
                  aria-pressed={addMode() === 'json'}
                  onClick={() => {
                    syncAddFormToJson()
                    setAddMode('json')
                  }}
                  aria-label="Paste JSON / JSON Text"
                  style={{ flex: 1 }}
                >
                  JSON Text
                </button>
              </div>
            </div>

            <form onSubmit={handleAddSubmit}>
              <div class="modal-body">
                <Show when={addMode() === 'form'}>
                  <div style={{ 'margin-bottom': '0.85rem' }}>
                    <label style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                      IP Address
                    </label>
                    <input
                      type="text"
                      required
                      placeholder="e.g. 103.21.58.10 or 2401:c080::1"
                      value={addForm().ip}
                      onInput={(e) => setAddForm({ ...addForm(), ip: e.currentTarget.value })}
                      style={{
                        width: '100%',
                        padding: '0.55rem 0.75rem',
                        border: '1px solid var(--color-border)',
                        'border-radius': '6px',
                        background: 'var(--color-bg)',
                        color: 'var(--color-text)',
                        'box-sizing': 'border-box',
                        'font-size': '0.875rem',
                      }}
                    />
                  </div>
                  <div style={{ display: 'grid', 'grid-template-columns': '2fr 1fr', gap: '0.85rem', 'margin-bottom': '0.85rem' }}>
                    <div>
                      <label for="add-ip-host" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Gateway Host
                      </label>
                      <input
                        id="add-ip-host"
                        type="text"
                        required
                        value={addForm().host}
                        onInput={(e) => setAddForm({ ...addForm(), host: e.currentTarget.value })}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                    <div>
                      <label for="add-ip-port" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Port
                      </label>
                      <ThemedNumberInput
                        id="add-ip-port"
                        aria-label="Port"
                        required
                        value={addForm().port}
                        onInput={(e) => setAddForm({ ...addForm(), port: parseInt(e.currentTarget.value) || 443 })}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                  </div>
                  <div class="admin-form-grid-2col" style={{ 'margin-bottom': '0.85rem' }}>
                    <div>
                      <label for="add-ip-user" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Username
                      </label>
                      <input
                        id="add-ip-user"
                        type="text"
                        required
                        placeholder="aip_live_..."
                        value={addForm().user_id}
                        onInput={(e) => setAddForm({ ...addForm(), user_id: e.currentTarget.value })}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                    <div>
                      <label for="add-ip-pass" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Password
                      </label>
                      <input
                        id="add-ip-pass"
                        type="password"
                        required
                        placeholder="aip_sec_..."
                        value={addForm().password}
                        onInput={(e) => setAddForm({ ...addForm(), password: e.currentTarget.value })}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                  </div>
                  <div class="admin-form-grid-2col" style={{ 'margin-bottom': '0.85rem' }}>
                    <div>
                      <label for="add-ip-valid-until" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Valid Until
                      </label>
                      <input
                        id="add-ip-valid-until"
                        type="date"
                        required
                        value={addForm().valid_until}
                        onInput={(e) => setAddForm({ ...addForm(), valid_until: e.currentTarget.value })}
                        style={{
                          width: '100%',
                          padding: '0.55rem 0.75rem',
                          border: '1px solid var(--color-border)',
                          'border-radius': '6px',
                          background: 'var(--color-bg)',
                          color: 'var(--color-text)',
                          'box-sizing': 'border-box',
                          'font-size': '0.875rem',
                        }}
                      />
                    </div>
                    <div>
                      <label for="add-ip-plan" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Subscription Plan
                      </label>
                      <ThemedSelect
                        id="add-ip-plan"
                        aria-label="Subscription Plan"
                        value={addForm().plan}
                        onChange={(val) => setAddForm({ ...addForm(), plan: val })}
                        options={PLAN_OPTIONS}
                      />
                    </div>
                  </div>
                  <div class="admin-form-grid-2col">
                    <div>
                      <label for="add-ip-type" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        IP Type
                      </label>
                      <ThemedSelect
                        id="add-ip-type"
                        aria-label="IP Type"
                        value={addForm().type}
                        onChange={(val) => setAddForm({ ...addForm(), type: val })}
                        options={IP_TYPE_OPTIONS}
                      />
                    </div>
                    <div>
                      <label for="add-ip-whitelisted-brokers" style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, 'margin-bottom': '0.35rem', color: 'var(--color-text)' }}>
                        Whitelisted Brokers
                      </label>
                      <ThemedSelect
                        id="add-ip-whitelisted-brokers"
                        aria-label="Whitelisted Brokers"
                        value={addForm().whitelisted_brokers}
                        onChange={(val) => setAddForm({ ...addForm(), whitelisted_brokers: val })}
                        options={getWhitelistedBrokerSelectOptions(addForm().whitelisted_brokers)}
                      />
                    </div>
                  </div>
                </Show>

                <Show when={addMode() === 'json'}>
                  <div style={{ 'margin-bottom': '0.85rem' }}>
                    <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', 'margin-bottom': '0.35rem' }}>
                      <label
                        for="add-json-textarea"
                        style={{ display: 'block', 'font-size': '0.825rem', 'font-weight': 600, color: 'var(--color-text)' }}
                      >
                        Individual IP JSON
                      </label>
                      <span style={{ 'font-size': '0.75rem', color: 'var(--color-text-secondary)' }}>
                        Single proxy IP object
                      </span>
                    </div>
                    <textarea
                      id="add-json-textarea"
                      aria-label="Individual IP JSON"
                      rows={11}
                      value={addJsonText()}
                      onInput={(e) => {
                        setAddJsonText(e.currentTarget.value)
                        setAddJsonError(null)
                      }}
                      style={{
                        width: '100%',
                        padding: '0.75rem 0.85rem',
                        border: '1px solid var(--color-border)',
                        'border-radius': '8px',
                        background: 'var(--color-bg)',
                        color: 'var(--color-text)',
                        'font-family': 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                        'font-size': '0.825rem',
                        'line-height': 1.5,
                        'box-sizing': 'border-box',
                        resize: 'vertical',
                      }}
                    />
                  </div>

                  <Show when={addJsonText().trim().length > 0}>
                    <div style={{ 'margin-bottom': '0.75rem' }}>
                      <Show when={addJsonValidation().valid}>
                        <div style={{
                          display: 'inline-flex',
                          'align-items': 'center',
                          gap: '0.4rem',
                          padding: '0.35rem 0.65rem',
                          'border-radius': '6px',
                          background: 'rgba(4, 180, 136, 0.12)',
                          border: '1px solid rgba(4, 180, 136, 0.25)',
                          color: 'var(--color-accent)',
                          'font-size': '0.78rem',
                          'font-weight': 500,
                        }}>
                          <span>✓</span> Valid JSON ({addJsonValidation().parsed?.ip})
                        </div>
                      </Show>
                      <Show when={!addJsonValidation().valid}>
                        <div style={{
                          display: 'inline-flex',
                          'align-items': 'center',
                          gap: '0.4rem',
                          padding: '0.35rem 0.65rem',
                          'border-radius': '6px',
                          background: 'rgba(239, 68, 68, 0.1)',
                          border: '1px solid rgba(239, 68, 68, 0.2)',
                          color: '#ef4444',
                          'font-size': '0.78rem',
                          'font-weight': 500,
                        }}>
                          <span>⚠</span> {addJsonValidation().error}
                        </div>
                      </Show>
                    </div>
                  </Show>

                  <Show when={addJsonError()}>
                    <div style={{
                      padding: '0.6rem 0.8rem',
                      'border-radius': '6px',
                      background: 'rgba(239, 68, 68, 0.1)',
                      border: '1px solid rgba(239, 68, 68, 0.2)',
                      color: '#ef4444',
                      'font-size': '0.8rem',
                      'margin-bottom': '0.75rem',
                    }}>
                      {addJsonError()}
                    </div>
                  </Show>
                </Show>
              </div>
              <div class="modal-footer">
                <button
                  type="button"
                  class="btn-secondary"
                  onClick={() => setAddModalOpen(false)}
                  disabled={addSubmitting()}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  class="btn-primary"
                  disabled={addSubmitting() || (addMode() === 'json' && (!addJsonText().trim() || !addJsonValidation().valid))}
                >
                  {addSubmitting() ? 'Adding...' : 'Add Proxy IP'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Show>
    </AdminLayout>
  )
}
