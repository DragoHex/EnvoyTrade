import { render, screen, waitFor, within } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { GroupManagePage } from './GroupManagePage'
import * as api from '../api'

function renderPage() {
  window.history.pushState({}, '', '/accounts/groups/m1')
  return render(() => (
    <Router>
      <Route path="/accounts" component={() => <p>accounts page</p>} />
      <Route path="/accounts/groups/:masterId" component={GroupManagePage} />
    </Router>
  ))
}

const master: api.Account = {
  id: 'm1',
  name: 'Master Account',
  role: 'master',
  broker: 'kite',
  brokerAccountId: 'ZX1234',
  masterId: null,
  cloneFactor: null,
  maxQtyPerOrder: null,
  enabled: true,
  active: true,
  status: 'ok',
}
const follower: api.Account = {
  id: 'f1',
  name: 'Follower Account',
  role: 'follower',
  broker: 'kite',
  brokerAccountId: 'ZY5678',
  masterId: 'm1',
  cloneFactor: '0.5',
  maxQtyPerOrder: 100,
  enabled: true,
  active: true,
  status: 'ok',
}
const unattached: api.Account = {
  id: 'f2',
  name: 'Unattached Follower',
  role: 'follower',
  broker: 'kite',
  brokerAccountId: 'ZZ0000',
  masterId: null,
  cloneFactor: null,
  maxQtyPerOrder: null,
  enabled: false,
  active: true,
  status: 'ok',
}

