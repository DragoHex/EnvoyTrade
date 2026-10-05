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

  it('Square Off requires modal confirmation before calling squareOffAccount', async () => {
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
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const squareOffSpy = vi.spyOn(api, 'squareOffAccount').mockResolvedValue({ accountId: 'f1', status: 'completed', orders: [] })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const followerRow = within(screen.getByTestId('account-row'))
    await userEvent.click(followerRow.getByLabelText('Square Off'))
    expect(squareOffSpy).not.toHaveBeenCalled()

    await userEvent.click(screen.getByText('Confirm'))
    expect(squareOffSpy).toHaveBeenCalledWith('f1', undefined)
  })

  it('Master Square Off requires modal confirmation before calling squareOffGroup', async () => {
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
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const squareOffGroupSpy = vi.spyOn(api, 'squareOffGroup').mockResolvedValue({
      groupId: 'g1',
      status: 'completed',
      account: { accountId: 'm1', status: 'completed', orders: [] },
      followers: [],
    })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const masterRow = within(screen.getByTestId('master-row'))
    await userEvent.click(masterRow.getByLabelText('Square Off'))
    expect(squareOffGroupSpy).not.toHaveBeenCalled()

    await userEvent.click(screen.getByText('Confirm'))
    expect(squareOffGroupSpy).toHaveBeenCalledWith('g1', undefined)
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

  it('keeps opened dropdowns on Dashboard independent without cross-account data contamination', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Alpha Group', masterId: 'm1', masterAccountId: 'MST001', broker: 'zerodha', followerCount: 2, status: 'ok' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Alpha Group',
      masterId: 'm1',
      masterAccountId: 'MST001',
      masterName: 'Master Alice',
      masterActive: true,
      followers: [
        { accountId: 'f1', name: 'Follower One', brokerAccountId: 'FOL001', enabled: true, status: 'ok', netQty: 10 },
        { accountId: 'f2', name: 'Follower Two', brokerAccountId: 'FOL002', enabled: true, status: 'ok', netQty: 50 },
      ],
    })

    vi.spyOn(api, 'getAccountOrders').mockImplementation(async (accId) => {
      if (accId === 'f1') {
        return {
          summary: { netQty: 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 100, realizedPnl: 0, accountValue: 10000, status: 'online' },
          counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
          pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
          openPositions: [{ product: 'CNC', instrument: 'TCS-F1', qty: 10, avgPrice: '3500.00', ltp: '3550.00', mtm: '100.00', action: 'exit' }],
          closedPositions: [],
          holdings: [],
          openOrders: [],
          closedOrders: [],
          rejectedOrders: [],
        }
      }
      if (accId === 'f2') {
        return {
          summary: { netQty: 50, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 500, realizedPnl: 0, accountValue: 50000, status: 'online' },
          counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
          pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
          openPositions: [{ product: 'CNC', instrument: 'INFY-F2', qty: 50, avgPrice: '1500.00', ltp: '1520.00', mtm: '500.00', action: 'exit' }],
          closedPositions: [],
          holdings: [],
          openOrders: [],
          closedOrders: [],
          rejectedOrders: [],
        }
      }
      return {
        summary: { netQty: 0, openPositionsCount: 0, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
        counts: { openPositions: 0, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
        pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 0, totalPages: 0 },
        openPositions: [],
        closedPositions: [],
        holdings: [],
        openOrders: [],
        closedOrders: [],
        rejectedOrders: [],
      }
    })

    render(() => <Dashboard />)

    expect(await screen.findByText('FOL001')).toBeInTheDocument()
    expect(screen.getByText('FOL002')).toBeInTheDocument()

    // Expand both follower rows
    const expandButtons = screen.getAllByTestId('expand-row-btn')
    // Follower rows are rows 1 and 2 (row 0 is master row)
    await userEvent.click(expandButtons[1]) // expand f1
    await userEvent.click(expandButtons[2]) // expand f2

    // Both expansion rows are now open
    const detailsDrawers = await screen.findAllByTestId('account-order-details')
    expect(detailsDrawers).toHaveLength(2)

    // Verify f1 shows TCS-F1 and f2 shows INFY-F2
    expect(await screen.findByText('TCS-F1')).toBeInTheDocument()
    expect(await screen.findByText('INFY-F2')).toBeInTheDocument()

    // Drawer 0 belongs to f1 and must contain TCS-F1, not INFY-F2
    expect(detailsDrawers[0].textContent).toContain('TCS-F1')
    expect(detailsDrawers[0].textContent).not.toContain('INFY-F2')

    // Drawer 1 belongs to f2 and must contain INFY-F2, not TCS-F1
    expect(detailsDrawers[1].textContent).toContain('INFY-F2')
    expect(detailsDrawers[1].textContent).not.toContain('TCS-F1')
  })
})
