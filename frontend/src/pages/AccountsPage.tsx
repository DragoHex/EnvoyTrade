import { createResource, createSignal, For, Show } from 'solid-js'
import { useNavigate } from '@solidjs/router'
import {
  getGroups,
  getAccounts,
  createAccount,
  patchAccount,
  deleteAccount,
  removeAccountFromGroup,
  type Account,
  type CreateAccountRequest,
} from '../api'
import { AccountsTable } from '../components/AccountsTable'
import { AccountDetailDrawer } from '../components/AccountDetailDrawer'
import { ResultToast, type ToastResult } from '../components/ResultToast'

export function AccountsPage() {
  const navigate = useNavigate()
  const [tab, setTab] = createSignal<'groups' | 'accounts'>('groups')
  const [groups] = createResource(getGroups)
  const [accounts, { refetch }] = createResource(() => getAccounts())
  const [drawerOpen, setDrawerOpen] = createSignal(false)
  const [editing, setEditing] = createSignal<Account | null>(null)
  const [toast, setToast] = createSignal<ToastResult | null>(null)

  const openCreate = () => {
    setEditing(null)
    setDrawerOpen(true)
  }
  const openEdit = (a: Account) => {
    setEditing(a)
    setDrawerOpen(true)
  }

  const handleCreate = async (body: CreateAccountRequest) => {
    await createAccount(body)
    refetch()
  }
  const handleSave = async (id: string, patch: Record<string, unknown>) => {
    await patchAccount(id, patch as any)
    refetch()
  }
  const handleRemoveFromGroup = async (a: Account) => {
    try {
      await removeAccountFromGroup(a.id)
      refetch()
    } catch (e) {
      setToast({ kind: 'error', message: e instanceof Error ? e.message : 'Failed to remove from group.' })
    }
  }
  const handleDelete = async (a: Account) => {
    try {
      await deleteAccount(a.id)
      refetch()
    } catch (e) {
      setToast({ kind: 'error', message: e instanceof Error ? e.message : 'Delete failed.' })
    }
  }

  return (
    <div class="page">
      <h1>Accounts</h1>
      <div role="tablist">
        <button type="button" aria-pressed={tab() === 'groups'} onClick={() => setTab('groups')}>
          Groups
        </button>
        <button type="button" aria-pressed={tab() === 'accounts'} onClick={() => setTab('accounts')}>
          Accounts
        </button>
      </div>

      <Show when={tab() === 'groups'}>
        <Show when={groups()} fallback={<p>Loading…</p>}>
          {(gs) => (
            <table>
              <thead>
                <tr>
                  <th>Account ID</th>
                  <th>Broker User ID</th>
                  <th>Broker</th>
                  <th>Follower Count</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                <For each={gs()}>
                  {(g) => (
                    <tr data-testid="group-row" onClick={() => navigate(`/accounts/groups/${g.masterId}`)}>
                      <td>{g.masterId}</td>
                      <td>{g.masterAccountId}</td>
                      <td>{g.broker}</td>
                      <td>{g.followerCount}</td>
                      <td>{g.status}</td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          )}
        </Show>
      </Show>

      <Show when={tab() === 'accounts'}>
        <button type="button" onClick={openCreate}>
          Add Account
        </button>
        <Show when={accounts()} fallback={<p>Loading…</p>}>
          {(as) => (
            <AccountsTable
              accounts={as()}
              onEdit={openEdit}
              onRemoveFromGroup={handleRemoveFromGroup}
              onDelete={handleDelete}
            />
          )}
        </Show>
      </Show>

      <AccountDetailDrawer
        open={drawerOpen()}
        account={editing()}
        masters={(accounts() ?? []).filter((a) => a.role === 'master')}
        onClose={() => setDrawerOpen(false)}
        onCreate={handleCreate}
        onSave={handleSave}
      />
      <ResultToast result={toast()} onDismiss={() => setToast(null)} />
    </div>
  )
}