describe('GroupManagePage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('hydrates via getGroupDetail + getAccounts(ids) and renders master + followers', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    renderPage()

    expect(await screen.findByText('ZY5678')).toBeInTheDocument()
    expect(getAccountsSpy).toHaveBeenCalledWith(['m1', 'f1'])
    expect(screen.getByText('← Back')).toBeInTheDocument()
    expect(screen.getByLabelText('Master')).toBeInTheDocument()
    expect(screen.getByLabelText('Follower')).toBeInTheDocument()
  })

  it('renders remove button as an icon with tooltip and accessible label, and copy-toggles for members', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    renderPage()
    await screen.findByText('ZY5678')

    const removeBtn = screen.getByRole('button', { name: 'Remove from group' })
    expect(removeBtn).toHaveAttribute('data-tooltip', 'Remove from Group')
    expect(removeBtn).toHaveAttribute('title', 'Remove from Group')
    expect(removeBtn.querySelector('svg')).not.toBeNull()

    const toggles = screen.getAllByRole('checkbox', { name: /Toggle active for/ })
    expect(toggles).toHaveLength(2)
    expect(toggles[0]).toBeChecked()
    expect(toggles[1]).toBeChecked()
  })

  it('toggling active on follower calls patchAccount with targeted getAccount update', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({ ...follower, enabled: false })
    const patchSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({ enabled: false })

    renderPage()
    await screen.findByText('ZY5678')
    const initialCallCount = getAccountsSpy.mock.calls.length

    const toggle = screen.getByRole('checkbox', { name: `Toggle active for ${follower.name}` })
    expect(toggle).toBeChecked()

    await userEvent.click(toggle)
    expect(patchSpy).toHaveBeenCalledWith('f1', { enabled: false })
    await waitFor(() => expect(screen.getByText('Follower disabled successfully.')).toBeInTheDocument())
    expect(getAccountSpy).toHaveBeenCalledWith('f1')
    expect(getAccountsSpy).toHaveBeenCalledTimes(initialCallCount)
  })

  it('toggling a disabled follower immediately enables the toggle and un-greys the row without page refresh', async () => {
    const disabledFollower: api.Account = { ...follower, enabled: false }
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: false, status: 'ok' }],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, disabledFollower])
    const patchSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({ enabled: true })
    vi.spyOn(api, 'getAccount').mockResolvedValue({ ...follower, enabled: true })

    renderPage()
    await screen.findByText('ZY5678')

    const followerRow = screen.getByText('ZY5678').closest('tr')!
    expect(followerRow).toHaveAttribute('data-disabled', 'true')

    const toggle = screen.getByRole('checkbox', { name: `Toggle active for ${follower.name}` })
    expect(toggle).not.toBeChecked()

    // Click toggle to enable
    await userEvent.click(toggle)

    // Immediate optimistic update: toggle is checked and row is un-greyed (data-disabled="false")
    expect(toggle).toBeChecked()
    expect(followerRow).toHaveAttribute('data-disabled', 'false')

    // Backend call was issued
    expect(patchSpy).toHaveBeenCalledWith('f1', { enabled: true })

    // Toast appears and state remains enabled
    await waitFor(() => expect(screen.getByText('Follower enabled successfully.')).toBeInTheDocument())
    expect(toggle).toBeChecked()
    expect(followerRow).toHaveAttribute('data-disabled', 'false')
  })

  it('reverts optimistic toggle and shows error toast when patchAccount fails', async () => {
    const disabledFollower: api.Account = { ...follower, enabled: false }
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: false, status: 'ok' }],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, disabledFollower])
    vi.spyOn(api, 'patchAccount').mockRejectedValue(new Error('Network error'))

    renderPage()
    await screen.findByText('ZY5678')

    const followerRow = screen.getByText('ZY5678').closest('tr')!
    expect(followerRow).toHaveAttribute('data-disabled', 'true')

    const toggle = screen.getByRole('checkbox', { name: `Toggle active for ${follower.name}` })
    expect(toggle).not.toBeChecked()

    await userEvent.click(toggle)

    // Reverts to disabled state after failure
    await waitFor(() => expect(screen.getByText('Network error')).toBeInTheDocument())
    expect(toggle).not.toBeChecked()
    expect(followerRow).toHaveAttribute('data-disabled', 'true')
  })

  it('toggling active on master account updates immediately without page refresh', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])
    const patchSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({ active: false })
    vi.spyOn(api, 'getAccount').mockResolvedValue({ ...master, active: false })

    renderPage()
    await screen.findByText('ZX1234')

    const masterRow = screen.getByText('ZX1234').closest('tr')!
    expect(masterRow).toHaveAttribute('data-disabled', 'false')

    const toggle = screen.getByRole('checkbox', { name: `Toggle active for ${master.name}` })
    expect(toggle).toBeChecked()

    await userEvent.click(toggle)

    // Immediately deactivates and dims row
    expect(toggle).not.toBeChecked()
    expect(masterRow).toHaveAttribute('data-disabled', 'true')
    expect(patchSpy).toHaveBeenCalledWith('m1', { active: false })

    await waitFor(() => expect(screen.getByText('Master deactivated successfully.')).toBeInTheDocument())
  })

  it('Remove from group opens confirmation modal before calling removeAccountFromGroup and updates locally', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({ ...follower, masterId: null, groupId: null, groupName: null })
    const removeSpy = vi.spyOn(api, 'removeAccountFromGroup').mockResolvedValue(undefined)

    renderPage()
    await screen.findByText('ZY5678')
    const initialCallCount = getAccountsSpy.mock.calls.length

    await userEvent.click(screen.getByRole('button', { name: 'Remove from group' }))

    expect(removeSpy).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', { name: 'Confirm Remove from group' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(removeSpy).toHaveBeenCalledWith('f1')
    await waitFor(() => expect(screen.getByText('Follower removed from group.')).toBeInTheDocument())
    expect(getAccountSpy).toHaveBeenCalledWith('f1')
    expect(getAccountsSpy).toHaveBeenCalledTimes(initialCallCount)
  })

  it('Add account to group calls addAccountToGroup with the selected account and terms and reflects immediately in the table', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockImplementation((ids?: string[]) => {
      return Promise.resolve(ids ? [master] : [master, unattached])
    })
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({
      ...unattached,
      masterId: 'm1',
      groupId: 'm1',
      cloneFactor: '0.5',
      enabled: true,
    })
    const addSpy = vi.spyOn(api, 'addAccountToGroup').mockResolvedValue(undefined)

    renderPage()
    await screen.findByText('ZX1234')
    expect(screen.getByText(/No follower accounts attached to this group yet/)).toBeInTheDocument()
    const initialCallCount = getAccountsSpy.mock.calls.length

    await userEvent.selectOptions(screen.getByLabelText('Account'), 'f2')
    await userEvent.clear(screen.getByLabelText('Clone Factor'))
    await userEvent.type(screen.getByLabelText('Clone Factor'), '0.5')
    await userEvent.click(screen.getByRole('button', { name: 'Add to group' }))

    await waitFor(() =>
      expect(addSpy).toHaveBeenCalledWith('m1', {
        accountId: 'f2',
        cloneFactor: '0.5',
        maxQtyPerOrder: undefined,
      }),
    )
    expect(getAccountSpy).toHaveBeenCalledWith('f2')
    expect(getAccountsSpy).toHaveBeenCalledTimes(initialCallCount)

    // The newly added follower row immediately reflects in the table
    expect(await screen.findByText('ZZ0000')).toBeInTheDocument()
    expect(screen.getByText('0.5')).toBeInTheDocument()
    expect(screen.queryByText(/No follower accounts attached to this group yet/)).not.toBeInTheDocument()

    // Success toast is shown
    expect(screen.getByText('Follower added to group successfully.')).toBeInTheDocument()
  })
  it('EditGroupModal in GroupManagePage includes all masters for master swap', async () => {
    const otherMaster: api.Account = {
      ...master,
      id: 'm2',
      brokerAccountId: 'ZX5678',
      name: 'Other Master',
    }
    const freeMaster: api.Account = {
      ...master,
      id: 'm3',
      brokerAccountId: 'ZX9999',
      name: 'Free Master',
    }

    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'kite', followerCount: 0, status: 'ok' },
      { id: 'g2', name: 'Group 2', masterId: 'm2', masterAccountId: 'ZX5678', broker: 'kite', followerCount: 0, status: 'ok' },
    ])
    vi.spyOn(api, 'getAccounts').mockImplementation((ids?: string[]) => {
      return Promise.resolve(ids ? [master] : [master, otherMaster, freeMaster])
    })

    renderPage()
    expect(await screen.findByText('ZX1234')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Edit Group' }))
    expect(screen.getByRole('dialog', { name: 'Edit Group' })).toBeInTheDocument()

    const dialog = screen.getByRole('dialog', { name: 'Edit Group' })
    const select = within(dialog).getByLabelText(/Master Account/)
    const options = Array.from(select.querySelectorAll('option')).map((o) => (o as HTMLOptionElement).value)
    // Should include current master m1, and other masters m2 and m3 for master swap
    expect(options).toContain('m1')
    expect(options).toContain('m2')
    expect(options).toContain('m3')
  })

  it('clicking account dropdown on group page displays unattached follower accounts and allows selection', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
    vi.spyOn(api, 'getAccounts').mockImplementation((ids?: string[]) => {
      return Promise.resolve(ids ? [master] : [master, unattached])
    })

    renderPage()
    await screen.findByText('ZX1234')

    // Add Follower card should have group-add-card class to prevent dropdown clipping
    const addCard = screen.getByText('Add Follower to Group').closest('.accounts-card')
    expect(addCard).toHaveClass('group-add-card')

    // Initially "Add to group" button should be disabled because no account is selected
    const addBtn = screen.getByRole('button', { name: 'Add to group' })
    expect(addBtn).toBeDisabled()

    const trigger = screen.getByRole('button', { name: /select an account/i })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    expect(trigger).toHaveTextContent('Select an Account')
    expect(trigger.querySelector('span')).toHaveStyle({ color: 'var(--color-text-secondary, #9da3ae)' })

    await userEvent.click(trigger)

    const listbox = screen.getByRole('listbox', { name: 'Account' })
    expect(listbox).toBeInTheDocument()

    const options = within(listbox).getAllByRole('option')
    expect(options).toHaveLength(1)
    expect(options[0].textContent).toContain('Unattached Follower (ZZ0000)')

    // Click the unattached follower option to select it
    await userEvent.click(options[0])

    // Dropdown listbox closes and trigger updates with standard text color
    expect(screen.queryByRole('listbox', { name: 'Account' })).not.toBeInTheDocument()
    const selectedTrigger = screen.getByRole('button', { name: /Unattached Follower \(ZZ0000\)/i })
    expect(selectedTrigger).toBeInTheDocument()
    expect(selectedTrigger.querySelector('span')).toHaveStyle({ color: 'var(--color-text, #f0f0f2)' })

    // Add to group button is now enabled
    expect(addBtn).not.toBeDisabled()
  })

  it('when no unattached follower accounts exist, account dropdown and add button are disabled with helpful message', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    // Only master and follower (already attached to group) exist — no unattached followers
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    renderPage()
    await screen.findByText('ZX1234')

    const trigger = screen.getByRole('button', { name: /no unassigned follower accounts available/i })
    expect(trigger).toBeInTheDocument()

    // Clicking disabled trigger does not open listbox
    await userEvent.click(trigger)
    expect(screen.queryByRole('listbox', { name: 'Account' })).not.toBeInTheDocument()

    const addBtn = screen.getByRole('button', { name: 'Add to group' })
    expect(addBtn).toBeDisabled()
  })

  it('does not flash raw UUID in header while loading directly without state', async () => {
    let resolveDetail!: (value: any) => void
    const detailPromise = new Promise((resolve) => {
      resolveDetail = resolve
    })
    vi.spyOn(api, 'getGroupDetail').mockReturnValue(detailPromise as any)
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    const testUuid = '64b8a3e9-74d1-419b-b0b1-38290fd32a9e'
    window.history.pushState({}, '', `/accounts/groups/${testUuid}`)
    render(() => (
      <Router>
        <Route path="/accounts/groups/:masterId" component={GroupManagePage} />
      </Router>
    ))

    // During initial load, the UUID must NEVER be present in the document
    expect(screen.queryByText(new RegExp(testUuid))).not.toBeInTheDocument()
    // Skeleton bar is present while loading
    expect(screen.getByLabelText('Loading group name')).toBeInTheDocument()

    // Now resolve the detail
    resolveDetail({
      id: testUuid,
      name: 'Alpha Growth',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })

    expect(await screen.findByText('Group: Alpha Growth')).toBeInTheDocument()
    expect(screen.queryByText(new RegExp(testUuid))).not.toBeInTheDocument()
  })

  it('renders group name immediately on frame 0 when provided via navigation state', async () => {
    let resolveDetail!: (value: any) => void
    const detailPromise = new Promise((resolve) => {
      resolveDetail = resolve
    })
    vi.spyOn(api, 'getGroupDetail').mockReturnValue(detailPromise as any)
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    const testUuid = '64b8a3e9-74d1-419b-b0b1-38290fd32a9e'
    window.history.pushState({ name: 'Alpha Growth' }, '', `/accounts/groups/${testUuid}`)
    render(() => (
      <Router>
        <Route path="/accounts/groups/:masterId" component={GroupManagePage} />
      </Router>
    ))

    // Immediately rendered from state before detailPromise resolves
    expect(screen.getByText('Group: Alpha Growth')).toBeInTheDocument()
    expect(screen.queryByText(new RegExp(testUuid))).not.toBeInTheDocument()

    resolveDetail({
      id: testUuid,
      name: 'Alpha Growth',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
  })

  it('renders Broker column with BrokerLogo in broker-cell-centered without overlapping Role column', async () => {
    const testbrokerFollower: api.Account = {
      ...follower,
      id: 'f-tb',
      name: 'Mock Follower',
      broker: 'testbroker',
      brokerAccountId: 'TB1234',
    }
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [
        { accountId: 'f1', name: 'Follower Account', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' },
        { accountId: 'f-tb', name: 'Mock Follower', brokerAccountId: 'TB1234', enabled: true, status: 'ok' },
      ],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower, testbrokerFollower])

    renderPage()
    await screen.findByText('TB1234')

    // Broker and Role column headers have distinct column classes
    const brokerHeader = screen.getByRole('columnheader', { name: 'Broker' })
    const roleHeader = screen.getByRole('columnheader', { name: 'Role' })
    expect(brokerHeader).toBeInTheDocument()
    expect(brokerHeader).toHaveClass('col-broker')
    expect(roleHeader).toBeInTheDocument()
    expect(roleHeader).toHaveClass('col-role')

    // Table rows have broker-cell-centered and role-cell-centered wrappers
    const brokerCells = document.querySelectorAll('td.col-broker .broker-cell-centered')
    const roleCells = document.querySelectorAll('td.col-role .role-cell-centered')
    expect(brokerCells.length).toBe(3)
    expect(roleCells.length).toBe(3)

    // Zerodha Kite SVG logo rendered for kite accounts
    const kiteLogos = screen.getAllByLabelText('Zerodha Kite')
    expect(kiteLogos.length).toBeGreaterThanOrEqual(1)

    // TestBroker fallback badge rendered for mock broker
    expect(screen.getByText('TEST')).toBeInTheDocument()
    expect(screen.getByLabelText('Test Broker (Mock)')).toBeInTheDocument()
  })

  it('disables Edit Group button while group detail is loading', async () => {
    let resolveDetail!: (value: any) => void
    const detailPromise = new Promise((resolve) => {
      resolveDetail = resolve
    })
    vi.spyOn(api, 'getGroupDetail').mockReturnValue(detailPromise as any)
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    renderPage()

    const editBtn = screen.getByRole('button', { name: 'Edit Group' })
    expect(editBtn).toBeDisabled()

    resolveDetail({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })

    await waitFor(() => expect(editBtn).not.toBeDisabled())
  })

  it('shows Loading accounts in Add Follower dropdown while allAccounts is fetching', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
    let resolveAccounts!: (value: any) => void
    const accountsPromise = new Promise((resolve) => {
      resolveAccounts = resolve
    })
    vi.spyOn(api, 'getAccounts').mockReturnValue(accountsPromise as any)

    renderPage()

    const loadingTrigger = screen.getByRole('button', { name: /loading accounts/i })
    expect(loadingTrigger).toBeInTheDocument()
    expect(loadingTrigger).toHaveAttribute('aria-disabled', 'true')

    resolveAccounts([master, follower])
  })

  it('Add Follower form inputs have proper numeric validation attributes', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, unattached])

    renderPage()
    await screen.findByText('ZX1234')

    const cloneInput = screen.getByLabelText('Clone Factor')
    expect(cloneInput).toHaveAttribute('type', 'number')
    expect(cloneInput).toHaveAttribute('step', 'any')
    expect(cloneInput).toHaveAttribute('min', '0.01')

    const qtyInput = screen.getByLabelText('Max Qty/Order')
    expect(qtyInput).toHaveAttribute('type', 'number')
    expect(qtyInput).toHaveAttribute('step', '1')
    expect(qtyInput).toHaveAttribute('min', '1')
  })

  it('renders helper note when group has only a master and zero followers', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'm1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterActive: true,
      followers: [],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master])

    renderPage()
    await screen.findByText('ZX1234')

    expect(screen.getByText(/no follower accounts attached to this group yet/i)).toBeInTheDocument()
  })
})
