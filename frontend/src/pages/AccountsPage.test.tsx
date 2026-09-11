import { render, screen, waitFor } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AccountsPage } from './AccountsPage'
import * as api from '../api'

function renderPage(initialPath = '/accounts') {
  window.history.pushState({}, '', initialPath)
  return render(() => (
    <Router>
      <Route path="/accounts" component={AccountsPage} />
      <Route path="/accounts/groups/:masterId" component={() => <p>group page</p>} />
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

describe('AccountsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('Groups tab (default) lists groups from getGroups; row click navigates to the group page', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'kite', followerCount: 1, status: 'ok' },
    ])
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master])

    renderPage()

    expect(await screen.findByText('ZX1234')).toBeInTheDocument()
    const brokerLogo = screen.getByLabelText('Zerodha Kite')
    expect(brokerLogo).toHaveAttribute('data-tooltip', 'Zerodha Kite')
    expect(brokerLogo.querySelector('svg.broker-logo-svg')).not.toBeNull()

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

  it('toggling active on an account calls patchAccount with toggled value and updates targeted account', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master])
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({ ...master, active: false })
    const patchSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({ ...master, active: false })

    renderPage('/accounts?tab=accounts')
    await screen.findByText('ZX1234')

    const toggle = screen.getByRole('checkbox', { name: `Toggle active for ${master.name}` })
    expect(toggle).toBeChecked()

    await userEvent.click(toggle)
    expect(patchSpy).toHaveBeenCalledWith('m1', { active: false })
    await waitFor(() => expect(screen.getByText('Account deactivated successfully.')).toBeInTheDocument())
    expect(getAccountSpy).toHaveBeenCalledWith('m1')
    expect(getAccountsSpy).toHaveBeenCalledTimes(1)
    expect(toggle).not.toBeChecked()
  })

  it('deleting an account opens confirmation modal before calling deleteAccount and removes locally without refetching all', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([master])
    const deleteSpy = vi.spyOn(api, 'deleteAccount').mockResolvedValue(undefined)

    renderPage()
    await userEvent.click(screen.getByRole('button', { name: 'Accounts' }))
    await screen.findByText('ZX1234')

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(deleteSpy).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(deleteSpy).toHaveBeenCalledWith('m1')
    await waitFor(() => expect(screen.getByText('Account deleted successfully.')).toBeInTheDocument())
    expect(getAccountsSpy).toHaveBeenCalledTimes(1)
    expect(screen.queryByText('ZX1234')).not.toBeInTheDocument()
  })

  it('removing follower from group opens confirmation modal before calling removeAccountFromGroup and updates targeted account', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    const getAccountsSpy = vi.spyOn(api, 'getAccounts').mockResolvedValue([follower])
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({ ...follower, masterId: null, groupId: null, groupName: null })
    const removeSpy = vi.spyOn(api, 'removeAccountFromGroup').mockResolvedValue(undefined)

    renderPage()
    await userEvent.click(screen.getByRole('button', { name: 'Accounts' }))
    await screen.findByText('ZY5678')

    await userEvent.click(screen.getByRole('button', { name: 'Remove from group' }))
    expect(removeSpy).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(removeSpy).toHaveBeenCalledWith('f1')
    await waitFor(() => expect(screen.getByText('Removed from group successfully.')).toBeInTheDocument())
    expect(getAccountSpy).toHaveBeenCalledWith('f1')
    expect(getAccountsSpy).toHaveBeenCalledTimes(1)
  })

  it('opens Accounts tab directly when initial URL has ?tab=accounts and updates search params on toggle', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    vi.spyOn(api, 'getAccounts').mockResolvedValue([master])

    renderPage('/accounts?tab=accounts')

    expect(await screen.findByText('ZX1234')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add Account' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Accounts' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Groups' })).toHaveAttribute('aria-pressed', 'false')

    await userEvent.click(screen.getByRole('button', { name: 'Groups' }))
    expect(screen.getByRole('button', { name: 'Groups' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.queryByRole('button', { name: 'Add Account' })).not.toBeInTheDocument()
    expect(window.location.search).toBe('')

    await userEvent.click(screen.getByRole('button', { name: 'Accounts' }))
    expect(screen.getByRole('button', { name: 'Accounts' })).toHaveAttribute('aria-pressed', 'true')
    expect(window.location.search).toBe('?tab=accounts')
  })
})
