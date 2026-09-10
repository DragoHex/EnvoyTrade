import { render, screen, waitFor } from '@solidjs/testing-library'
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
  capitalRatio: null,
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
  capitalRatio: '0.5',
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
  capitalRatio: null,
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

  it('Add account to group calls addAccountToGroup with the selected account and terms', async () => {
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
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({ ...unattached, masterId: 'm1' })
    const addSpy = vi.spyOn(api, 'addAccountToGroup').mockResolvedValue(undefined)

    renderPage()
    await screen.findByText('ZX1234')
    const initialCallCount = getAccountsSpy.mock.calls.length

    await userEvent.selectOptions(screen.getByLabelText('Account'), 'f2')
    await userEvent.type(screen.getByLabelText('Capital Ratio'), '0.5')
    await userEvent.click(screen.getByRole('button', { name: 'Add to group' }))

    await waitFor(() =>
      expect(addSpy).toHaveBeenCalledWith('m1', {
        accountId: 'f2',
        capitalRatio: '0.5',
        maxQtyPerOrder: undefined,
      }),
    )
    expect(getAccountSpy).toHaveBeenCalledWith('f2')
    expect(getAccountsSpy).toHaveBeenCalledTimes(initialCallCount)
  })
})
