import { createEffect, createMemo, createResource, createSignal, Index, onCleanup, Show } from 'solid-js'
import { useParams, useLocation, A } from '@solidjs/router'
import {
  getGroupDetail,
  getGroups,
  getAccounts,
  getAccount,
  patchAccount,
  removeAccountFromGroup,
  addAccountToGroup,
  patchGroup,
  type Account,
  type GroupSummary,
} from '../api'
import { ConfirmActionModal } from '../components/ConfirmActionModal'
import { EditGroupModal } from '../components/EditGroupModal'
import { AccountDetailDrawer } from '../components/AccountDetailDrawer'
import { ResultToast, type ToastResult } from '../components/ResultToast'
import { LoadingTimeout, TableSkeleton } from '../components/Skeleton'
import { StatusDot } from '../components/StatusDot'
import { CopyToggle } from '../components/CopyToggle'
import { UserMinusIcon } from '../components/icons'
import { RoleIcon } from '../components/RoleIcon'
import { BrokerLogo } from '../components/BrokerLogo'
import { ThemedSelect } from '../components/ThemedSelect'
import { ThemedNumberInput } from '../components/ThemedNumberInput'
import { formatAuthError } from '../utils/authError'


const TOAST_DISMISS_MS = 4000

// GroupManagePage (/accounts/groups/:masterId) configures one group's
// accounts: activate/deactivate, remove a follower, add an existing
// follower, or edit group name and swap master account.
export function GroupManagePage() {
  const params = useParams<{ masterId: string }>()
  const location = useLocation<{ name?: string }>()
  const [detail, { mutate: mutateDetail, refetch: refetchDetail }] = createResource(
    () => params.masterId,
    getGroupDetail
  )
  const [members, setMembers] = createSignal<Account[]>()
  const [editingAccount, setEditingAccount] = createSignal<Account | null>(null)
  const [drawerOpen, setDrawerOpen] = createSignal(false)
  const [allAccounts, { mutate: mutateAll, refetch: refetchAll }] = createResource(() => getAccounts())
  const [allGroups] = createResource(
    () => (drawerOpen() ? true : null),
    () => getGroups().catch(() => [])
  )
  const availableGroups = createMemo<GroupSummary[]>(() => {
    if (allGroups() && allGroups()!.length > 0) return allGroups()!
    const d = detail()
    if (!d) return []
    return [
      {
        id: d.id,
        name: d.name,
        masterId: d.masterId,
        masterAccountId: d.masterAccountId,
        broker: masterAccount()?.broker || 'kite',
        followerCount: (members() ?? []).filter((m) => m.role === 'follower').length,
        status: masterAccount()?.status || 'ok',
      },
    ]
  })

  const initialName = () => location.state?.name
  const groupTitle = () => detail()?.name || initialName() || detail()?.masterAccountId || masterAccount()?.brokerAccountId

  const loadMembersForGroup = async (d: { masterId: string; followers: { accountId: string }[] }) => {
    const ids = [d.masterId, ...d.followers.map((f) => f.accountId)]
    const accs = await getAccounts(ids)
    const master = accs.find((a) => a.id === d.masterId)
    const followers = accs
      .filter((a) => a.id !== d.masterId)
      .sort((a, b) =>
        (a.name || a.brokerAccountId).localeCompare(b.name || b.brokerAccountId, undefined, { sensitivity: 'base' })
      )
    setMembers(master ? [master, ...followers] : accs)
  }

  let lastLoadedId = ''
  createEffect(() => {
    const currentId = params.masterId
    if (currentId !== lastLoadedId) {
      setMembers(undefined)
    }
    const d = detail()
    if (d && currentId && lastLoadedId !== currentId) {
      lastLoadedId = currentId
      loadMembersForGroup(d)
    }
  })

  const [togglingId, setTogglingId] = createSignal<string | null>(null)
  const [selectedAccountId, setSelectedAccountId] = createSignal('')
  const [cloneFactor, setCloneFactor] = createSignal('1')
  const [maxQtyPerOrder, setMaxQtyPerOrder] = createSignal('')
  const [pendingRemove, setPendingRemove] = createSignal<Account | null>(null)
  const [editModalOpen, setEditModalOpen] = createSignal(false)
  const [toast, setToast] = createSignal<ToastResult | null>(null)
  const [submittingAdd, setSubmittingAdd] = createSignal(false)

  const openEdit = (a: Account) => {
    setEditingAccount(a)
    setDrawerOpen(true)
  }

  const handleSaveAccount = async (id: string, patch: Record<string, unknown>) => {
    try {
      await patchAccount(id, patch as any)
      const updated = await getAccount(id)
      setMembers((prev) => prev?.map((m) => (m.id === id ? updated : m)))
      mutateAll((prev) => prev?.map((m) => (m.id === id ? updated : m)))
      setDrawerOpen(false)
      setEditingAccount(null)
      showToast({ kind: 'success', message: 'Account saved successfully.' })
      refetchDetail()
    } catch (e) {
      showToast({ kind: 'error', message: e instanceof Error ? e.message : 'Save failed.' })
      throw e
    }
  }

  let dismissTimer: ReturnType<typeof setTimeout> | undefined
  onCleanup(() => clearTimeout(dismissTimer))

  const showToast = (result: ToastResult) => {
    setToast(result)
    clearTimeout(dismissTimer)
    dismissTimer = setTimeout(() => setToast(null), TOAST_DISMISS_MS)
  }

  const handleToggleActive = async (a: Account, next: boolean) => {
    // Save previous state for rollback
    const prevMembers = members()
    const prevDetail = detail()

    // 1. Optimistic update: immediately update members and detail
    setMembers((prev) =>
      prev?.map((m) =>
        m.id === a.id
          ? a.role === 'master'
            ? { ...m, active: next }
            : { ...m, enabled: next }
          : m,
      ),
    )
    mutateDetail((prev) => {
      if (!prev) return prev
      if (a.role === 'master') {
        return { ...prev, masterActive: next }
      }
      return {
        ...prev,
        followers: prev.followers.map((f) =>
          f.accountId === a.id ? { ...f, enabled: next } : f,
        ),
      }
    })

    setTogglingId(a.id)
    try {
      if (a.role === 'master') {
        await patchAccount(a.id, { active: next })
        try {
          const updated = await getAccount(a.id)
          setMembers((prev) => prev?.map((m) => (m.id === a.id ? updated : m)))
          mutateAll((prev) => prev?.map((acc) => (acc.id === a.id ? updated : acc)))
          mutateDetail((prev) => (prev ? { ...prev, masterActive: updated.active } : prev))
        } catch {
          // If secondary getAccount fails, optimistic state is preserved
        }
        showToast({
          kind: 'success',
          message: `Master ${next ? 'activated' : 'deactivated'} successfully.`,
        })
      } else {
        await patchAccount(a.id, { enabled: next })
        try {
          const updated = await getAccount(a.id)
          setMembers((prev) => prev?.map((m) => (m.id === a.id ? updated : m)))
          mutateAll((prev) => prev?.map((acc) => (acc.id === a.id ? updated : acc)))
          mutateDetail((prev) =>
            prev
              ? {
                  ...prev,
                  followers: prev.followers.map((f) =>
                    f.accountId === a.id ? { ...f, enabled: updated.enabled } : f,
                  ),
                }
              : prev,
          )
        } catch {
          // If secondary getAccount fails, optimistic state is preserved
        }
        showToast({
          kind: 'success',
          message: `Follower ${next ? 'enabled' : 'disabled'} successfully.`,
        })
      }
    } catch (e) {
      // Revert optimistic update
      setMembers(prevMembers)
      mutateDetail(prevDetail)
      showToast({
        kind: 'error',
        message: e instanceof Error ? e.message : 'State update failed.',
      })
    } finally {
      setTogglingId(null)
    }
  }

  const confirmRemoveFromGroup = async () => {
    const follower = pendingRemove()
    if (!follower) return
    setPendingRemove(null)

    try {
      await removeAccountFromGroup(follower.id)
      setMembers((prev) => prev?.filter((m) => m.id !== follower.id))
      mutateDetail((prev) =>
        prev ? { ...prev, followers: prev.followers.filter((f) => f.accountId !== follower.id) } : prev,
      )
      try {
        const updated = await getAccount(follower.id)
        mutateAll((prev) => prev?.map((acc) => (acc.id === follower.id ? updated : acc)))
      } catch {
        mutateAll((prev) =>
          prev?.map((acc) =>
            acc.id === follower.id ? { ...acc, masterId: null, groupId: null, groupName: null } : acc,
          ),
        )
      }
      showToast({ kind: 'success', message: 'Follower removed from group.' })
    } catch (e) {
      showToast({
        kind: 'error',
        message: e instanceof Error ? e.message : 'Failed to remove follower.',
      })
    }
  }

  const handleAddToGroup = async (e: Event) => {
    e.preventDefault()
    setSubmittingAdd(true)
    const accountId = selectedAccountId()
    const target = allAccounts()?.find((a) => a.id === accountId)
    const cf = cloneFactor() || '1'
    const maxQty = maxQtyPerOrder() === '' ? undefined : Number(maxQtyPerOrder())

    const optimisticFollower: Account | undefined = target
      ? {
          ...target,
          groupId: detail()?.id || params.masterId,
          masterId: detail()?.masterId || params.masterId,
          cloneFactor: cf,
          maxQtyPerOrder: maxQty ?? null,
          enabled: true,
          active: true,
        }
      : undefined

    const prevMembers = members()
    const prevDetail = detail()
    const prevAll = allAccounts()

    if (optimisticFollower) {
      setMembers((prev) => {
        if (!prev) return [optimisticFollower]
        const master = prev.find((m) => m.role === 'master')
        const followers = [
          ...prev.filter((m) => m.role !== 'master' && m.id !== optimisticFollower.id),
          optimisticFollower,
        ].sort((a, b) =>
          (a.name || a.brokerAccountId).localeCompare(b.name || b.brokerAccountId, undefined, { sensitivity: 'base' })
        )
        return master ? [master, ...followers] : followers
      })
      mutateDetail((prev) =>
        prev
          ? {
              ...prev,
              followers: [
                ...prev.followers.filter((f) => f.accountId !== optimisticFollower.id),
                {
                  accountId: optimisticFollower.id,
                  name: optimisticFollower.name,
                  brokerAccountId: optimisticFollower.brokerAccountId,
                  enabled: true,
                  status: optimisticFollower.status as any,
                },
              ],
            }
          : prev,
      )
      mutateAll((prev) => prev?.map((acc) => (acc.id === optimisticFollower.id ? optimisticFollower : acc)))
      setSelectedAccountId('')
      setCloneFactor('1')
      setMaxQtyPerOrder('')
    }

    try {
      const groupId = detail()?.id || params.masterId
      await addAccountToGroup(groupId, {
        accountId,
        cloneFactor: cf,
        maxQtyPerOrder: maxQty,
      })
      try {
        const added = await getAccount(accountId)
        setMembers((prev) => {
          if (!prev) return [added]
          const master = prev.find((m) => m.role === 'master')
          const followers = [
            ...prev.filter((m) => m.role !== 'master' && m.id !== added.id),
            added,
          ].sort((a, b) =>
            (a.name || a.brokerAccountId).localeCompare(b.name || b.brokerAccountId, undefined, { sensitivity: 'base' })
          )
          return master ? [master, ...followers] : followers
        })
        mutateDetail((prev) =>
          prev
            ? {
                ...prev,
                followers: [
                  ...prev.followers.filter((f) => f.accountId !== added.id),
                  {
                    accountId: added.id,
                    name: added.name,
                    brokerAccountId: added.brokerAccountId,
                    enabled: added.enabled,
                    status: added.status as any,
                  },
                ],
              }
            : prev,
        )
        mutateAll((prev) => prev?.map((acc) => (acc.id === added.id ? added : acc)))
      } catch {
        // If secondary getAccount fails, optimistic follower is already applied
      }
      showToast({ kind: 'success', message: 'Follower added to group successfully.' })
    } catch (err) {
      if (optimisticFollower) {
        setMembers(prevMembers)
        mutateDetail(prevDetail)
        mutateAll(prevAll)
        setSelectedAccountId(accountId)
        setCloneFactor(cf)
        setMaxQtyPerOrder(maxQty != null ? String(maxQty) : '')
      }
      showToast({
        kind: 'error',
        message: err instanceof Error ? err.message : 'Failed to add follower to group.',
      })
    } finally {
      setSubmittingAdd(false)
    }
  }

  const handleSaveGroup = async (name: string, masterId: string) => {
    const groupId = detail()?.id || params.masterId
    await patchGroup(groupId, { name, masterId })
    showToast({ kind: 'success', message: 'Group updated successfully.' })
    lastLoadedId = ''
    await refetchAll()
    const d = await refetchDetail()
    if (d) {
      await loadMembersForGroup(d)
    }
  }

  const unattachedFollowers = () =>
    (allAccounts() ?? [])
      .filter((a) => a.role === 'follower' && (a.masterId == null || a.masterId === '') && (a.groupId == null || a.groupId === ''))
      .sort((a, b) =>
        (a.name || a.brokerAccountId).localeCompare(b.name || b.brokerAccountId, undefined, { sensitivity: 'base' })
      )

  const masterAccount = () => (members() ?? []).find((a) => a.role === 'master')
  const masters = () =>
    (allAccounts() ?? [])
      .filter((a) => a.role === 'master')
      .sort((a, b) =>
        (a.name || a.brokerAccountId).localeCompare(b.name || b.brokerAccountId, undefined, { sensitivity: 'base' })
      )

  return (
    <div class="page">
      <div class="group-header">
        <A href="/accounts" class="back-link">
          ← Back
        </A>
        <div class="group-title-row" style={{ display: 'flex', 'align-items': 'center', 'justify-content': 'space-between' }}>
          <div style={{ display: 'flex', 'align-items': 'center', gap: '0.75rem' }}>
            <h1>
              Group:{' '}
              <Show when={groupTitle()} fallback={<span class="skeleton-bar group-title-skeleton" aria-label="Loading group name" />}>
                {groupTitle()}
              </Show>
            </h1>
            <Show when={masterAccount()}>
              {(m) => <StatusDot status={m().status} />}
            </Show>
          </div>
          <button type="button" class="btn-primary" disabled={!detail()} onClick={() => setEditModalOpen(true)}>
            Edit Group
          </button>
        </div>
      </div>

      <section class="accounts-card">
        <h2>Group Members</h2>
        <Show when={members()} fallback={<LoadingTimeout><TableSkeleton /></LoadingTimeout>}>
          {(ms) => (
            <div class="account-table-container">
              <table class="group-members-table">
                <thead>
                  <tr>
                    <th class="col-toggle"></th>
                    <th class="col-name">Name</th>
                    <th class="col-broker-id">Broker User ID</th>
                    <th class="col-broker">Broker</th>
                    <th class="col-role">Role</th>
                    <th class="col-clone-factor">Clone Factor</th>
                    <th class="col-max-qty">Max Qty/Order</th>
                    <th class="col-status">Status</th>
                    <th class="col-actions"></th>
                  </tr>
                </thead>
                <tbody>
                  <Index each={ms()}>
                    {(a) => (
                      <tr data-disabled={a().role === 'master' ? !a().active : !a().enabled}>
                        <td class="col-toggle">
                          <CopyToggle
                            enabled={a().role === 'master' ? a().active : a().enabled}
                            disabled={togglingId() === a().id}
                            ariaLabel={`Toggle active for ${a().name || a().brokerAccountId}`}
                            onToggle={(next) => handleToggleActive(a(), next)}
                          />
                        </td>
                        <td class="col-name">{a().name || '—'}</td>
                        <td class="col-broker-id">{a().brokerAccountId}</td>
                        <td class="col-broker">
                          <span class="broker-cell-centered">
                            <BrokerLogo broker={a().broker} />
                          </span>
                        </td>
                        <td class="col-role">
                          <span class="role-cell-centered">
                            <RoleIcon role={a().role} />
                          </span>
                        </td>
                        <td class="col-clone-factor">{a().role === 'follower' ? (a().cloneFactor ?? '—') : '—'}</td>
                        <td class="col-max-qty">{a().role === 'follower' ? (a().maxQtyPerOrder ?? '—') : '—'}</td>
                        <td class="col-status">
                          {(() => {
                            const isErr = a().status === 'error' || a().authStatus === 'error'
                            const tooltip = isErr
                              ? (a().authError ? `${formatAuthError(a().authError)} (click to edit)` : 'Account error - click to edit')
                              : (a().status === 'pending' || a().status === 'syncing' ? 'Connecting / Syncing...' : 'Active')
                            return (
                              <span
                                class="status-cell-centered"
                                title={tooltip}
                                style={{ cursor: isErr ? 'pointer' : 'default' }}
                                onClick={(e) => {
                                  if (isErr) {
                                    e.stopPropagation()
                                    openEdit(a())
                                  }
                                }}
                              >
                                <StatusDot status={isErr ? 'error' : a().status} />
                              </span>
                            )
                          })()}
                        </td>
                        <td class="col-actions">
                          <div class="table-actions">
                            <Show when={a().role === 'follower'}>
                              <button
                                type="button"
                                class="icon-button icon-button-danger"
                                aria-label="Remove from group"
                                data-tooltip="Remove from Group"
                                title="Remove from Group"
                                onClick={() => setPendingRemove(a())}
                              >
                                <UserMinusIcon />
                              </button>
                            </Show>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Index>
                </tbody>
              </table>
              <Show when={members() && members()!.length === 1}>
                <div class="group-empty-followers-note">
                  No follower accounts attached to this group yet. Use the form below to add one.
                </div>
              </Show>
            </div>
          )}
        </Show>
      </section>

      <section class="accounts-card group-add-card">
        <h3>Add Follower to Group</h3>
        <form onSubmit={handleAddToGroup} class="group-add-form">
          <label>
            Account
            <ThemedSelect
              aria-label="Account"
              value={selectedAccountId()}
              placeholder="Select an Account"
              disabled={allAccounts.loading || unattachedFollowers().length === 0}
              onChange={(val) => setSelectedAccountId(val)}
              required
              options={
                allAccounts.loading
                  ? [{ value: '', label: 'Loading accounts...', disabled: true }]
                  : unattachedFollowers().length === 0
                    ? [{ value: '', label: 'No unassigned follower accounts available', disabled: true }]
                    : unattachedFollowers().map((a) => ({
                        value: a.id,
                        label: a.name ? `${a.name} (${a.brokerAccountId})` : a.brokerAccountId,
                      }))
              }
            />
          </label>
          <label>
            Clone Factor
            <ThemedNumberInput
              aria-label="Clone Factor"
              step="any"
              min="0.01"
              value={cloneFactor()}
              placeholder="e.g. 1.0"
              required
              onInput={(e) => setCloneFactor(e.currentTarget.value)}
            />
          </label>
          <label>
            Max Qty/Order
            <ThemedNumberInput
              aria-label="Max Qty/Order"
              step="1"
              min="1"
              value={maxQtyPerOrder()}
              placeholder="optional cap"
              onInput={(e) => setMaxQtyPerOrder(e.currentTarget.value)}
            />
          </label>
          <button
            type="submit"
            class="btn-primary"
            disabled={submittingAdd() || unattachedFollowers().length === 0 || !selectedAccountId()}
          >
            Add to group
          </button>
        </form>
      </section>

      <EditGroupModal
        open={editModalOpen()}
        initialName={detail()?.name || ''}
        currentMasterId={detail()?.masterId || ''}
        masters={masters()}
        onClose={() => setEditModalOpen(false)}
        onSave={handleSaveGroup}
      />

      <ConfirmActionModal
        open={pendingRemove() !== null}
        label="Remove from group"
        onConfirm={confirmRemoveFromGroup}
        onCancel={() => setPendingRemove(null)}
      />

      <AccountDetailDrawer
        open={drawerOpen()}
        account={editingAccount()}
        groups={availableGroups()}
        existingAccounts={allAccounts() ?? []}
        onClose={() => {
          setDrawerOpen(false)
          setEditingAccount(null)
        }}
        onCreate={async () => {}}
        onSave={handleSaveAccount}
      />

      <ResultToast result={toast()} onDismiss={() => setToast(null)} />
    </div>
  )
}
