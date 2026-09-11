import { createEffect, createResource, createSignal, Index, onCleanup, Show } from 'solid-js'
import { useParams, A } from '@solidjs/router'
import {
  getGroupDetail,
  getAccounts,
  getAccount,
  patchAccount,
  removeAccountFromGroup,
  addAccountToGroup,
  patchGroup,
  type Account,
} from '../api'
import { ConfirmActionModal } from '../components/ConfirmActionModal'
import { EditGroupModal } from '../components/EditGroupModal'
import { ResultToast, type ToastResult } from '../components/ResultToast'
import { LoadingTimeout, TableSkeleton } from '../components/Skeleton'
import { StatusDot } from '../components/StatusDot'
import { CopyToggle } from '../components/CopyToggle'
import { UserMinusIcon } from '../components/icons'
import { RoleIcon } from '../components/RoleIcon'

const TOAST_DISMISS_MS = 4000

// GroupManagePage (/accounts/groups/:masterId) configures one group's
// accounts: activate/deactivate, remove a follower, add an existing
// follower, or edit group name and swap master account.
export function GroupManagePage() {
  const params = useParams<{ masterId: string }>()
  const [detail, { mutate: mutateDetail, refetch: refetchDetail }] = createResource(
    () => params.masterId,
    getGroupDetail
  )
  const [members, setMembers] = createSignal<Account[]>()
  const [allAccounts, { mutate: mutateAll }] = createResource(() => getAccounts())

  const loadMembersForGroup = async (d: { masterId: string; followers: { accountId: string }[] }) => {
    const ids = [d.masterId, ...d.followers.map((f) => f.accountId)]
    const accs = await getAccounts(ids)
    setMembers(accs)
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
  const [capitalRatio, setCapitalRatio] = createSignal('')
  const [maxQtyPerOrder, setMaxQtyPerOrder] = createSignal('')
  const [pendingRemove, setPendingRemove] = createSignal<Account | null>(null)
  const [editModalOpen, setEditModalOpen] = createSignal(false)
  const [toast, setToast] = createSignal<ToastResult | null>(null)
  const [submittingAdd, setSubmittingAdd] = createSignal(false)

  let dismissTimer: ReturnType<typeof setTimeout> | undefined
  onCleanup(() => clearTimeout(dismissTimer))

  const showToast = (result: ToastResult) => {
    setToast(result)
    clearTimeout(dismissTimer)
    dismissTimer = setTimeout(() => setToast(null), TOAST_DISMISS_MS)
  }

  const handleToggleActive = async (a: Account, next: boolean) => {
    setTogglingId(a.id)
    try {
      if (a.role === 'master') {
        await patchAccount(a.id, { active: next })
        const updated = await getAccount(a.id)
        setMembers((prev) => prev?.map((m) => (m.id === a.id ? updated : m)))
        mutateAll((prev) => prev?.map((acc) => (acc.id === a.id ? updated : acc)))
        mutateDetail((prev) => (prev ? { ...prev, masterActive: updated.active } : prev))
        showToast({
          kind: 'success',
          message: `Master ${next ? 'activated' : 'deactivated'} successfully.`,
        })
      } else {
        await patchAccount(a.id, { enabled: next })
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
        showToast({
          kind: 'success',
          message: `Follower ${next ? 'enabled' : 'disabled'} successfully.`,
        })
      }
    } catch (e) {
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
    try {
      const groupId = detail()?.id || params.masterId
      await addAccountToGroup(groupId, {
        accountId,
        capitalRatio: capitalRatio(),
        maxQtyPerOrder: maxQtyPerOrder() === '' ? undefined : Number(maxQtyPerOrder()),
      })
      const added = await getAccount(accountId)
      setMembers((prev) => (prev ? [...prev, added] : [added]))
      mutateDetail((prev) =>
        prev
          ? {
              ...prev,
              followers: [
                ...prev.followers,
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
      setSelectedAccountId('')
      setCapitalRatio('')
      setMaxQtyPerOrder('')
      showToast({ kind: 'success', message: 'Follower added to group successfully.' })
    } catch (err) {
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
    const d = await refetchDetail()
    if (d) {
      await loadMembersForGroup(d)
    }
  }

  const unattachedFollowers = () =>
    (allAccounts() ?? []).filter((a) => a.role === 'follower' && a.masterId == null)

  const masterAccount = () => (members() ?? []).find((a) => a.role === 'master')
  const masters = () => (allAccounts() ?? []).filter((a) => a.role === 'master')

  return (
    <div class="page">
      <div class="group-header">
        <A href="/accounts" class="back-link">
          ← Back
        </A>
        <div class="group-title-row" style={{ display: 'flex', 'align-items': 'center', 'justify-content': 'space-between' }}>
          <div style={{ display: 'flex', 'align-items': 'center', gap: '0.75rem' }}>
            <h1>
              Group: {detail()?.name || detail()?.masterAccountId || masterAccount()?.brokerAccountId || params.masterId}
            </h1>
            <Show when={masterAccount()}>
              {(m) => <StatusDot status={m().status} />}
            </Show>
          </div>
          <button type="button" class="btn-primary" onClick={() => setEditModalOpen(true)}>
            Edit Group
          </button>
        </div>
      </div>

      <section class="accounts-card">
        <h2>Group Members</h2>
        <Show when={members()} fallback={<LoadingTimeout><TableSkeleton /></LoadingTimeout>}>
          {(ms) => (
            <table class="group-members-table">
              <thead>
                <tr>
                  <th class="col-toggle"></th>
                  <th class="col-name">Name</th>
                  <th class="col-broker-id">Broker User ID</th>
                  <th class="col-role">Role</th>
                  <th class="col-capital-ratio">Capital Ratio</th>
                  <th class="col-max-qty">Max Qty/Order</th>
                  <th class="col-status">Status</th>
                  <th class="col-actions"></th>
                </tr>
              </thead>
              <tbody>
                <Index each={ms()}>
                  {(a) => (
                    <tr>
                      <td class="col-toggle">
                        <CopyToggle
                          enabled={a().role === 'master' ? a().active : a().enabled}
                          disabled={togglingId() === a().id}
                          ariaLabel={`Toggle active for ${a().name || a().brokerAccountId}`}
                          title={
                            a().role === 'master'
                              ? a().active
                                ? 'Active — click to deactivate'
                                : 'Inactive — click to activate'
                              : a().enabled
                                ? 'Enabled — click to disable'
                                : 'Disabled — click to enable'
                          }
                          onToggle={(next) => handleToggleActive(a(), next)}
                        />
                      </td>
                      <td class="col-name">{a().name || '—'}</td>
                      <td class="col-broker-id">{a().brokerAccountId}</td>
                      <td class="col-role">
                        <span class="role-cell-centered">
                          <RoleIcon role={a().role} />
                        </span>
                      </td>
                      <td class="col-capital-ratio">{a().role === 'follower' ? (a().capitalRatio ?? '—') : '—'}</td>
                      <td class="col-max-qty">{a().role === 'follower' ? (a().maxQtyPerOrder ?? '—') : '—'}</td>
                      <td class="col-status">
                        <span class="status-cell-centered" title={a().status}>
                          <StatusDot status={a().status} />
                        </span>
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
          )}
        </Show>
      </section>

      <section class="accounts-card">
        <h3>Add Follower to Group</h3>
        <form onSubmit={handleAddToGroup} class="group-add-form">
          <label>
            Account
            <select
              value={selectedAccountId()}
              onChange={(e) => setSelectedAccountId(e.currentTarget.value)}
              required
            >
              <option value="" disabled>
                select an account
              </option>
              <Index each={unattachedFollowers()}>
                {(a) => (
                  <option value={a().id}>
                    {a().name ? `${a().name} (${a().brokerAccountId})` : a().brokerAccountId}
                  </option>
                )}
              </Index>
            </select>
          </label>
          <label>
            Capital Ratio
            <input
              value={capitalRatio()}
              placeholder="e.g. 1.0"
              required
              onInput={(e) => setCapitalRatio(e.currentTarget.value)}
            />
          </label>
          <label>
            Max Qty/Order
            <input
              value={maxQtyPerOrder()}
              placeholder="optional cap"
              onInput={(e) => setMaxQtyPerOrder(e.currentTarget.value)}
            />
          </label>
          <button type="submit" class="btn-primary" disabled={submittingAdd()}>
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

      <ResultToast result={toast()} onDismiss={() => setToast(null)} />
    </div>
  )
}
