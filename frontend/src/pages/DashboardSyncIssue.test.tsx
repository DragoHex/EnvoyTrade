import { render, screen, fireEvent, waitFor, within } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { Dashboard } from './Dashboard'
import * as api from '../api'

describe('Dashboard Multiple Dropdowns and Tab Switching Isolation', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps Master and Follower dropdown data strictly isolated when both are opened and tabs are clicked', async () => {
    const masterId = 'm1-uuid'
    const follower1Id = 'f1-uuid'
    const follower2Id = 'f2-uuid'

    vi.spyOn(api, 'getGroups').mockResolvedValue([
      {
        id: 'g1',
        name: 'MASTER01',
        masterId,
        masterAccountId: 'MASTER01',
        broker: 'testbroker',
        followerCount: 2,
        status: 'ok',
        active: true,
      },
    ])

    vi.spyOn(api, 'getGroupDetail').mockResolvedValue({
      id: 'g1',
      name: 'MASTER01',
      masterId,
      masterAccountId: 'MASTER01',
      masterName: 'Rajesh Sharma',
      masterActive: true,
      masterNetQty: 200,
      masterOpenPositionsCount: 1,
      masterClosedPositionsCount: 5,
      masterOpenOrdersCount: 0,
      masterTotalMtm: 380,
      followers: [
        {
          accountId: follower1Id,
          name: 'Amit Verma',
          brokerAccountId: 'FOLLOW01A',
          enabled: true,
          status: 'ok',
          netQty: 0,
          openPositionsCount: 0,
          closedPositionsCount: 3,
          openOrdersCount: 1,
          totalMtm: 0,
        },
        {
          accountId: follower2Id,
          name: 'Sneha Kulkarni',
          brokerAccountId: 'FOLLOW01B',
          enabled: true,
          status: 'ok',
          netQty: 200,
          openPositionsCount: 1,
          closedPositionsCount: 3,
          openOrdersCount: 0,
          totalMtm: 380,
        },
      ],
    })

    const masterOrdersResponse: api.AccountOrdersResponse = {
      accountId: masterId,
      role: 'master',
      brokerAccountId: 'MASTER01',
      summary: {
        netQty: 200,
        openPositionsCount: 1,
        closedPositionsCount: 5,
        pendingOrdersCount: 0,
        totalMtm: 380,
        realizedPnl: 380,
        accountValue: 20000000,
        status: 'online',
      },
      counts: {
        openPositions: 1,
        closedPositions: 5,
        holdings: 0,
        openOrders: 0,
        closedOrders: 10,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'open_positions',
        page: 1,
        limit: 10,
        totalCount: 1,
        totalPages: 1,
      },
      openPositions: [
        {
          product: 'NRML',
          instrument: 'CRUDEOIL17SEP26P8200',
          qty: 200,
          avgPrice: '29.70',
          ltp: '29.70',
          mtm: '0',
          pnl: '0',
          action: 'exit',
        },
      ],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    const follower1OrdersResponse: api.AccountOrdersResponse = {
      accountId: follower1Id,
      role: 'follower',
      brokerAccountId: 'FOLLOW01A',
      summary: {
        netQty: 0,
        openPositionsCount: 0,
        closedPositionsCount: 3,
        pendingOrdersCount: 1,
        totalMtm: 0,
        realizedPnl: 0,
        accountValue: 20000000,
        status: 'online',
      },
      counts: {
        openPositions: 0,
        closedPositions: 3,
        holdings: 0,
        openOrders: 1,
        closedOrders: 5,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'open_positions',
        page: 1,
        limit: 10,
        totalCount: 0,
        totalPages: 0,
      },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockImplementation(async (accId, tab) => {
      if (accId === masterId) {
        return {
          ...masterOrdersResponse,
          pagination: { ...masterOrdersResponse.pagination!, tab: tab ?? 'open_positions' },
        }
      }
      if (accId === follower1Id) {
        return {
          ...follower1OrdersResponse,
          pagination: { ...follower1OrdersResponse.pagination!, tab: tab ?? 'open_positions' },
        }
      }
      return {
        ...follower1OrdersResponse,
        accountId: accId,
        pagination: { ...follower1OrdersResponse.pagination!, tab: tab ?? 'open_positions' },
      }
    })

    render(() => <Dashboard />)

    // Wait for group to render
    expect(await screen.findByText('MASTER01 (Master)')).toBeInTheDocument()
    expect(screen.getByText('FOLLOW01A')).toBeInTheDocument()

    // Find all expand buttons: first is Master, second is Follower 1
    const expandButtons = screen.getAllByTestId('expand-row-btn')
    expect(expandButtons.length).toBe(3) // Master + 2 followers

    // 1. Expand Master row
    fireEvent.click(expandButtons[0])

    // Wait for Master's order details drawer to appear
    await waitFor(() => {
      expect(screen.getAllByTestId('account-order-details').length).toBe(1)
    })

    const masterDrawer = screen.getAllByTestId('account-order-details')[0]
    // Verify Master shows CRUDEOIL17SEP26P8200
    expect(within(masterDrawer).getByText('CRUDEOIL17SEP26P8200')).toBeInTheDocument()
    expect(within(masterDrawer).getByText('Open Position (1)')).toBeInTheDocument()

    // 2. Expand Follower 1 row while Master is STILL expanded
    fireEvent.click(expandButtons[1])

    await waitFor(() => {
      expect(screen.getAllByTestId('account-order-details').length).toBe(2)
    })

    const follower1Drawer = screen.getAllByTestId('account-order-details')[1]

    // Follower 1 MUST show Open Position (0) and "No data", NOT Master's position
    await waitFor(() => {
      expect(within(follower1Drawer).getByText('Open Position (0)')).toBeInTheDocument()
    })
    expect(within(follower1Drawer).queryByText('CRUDEOIL17SEP26P8200')).not.toBeInTheDocument()
    expect(within(follower1Drawer).getByText('No data')).toBeInTheDocument()

    // Master MUST still show CRUDEOIL17SEP26P8200
    expect(within(masterDrawer).getByText('CRUDEOIL17SEP26P8200')).toBeInTheDocument()

    // 3. Click "Open Position" tab on Master
    const masterOpenPosBtn = within(masterDrawer).getByRole('tab', { name: /Open Position/i })
    fireEvent.click(masterOpenPosBtn)

    // Follower 1 MUST STILL show "No data" and NOT receive Master's position!
    expect(within(follower1Drawer).queryByText('CRUDEOIL17SEP26P8200')).not.toBeInTheDocument()
    expect(within(follower1Drawer).getByText('No data')).toBeInTheDocument()

    // 4. Click "Open Position" tab on Follower 1
    const followerOpenPosBtn = within(follower1Drawer).getByRole('tab', { name: /Open Position/i })
    fireEvent.click(followerOpenPosBtn)

    // Master MUST STILL show CRUDEOIL17SEP26P8200 and NOT get wiped!
    expect(within(masterDrawer).getByText('CRUDEOIL17SEP26P8200')).toBeInTheDocument()
    expect(within(follower1Drawer).queryByText('CRUDEOIL17SEP26P8200')).not.toBeInTheDocument()
  })
})
