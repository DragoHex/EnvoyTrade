import { DataTable, type Column } from './DataTable'
import { StatusDot } from './StatusDot'
import type { Account } from '../api'

export function AccountsTable(props: {
  accounts: Account[]
  onEdit: (account: Account) => void
  onRemoveFromGroup: (account: Account) => void
  onDelete: (account: Account) => void
}) {
  const columns: Column<Account>[] = [
    { header: 'Account ID', cell: (a) => a.id },
    { header: 'Role', cell: (a) => a.role },
    { header: 'Broker User ID', cell: (a) => a.brokerAccountId },
    { header: 'Broker', cell: (a) => a.broker },
    { header: 'Group', cell: (a) => a.masterId ?? '—' },
    { header: 'Capital Ratio', cell: (a) => a.capitalRatio ?? '—' },
    { header: 'Max Qty/Order', cell: (a) => a.maxQtyPerOrder ?? '—' },
    { header: 'Active', cell: (a) => (a.active ? 'Yes' : 'No') },
    { header: 'Status', cell: (a) => <StatusDot status={a.status} /> },
    {
      header: '',
      cell: (a) => (
        <>
          <button type="button" onClick={() => props.onEdit(a)}>
            Edit
          </button>
          {a.masterId != null && (
            <button type="button" onClick={() => props.onRemoveFromGroup(a)}>
              Remove from group
            </button>
          )}
          <button type="button" onClick={() => props.onDelete(a)}>
            Delete
          </button>
        </>
      ),
    },
  ]

  return <DataTable columns={columns} rows={props.accounts} rowKey={(a) => a.id} />
}
