import { createEffect, createSignal, For, on, Show, untrack } from 'solid-js'
import {
  fetchAvailableProxyIPs,
  type Account,
  type AvailableProxyIPsResponse,
  type CreateAccountRequest,
  type GroupSummary,
} from '../api'
import { BrokerLogo, SUPPORTED_BROKERS } from './BrokerLogo'
import { isValidIP } from '../utils/ip'

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
  const [availableIPs, setAvailableIPs] = createSignal<AvailableProxyIPsResponse>({ ipv4: [], ipv6: [] })
  const [ipType, setIpType] = createSignal<'na' | 'ipv4' | 'ipv6'>('ipv4')
  const [ip, setIp] = createSignal('')
  const [cloneFactor, setCloneFactor] = createSignal('1')
  const [maxQtyPerOrder, setMaxQtyPerOrder] = createSignal('')
  const [groupId, setGroupId] = createSignal('')
  const [status, setStatus] = createSignal('ok')
  const [error, setError] = createSignal<string | null>(null)
  const [saving, setSaving] = createSignal(false)

  createEffect(
    on(
      () => [props.open, props.account?.id] as const,
      ([isOpen]) => {
        if (!isOpen) return
        const a = props.account
        setName(a?.name ?? '')
        const currentRole = a?.role ?? 'follower'
        setRole(currentRole)
        setBroker(a?.broker ?? 'kite')
        setBrokerAccountId(a?.brokerAccountId ?? '')
        setApiKey(a?.apiKey ?? '')
        setApiSecret(a?.apiSecret ?? '')
        setPassword('')
        setTotpSecret('')
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
            if (!a && currentRole === 'follower' && !untrack(ip)) {
              if (ips.ipv4 && ips.ipv4.length > 0) {
                setIpType('ipv4')
                setIp(ips.ipv4[0].ipAddress)
              } else if (ips.ipv6 && ips.ipv6.length > 0) {
                setIpType('ipv6')
                setIp(ips.ipv6[0].ipAddress)
              }
            }
          })
          .catch(() => {})
      }
    )
  )

  const isFollower = () => role() === 'follower'
  const isEdit = () => props.account != null

  const handleRoleChange = (newRole: 'master' | 'follower') => {
    setRole(newRole)
    if (newRole === 'master') {
      setIpType('na')
      setIp('')
    } else {
      if (ipType() === 'na') {
        const pool = availableIPs()
        if (pool.ipv4 && pool.ipv4.length > 0) {
          setIpType('ipv4')
          setIp(pool.ipv4[0].ipAddress)
        } else if (pool.ipv6 && pool.ipv6.length > 0) {
          setIpType('ipv6')
          setIp(pool.ipv6[0].ipAddress)
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
    const list = type === 'ipv4' ? (pool.ipv4 || []) : (pool.ipv6 || [])
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

      if (isEdit()) {
        const a = props.account!
        const patch: Record<string, unknown> = {}
        if (name() !== (a.name ?? '')) patch.name = name()
        const trimmedApiKey = apiKey().trim()
        if (trimmedApiKey !== (a.apiKey ?? '')) patch.apiKey = trimmedApiKey
        const trimmedApiSecret = apiSecret().trim()
        if (trimmedApiSecret !== (a.apiSecret ?? '')) patch.apiSecret = trimmedApiSecret
        const trimmedPass = password().trim()
        const trimmedTotp = totpSecret().trim()
        if (trimmedPass) patch.password = trimmedPass
        if (trimmedTotp) patch.totpSecret = trimmedTotp
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
          apiKey: apiKey().trim(),
          apiSecret: apiSecret().trim(),
        }
        const trimmedPass = password().trim()
        const trimmedTotp = totpSecret().trim()
        if (trimmedPass) body.password = trimmedPass
        if (trimmedTotp) body.totpSecret = trimmedTotp
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
                <strong>Authentication required:</strong> {props.account?.authError || 'Failed to authenticate with Kite.'}
                <br />
                Please re-enter your Kite password and TOTP secret below to reconnect.
              </div>
            </Show>
            <label>
              Broker
              <div style={{ display: 'flex', 'align-items': 'center', gap: '0.65rem' }}>
                <BrokerLogo broker={broker()} size={20} />
                <select
                  value={broker()}
                  disabled={isEdit()}
                  onChange={(e) => setBroker(e.currentTarget.value)}
                  style={{ flex: '1' }}
                >
                  <For each={SUPPORTED_BROKERS}>
                    {(b) => <option value={b.id}>{b.name}</option>}
                  </For>
                </select>
              </div>
            </label>
            <label>
              Role
              <select
                value={role()}
                disabled={isEdit()}
                onChange={(e) => handleRoleChange(e.currentTarget.value as 'master' | 'follower')}
              >
                <option value="follower">Follower</option>
                <option value="master">Master</option>
              </select>
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
              Broker User ID
              <input
                value={brokerAccountId()}
                disabled={isEdit()}
                onInput={(e) => setBrokerAccountId(e.currentTarget.value)}
                required
              />
            </label>
            <label>
              API Key
              <input value={apiKey()} onInput={(e) => setApiKey(e.currentTarget.value)} />
            </label>
            <label>
              API Secret
              <input value={apiSecret()} onInput={(e) => setApiSecret(e.currentTarget.value)} />
            </label>
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
            <label>
              IP Type {isFollower() ? '*' : ''}
              <select
                aria-label="IP Type"
                value={ipType()}
                onChange={(e) => handleIPTypeChange(e.currentTarget.value as 'na' | 'ipv4' | 'ipv6')}
              >
                <Show when={!isFollower()}>
                  <option value="na">NA (Direct / No Proxy)</option>
                </Show>
                <option value="ipv4">IPv4</option>
                <option value="ipv6">IPv6</option>
              </select>
            </label>
            <label>
              IP Address {isFollower() ? '*' : '(Optional)'}
              <input
                value={ip()}
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
              <Show when={ipType() !== 'na' && !ip()}>
                <span style={{ color: '#ef4444', 'font-size': '0.75rem', 'margin-top': '0.25rem', display: 'block' }}>
                  No available {ipType().toUpperCase()} address in proxy pool.
                </span>
              </Show>
              <Show when={ip()}>
                <span style={{ color: '#059669', 'font-size': '0.75rem', 'margin-top': '0.25rem', display: 'block' }}>
                  Assigned {ipType() === 'ipv6' ? 'IPv6' : 'IPv4'} proxy IP: <strong>{ip()}</strong>
                </span>
              </Show>
            </label>
            <Show when={isFollower()}>
              <label>
                Group (Optional)
                <select value={groupId()} onChange={(e) => setGroupId(e.currentTarget.value)}>
                  <option value="">None (Unassigned)</option>
                  <For each={props.groups || []}>
                    {(g) => (
                      <option value={g.id}>
                        {g.name} ({g.masterName || g.masterAccountId})
                      </option>
                    )}
                  </For>
                </select>
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
                <select value={status()} onChange={(e) => setStatus(e.currentTarget.value)}>
                  <option value="ok">ok</option>
                  <option value="error">error</option>
                </select>
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
