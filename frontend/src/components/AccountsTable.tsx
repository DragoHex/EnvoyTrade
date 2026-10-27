import { Show, createSignal } from 'solid-js'
import type { Account, GroupSummary } from '../api'
import { DataTable, type Column } from './DataTable'
import { StatusDot } from './StatusDot'
import { ChevronDownIcon, EditIcon, TrashIcon, UserMinusIcon } from './icons'
import { BrokerLogo } from './BrokerLogo'
import { CopyToggle } from './CopyToggle'
import { RoleIcon } from './RoleIcon'
import { AccountHoldingsRow } from './AccountHoldingsRow'
import { formatAuthError } from '../utils/authError'

// AccountsTable lists accounts with quick-action toggles and row-level
// edit/remove/delete buttons (docs/plans/UI-PLAN.md §3).
export function AccountsTable(props: {
  accounts: Account[]
  groups?: GroupSummary[]
  emptyMessage?: string
  onEdit: (account: Account) => void
  onRemoveFromGroup?: (account: Account) => void
  onDelete?: (account: Account) => void
  onToggleActive?: (id: string, nextActive: boolean) => Promise<void>
}) {
  const [togglingId, setTogglingId] = createSignal<string | null>(null)
  const [expandedId, setExpandedId] = createSignal<string | null>(null)

  const resolveGroupName = (a: Account): string | null => {
    if (a.groupName && a.groupName.trim() !== '') {
      return a.groupName
    }
    if (props.groups && props.groups.length > 0) {
      if (a.groupId) {
        const matched = props.groups.find((g) => g.id === a.groupId)
        if (matched) return matched.name
      }
      if (a.masterId) {
        const matched = props.groups.find((g) => g.masterId === a.masterId || g.id === a.masterId)
        if (matched) return matched.name
      }
    }
    return null
  }

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

  const toggleExpand = (id: string) => {
    setExpandedId((curr) => (curr === id ? null : id))
  }

  const columns: Column<Account>[] = [
    {
      header: '',
      headerClass: 'col-toggle',
      cellClass: 'col-toggle',
      cell: (a) => (
        <CopyToggle
          enabled={a.active}
          disabled={!props.onToggleActive || togglingId() === a.id}
          ariaLabel={`Toggle active for ${a.name || a.brokerAccountId}`}
          onToggle={(next) => handleToggle(a, next)}
        />
      ),
    },
    {
      header: 'Name',
      headerClass: 'col-name',
      cellClass: 'col-name',
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
      headerClass: 'col-broker-id',
      cellClass: 'col-broker-id',
      cell: (a) => a.brokerAccountId,
    },
    {
      header: 'Broker',
      headerClass: 'col-broker',
      cellClass: 'col-broker',
      cell: (a) => <BrokerLogo broker={a.broker} />,
    },
    {
      header: 'Group',
      headerClass: 'col-group',
      cellClass: 'col-group',
      cell: (a) => {
        const name = resolveGroupName(a)
        return (
          <Show when={name} fallback={<span>—</span>}>
            <span>{name}</span>
          </Show>
        )
      },
    },
    {
      header: 'Clone Factor',
      headerClass: 'col-center col-clone-factor',
      cellClass: 'col-center col-clone-factor',
      cell: (a) => a.cloneFactor ?? '—',
    },
    {
      header: 'Max Qty/Order',
      headerClass: 'col-center col-max-qty',
      cellClass: 'col-center col-max-qty',
      cell: (a) => (a.maxQtyPerOrder != null ? a.maxQtyPerOrder : '—'),
    },
    {
      header: 'Status',
      headerClass: 'col-status',
      cellClass: 'col-status',
      cell: (a) => {
        const s = a.status === 'error' || a.authStatus === 'error' ? 'error' : (a.status === 'pending' || a.authStatus === 'pending' ? 'pending' : a.status)
        const tooltip = s === 'error' ? (a.authError ? `${formatAuthError(a.authError)} (click to edit)` : 'Account error - click to edit') : (s === 'pending' ? 'Connecting / Syncing...' : 'Active')
        return (
          <span
            class="status-cell-centered"
            title={tooltip}
            style={{ cursor: s === 'error' ? 'pointer' : 'default' }}
            onClick={(e) => {
              if (s === 'error') {
                e.stopPropagation()
                props.onEdit(a)
              }
            }}
          >
            <StatusDot status={s} />
          </span>
        )
      },
    },
    {
      header: '',
      headerClass: 'col-actions',
      cellClass: 'col-actions',
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
    {
      header: '',
      headerClass: 'th-expand-col col-expand',
      cellClass: 'row-expand-cell col-expand',
      cell: (a) => (
        <button
          type="button"
          class={`row-expand-toggle-btn ${expandedId() === a.id ? 'row-expand-open' : ''}`}
          aria-label={expandedId() === a.id ? 'Collapse holdings' : 'Expand holdings'}
          data-tooltip={expandedId() === a.id ? 'Hide holdings' : 'View Holdings'}
          onClick={() => toggleExpand(a.id)}
          data-testid={`expand-holdings-btn-${a.id}`}
        >
          <ChevronDownIcon class={`chevron-icon ${expandedId() === a.id ? 'chevron-rotated' : ''}`} />
        </button>
      ),
    },
  ]

  return (
    <div class="account-table-container">
      <DataTable
        tableClass="accounts-table"
        columns={columns}
        rows={props.accounts}
        rowKey={(a) => a.id}
        emptyMessage={props.emptyMessage}
        renderExpandedRow={(a) => (
          <Show when={expandedId() === a.id}>
            <AccountHoldingsRow
              accountId={a.id}
              accountName={a.name}
              brokerAccountId={a.brokerAccountId}
              colspan={columns.length}
            />
          </Show>
        )}
      />
    </div>
  )
}
