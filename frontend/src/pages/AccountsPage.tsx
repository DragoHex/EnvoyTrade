import { createResource, createSignal, Index, onCleanup, Show } from 'solid-js'
import { useNavigate, useSearchParams } from '@solidjs/router'
import {
  getGroups,
  getAccounts,
  getAccount,
  createAccount,
  patchAccount,
  deleteAccount,
  removeAccountFromGroup,
  createGroup,
  patchGroup,
  deleteGroup,
  type Account,
  type CreateAccountRequest,
  type GroupSummary,
} from '../api'
import { AccountsTable } from '../components/AccountsTable'
import { AccountDetailDrawer } from '../components/AccountDetailDrawer'
import { CreateGroupModal } from '../components/CreateGroupModal'
import { EditGroupModal } from '../components/EditGroupModal'
import { ConfirmActionModal } from '../components/ConfirmActionModal'
import { ResultToast, type ToastResult } from '../components/ResultToast'
import { LoadingTimeout, TableSkeleton } from '../components/Skeleton'
import { StatusDot } from '../components/StatusDot'
import { BrokerLogo } from '../components/BrokerLogo'
import { EditIcon, TrashIcon } from '../components/icons'

const TOAST_DISMISS_MS = 4000

type PendingAction =
  | { type: 'delete'; account: Account }
  | { type: 'remove_from_group'; account: Account }
  | { type: 'delete_group'; group: GroupSummary }
  | null

