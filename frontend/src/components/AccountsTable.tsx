import { Show, createSignal } from 'solid-js'
import type { Account } from '../api'
import { DataTable, type Column } from './DataTable'
import { StatusDot } from './StatusDot'
import { EditIcon, TrashIcon, UserMinusIcon } from './icons'
import { BrokerLogo } from './BrokerLogo'
import { CopyToggle } from './CopyToggle'
import { RoleIcon } from './RoleIcon'

// AccountsTable lists all accounts across both roles with quick-action
// toggles and row-level edit/remove/delete buttons (docs/UI-PLAN.md §3).
export function AccountsTable(props: {
  accounts: Account[]
  onEdit: (account: Account) => void
  onRemoveFromGroup?: (account: Account) => void
  onDelete?: (account: Account) => void
  onToggleActive?: (id: string, nextActive: boolean) => Promise<void>
}) {
  const [togglingId, setTogglingId] = createSignal<string | null>(null)

  const handleToggle = async (a: Account, next: boolean) => {
    if (!props.onToggleActive) return
    setTogglingId(a.id)
    try {
      await props.onToggleActive(a.id, next)
    } catch {
      // Toast/error handling is managed by onToggleActive in AccountsPage
    } finally {
      setTogglingId(null)
    }
  }

  const columns: Column<Account>[] = [
    {
      header: '',
      cell: (a) => (
        <CopyToggle
          enabled={a.active}
          disabled={!props.onToggleActive || togglingId() === a.id}
          ariaLabel={`Toggle active for ${a.name || a.brokerAccountId}`}
          title={a.active ? 'Active — click to deactivate' : 'Inactive — click to activate'}
          onToggle={(next) => handleToggle(a, next)}
        />
      ),
    },
    {
      header: 'Name',
      cell: (a) => (
        <span class="account-name-cell" title={a.name || a.brokerAccountId}>
          {a.name || '—'}
        </span>
      ),
    },
    {
      header: 'Role',
      headerClass: 'col-role',
      cellClass: 'col-role',
      cell: (a) => (
        <span class="role-cell-centered">
          <RoleIcon role={a.role} />
        </span>
      ),
    },
    {
      header: 'Broker User ID',
      cell: (a) => a.brokerAccountId,
    },
    {
      header: 'Broker',
      cell: (a) => <BrokerLogo broker={a.broker} />,
    },
    {
      header: 'Group',
      cell: (a) => (
        <Show when={a.groupName || a.groupId || a.masterId} fallback={<span>—</span>}>
          <span>{a.groupName || a.groupId || a.masterId}</span>
        </Show>
      ),
    },
    {
      header: 'Capital Ratio',
      cell: (a) => a.capitalRatio ?? '—',
    },
    {
      header: 'Max Qty/Order',
      cell: (a) => (a.maxQtyPerOrder != null ? a.maxQtyPerOrder : '—'),
    },
    {
      header: 'Status',
      headerClass: 'col-status',
      cellClass: 'col-status',
      cell: (a) => (
        <span class="status-cell-centered" title={a.status}>
          <StatusDot status={a.status} />
        </span>
      ),
    },
    {
      header: '',
      cell: (a) => (
        <div class="table-actions">
          <button
            type="button"
            class="icon-button"
            aria-label="Edit"
            data-tooltip="Edit Account"
            title="Edit Account"
            onClick={() => props.onEdit(a)}
          >
            <EditIcon />
          </button>
          {a.role === 'follower' && (a.masterId != null || a.groupId != null) && props.onRemoveFromGroup && (
            <button
              type="button"
              class="icon-button icon-button-danger"
              aria-label="Remove from group"
              data-tooltip="Remove from Group"
              title="Remove from Group"
              onClick={() => props.onRemoveFromGroup!(a)}
            >
              <UserMinusIcon />
            </button>
          )}
          {props.onDelete && (
            <button
              type="button"
              class="icon-button icon-button-danger"
              aria-label="Delete"
              data-tooltip="Delete Account"
              title="Delete Account"
              onClick={() => props.onDelete!(a)}
            >
              <TrashIcon />
            </button>
          )}
        </div>
      ),
    },
  ]

  return <DataTable tableClass="accounts-table" columns={columns} rows={props.accounts} rowKey={(a) => a.id} />
}
