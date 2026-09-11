import { createEffect, createSignal, Show } from 'solid-js'
import type { Account } from '../api'

export function CreateGroupModal(props: {
  open: boolean
  masters: Account[]
  onClose: () => void
  onCreate: (name: string, masterId: string) => Promise<void>
}) {
  const [name, setName] = createSignal('')
  const [masterId, setMasterId] = createSignal('')
  const [error, setError] = createSignal<string | null>(null)
  const [saving, setSaving] = createSignal(false)

  createEffect(() => {
    if (props.open) {
      setName('')
      setMasterId(props.masters.length > 0 ? props.masters[0].id : '')
      setError(null)
      setSaving(false)
    }
  })

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    if (!name().trim()) {
      setError('Group name is required.')
      return
    }
    if (!masterId()) {
      setError('A master account must be selected.')
      return
    }
    setSaving(true)
    setError(null)
    try {
      await props.onCreate(name().trim(), masterId())
      props.onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create group.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Show when={props.open}>
      <div class="confirm-modal-overlay" onClick={props.onClose}>
        <div
          role="dialog"
          aria-label="Create Group"
          class="confirm-modal"
          style={{ width: '400px', padding: '1.5rem' }}
          onClick={(e) => e.stopPropagation()}
        >
          <div class="drawer-header" style={{ "margin-bottom": "1rem" }}>
            <h3 style={{ margin: 0 }}>Create Group</h3>
            <button type="button" class="drawer-close" aria-label="Close" onClick={props.onClose}>
              ×
            </button>
          </div>
          <form onSubmit={handleSubmit} class="drawer-form">
            <label>
              Group Name
              <input
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="e.g. Momentum Nifty"
                required
              />
            </label>
            <label>
              Master Account
              <select
                value={masterId()}
                onChange={(e) => setMasterId(e.currentTarget.value)}
                required
              >
                <option value="" disabled>
                  select master account
                </option>
                {props.masters.map((m) => (
                  <option value={m.id}>
                    {m.name ? `${m.name} (${m.brokerAccountId})` : m.brokerAccountId}
                  </option>
                ))}
              </select>
            </label>
            <Show when={error()}>{(msg) => <p role="alert" style={{ color: "#e53e3e", margin: "0.5rem 0" }}>{msg()}</p>}</Show>
            <div class="confirm-modal-actions" style={{ "margin-top": "1rem" }}>
              <button type="button" class="confirm-modal-cancel" onClick={props.onClose}>
                Cancel
              </button>
              <button type="submit" class="confirm-modal-confirm" disabled={saving()}>
                Create
              </button>
            </div>
          </form>
        </div>
      </div>
    </Show>
  )
}
