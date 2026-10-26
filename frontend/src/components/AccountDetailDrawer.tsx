import { createEffect, createMemo, createSignal, For, on, onCleanup, Show, untrack } from 'solid-js'
import {
  fetchAvailableProxyIPs,
  type Account,
  type AvailableProxyIPsResponse,
  type CreateAccountRequest,
  type GroupSummary,
  type ProxyIP,
} from '../api'
import { BrokerLogo, SUPPORTED_BROKERS } from './BrokerLogo'
import { ThemedSelect } from './ThemedSelect'
import { isValidIP } from '../utils/ip'

export function isCompatibleIP(ipBrokers: string | undefined, brokerVal: string): boolean {
  if (!ipBrokers || !ipBrokers.trim()) return true
  const b = brokerVal.toLowerCase()
  return ipBrokers
    .toLowerCase()
    .split(',')
    .map((s) => s.trim())
    .some((c) => c === b || (b === 'kite' && c === 'zerodha') || (b === 'zerodha' && c === 'kite'))
}

export function getPrioritizedCompatibleIPs(ips: ProxyIP[], brokerVal: string): ProxyIP[] {
  const compatible = ips.filter((p) => isCompatibleIP(p.whitelistedBrokers, brokerVal))
  return [...compatible].sort((a, b) => {
    const aSpecialized = a.whitelistedBrokers && a.whitelistedBrokers.trim() !== ''
    const bSpecialized = b.whitelistedBrokers && b.whitelistedBrokers.trim() !== ''
    if (aSpecialized && !bSpecialized) return -1
    if (!aSpecialized && bSpecialized) return 1
    return a.ipAddress.localeCompare(b.ipAddress, undefined, { numeric: true })
  })
}

