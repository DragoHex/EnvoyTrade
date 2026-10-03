import { render, screen, waitFor, within } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { Dashboard } from './Dashboard'
import * as api from '../api'

describe('Dashboard', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders one GroupCard per group, with that group detail', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })

    render(() => <Dashboard />)

    expect(await screen.findByText(/ZX1234/)).toBeInTheDocument()
    expect(await screen.findByText('ZY5678')).toBeInTheDocument()
  })

  it('CopyToggle off calls patchAccount({enabled:false}) and refetches the group', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok' },
    ])
    const detailSpy = vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const patchSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({ enabled: false })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    await userEvent.click(screen.getByRole('checkbox'))

    expect(patchSpy).toHaveBeenCalledWith('f1', { enabled: false })
    await waitFor(() => expect(detailSpy).toHaveBeenCalledTimes(2))
  })

  it('Rebalance calls postAction and refetches the group', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok' },
    ])
    const detailSpy = vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const actionSpy = vi.spyOn(api, 'postAction').mockResolvedValue({ type: 'rebalance', status: 'accepted' })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const followerRow = within(screen.getByTestId('account-row'))
    await userEvent.click(followerRow.getByLabelText('Rebalance'))

    expect(actionSpy).toHaveBeenCalledWith('f1', 'rebalance')
    await waitFor(() => expect(detailSpy).toHaveBeenCalledTimes(2))
  })

  it('Square Off requires modal confirmation before calling postAction', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const actionSpy = vi.spyOn(api, 'postAction').mockResolvedValue({ type: 'square_off', status: 'accepted' })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const followerRow = within(screen.getByTestId('account-row'))
    await userEvent.click(followerRow.getByLabelText('Square Off'))
    expect(actionSpy).not.toHaveBeenCalled()

    await userEvent.click(screen.getByText('Confirm'))
    expect(actionSpy).toHaveBeenCalledWith('f1', 'square_off')
  })

  it('renders empty state message and link to accounts when no groups exist', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([])

    render(() => <Dashboard />)

    expect(await screen.findByText(/No group added\. Please go to/)).toBeInTheDocument()
    const accountsLink = screen.getByRole('link', { name: 'accounts' })
    expect(accountsLink).toBeInTheDocument()
    expect(accountsLink).toHaveAttribute('href', '/accounts')
    expect(screen.getByTestId('order-empty-state')).toBeInTheDocument()
  })

  it('renders master and follower metrics auto-filled on initial page load without expanding', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      masterNetQty: 200,
      masterOpenPositionsCount: 3,
      masterClosedPositionsCount: 1,
      masterOpenOrdersCount: 2,
      masterTotalMtm: 4500.5,
      masterAvailableCash: 100000,
      masterAvailableMargin: 250000,
      followers: [
        {
          accountId: 'f1',
          name: 'Follower 1',
          brokerAccountId: 'ZY5678',
          enabled: true,
          status: 'ok',
          netQty: 100,
          openPositionsCount: 2,
          closedPositionsCount: 1,
          openOrdersCount: 1,
          totalMtm: 2250.25,
          availableCash: 50000,
          availableMargin: 125000,
        },
      ],
    })

    render(() => <Dashboard />)

    expect(await screen.findByText('200')).toBeInTheDocument()
    expect(screen.getByText('3/1')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
    expect(screen.getByText('2/1')).toBeInTheDocument()
    expect(screen.getByText('₹4,500.50')).toBeInTheDocument()
    expect(screen.getByText('₹2,250.25')).toBeInTheDocument()
  })
})
