import { createSignal, onCleanup, Show } from 'solid-js'
import { CopyToggle } from './CopyToggle'
import { ConfirmActionModal } from './ConfirmActionModal'
import { ResultToast, type ToastResult } from './ResultToast'
import { StatusDot } from './StatusDot'
import { BlockIcon, CropSquareIcon, LogoutIcon, PlayCircleIcon, SyncIcon } from './icons'
import type { GroupFollower } from '../api'

type DestructiveAction = 'square_off' | 'exit_open_orders' | null
const TOAST_DISMISS_MS = 4000

export function AccountRow(props: {
  follower: GroupFollower
  isMaster?: boolean
  actionsDisabled?: boolean
  toggleDisabled?: boolean
  onToggleCopy: (next: boolean) => void
  onRebalance: () => Promise<void>
  onSquareOff: () => void
  onExitOpenOrders: () => void
}) {
  const [pending, setPending] = createSignal<DestructiveAction>(null)
  const [rebalancing, setRebalancing] = createSignal(false)
  const [toast, setToast] = createSignal<ToastResult | null>(null)
  let dismissTimer: ReturnType<typeof setTimeout> | undefined
  onCleanup(() => clearTimeout(dismissTimer))

  const showToast = (result: ToastResult) => {
    setToast(result)
    clearTimeout(dismissTimer)
    dismissTimer = setTimeout(() => setToast(null), TOAST_DISMISS_MS)
  }

  const handleRebalance = async () => {
    setRebalancing(true)
    try {
      await props.onRebalance()
      showToast({ kind: 'success', message: 'Rebalance triggered successfully.' })
    } catch (e) {
      showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Rebalance failed.' })
    } finally {
      setRebalancing(false)
    }
  }

  const confirm = () => {
    if (pending() === 'square_off') props.onSquareOff()
    if (pending() === 'exit_open_orders') props.onExitOpenOrders()
    setPending(null)
  }

  return (
    <tr
      data-testid={props.isMaster ? 'master-row' : 'account-row'}
      data-disabled={!!props.actionsDisabled}
    >
      <td>
        <Show when={!props.isMaster}>
          <CopyToggle
            enabled={props.follower.enabled}
            disabled={props.toggleDisabled}
            onToggle={props.onToggleCopy}
          />
        </Show>
        <Show when={props.isMaster}>
          <button
            type="button"
            class="icon-button icon-button-danger"
            aria-label={props.follower.enabled ? 'Stop Copy' : 'Start Copy'}
            data-tooltip={props.follower.enabled ? 'Stop' : 'Start'}
            onClick={() => props.onToggleCopy(!props.follower.enabled)}
          >
            {props.follower.enabled ? <BlockIcon /> : <PlayCircleIcon />}
          </button>
        </Show>
      </td>
      <td>{props.follower.name || '—'}</td>
      <td>{props.follower.brokerAccountId}</td>
      <td>—</td>
      <td>—</td>
      <td>—</td>
      <td>—</td>
      <td>—</td>
      <td>—</td>
      <td>
        <StatusDot status={props.follower.status} />
      </td>
      <td>
        <button
          type="button"
          class="icon-button icon-button-primary"
          aria-label="Rebalance"
          data-tooltip="Rebalance"
          disabled={rebalancing() || props.actionsDisabled}
          onClick={handleRebalance}
        >
          <SyncIcon spinning={rebalancing()} />
        </button>
        <button
          type="button"
          class="icon-button icon-button-danger"
          aria-label="Square Off"
          data-tooltip="Sq.off"
          disabled={props.actionsDisabled}
          onClick={() => setPending('square_off')}
        >
          <CropSquareIcon />
        </button>
        <button
          type="button"
          class="icon-button icon-button-danger"
          aria-label="Exit Open Orders"
          data-tooltip="Exit Open Orders"
          disabled={props.actionsDisabled}
          onClick={() => setPending('exit_open_orders')}
        >
          <LogoutIcon />
        </button>
      </td>
      <ConfirmActionModal
        open={pending() !== null}
        label={pending() === 'square_off' ? 'Square Off' : 'Exit Open Orders'}
        onConfirm={confirm}
        onCancel={() => setPending(null)}
      />
      <ResultToast result={toast()} onDismiss={() => setToast(null)} />
    </tr>
  )
}
