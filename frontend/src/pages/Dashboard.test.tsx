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

  it('Follower Rebalance requires modal confirmation before calling rebalanceAccount', async () => {
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
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower 1',
      broker_account_id: 'ZY5678',
      enabled: true,
      clone_factor: '1',
      symbols: [
        {
          exchange: 'NSE',
          tradingsymbol: 'INFY',
          product: 'CNC',
          lot_size: 1,
          master_qty: 10,
          target_qty: 10,
          follower_qty: 0,
          drift_qty: 10,
          action: 'BUY',
        },
      ],
    })
    const rebalanceSpy = vi.spyOn(api, 'rebalanceAccount').mockResolvedValue({
      action: 'rebalance',
      status: 'completed',
      account_id: 'f1',
      followers_affected: 1,
      cancelled_orders: 0,
      orders_placed: 1,
      orders: [],
    })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const followerRow = within(screen.getByTestId('account-row'))
    await userEvent.click(followerRow.getByLabelText('Rebalance'))
    expect(rebalanceSpy).not.toHaveBeenCalled()

    expect(await screen.findByText('INFY')).toBeInTheDocument()
    const modal = within(screen.getByRole('dialog'))
    await userEvent.click(modal.getByRole('button', { name: /Rebalance/i }))
    expect(rebalanceSpy).toHaveBeenCalledWith('f1')
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

    await userEvent.click(screen.getByText('Sq-off'))
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

    await userEvent.click(screen.getByText('Sq-off'))
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

  it('displays yellow group status dot and rebalance notification dots when active follower drift exists', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Alpha Group', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok', hasImbalance: true },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Alpha Group',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      hasImbalance: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 1,
      followers_with_drift: 1,
      drifts: [
        {
          account_id: 'f1',
          account_name: 'Follower 1',
          broker_account_id: 'ZY5678',
          enabled: true,
          clone_factor: '1',
          symbols: [
            {
              exchange: 'NFO',
              tradingsymbol: 'NIFTY26OCTFUT',
              product: 'NRML',
              lot_size: 50,
              master_qty: 50,
              target_qty: 50,
              follower_qty: 0,
              drift_qty: 50,
              action: 'BUY',
            },
          ],
        },
      ],
    })

    render(() => <Dashboard />)

    expect(await screen.findByText('Alpha Group')).toBeInTheDocument()

    // Status dot next to group name should be yellow ("warning")
    await waitFor(() => {
      const heading = screen.getByRole('heading', { level: 2 })
      const statusDot = within(heading).getByTestId('status-dot')
      expect(statusDot).toHaveAttribute('data-status', 'warning')
    })

    // Rebalance buttons for master and follower f1 should have notification dots
    const rebalanceButtons = screen.getAllByRole('button', { name: 'Rebalance' })
    expect(within(rebalanceButtons[0]).getByTestId('rebalance-notification-dot')).toBeInTheDocument()
    expect(within(rebalanceButtons[1]).getByTestId('rebalance-notification-dot')).toBeInTheDocument()
  })

  it('keeps green group status dot and does not fetch diff when group is in equilibrium', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Alpha Group', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok', hasImbalance: false },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Alpha Group',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      hasImbalance: false,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
    })
    const diffSpy = vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 1,
      followers_with_drift: 0,
      drifts: [],
    })

    render(() => <Dashboard />)

    expect(await screen.findByText('Alpha Group')).toBeInTheDocument()

    const heading = screen.getByRole('heading', { level: 2 })
    const statusDot = within(heading).getByTestId('status-dot')
    expect(statusDot).toHaveAttribute('data-status', 'ok')

    const rebalanceButtons = screen.getAllByRole('button', { name: 'Rebalance' })
    expect(within(rebalanceButtons[0]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()
    expect(within(rebalanceButtons[1]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()

    // Assert that diff was never fetched because hasImbalance is false
    expect(diffSpy).not.toHaveBeenCalled()
  })

  it('periodically polls getGroupRebalanceDiff every 30s when hasImbalance is true', async () => {
    vi.useFakeTimers()
    try {
      vi.spyOn(api, 'getGroups').mockResolvedValue([
        { id: 'g1', name: 'Alpha Group', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok', hasImbalance: true },
      ])
      vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
        id: 'g1',
        name: 'Alpha Group',
        masterId: 'm1',
        masterAccountId: 'ZX1234',
        masterName: 'Alice Trader',
        masterActive: true,
        hasImbalance: true,
        followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'ok' }],
      })
      const diffSpy = vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
        group_id: 'g1',
        master_id: 'm1',
        followers_evaluated: 1,
        followers_with_drift: 1,
        drifts: [],
      })

      render(() => <Dashboard />)
      await vi.advanceTimersByTimeAsync(10)

      expect(diffSpy).toHaveBeenCalledTimes(1)

      // Fast-forward 30 seconds
      await vi.advanceTimersByTimeAsync(30_000)
      expect(diffSpy).toHaveBeenCalledTimes(2)

      // Fast-forward another 30 seconds
      await vi.advanceTimersByTimeAsync(30_000)
      expect(diffSpy).toHaveBeenCalledTimes(3)
    } finally {
      vi.useRealTimers()
    }
  })

  it('treats imbalance as resolved if all drifting followers are disabled', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Alpha Group', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'ok', hasImbalance: true },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Alpha Group',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      hasImbalance: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: false, status: 'ok' }],
    })
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 1,
      followers_with_drift: 1,
      drifts: [
        {
          account_id: 'f1',
          account_name: 'Follower 1',
          broker_account_id: 'ZY5678',
          enabled: false,
          clone_factor: '1',
          symbols: [
            {
              exchange: 'NSE',
              tradingsymbol: 'INFY',
              product: 'CNC',
              lot_size: 1,
              master_qty: 10,
              target_qty: 10,
              follower_qty: 0,
              drift_qty: 10,
              action: 'BUY',
            },
          ],
        },
      ],
    })

    render(() => <Dashboard />)
    expect(await screen.findByText('Alpha Group')).toBeInTheDocument()

    await waitFor(() => {
      const heading = screen.getByRole('heading', { level: 2 })
      const statusDot = within(heading).getByTestId('status-dot')
      expect(statusDot).toHaveAttribute('data-status', 'ok')
    })
  })

  it('renders DashboardHeroCard displaying EnvoyTrade branding and updates MTM from group details', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Alpha Traders', masterId: 'm1', masterAccountId: 'AT001', broker: 'zerodha', followerCount: 1, status: 'ok' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Alpha Traders',
      masterId: 'm1',
      masterAccountId: 'AT001',
      masterActive: true,
      masterTotalMtm: 4500.5,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'FO100', enabled: true, status: 'ok', totalMtm: 1200 }],
    })

    render(() => <Dashboard />)

    // Hero card renders
    const heroCard = await screen.findByTestId('dashboard-hero-card')
    expect(heroCard).toBeInTheDocument()
    expect(within(heroCard).getByText('EnvoyTrade')).toBeInTheDocument()

    // Default scope is all masters: masterTotalMtm = 4500.50
    await waitFor(() => {
      expect(within(heroCard).getByText('+₹4,500.50')).toBeInTheDocument()
    })
  })

  it('batches concurrent group details across multiple groups and reflects aggregated MTM in Hero Card', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Alpha Traders', masterId: 'm1', masterAccountId: 'AT001', broker: 'zerodha', followerCount: 1, status: 'ok', active: true },
      { id: 'g2', name: 'Beta Traders', masterId: 'm2', masterAccountId: 'BT002', broker: 'zerodha', followerCount: 1, status: 'ok', active: true },
    ])
    vi.spyOn(api, 'getGroupDetail').mockImplementation(async (id: string) => {
      if (id === 'g1') {
        return {
          id: 'g1',
          name: 'Alpha Traders',
          masterId: 'm1',
          masterAccountId: 'AT001',
          masterActive: true,
          masterTotalMtm: 3000,
          followers: [],
        }
      }
      return {
        id: 'g2',
        name: 'Beta Traders',
        masterId: 'm2',
        masterAccountId: 'BT002',
        masterActive: true,
        masterTotalMtm: 4000,
        followers: [],
      }
    })

    render(() => <Dashboard />)

    const heroCard = await screen.findByTestId('dashboard-hero-card')
    expect(heroCard).toBeInTheDocument()

    // Aggregates both masters: 3000 + 4000 = 7000
    await waitFor(() => {
      expect(within(heroCard).getByText('+₹7,000.00')).toBeInTheDocument()
    })
  })

  it('clicking error status dot on an account row opens AccountDetailDrawer with auth error banner', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'error' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'error' }],
    })
    const getAccountSpy = vi.spyOn(api, 'getAccount').mockResolvedValue({
      id: 'f1',
      name: 'Follower 1',
      role: 'follower',
      broker: 'kite',
      brokerAccountId: 'ZY5678',
      masterId: 'm1',
      cloneFactor: '1',
      maxQtyPerOrder: null,
      enabled: true,
      active: true,
      status: 'error',
      authStatus: 'error',
      authError: 'Session expired or token invalid',
    })

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const statusDots = screen.getAllByTitle('Account error - click to edit')
    expect(statusDots.length).toBeGreaterThan(0)

    await userEvent.click(statusDots[statusDots.length - 1])

    expect(getAccountSpy).toHaveBeenCalledWith('f1')
    expect(await screen.findByText('Edit Account')).toBeInTheDocument()
    expect(await screen.findByText(/Authentication required:/)).toBeInTheDocument()
    expect(screen.getByText(/Session expired or token invalid/)).toBeInTheDocument()
  })

  it('saving edited account in drawer calls patchAccount and shows success toast', async () => {
    vi.spyOn(api, 'getGroups').mockResolvedValue([
      { id: 'g1', name: 'Group 1', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'zerodha', followerCount: 1, status: 'error' },
    ])
    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'Group 1',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Alice Trader',
      masterActive: true,
      followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'ZY5678', enabled: true, status: 'error' }],
    })
    vi.spyOn(api, 'getAccount').mockResolvedValue({
      id: 'f1',
      name: 'Follower 1',
      role: 'follower',
      broker: 'kite',
      brokerAccountId: 'ZY5678',
      masterId: 'm1',
      cloneFactor: '1',
      maxQtyPerOrder: null,
      enabled: true,
      active: true,
      status: 'error',
      authStatus: 'error',
      authError: 'Session expired',
    })
    const patchAccountSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({} as any)

    render(() => <Dashboard />)
    await screen.findByText('ZY5678')

    const statusDots = screen.getAllByTitle('Account error - click to edit')
    await userEvent.click(statusDots[statusDots.length - 1])

    expect(await screen.findByText('Edit Account')).toBeInTheDocument()

    const nameInput = screen.getByLabelText('Name')
    await userEvent.clear(nameInput)
    await userEvent.type(nameInput, 'Updated Follower')

    const saveBtn = screen.getByRole('button', { name: /^Save$/i })
    await userEvent.click(saveBtn)

    expect(patchAccountSpy).toHaveBeenCalledWith('f1', expect.objectContaining({ name: 'Updated Follower' }))
    expect(await screen.findByText('Account saved successfully.')).toBeInTheDocument()
  })
})