// AccountDetailDrawer is the Accounts page's create/edit form
// (docs/plans/UI-PLAN.md's field table). account=null means create mode;
// otherwise it edits that account. Role is fixed at create time and
// locked thereafter.
export function AccountDetailDrawer(props: {
  open: boolean
  account: Account | null
  groups: GroupSummary[]
  onClose: () => void
  onCreate: (body: CreateAccountRequest) => Promise<void>
  onSave: (id: string, patch: Record<string, unknown>) => Promise<void>
}) {
  const [name, setName] = createSignal('')
  const [role, setRole] = createSignal<'master' | 'follower'>('follower')
  const [broker, setBroker] = createSignal('kite')
  const [brokerAccountId, setBrokerAccountId] = createSignal('')
  const [apiKey, setApiKey] = createSignal('')
  const [apiSecret, setApiSecret] = createSignal('')
  const [password, setPassword] = createSignal('')
  const [totpSecret, setTotpSecret] = createSignal('')
  const [accessToken, setAccessToken] = createSignal('')
  const [availableIPs, setAvailableIPs] = createSignal<AvailableProxyIPsResponse>({ ipv4: [], ipv6: [] })
  const [ipType, setIpType] = createSignal<'na' | 'ipv4' | 'ipv6'>('ipv4')
  const [ip, setIp] = createSignal('')
  const [cloneFactor, setCloneFactor] = createSignal('1')
  const [maxQtyPerOrder, setMaxQtyPerOrder] = createSignal('')
  const [groupId, setGroupId] = createSignal('')
  const [status, setStatus] = createSignal('ok')
  const [error, setError] = createSignal<string | null>(null)
  const [saving, setSaving] = createSignal(false)
  const [isDropdownOpen, setIsDropdownOpen] = createSignal(false)
  let containerRef: HTMLDivElement | undefined

  createEffect(() => {
    if (!isDropdownOpen()) return
    const handlePointerDown = (e: MouseEvent) => {
      const target = e.target as Node | null
      if (!target || containerRef?.contains(target)) return
      setIsDropdownOpen(false)
    }
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsDropdownOpen(false)
      }
    }
    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    onCleanup(() => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    })
  })

  createEffect(
    on(
      () => [props.open, props.account?.id] as const,
      ([isOpen]) => {
        setIsDropdownOpen(false)
        if (!isOpen) return
        const a = props.account
        setName(a?.name ?? '')
        const currentRole = a?.role ?? 'follower'
        const currentBroker = a?.broker ?? 'kite'
        setRole(currentRole)
        setBroker(currentBroker)
        setBrokerAccountId(a?.brokerAccountId ?? '')
        setApiKey(a?.apiKey ?? '')
        setApiSecret(a?.apiSecret ?? '')
        setPassword('')
        setTotpSecret('')
        setAccessToken('')
        setCloneFactor(a?.cloneFactor ?? '1')
        setMaxQtyPerOrder(a?.maxQtyPerOrder != null ? String(a.maxQtyPerOrder) : '')
        setGroupId(a?.groupId ?? '')
        setStatus(a?.status ?? 'ok')
        setError(null)

        // Synchronous initial IP state
        if (a?.ip) {
          setIp(a.ip)
          setIpType(a.ip.includes(':') ? 'ipv6' : 'ipv4')
        } else {
          setIp('')
          setIpType(currentRole === 'master' ? 'na' : 'ipv4')
        }

        // Load available proxy IPs
        fetchAvailableProxyIPs(a?.id)
          .then((ips) => {
            setAvailableIPs(ips)
            // In create mode for follower, auto-assign first available IP if still empty
            if (!a && untrack(role) === 'follower' && !untrack(ip)) {
              const b = untrack(broker)
              const v4 = getPrioritizedCompatibleIPs(ips.ipv4 || [], b)
              const v6 = getPrioritizedCompatibleIPs(ips.ipv6 || [], b)
              if (v4.length > 0) {
                setIpType('ipv4')
                setIp(v4[0].ipAddress)
              } else if (v6.length > 0) {
                setIpType('ipv6')
                setIp(v6[0].ipAddress)
              }
            }
          })
          .catch(() => {})
      }
    )
  )

  const isFollower = () => role() === 'follower'
  const isEdit = () => props.account != null

  const compatibleIPs = createMemo(() => {
    const pool = availableIPs()
    const list = ipType() === 'ipv6' ? (pool.ipv6 || []) : (pool.ipv4 || [])
    return getPrioritizedCompatibleIPs(list, broker())
  })

  const handleBrokerChange = (newBroker: string) => {
    setBroker(newBroker)
    if (!isEdit() && isFollower()) {
      const pool = availableIPs()
      const currentList = ipType() === 'ipv6' ? (pool.ipv6 || []) : (pool.ipv4 || [])
      const prioritized = getPrioritizedCompatibleIPs(currentList, newBroker)
      const curr = ip()
      const currItem = prioritized.find((item) => item.ipAddress === curr)
      const currIsSpecialized = currItem?.whitelistedBrokers && currItem.whitelistedBrokers.trim() !== ''
      const bestIsSpecialized = prioritized[0]?.whitelistedBrokers && prioritized[0].whitelistedBrokers.trim() !== ''

      if (!currItem || (bestIsSpecialized && !currIsSpecialized)) {
        if (prioritized.length > 0) {
          setIp(prioritized[0].ipAddress)
        } else {
          const altType = ipType() === 'ipv6' ? 'ipv4' : 'ipv6'
          const altList = getPrioritizedCompatibleIPs(
            altType === 'ipv6' ? (pool.ipv6 || []) : (pool.ipv4 || []),
            newBroker
          )
          if (altList.length > 0) {
            setIpType(altType)
            setIp(altList[0].ipAddress)
          } else {
            setIp('')
          }
        }
      }
    }
  }

  const handleRoleChange = (newRole: 'master' | 'follower') => {
    setRole(newRole)
    if (newRole === 'master') {
      setIpType('na')
      setIp('')
    } else {
      if (ipType() === 'na') {
        const pool = availableIPs()
        const b = broker()
        const v4 = getPrioritizedCompatibleIPs(pool.ipv4 || [], b)
        const v6 = getPrioritizedCompatibleIPs(pool.ipv6 || [], b)
        if (v4.length > 0) {
          setIpType('ipv4')
          setIp(v4[0].ipAddress)
        } else if (v6.length > 0) {
          setIpType('ipv6')
          setIp(v6[0].ipAddress)
        } else {
          setIpType('ipv4')
          setIp('')
        }
      }
    }
  }

  const handleIPTypeChange = (type: 'na' | 'ipv4' | 'ipv6') => {
    setIpType(type)
    if (type === 'na') {
      setIp('')
      return
    }
    const pool = availableIPs()
    const b = broker()
    const list = getPrioritizedCompatibleIPs(type === 'ipv4' ? (pool.ipv4 || []) : (pool.ipv6 || []), b)
    const existing = props.account?.ip
    if (existing && list.some((item) => item.ipAddress === existing)) {
      setIp(existing)
    } else if (list.length > 0) {
      setIp(list[0].ipAddress)
    } else {
      setIp('')
    }
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      const trimmedIP = ip().trim()
      if (trimmedIP && !isValidIP(trimmedIP)) {
        setError('Invalid IP Address: must be a valid IPv4 or IPv6 address.')
        return
      }

      const trimmedBroker = broker()
      const trimmedApiSecret = apiSecret().trim()
      if ((trimmedBroker === 'kite' || trimmedBroker === 'zerodha') && trimmedApiSecret && trimmedApiSecret.length > 20 && trimmedApiSecret.length !== 32) {
        setError(`Zerodha API Secret must be exactly 32 characters (currently ${trimmedApiSecret.length} characters).`)
        return
      }

      if (isEdit()) {
        const a = props.account!
        const patch: Record<string, unknown> = {}
        if (name() !== (a.name ?? '')) patch.name = name()
        if (broker() !== 'dhan') {
          const trimmedApiKey = apiKey().trim()
          if (trimmedApiKey !== (a.apiKey ?? '')) patch.apiKey = trimmedApiKey
          const trimmedApiSecret = apiSecret().trim()
          if (trimmedApiSecret !== (a.apiSecret ?? '')) patch.apiSecret = trimmedApiSecret
          const trimmedPass = password().trim()
          const trimmedTotp = totpSecret().trim()
          if (trimmedPass) patch.password = trimmedPass
          if (trimmedTotp) patch.totpSecret = trimmedTotp
        }
        const trimmedToken = accessToken().trim()
        if (trimmedToken) patch.accessToken = trimmedToken
        if (trimmedIP !== (a.ip ?? '')) {
          if (isFollower() && !trimmedIP) {
            setError('IP Address is required for follower accounts.')
            return
          }
          patch.ip = trimmedIP
        }
        if (isFollower()) {
          const currentGroupId = a.groupId ?? ''
          if (groupId() !== currentGroupId) {
            patch.groupId = groupId() // empty string "" signals detachment on backend
          }
          if (groupId()) {
            if (cloneFactor() !== (a.cloneFactor ?? '1')) patch.cloneFactor = cloneFactor()
            const maxQty = maxQtyPerOrder() === '' ? null : Number(maxQtyPerOrder())
            if (maxQty !== a.maxQtyPerOrder) patch.maxQtyPerOrder = maxQty
          }
        }
        if (status() !== a.status) patch.status = status()
        if (Object.keys(patch).length > 0) await props.onSave(a.id, patch)
      } else {
        const body: CreateAccountRequest = {
          name: name().trim() || brokerAccountId().trim(),
          role: role(),
          broker: broker(),
          brokerAccountId: brokerAccountId().trim(),
        }
        if (broker() !== 'dhan') {
          const trimmedApiKey = apiKey().trim()
          if (trimmedApiKey) body.apiKey = trimmedApiKey
          const trimmedApiSecret = apiSecret().trim()
          if (trimmedApiSecret) body.apiSecret = trimmedApiSecret
          const trimmedPass = password().trim()
          const trimmedTotp = totpSecret().trim()
          if (trimmedPass) body.password = trimmedPass
          if (trimmedTotp) body.totpSecret = trimmedTotp
        }
        const trimmedToken = accessToken().trim()
        if (trimmedToken) body.accessToken = trimmedToken
        if (trimmedIP) {
          body.ip = trimmedIP
        }
        if (isFollower()) {
          if (!trimmedIP) {
            setError('IP Address is required for follower accounts.')
            return
          }
          if (groupId()) {
            body.groupId = groupId()
            body.cloneFactor = cloneFactor() || '1'
            if (maxQtyPerOrder().trim()) {
              body.maxQtyPerOrder = Number(maxQtyPerOrder())
            }
          }
        }
        await props.onCreate(body)
      }
      props.onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Show when={props.open}>
      <div class="drawer-overlay" onClick={props.onClose}>
        <div
          role="dialog"
          aria-label={isEdit() ? 'Edit Account' : 'Add Account'}
          class="account-drawer"
          onClick={(e) => e.stopPropagation()}
        >
          <div class="drawer-header">
            <h2>{isEdit() ? 'Edit Account' : 'Add Account'}</h2>
            <button type="button" class="drawer-close" aria-label="Close" onClick={props.onClose}>
              ×
            </button>
          </div>
          <form onSubmit={handleSubmit} class="drawer-form">
            <Show when={props.account?.authStatus === 'error'}>
              <div
                class="auth-error-banner"
                role="alert"
                style={{
                  padding: '0.75rem',
                  'background-color': 'rgba(239, 68, 68, 0.1)',
                  border: '1px solid #ef4444',
                  'border-radius': '6px',
                  'margin-bottom': '1rem',
                  color: '#ef4444',
                  'font-size': '0.875rem',
                }}
              >
                <strong>Authentication required:</strong> {props.account?.authError || 'Failed to authenticate with broker.'}
                <br />
                {broker() === 'dhan'
                  ? 'Please enter your Dhan Access Token below to reconnect.'
                  : 'Please re-enter your Kite password and TOTP secret below to reconnect.'}
              </div>
            </Show>
            <label>
              Broker
              <div style={{ display: 'flex', 'align-items': 'center', gap: '0.65rem' }}>
                <ThemedSelect
                  aria-label="Broker"
                  value={broker()}
                  disabled={isEdit()}
                  onChange={(val) => handleBrokerChange(val)}
                  prefix={<BrokerLogo broker={broker()} size={20} />}
                  options={SUPPORTED_BROKERS.map((b) => ({
                    value: b.id,
                    label: b.name,
                    icon: () => <BrokerLogo broker={b.id} size={18} />,
                  }))}
                />
              </div>
            </label>
            <label>
              Role
              <ThemedSelect
                aria-label="Role"
                value={role()}
                disabled={isEdit()}
                onChange={(val) => handleRoleChange(val as 'master' | 'follower')}
                options={[
                  { value: 'follower', label: 'Follower' },
                  { value: 'master', label: 'Master' },
                ]}
              />
            </label>
            <label>
              Name
              <input
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="e.g. Primary Trading Account"
              />
            </label>
            <label>
              {broker() === 'dhan' ? 'Dhan Client ID' : 'Broker User ID'}
              <input
                value={brokerAccountId()}
                disabled={isEdit()}
                onInput={(e) => setBrokerAccountId(e.currentTarget.value)}
                placeholder={broker() === 'dhan' ? 'e.g. 1000000001' : ''}
                required
              />
            </label>
            <Show when={broker() === 'dhan'}>
              <label>
                Access Token (JWT)
                <input
                  type="password"
                  value={accessToken()}
                  onInput={(e) => setAccessToken(e.currentTarget.value)}
                  placeholder={isEdit() ? '•••••••• (leave blank to keep unchanged)' : '24-hour Access Token from Dhan portal'}
                  autocomplete="new-password"
                />
              </label>
            </Show>
            <Show when={broker() !== 'dhan'}>
              <label>
                API Key
                <input value={apiKey()} onInput={(e) => setApiKey(e.currentTarget.value)} />
              </label>
              <label>
                API Secret
                <input value={apiSecret()} onInput={(e) => setApiSecret(e.currentTarget.value.trim())} />
              </label>
              <Show when={(broker() === 'kite' || broker() === 'zerodha') && apiSecret().trim().length > 0}>
                <span
                  style={{
                    'font-size': '0.75rem',
                    color: apiSecret().trim().length === 32 ? 'var(--color-accent)' : '#e5484d',
                    'margin-top': '0.25rem',
                    'margin-bottom': '0.5rem',
                    display: 'block',
                  }}
                >
                  {apiSecret().trim().length}/32 characters{apiSecret().trim().length !== 32 ? ' (must be 32)' : ' (valid length)'}
                </span>
              </Show>
              <label>
                Password (Kite Login)
                <input
                  type="password"
                  value={password()}
                  onInput={(e) => setPassword(e.currentTarget.value)}
                  placeholder={isEdit() ? '•••••••• (leave blank to keep unchanged)' : 'Zerodha Kite Password'}
                  autocomplete="new-password"
                />
              </label>
              <label>
                TOTP Secret Key
                <input
                  type="text"
                  value={totpSecret()}
                  onInput={(e) => setTotpSecret(e.currentTarget.value)}
                  placeholder={isEdit() ? 'Leave blank to keep unchanged' : 'Base32 2FA secret (e.g. JBSWY3DPEHPK3PXP)'}
                />
              </label>
            </Show>
            <label>
              IP Type {isFollower() ? '*' : ''}
              <ThemedSelect
                aria-label="IP Type"
                value={ipType()}
                onChange={(val) => handleIPTypeChange(val as 'na' | 'ipv4' | 'ipv6')}
                options={[
                  ...(!isFollower() ? [{ value: 'na', label: 'NA (Direct / No Proxy)' }] : []),
                  { value: 'ipv4', label: 'IPv4' },
                  { value: 'ipv6', label: 'IPv6' },
                ]}
              />
            </label>
            <label>
              IP Address {isFollower() ? '*' : '(Optional)'}
              <div ref={containerRef} style={{ position: 'relative' }}>
                <input
                  autocomplete="off"
                  value={ip()}
                  style={{
                    'padding-right': compatibleIPs().length > 0 ? '2.25rem' : '0.75rem',
                  }}
                  onClick={(e) => {
                    e.currentTarget.select()
                    if (compatibleIPs().length > 0) setIsDropdownOpen(true)
                  }}
                  onFocus={(e) => e.currentTarget.select()}
                  onInput={(e) => {
                    setIp(e.currentTarget.value)
                    const val = e.currentTarget.value.trim()
                    if (!val) {
                      if (!isFollower()) setIpType('na')
                    } else if (val.includes(':')) {
                      setIpType('ipv6')
                    } else {
                      setIpType('ipv4')
                    }
                  }}
                  placeholder={ipType() === 'na' ? 'None (Direct connection)' : 'e.g. 148.113.41.42 or 2402:1f00:...'}
                />
                <Show when={compatibleIPs().length > 0}>
                  <button
                    type="button"
                    aria-label="Toggle IP dropdown"
                    tabIndex={-1}
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      setIsDropdownOpen(!isDropdownOpen())
                    }}
                    style={{
                      position: 'absolute',
                      right: '0.5rem',
                      top: '50%',
                      transform: 'translateY(-50%)',
                      background: 'transparent',
                      border: 'none',
                      padding: '0.25rem',
                      cursor: 'pointer',
                      color: 'var(--color-text-muted, #94a3b8)',
                      display: 'flex',
                      'align-items': 'center',
                      'justify-content': 'center',
                      'border-radius': '4px',
                    }}
                  >
                    <svg
                      width="14"
                      height="14"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      style={{
                        transform: isDropdownOpen() ? 'rotate(180deg)' : 'rotate(0deg)',
                        transition: 'transform 0.15s ease',
                      }}
                    >
                      <polyline points="6 9 12 15 18 9" />
                    </svg>
                  </button>
                </Show>
                <Show when={isDropdownOpen() && compatibleIPs().length > 0}>
                  <div
                    role="listbox"
                    aria-label="Compatible Proxy IPs"
                    onMouseDown={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                    }}
                    onClick={(e) => e.stopPropagation()}
                    style={{
                      position: 'absolute',
                      top: 'calc(100% + 4px)',
                      left: 0,
                      right: 0,
                      'z-index': 50,
                      background: 'var(--color-surface, #1a1c24)',
                      border: '1px solid var(--color-border, #2a2c34)',
                      'border-radius': '8px',
                      'box-shadow': '0 10px 25px -5px rgba(0, 0, 0, 0.3), 0 8px 10px -6px rgba(0, 0, 0, 0.2)',
                      'max-height': '220px',
                      'overflow-y': 'auto',
                      padding: '0.35rem',
                    }}
                  >
                    <For each={compatibleIPs()}>
                      {(item) => {
                        const isSelected = () => item.ipAddress === ip()
                        const isSpecialized = () => Boolean(item.whitelistedBrokers && item.whitelistedBrokers.trim())
                        return (
                          <div
                            role="option"
                            aria-selected={isSelected()}
                            onMouseDown={(e) => {
                              e.preventDefault()
                              e.stopPropagation()
                            }}
                            onClick={(e) => {
                              e.preventDefault()
                              e.stopPropagation()
                              setIp(item.ipAddress)
                              setIsDropdownOpen(false)
                            }}
                            style={{
                              display: 'flex',
                              'align-items': 'center',
                              'justify-content': 'space-between',
                              padding: '0.5rem 0.65rem',
                              'border-radius': '6px',
                              cursor: 'pointer',
                              'font-size': '0.85rem',
                              background: isSelected() ? 'rgba(4, 180, 136, 0.12)' : 'transparent',
                              color: isSelected() ? 'var(--color-accent, #04b488)' : 'var(--color-text, #f0f0f2)',
                              'font-weight': isSelected() ? 600 : 400,
                              transition: 'background 0.12s ease',
                              'margin-bottom': '2px',
                            }}
                            onMouseEnter={(e) => {
                              if (!isSelected()) e.currentTarget.style.background = 'rgba(255, 255, 255, 0.05)'
                            }}
                            onMouseLeave={(e) => {
                              if (!isSelected()) e.currentTarget.style.background = 'transparent'
                            }}
                          >
                            <div style={{ display: 'flex', 'align-items': 'center', gap: '0.5rem' }}>
                              <span style={{ 'font-family': 'monospace', 'font-size': '0.875rem' }}>
                                {item.ipAddress}
                              </span>
                              <Show when={isSelected()}>
                                <svg
                                  width="14"
                                  height="14"
                                  viewBox="0 0 24 24"
                                  fill="none"
                                  stroke="currentColor"
                                  stroke-width="2.5"
                                  stroke-linecap="round"
                                  stroke-linejoin="round"
                                >
                                  <polyline points="20 6 9 17 4 12" />
                                </svg>
                              </Show>
                            </div>
                            <span
                              style={{
                                'font-size': '0.7rem',
                                padding: '0.15rem 0.45rem',
                                'border-radius': '4px',
                                'font-weight': 600,
                                background: isSpecialized() ? 'rgba(4, 180, 136, 0.15)' : 'rgba(100, 116, 139, 0.15)',
                                color: isSpecialized() ? 'var(--color-accent, #04b488)' : 'var(--color-text-muted, #94a3b8)',
                                border: isSpecialized()
                                  ? '1px solid rgba(4, 180, 136, 0.3)'
                                  : '1px solid rgba(100, 116, 139, 0.25)',
                              }}
                            >
                              {isSpecialized() ? `${item.whitelistedBrokers} Specialized` : 'All Brokers'}
                            </span>
                          </div>
                        )
                      }}
                    </For>
                  </div>
                </Show>
              </div>
              <Show when={ipType() !== 'na' && !ip()}>
                <span style={{ color: '#ef4444', 'font-size': '0.75rem', 'margin-top': '0.25rem', display: 'block' }}>
                  No available {ipType().toUpperCase()} address in proxy pool.
                </span>
              </Show>
              <Show when={ip()}>
                <span style={{ color: '#059669', 'font-size': '0.75rem', 'margin-top': '0.25rem', display: 'block' }}>
                  Assigned {ipType() === 'ipv6' ? 'IPv6' : 'IPv4'} proxy IP: <strong>{ip()}</strong>
                  <Show when={compatibleIPs().find((p) => p.ipAddress === ip())?.whitelistedBrokers}>
                    {' '}
                    <span
                      style={{
                        'font-size': '0.7rem',
                        padding: '0.1rem 0.45rem',
                        'border-radius': '4px',
                        background: 'rgba(5, 150, 105, 0.12)',
                        color: '#059669',
                        'font-weight': 600,
                        'margin-left': '0.4rem',
                      }}
                    >
                      {compatibleIPs().find((p) => p.ipAddress === ip())!.whitelistedBrokers} Specialized
                    </span>
                  </Show>
                  <Show when={compatibleIPs().some((p) => p.ipAddress === ip()) && !compatibleIPs().find((p) => p.ipAddress === ip())?.whitelistedBrokers}>
                    {' '}
                    <span
                      style={{
                        'font-size': '0.7rem',
                        padding: '0.1rem 0.45rem',
                        'border-radius': '4px',
                        background: 'rgba(100, 116, 139, 0.12)',
                        color: 'var(--color-text-muted, #64748b)',
                        'font-weight': 500,
                        'margin-left': '0.4rem',
                      }}
                    >
                      All Brokers (Unrestricted)
                    </span>
                  </Show>
                </span>
              </Show>
            </label>
            <Show when={isFollower()}>
              <label>
                Group (Optional)
                <ThemedSelect
                  aria-label="Group (Optional)"
                  value={groupId()}
                  onChange={(val) => setGroupId(val)}
                  options={[
                    { value: '', label: 'None (Unassigned)' },
                    ...(props.groups || []).map((g) => ({
                      value: g.id,
                      label: `${g.name} (${g.masterName || g.masterAccountId})`,
                    })),
                  ]}
                />
              </label>
              <Show when={groupId()}>
                <label>
                  Clone Factor
                  <input value={cloneFactor()} onInput={(e) => setCloneFactor(e.currentTarget.value)} />
                </label>
                <label>
                  Max Qty/Order (Optional)
                  <input
                    value={maxQtyPerOrder()}
                    onInput={(e) => setMaxQtyPerOrder(e.currentTarget.value)}
                    placeholder="Leave blank for unlimited"
                  />
                </label>
              </Show>
            </Show>
            <Show when={isEdit()}>
              <label>
                Status
                <ThemedSelect
                  aria-label="Status"
                  value={status()}
                  onChange={(val) => setStatus(val)}
                  options={[
                    { value: 'ok', label: 'ok', badge: { text: 'OK', variant: 'emerald' } },
                    { value: 'error', label: 'error', badge: { text: 'Error', variant: 'amber' } },
                  ]}
                />
              </label>
            </Show>
            <Show when={error()}>{(msg) => <p role="alert">{msg()}</p>}</Show>
            <div class="drawer-actions">
              <button type="button" class="drawer-btn-cancel" onClick={props.onClose}>
                Cancel
              </button>
              <button type="submit" class="drawer-btn-save" disabled={saving()}>
                Save
              </button>
            </div>
          </form>
        </div>
      </div>
    </Show>
  )
}