export function AccountsPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams<{ tab?: string }>()
  const tab = () => (searchParams.tab === 'accounts' ? 'accounts' : 'groups')
  const setTab = (nextTab: 'groups' | 'accounts') => {
    setSearchParams(
      { tab: nextTab === 'groups' ? undefined : nextTab },
      { replace: true }
    )
  }

  const [groups, { refetch: refetchGroups, mutate: mutateGroups }] = createResource(getGroups)
  const [accounts, { mutate: mutateAccounts }] = createResource(() => getAccounts())
  const [drawerOpen, setDrawerOpen] = createSignal(false)
  const [createGroupOpen, setCreateGroupOpen] = createSignal(false)
  const [editingGroup, setEditingGroup] = createSignal<GroupSummary | null>(null)
  const [editing, setEditing] = createSignal<Account | null>(null)
  const [toast, setToast] = createSignal<ToastResult | null>(null)
  const [pendingAction, setPendingAction] = createSignal<PendingAction>(null)

  let dismissTimer: ReturnType<typeof setTimeout> | undefined
  onCleanup(() => clearTimeout(dismissTimer))

  const showToast = (result: ToastResult) => {
    setToast(result)
    clearTimeout(dismissTimer)
    dismissTimer = setTimeout(() => setToast(null), TOAST_DISMISS_MS)
  }

  const openCreate = () => {
    setEditing(null)
    setDrawerOpen(true)
  }
  const openEdit = (a: Account) => {
    setEditing(a)
    setDrawerOpen(true)
  }

  const handleCreate = async (body: CreateAccountRequest) => {
    try {
      const created = await createAccount(body)
      showToast({ kind: 'success', message: 'Account created successfully.' })
      mutateAccounts((prev) => (prev ? [...prev, created] : [created]))
    } catch (e) {
      showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Create failed.' })
      throw e
    }
  }

  const handleSaveGroup = async (name: string, masterId: string) => {
    const g = editingGroup()
    if (!g) return
    try {
      await patchGroup(g.id || g.masterId, { name, masterId })
      showToast({ kind: 'success', message: 'Group updated successfully.' })
      setEditingGroup(null)
      refetchGroups()
    } catch (e) {
      showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Failed to update group.' })
      throw e
    }
  }

  const handleCreateGroup = async (name: string, masterId: string) => {
    try {
      await createGroup({ name, masterId })
      showToast({ kind: 'success', message: 'Group created successfully.' })
      refetchGroups()
    } catch (e) {
      showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Failed to create group.' })
      throw e
    }
  }

  const handleSave = async (id: string, patch: Record<string, unknown>) => {
    try {
      await patchAccount(id, patch as any)
      const updated = await getAccount(id)
      mutateAccounts((prev) => prev?.map((a) => (a.id === id ? updated : a)))
      showToast({ kind: 'success', message: 'Account saved successfully.' })
    } catch (e) {
      showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Save failed.' })
      throw e
    }
  }

  const handleToggleActive = async (id: string, nextActive: boolean) => {
    try {
      await patchAccount(id, { active: nextActive })
      const updated = await getAccount(id)
      mutateAccounts((prev) => prev?.map((a) => (a.id === id ? updated : a)))
      showToast({
        kind: 'success',
        message: `Account ${nextActive ? 'activated' : 'deactivated'} successfully.`,
      })
    } catch (e) {
      showToast({
        kind: 'error',
        message: e instanceof Error ? e.message : 'Failed to toggle account.',
      })
      throw e
    }
  }

  const confirmPendingAction = async () => {
    const pending = pendingAction()
    if (!pending) return
    setPendingAction(null)

    if (pending.type === 'delete') {
      try {
        await deleteAccount(pending.account.id)
        mutateAccounts((prev) => prev?.filter((a) => a.id !== pending.account.id))
        showToast({ kind: 'success', message: 'Account deleted successfully.' })
      } catch (e) {
        showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Delete failed.' })
      }
    } else if (pending.type === 'remove_from_group') {
      try {
        await removeAccountFromGroup(pending.account.id)
        const updated = await getAccount(pending.account.id)
        mutateAccounts((prev) => prev?.map((a) => (a.id === pending.account.id ? updated : a)))
        showToast({ kind: 'success', message: 'Removed from group successfully.' })
      } catch (e) {
        showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Failed to remove from group.' })
      }
    } else if (pending.type === 'delete_group') {
      try {
        await deleteGroup(pending.group.id || pending.group.masterId)
        mutateGroups((prev) => prev?.filter((g) => (g.id || g.masterId) !== (pending.group.id || pending.group.masterId)))
        showToast({ kind: 'success', message: 'Group deleted successfully.' })
      } catch (e) {
        showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Failed to delete group.' })
      }
    }
  }

  const masters = () => accounts()?.filter((a) => a.role === 'master') ?? []

  return (
    <div class="page">
      <div class="accounts-header">
        <div class="accounts-header-left" />
        <div class="accounts-header-center">
          <div role="tablist" class="tab-group">
            <button
              type="button"
              class="tab-btn"
              aria-pressed={tab() === 'groups'}
              classList={{ active: tab() === 'groups' }}
              onClick={() => setTab('groups')}
            >
              Groups
            </button>
            <button
              type="button"
              class="tab-btn"
              aria-pressed={tab() === 'accounts'}
              classList={{ active: tab() === 'accounts' }}
              onClick={() => setTab('accounts')}
            >
              Accounts
            </button>
          </div>
        </div>
        <div class="accounts-header-right">
          <Show when={tab() === 'groups'}>
            <button type="button" class="btn-primary" onClick={() => setCreateGroupOpen(true)}>
              Create Group
            </button>
          </Show>
          <Show when={tab() === 'accounts'}>
            <button type="button" class="btn-primary" onClick={openCreate}>
              Add Account
            </button>
          </Show>
        </div>
      </div>

      <Show when={tab() === 'groups'}>
        <Show when={groups()} fallback={<LoadingTimeout><TableSkeleton /></LoadingTimeout>}>
          {(gs) => (
            <section class="accounts-card">
              <table class="groups-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Master Account</th>
                    <th>Broker</th>
                    <th>Follower Count</th>
                    <th class="col-status">Status</th>
                    <th class="col-actions"></th>
                  </tr>
                </thead>
                <tbody>
                  <Index each={gs()}>
                    {(g) => (
                      <tr
                        class="clickable-row"
                        data-testid="group-row"
                        onClick={() => navigate(`/accounts/groups/${g().id || g().masterId}`)}
                      >
                        <td>{g().name || '—'}</td>
                        <td>
                          {g().masterName
                            ? `${g().masterName} (${g().masterAccountId})`
                            : g().masterAccountId}
                        </td>
                        <td>
                          <BrokerLogo broker={g().broker} />
                        </td>
                        <td>{g().followerCount}</td>
                        <td class="col-status">
                          <span class="status-cell-centered" title={g().status}>
                            <StatusDot status={g().status} />
                          </span>
                        </td>
                        <td class="col-actions">
                          <div class="table-actions">
                            <button
                              type="button"
                              class="icon-button"
                              aria-label="Edit"
                              data-tooltip="Edit Group"
                              title="Edit Group"
                              onClick={(e) => {
                                e.stopPropagation()
                                setEditingGroup(g())
                              }}
                            >
                              <EditIcon />
                            </button>
                            <button
                              type="button"
                              class="icon-button icon-button-danger"
                              aria-label="Delete"
                              data-tooltip="Delete Group"
                              title="Delete Group"
                              onClick={(e) => {
                                e.stopPropagation()
                                setPendingAction({ type: 'delete_group', group: g() })
                              }}
                            >
                              <TrashIcon />
                            </button>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Index>
                </tbody>
              </table>
            </section>
          )}
        </Show>
      </Show>

      <Show when={tab() === 'accounts'}>
        <Show when={accounts()} fallback={<LoadingTimeout><TableSkeleton /></LoadingTimeout>}>
          {(as) => (
            <section class="accounts-card">
              <AccountsTable
                accounts={as()}
                onToggleActive={handleToggleActive}
                onEdit={openEdit}
                onRemoveFromGroup={(a) => setPendingAction({ type: 'remove_from_group', account: a })}
                onDelete={(a) => setPendingAction({ type: 'delete', account: a })}
              />
            </section>
          )}
        </Show>
      </Show>

      <ConfirmActionModal
        open={pendingAction() !== null}
        label={pendingAction()?.type === 'delete' ? 'Delete' : pendingAction()?.type === 'delete_group' ? 'Delete group' : 'Remove from group'}
        onConfirm={confirmPendingAction}
        onCancel={() => setPendingAction(null)}
      />

      <CreateGroupModal
        open={createGroupOpen()}
        masters={masters()}
        onClose={() => setCreateGroupOpen(false)}
        onCreate={handleCreateGroup}
      />

      <EditGroupModal
        open={editingGroup() !== null}
        initialName={editingGroup()?.name || ''}
        currentMasterId={editingGroup()?.masterId || ''}
        masters={masters()}
        onClose={() => setEditingGroup(null)}
        onSave={handleSaveGroup}
      />

      <AccountDetailDrawer
        open={drawerOpen()}
        account={editing()}
        masters={masters()}
        onClose={() => setDrawerOpen(false)}
        onCreate={handleCreate}
        onSave={handleSave}
      />

      <ResultToast result={toast()} onDismiss={() => setToast(null)} />
    </div>
  )
}
