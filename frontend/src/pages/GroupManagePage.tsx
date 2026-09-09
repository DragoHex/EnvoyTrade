import { createResource, createSignal, For, Show } from 'solid-js'
import { useParams } from '@solidjs/router'
import { getGroupDetail, getAccounts, patchAccount, removeAccountFromGroup, addAccountToGroup, type Account } from '../api'

// GroupManagePage (/accounts/groups/:masterId) configures one group's
// accounts: activate/deactivate, remove a follower, add an existing
// unattached follower. Member IDs come from the existing GET
// /groups/{masterId} (unchanged); full fields are then batch-fetched via
// GET /accounts?ids=... (docs/APIs/accounts.md).
export function GroupManagePage() {
  const params = useParams<{ masterId: string }>()
  const [detail, { refetch: refetchDetail }] = createResource(() => params.masterId, getGroupDetail)
  const memberIDs = () => {
    const d = detail()
    return d ? [d.masterId, ...d.followers.map((f) => f.accountId)] : undefined
  }
  const [members, { refetch: refetchMembers }] = createResource(memberIDs, (ids) => getAccounts(ids))
  const [allAccounts, { refetch: refetchAll }] = createResource(() => getAccounts())

  const [selectedAccountId, setSelectedAccountId] = createSignal('')
  const [capitalRatio, setCapitalRatio] = createSignal('')
  const [maxQtyPerOrder, setMaxQtyPerOrder] = createSignal('')

  const refetch = () => {
    refetchDetail()
    refetchMembers()
    refetchAll()
  }

  const handleToggleActive = async (a: Account) => {
    if (a.role === 'master') await patchAccount(a.id, { active: !a.active })
    else await patchAccount(a.id, { enabled: !a.enabled })
    refetch()
  }
  const handleRemoveFromGroup = async (a: Account) => {
    await removeAccountFromGroup(a.id)
    refetch()
  }
  const handleAddToGroup = async (e: Event) => {
    e.preventDefault()
    await addAccountToGroup(params.masterId, {
      accountId: selectedAccountId(),
      capitalRatio: capitalRatio(),
      maxQtyPerOrder: maxQtyPerOrder() === '' ? undefined : Number(maxQtyPerOrder()),
    })
    setSelectedAccountId('')
    setCapitalRatio('')
    setMaxQtyPerOrder('')
    refetch()
  }

  const unattachedFollowers = () => (allAccounts() ?? []).filter((a) => a.role === 'follower' && a.masterId == null)

  return (
    <div class="page">
      <h1>Group</h1>
      <Show when={members()} fallback={<p>Loading…</p>}>
        {(ms) => (
          <table>
            <thead>
              <tr>
                <th>Broker User ID</th>
                <th>Role</th>
                <th>Capital Ratio</th>
                <th>Max Qty/Order</th>
                <th>State</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <For each={ms()}>
                {(a) => (
                  <tr>
                    <td>{a.brokerAccountId}</td>
                    <td>{a.role}</td>
                    <td>{a.role === 'follower' ? (a.capitalRatio ?? '—') : '—'}</td>
                    <td>{a.role === 'follower' ? (a.maxQtyPerOrder ?? '—') : '—'}</td>
                    <td>{a.role === 'master' ? (a.active ? 'active' : 'inactive') : a.enabled ? 'enabled' : 'disabled'}</td>
                    <td>
                      <button type="button" onClick={() => handleToggleActive(a)}>
                        {a.role === 'master' ? (a.active ? 'Deactivate' : 'Activate') : a.enabled ? 'Disable' : 'Enable'}
                      </button>
                      <Show when={a.role === 'follower'}>
                        <button type="button" onClick={() => handleRemoveFromGroup(a)}>
                          Remove from group
                        </button>
                      </Show>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        )}
      </Show>

      <form onSubmit={handleAddToGroup}>
        <label>
          Account
          <select value={selectedAccountId()} onChange={(e) => setSelectedAccountId(e.currentTarget.value)}>
            <option value="" disabled>
              select an account
            </option>
            <For each={unattachedFollowers()}>{(a) => <option value={a.id}>{a.brokerAccountId}</option>}</For>
          </select>
        </label>
        <label>
          Capital Ratio
          <input value={capitalRatio()} onInput={(e) => setCapitalRatio(e.currentTarget.value)} />
        </label>
        <label>
          Max Qty/Order
          <input value={maxQtyPerOrder()} onInput={(e) => setMaxQtyPerOrder(e.currentTarget.value)} />
        </label>
        <button type="submit">Add to group</button>
      </form>
    </div>
  )
}
