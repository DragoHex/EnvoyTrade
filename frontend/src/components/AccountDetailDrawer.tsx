import { createEffect, createSignal, Show } from 'solid-js'
import type { Account, CreateAccountRequest } from '../api'

// AccountDetailDrawer is the Accounts page's create/edit form
// (docs/UI-PLAN.md's field table). account=null means create mode;
// otherwise it edits that account. Role is fixed at create time and
// locked thereafter (follow_links.master_id can't be reassigned here).
export function AccountDetailDrawer(props: {
  open: boolean
  account: Account | null
  masters: Account[]
  onClose: () => void
  onCreate: (body: CreateAccountRequest) => Promise<void>
  onSave: (id: string, patch: Record<string, unknown>) => Promise<void>
}) {
  const [role, setRole] = createSignal<'master' | 'follower'>('master')
  const [brokerAccountId, setBrokerAccountId] = createSignal('')
  const [apiKey, setApiKey] = createSignal('')
  const [apiSecret, setApiSecret] = createSignal('')
  const [capitalRatio, setCapitalRatio] = createSignal('')
  const [maxQtyPerOrder, setMaxQtyPerOrder] = createSignal('')
  const [masterId, setMasterId] = createSignal('')
  const [status, setStatus] = createSignal('ok')
  const [error, setError] = createSignal<string | null>(null)
  const [saving, setSaving] = createSignal(false)

  createEffect(() => {
    if (!props.open) return
    const a = props.account
    setRole(a?.role ?? 'master')
    setBrokerAccountId(a?.brokerAccountId ?? '')
    setApiKey('')
    setApiSecret('')
    setCapitalRatio(a?.capitalRatio ?? '')
    setMaxQtyPerOrder(a?.maxQtyPerOrder != null ? String(a.maxQtyPerOrder) : '')
    setMasterId(a?.masterId ?? '')
    setStatus(a?.status ?? 'ok')
    setError(null)
  })

  const isFollower = () => role() === 'follower'
  const isEdit = () => props.account != null

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      if (isEdit()) {
        const a = props.account!
        const patch: Record<string, unknown> = {}
        if (isFollower()) {
          if (capitalRatio() !== (a.capitalRatio ?? '')) patch.capitalRatio = capitalRatio()
          const maxQty = maxQtyPerOrder() === '' ? null : Number(maxQtyPerOrder())
          if (maxQty !== a.maxQtyPerOrder) patch.maxQtyPerOrder = maxQty
        }
        if (status() !== a.status) patch.status = status()
        if (Object.keys(patch).length > 0) await props.onSave(a.id, patch)
      } else {
        const body: CreateAccountRequest = {
          role: role(), broker: 'kite', brokerAccountId: brokerAccountId(),
          apiKey: apiKey(), apiSecret: apiSecret(),
        }
        if (isFollower()) {
          body.capitalRatio = capitalRatio()
          body.maxQtyPerOrder = Number(maxQtyPerOrder())
          body.masterId = masterId()
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
      <div role="dialog" aria-label={isEdit() ? 'Edit Account' : 'Add Account'} class="account-drawer">
        <form onSubmit={handleSubmit}>
          <label>
            Broker
            <select value="kite" disabled>
              <option value="kite">Kite</option>
            </select>
          </label>
          <label>
            Role
            <select
              value={role()}
              disabled={isEdit()}
              onChange={(e) => setRole(e.currentTarget.value as 'master' | 'follower')}
            >
              <option value="master">master</option>
              <option value="follower">follower</option>
            </select>
          </label>
          <label>
            Broker User ID
            <input
              value={brokerAccountId()}
              disabled={isEdit()}
              onInput={(e) => setBrokerAccountId(e.currentTarget.value)}
            />
          </label>
          <label>
            API Key
            <input value={apiKey()} disabled={isEdit()} onInput={(e) => setApiKey(e.currentTarget.value)} />
          </label>
          <label>
            API Secret
            <input value={apiSecret()} disabled={isEdit()} onInput={(e) => setApiSecret(e.currentTarget.value)} />
          </label>
          <Show when={isFollower()}>
            <label>
              Capital Ratio
              <input value={capitalRatio()} onInput={(e) => setCapitalRatio(e.currentTarget.value)} />
            </label>
            <label>
              Max Qty/Order
              <input value={maxQtyPerOrder()} onInput={(e) => setMaxQtyPerOrder(e.currentTarget.value)} />
            </label>
            <Show when={!isEdit()}>
              <label>
                Master
                <select value={masterId()} onChange={(e) => setMasterId(e.currentTarget.value)}>
                  <option value="" disabled>
                    select a master
                  </option>
                  {props.masters.map((m) => (
                    <option value={m.id}>{m.brokerAccountId}</option>
                  ))}
                </select>
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
          <button type="button" onClick={props.onClose}>
            Cancel
          </button>
          <button type="submit" disabled={saving()}>
            Save
          </button>
        </form>
      </div>
    </Show>
  )
}
