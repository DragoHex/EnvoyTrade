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
      <Route path="/accounts/groups/:masterId" component={GroupManagePage} />
    </Router>
  ))
}

const master: api.Account = {
  id: 'm1', role: 'master', broker: 'kite', brokerAccountId: 'ZX1234', masterId: null,
  capitalRatio: null, maxQtyPerOrder: null, enabled: true, active: true, status: 'ok',
}
const follower: api.Account = {
  id: 'f1', role: 'follower', broker: 'kite', brokerAccountId: 'ZY5678', masterId: 'm1',
  capitalRatio: '0.5', maxQtyPerOrder: 100, enabled: true, active: true, status: 'ok',
}
const unattached: api.Account = {
  id: 'f2', role: 'follower', broker: 'kite', brokerAccountId: 'ZZ0000', masterId: null,
  capitalRatio: null, maxQtyPerOrder: null, enabled: false, active: true, status: 'ok',
}

describe('GroupManagePage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('hydrates via getGroupDetail + getAccounts(ids) and renders master + followers', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      masterId: 'm1', masterAccountId: 'ZX1234', masterActive: true,
      followers: [{ accountId: 'f1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])

    renderPage()

    expect(await screen.findByText('ZY5678')).toBeInTheDocument()
    expect(getAccountsSpy).toHaveBeenCalledWith(['m1', 'f1'])
  })

  it('Remove from group calls removeAccountFromGroup and refetches', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      masterId: 'm1', masterAccountId: 'ZX1234', masterActive: true,
      followers: [{ accountId: 'f1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master, follower])
    const removeSpy = vi.spyOn(api, 'removeAccountFromGroup').mockResolvedValue(undefined)

    renderPage()
    await screen.findByText('ZY5678')
    await userEvent.click(screen.getByRole('button', { name: 'Remove from group' }))

    expect(removeSpy).toHaveBeenCalledWith('f1')
  })

  it('Add account to group calls addAccountToGroup with the selected account and terms', async () => {
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      masterId: 'm1', masterAccountId: 'ZX1234', masterActive: true, followers: [],
    })
    vi.spyOn(api, 'getAccounts').mockImplementation((ids?: string[]) =>
      Promise.resolve(ids ? [master] : [master, unattached]),
    )
    const addSpy = vi.spyOn(api, 'addAccountToGroup').mockResolvedValue(undefined)

    renderPage()
    await screen.findByText('ZX1234')

    await userEvent.selectOptions(screen.getByLabelText('Account'), 'f2')
    await userEvent.type(screen.getByLabelText('Capital Ratio'), '0.5')
    await userEvent.click(screen.getByRole('button', { name: 'Add to group' }))

    await waitFor(() => expect(addSpy).toHaveBeenCalledWith('m1', { accountId: 'f2', capitalRatio: '0.5', maxQtyPerOrder: undefined }))
  })
})
