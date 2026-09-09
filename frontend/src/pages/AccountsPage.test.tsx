import { render, screen, waitFor } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AccountsPage } from './AccountsPage'
import * as api from '../api'

function renderPage() {
  window.history.pushState({}, '', '/accounts')
  return render(() => (
    <Router>
      <Route path="/accounts" component={AccountsPage} />
      <Route path="/accounts/groups/:masterId" component={() => <p>group page</p>} />
    </Router>
  ))
}

const master: api.Account = {
  id: 'm1', role: 'master', broker: 'kite', brokerAccountId: 'ZX1234', masterId: null,
  capitalRatio: null, maxQtyPerOrder: null, enabled: true, active: true, status: 'ok',
}

describe('AccountsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('Groups tab (default) lists groups from getGroups; row click navigates to the group page', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { masterId: 'm1', masterAccountId: 'ZX1234', broker: 'kite', followerCount: 1, status: 'ok' },
    ])
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master])

    renderPage()

    expect(await screen.findByText('ZX1234')).toBeInTheDocument()
    await userEvent.click(screen.getByTestId('group-row'))
    expect(await screen.findByText('group page')).toBeInTheDocument()
  })

  it('Accounts tab lists accounts and Add Account opens an empty create drawer', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master])

    renderPage()
    await userEvent.click(screen.getByRole('button', { name: 'Accounts' }))

    expect(await screen.findByText('ZX1234')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Add Account' }))
    expect(screen.getByRole('dialog', { name: 'Add Account' })).toBeInTheDocument()
  })

  it('deleting an account calls deleteAccount and refetches', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master])
    const deleteSpy = vi.spyOn(api, 'deleteAccount').mockResolvedValue(undefined)

    renderPage()
    await userEvent.click(screen.getByRole('button', { name: 'Accounts' }))
    await screen.findByText('ZX1234')

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))

    expect(deleteSpy).toHaveBeenCalledWith('m1')
    await waitFor(() => expect(getAccountsSpy).toHaveBeenCalledTimes(2))
  })
})
