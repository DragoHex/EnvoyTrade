import { createEffect, createSignal, Show } from 'solid-js'
import type { Account } from '../api'

export function EditGroupModal(props: {
  open: boolean
  initialName: string
  currentMasterId: string
  masters: Account[]
  onClose: () => void
  onSave: (name: string, masterId: string) => Promise<void>
}) {
  const [name, setName] = createSignal('')
  const [masterId, setMasterId] = createSignal('')
  const [error, setError] = createSignal<string | null>(null)
  const [saving, setSaving] = createSignal(false)

  createEffect(() => {
    if (props.open) {
      setName(props.initialName)
      setMasterId(props.currentMasterId)
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
      await props.onSave(name().trim(), masterId())
      props.onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update group.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Show when={props.open}>
      <div class="confirm-modal-overlay" onClick={props.onClose}>
        <div
          role="dialog"
          aria-label="Edit Group"
          class="confirm-modal"
          style={{ width: '380px', padding: '1.25rem' }}
          onClick={(e) => e.stopPropagation()}
        >
          <div class="drawer-header" style={{ "margin-bottom": "0.75rem" }}>
            <h3 style={{ margin: 0 }}>Edit Group</h3>
            <button type="button" class="drawer-close" aria-label="Close" onClick={props.onClose}>
              ×
            </button>
          </div>
          <form onSubmit={handleSubmit} class="drawer-form" style={{ padding: 0, gap: '0.75rem' }}>
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
              Master Account (Swap Master)
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
            <div class="confirm-modal-actions" style={{ "margin-top": "0.75rem" }}>
              <button type="button" class="confirm-modal-cancel" onClick={props.onClose}>
                Cancel
              </button>
              <button type="submit" class="confirm-modal-confirm" disabled={saving()}>
                Save
              </button>
            </div>
          </form>
        </div>
      </div>
    </Show>
  )
}
