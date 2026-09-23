import { createResource, createSignal, onCleanup, Show } from 'solid-js'
import { CopyToggle } from './CopyToggle'
import { ConfirmActionModal } from './ConfirmActionModal'
import { ResultToast, type ToastResult } from './ResultToast'
import { StatusDot } from './StatusDot'
import { BlockIcon, CropSquareIcon, LogoutIcon, PlayCircleIcon, SyncIcon, ChevronDownIcon } from './icons'
import { AccountOrderDetails } from './AccountOrderDetails'
import { getAccountOrders, type GroupFollower } from '../api'

type DestructiveAction = 'square_off' | 'exit_open_orders' | null
const TOAST_DISMISS_MS = 4000

function formatCurrency(val: unknown): string {
  if (val === null || val === undefined || val === '') return '—'
  const n = typeof val === 'number' ? val : Number(val)
  if (Number.isNaN(n)) return '—'
  const sign = n < 0 ? '-' : ''
  const abs = Math.abs(n)
  return `${sign}₹${abs.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function AccountRow(props: {
  follower: GroupFollower
  isMaster?: boolean
  actionsDisabled?: boolean
  toggleDisabled?: boolean
  onToggleCopy: (next: boolean) => void | Promise<void>
  onRebalance: () => Promise<void>
  onSquareOff: () => void
  onExitOpenOrders: () => void
}) {
  const [pending, setPending] = createSignal<DestructiveAction>(null)
  const [rebalancing, setRebalancing] = createSignal(false)
  const [toast, setToast] = createSignal<ToastResult | null>(null)
  const [expanded, setExpanded] = createSignal(false)
  let dismissTimer: ReturnType<typeof setTimeout> | undefined
  onCleanup(() => clearTimeout(dismissTimer))

  // Fetch account orders/metrics on row expansion or if already expanded
  const [ordersData] = createResource(
    () => (expanded() ? props.follower.accountId : null),
    (accId) => getAccountOrders(accId, 'open_positions'),
  )

  const summary = () => ordersData()?.summary

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
    <>
      <tr
        data-testid={props.isMaster ? 'master-row' : 'account-row'}
        data-disabled={!!props.actionsDisabled}
        class={expanded() ? 'row-expanded' : ''}
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
              disabled={props.toggleDisabled}
              onClick={() => props.onToggleCopy(!props.follower.enabled)}
            >
              {props.follower.enabled ? <BlockIcon /> : <PlayCircleIcon />}
            </button>
          </Show>
        </td>
        <td>{props.follower.name || '—'}</td>
        <td>{props.follower.brokerAccountId}</td>
        <td>{summary() ? summary()!.netQty : (props.follower.netQty ?? '—')}</td>
        <td>
          {summary()
            ? `${summary()!.openPositionsCount ?? 0}/${summary()!.closedPositionsCount ?? 0}`
            : (props.follower.openPositionsCount != null ? `${props.follower.openPositionsCount}/${props.follower.closedPositionsCount ?? 0}` : '—')}
        </td>
        <td>{summary() ? (summary()!.pendingOrdersCount ?? 0) : (props.follower.openOrdersCount ?? '—')}</td>
        <td>{summary() ? formatCurrency(summary()!.totalMtm) : (props.follower.totalMtm != null ? formatCurrency(props.follower.totalMtm) : '—')}</td>
        <td>{summary() && summary()!.availableCash != null ? formatCurrency(summary()!.availableCash) : (props.follower.availableCash != null ? formatCurrency(props.follower.availableCash) : '—')}</td>
        <td>{summary() && summary()!.availableMargin != null ? formatCurrency(summary()!.availableMargin) : (props.follower.availableMargin != null ? formatCurrency(props.follower.availableMargin) : '—')}</td>
        <td>
          <StatusDot status={props.follower.status} />
        </td>
        <td>
          <div class="row-actions-group">
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
          </div>
        </td>
        <td class="row-expand-cell">
          <button
            type="button"
            class={`row-expand-toggle-btn ${expanded() ? 'row-expand-open' : ''}`}
            aria-label={expanded() ? 'Collapse details' : 'Expand details'}
            data-tooltip={expanded() ? 'Hide details' : 'View details'}
            onClick={() => setExpanded(!expanded())}
            data-testid="expand-row-btn"
          >
            <ChevronDownIcon class={`chevron-icon ${expanded() ? 'chevron-rotated' : ''}`} />
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

      <Show when={expanded()}>
        <tr class="account-details-expansion-row" data-testid="account-details-expansion-row">
          <td colspan="12" class="account-details-expansion-cell">
            <AccountOrderDetails
              accountId={props.follower.accountId}
              accountName={props.follower.name}
              brokerAccountId={props.follower.brokerAccountId}
            />
          </td>
        </tr>
      </Show>
    </>
  )
}
