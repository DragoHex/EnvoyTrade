import { render, screen, fireEvent } from '@solidjs/testing-library'
import { describe, expect, it, vi } from 'vitest'
import { AccountOrderDetails } from './AccountOrderDetails'
import * as api from '../api'

describe('AccountOrderDetails component', () => {
  it('renders persistent summary metrics, handles string decimals, and navigates between tabs without freezing', async () => {
    const mockData: api.AccountOrdersResponse = {
      summary: {
        netQty: -890,
        openPositionsCount: 1,
        closedPositionsCount: 1,
        pendingOrdersCount: 1,
        totalMtm: '380.00',
        realizedPnl: '0.00',
        accountValue: '2163520.84',
        status: 'online',
      },
      counts: {
        openPositions: 1,
        closedPositions: 1,
        holdings: 1,
        openOrders: 1,
        closedOrders: 1,
        rejectedOrders: 1,
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
          product: 'CNC',
          instrument: 'CRUDEOIL17SEP26C10600',
          qty: -100,
          avgPrice: '0.00/111.10',
          ltp: '114.40',
          mtm: '-330.00',
          action: 'exit',
        },
      ],
      closedPositions: [
        {
          product: 'CNC',
          instrument: 'CRUDEOIL17SEP26P8200',
          qty: 0,
          avgPrice: '23.00/40.20',
          ltp: '29.70',
          mtm: '1720.00',
        },
      ],
      holdings: [
        {
          instrument: 'ASIANPAINT-EQ',
          sellableQuantity: 1,
          buyAveragePrice: '2369.20',
          ltp: '2469.20',
          pnl: '100.00',
          action: 'exit',
        },
      ],
      openOrders: [
        {
          product: 'CNC',
          time: '2026-09-11 09:15:00',
          instrument: 'CRUDEOIL17SEP26P8400',
          quantity: 200,
          triggerPrice: '45.00',
          limitPrice: '46.00',
          type: 'B',
        },
      ],
      closedOrders: [
        {
          product: 'CNC',
          time: '2026-09-11 14:24:05',
          instrument: 'CRUDEOIL17SEP26C10600',
          quantity: 100,
          price: '111.10',
          type: 'S',
        },
      ],
      rejectedOrders: [
        {
          time: '2026-09-11 09:16:00',
          instrument: 'CRUDEOIL17SEP26P8500',
          quantity: 100,
          type: 'S',
          reason: 'Insufficient margin',
        },
      ],
    }

    vi.spyOn(api, 'getAccountOrders').mockResolvedValue(mockData)

    render(() => <AccountOrderDetails accountId="acc-123" />)

    // Verify summary metrics render properly
    expect(await screen.findByText('Net Qty')).toBeInTheDocument()
    expect(screen.getByText('-890')).toBeInTheDocument()
    expect(screen.getByText('1 / 1')).toBeInTheDocument()
    expect(screen.getByText('₹380.00')).toBeInTheDocument()
    expect(screen.getByText('₹21,63,520.84')).toBeInTheDocument()

    // Verify tabs render with item counts
    expect(screen.getByText('Open Position (1)')).toBeInTheDocument()
    expect(screen.getByText('Closed Position (1)')).toBeInTheDocument()
    expect(screen.getByText('Holding (1)')).toBeInTheDocument()
    expect(screen.getByText('Open Order (1)')).toBeInTheDocument()
    expect(screen.getByText('Closed Order (1)')).toBeInTheDocument()
    expect(screen.getByText('Rejected Order (1)')).toBeInTheDocument()

    // Default tab is Open Position: string decimal LTP and MTM are rendered safely
    expect(screen.getByText('CRUDEOIL17SEP26C10600')).toBeInTheDocument()
    expect(screen.getByText('114.40')).toBeInTheDocument()
    expect(screen.getByText('-₹330.00')).toBeInTheDocument()

    // Navigator is present for Open Position tab
    const nav = screen.getByTestId('order-pagination-navigator')
    expect(nav).toHaveTextContent('Showing 1–1 of 1 items')
    expect(nav).toHaveTextContent('Page 1 of 1')
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()

    // Switch to Closed Position tab
    fireEvent.click(screen.getByText('Closed Position (1)'))
    expect(await screen.findByText('CRUDEOIL17SEP26P8200')).toBeInTheDocument()
    expect(screen.getByText('29.70')).toBeInTheDocument()
    expect(screen.getByText('₹1,720.00')).toBeInTheDocument()

    // Switch to Holding tab
    fireEvent.click(screen.getByText('Holding (1)'))
    expect(await screen.findByText('ASIANPAINT-EQ')).toBeInTheDocument()
    expect(screen.getByText('2369.20')).toBeInTheDocument()
    expect(screen.getByText('2469.20')).toBeInTheDocument()
    expect(screen.getByText('₹100.00')).toBeInTheDocument()

    // Switch to Open Order tab
    fireEvent.click(screen.getByText('Open Order (1)'))
    expect(await screen.findByText('CRUDEOIL17SEP26P8400')).toBeInTheDocument()
    expect(screen.getByText('45.00')).toBeInTheDocument()
    expect(screen.getByText('46.00')).toBeInTheDocument()

    // Switch to Closed Order tab
    fireEvent.click(screen.getByText('Closed Order (1)'))
    expect(await screen.findByText('2026-09-11 14:24:05')).toBeInTheDocument()
    expect(screen.getByText('111.10')).toBeInTheDocument()

    // Switch to Rejected Order tab
    fireEvent.click(screen.getByText('Rejected Order (1)'))
    expect(await screen.findByText('CRUDEOIL17SEP26P8500')).toBeInTheDocument()
    expect(screen.getByText('Insufficient margin')).toBeInTheDocument()

    // Switch back to Open Position tab
    fireEvent.click(screen.getByText('Open Position (1)'))
    expect(await screen.findByText('CRUDEOIL17SEP26C10600')).toBeInTheDocument()
  })

  it('removes scale/balance and close buttons while keeping prohibit and exit actions', async () => {
    const mockData: api.AccountOrdersResponse = {
      summary: {
        netQty: 0,
        openPositionsCount: 0,
        closedPositionsCount: 0,
        pendingOrdersCount: 0,
        totalMtm: 0,
        realizedPnl: 0,
        accountValue: 0,
        status: 'online',
      },
      counts: {
        openPositions: 0,
        closedPositions: 0,
        holdings: 0,
        openOrders: 0,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'open_positions',
        page: 1,
        limit: 10,
        totalCount: 0,
        totalPages: 1,
      },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockResolvedValue(mockData)

    render(() => <AccountOrderDetails accountId="acc-123" />)

    await screen.findByText('Net Qty')

    // Scale / Balance and Close drawer buttons must NOT be present
    expect(screen.queryByLabelText('Scale / Balance')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Close drawer')).not.toBeInTheDocument()

    // Prohibit Trading and Exit / Logout buttons must be present
    expect(screen.getByLabelText('Prohibit Trading')).toBeInTheDocument()
    expect(screen.getByLabelText('Exit / Logout')).toBeInTheDocument()
  })

  it('supports database-level pagination: navigates pages, respects limits, and handles boundary disabled states', async () => {
    const getMockResponse = (tab: string, page: number, limit: number): api.AccountOrdersResponse => {
      const totalCount = 25
      const totalPages = Math.ceil(totalCount / limit)
      const startIndex = (page - 1) * limit
      const countOnPage = Math.min(limit, Math.max(0, totalCount - startIndex))

      const positions: api.PositionItem[] = Array.from({ length: countOnPage }, (_, i) => ({
        product: 'CNC',
        instrument: `INSTRUMENT-${startIndex + i + 1}`,
        qty: 10,
        avgPrice: '100.00',
        ltp: '105.00',
        mtm: '50.00',
        action: 'exit',
      }))

      return {
        summary: {
          netQty: 100,
          openPositionsCount: 25,
          closedPositionsCount: 0,
          pendingOrdersCount: 0,
          totalMtm: 1250,
          realizedPnl: 0,
          accountValue: 50000,
          status: 'online',
        },
        counts: {
          openPositions: 25,
          closedPositions: 0,
          holdings: 0,
          openOrders: 0,
          closedOrders: 0,
          rejectedOrders: 0,
        },
        pagination: {
          tab,
          page,
          limit,
          totalCount,
          totalPages,
        },
        openPositions: positions,
        closedPositions: [],
        holdings: [],
        openOrders: [],
        closedOrders: [],
        rejectedOrders: [],
      }
    }

    const spy = vi.spyOn(api, 'getAccountOrders').mockImplementation(async (_id, tab = 'open_positions', page = 1, limit = 10) => {
      return getMockResponse(tab, page, limit)
    })

    render(() => <AccountOrderDetails accountId="acc-page-test" />)

    // Page 1 initial render
    expect(await screen.findByText('INSTRUMENT-1')).toBeInTheDocument()
    expect(screen.getByText('INSTRUMENT-10')).toBeInTheDocument()
    expect(screen.queryByText('INSTRUMENT-11')).not.toBeInTheDocument()

    const getNav = () => screen.getByTestId('order-pagination-navigator')
    const getPrevBtn = () => screen.getByRole('button', { name: 'Previous page' })
    const getNextBtn = () => screen.getByRole('button', { name: 'Next page' })

    // Pagination info on Page 1
    expect(getNav()).toHaveTextContent('Showing 1–10 of 25 items')
    expect(getNav()).toHaveTextContent('Page 1 of 3')

    // Boundary: Prev disabled on first page, Next enabled
    expect(getPrevBtn()).toBeDisabled()
    expect(getNextBtn()).toBeEnabled()

    // Navigate to Page 2
    fireEvent.click(getNextBtn())
    expect(await screen.findByText('INSTRUMENT-11')).toBeInTheDocument()
    expect(screen.getByText('INSTRUMENT-20')).toBeInTheDocument()
    expect(screen.queryByText('INSTRUMENT-10')).not.toBeInTheDocument()

    expect(getNav()).toHaveTextContent('Showing 11–20 of 25 items')
    expect(getNav()).toHaveTextContent('Page 2 of 3')
    expect(getPrevBtn()).toBeEnabled()
    expect(getNextBtn()).toBeEnabled()

    // Navigate to Page 3 (last page with 5 items)
    fireEvent.click(getNextBtn())
    expect(await screen.findByText('INSTRUMENT-21')).toBeInTheDocument()
    expect(screen.getByText('INSTRUMENT-25')).toBeInTheDocument()
    expect(screen.queryByText('INSTRUMENT-26')).not.toBeInTheDocument()

    expect(getNav()).toHaveTextContent('Showing 21–25 of 25 items')
    expect(getNav()).toHaveTextContent('Page 3 of 3')
    // Boundary: Next disabled on last page, Prev enabled
    expect(getPrevBtn()).toBeEnabled()
    expect(getNextBtn()).toBeDisabled()

    // Navigate back to Page 2 using Prev
    fireEvent.click(getPrevBtn())
    expect(await screen.findByText('INSTRUMENT-11')).toBeInTheDocument()
    expect(getNav()).toHaveTextContent('Showing 11–20 of 25 items')
    expect(getNav()).toHaveTextContent('Page 2 of 3')

    // Verify spy call count and parameters
    expect(spy).toHaveBeenCalledWith('acc-page-test', 'open_positions', 1, 10)
    expect(spy).toHaveBeenCalledWith('acc-page-test', 'open_positions', 2, 10)
    expect(spy).toHaveBeenCalledWith('acc-page-test', 'open_positions', 3, 10)
  })
it('does not reload sub-tab component or column headings on clicking Next or Prev, only reloading table rows', async () => {
    const getMockResponse = (page: number): api.AccountOrdersResponse => ({
      summary: {
        netQty: 100,
        openPositionsCount: 20,
        closedPositionsCount: 0,
        pendingOrdersCount: 0,
        totalMtm: 1000,
        realizedPnl: 0,
        accountValue: 50000,
        status: 'online',
      },
      counts: {
        openPositions: 20,
        closedPositions: 0,
        holdings: 0,
        openOrders: 0,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'open_positions',
        page,
        limit: 10,
        totalCount: 20,
        totalPages: 2,
      },
      openPositions: Array.from({ length: 10 }, (_, i) => ({
        product: 'CNC',
        instrument: `ROW-${(page - 1) * 10 + i + 1}`,
        qty: 10,
        avgPrice: '100.00',
        ltp: '105.00',
        mtm: '50.00',
        action: 'exit',
      })),
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    vi.spyOn(api, 'getAccountOrders').mockImplementation(async (_id, _tab, page = 1) => {
      return getMockResponse(page)
    })

    render(() => <AccountOrderDetails accountId="acc-preserve-test" />)

    // Page 1 initial load
    expect(await screen.findByText('ROW-1')).toBeInTheDocument()

    // Grab reference to table thead, product header, and pagination navigator
    const thead = screen.getByRole('columnheader', { name: 'Product' }).closest('thead')
    const productHeader = screen.getByRole('columnheader', { name: 'Product' })
    const nav = screen.getByTestId('order-pagination-navigator')

    expect(thead).toBeInTheDocument()
    expect(productHeader).toBeInTheDocument()
    expect(nav).toBeInTheDocument()

    // Click Next
    const nextBtn = screen.getByRole('button', { name: 'Next page' })
    fireEvent.click(nextBtn)

    // Verify Page 2 rows are loaded
    expect(await screen.findByText('ROW-11')).toBeInTheDocument()
    expect(screen.queryByText('ROW-1')).not.toBeInTheDocument()

    // Verify column headings and navigator DOM nodes were NOT reloaded/remounted
    expect(thead?.isConnected).toBe(true)
    expect(productHeader.isConnected).toBe(true)
    expect(screen.getByRole('columnheader', { name: 'Product' })).toBe(productHeader)

    expect(nav.isConnected).toBe(true)
    expect(screen.getByTestId('order-pagination-navigator')).toBe(nav)

    // Click Prev
    const prevBtn = screen.getByRole('button', { name: 'Previous page' })
    fireEvent.click(prevBtn)

    // Verify Page 1 rows are loaded
    expect(await screen.findByText('ROW-1')).toBeInTheDocument()
    expect(screen.queryByText('ROW-11')).not.toBeInTheDocument()

    // Verify column headings and navigator DOM nodes are STILL the exact same nodes
    expect(thead?.isConnected).toBe(true)
    expect(productHeader.isConnected).toBe(true)
    expect(screen.getByRole('columnheader', { name: 'Product' })).toBe(productHeader)

    expect(nav.isConnected).toBe(true)
    expect(screen.getByTestId('order-pagination-navigator')).toBe(nav)
  })

  it('keeps column headings and navigator static without unmounting while in-flight and after resolution', async () => {
    let resolvePromise: (val: any) => void
    const mockData = (page: number) => ({
      summary: { netQty: 0, openPositionsCount: 20, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' as const },
      counts: { openPositions: 20, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page, limit: 10, totalCount: 20, totalPages: 2 },
      openPositions: [{ product: 'CNC', instrument: `POS-${page}`, qty: 1, avgPrice: '10', ltp: '12', mtm: '2', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    let callCount = 0
    vi.spyOn(api, 'getAccountOrders').mockImplementation(() => {
      callCount++
      if (callCount === 1) return Promise.resolve(mockData(1))
      return new Promise((resolve) => { resolvePromise = resolve })
    })

    render(() => <AccountOrderDetails accountId="acc-static-header" />)
    expect(await screen.findByText('POS-1')).toBeInTheDocument()

    const thead = screen.getByRole('columnheader', { name: 'Product' }).closest('thead')
    const headerCell = screen.getByRole('columnheader', { name: 'Product' })
    const tableContainer = thead?.closest('.order-table-container')

    expect(thead?.isConnected).toBe(true)
    expect(headerCell.isConnected).toBe(true)
    expect(tableContainer).not.toHaveClass('is-fetching')

    // Click Next
    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))

    // While in-flight: table has is-fetching, but thead and headers remain mounted
    expect(tableContainer).toHaveClass('is-fetching')
    expect(thead?.isConnected).toBe(true)
    expect(headerCell.isConnected).toBe(true)
    expect(screen.getByRole('columnheader', { name: 'Product' })).toBe(headerCell)

    // Resolve second page
    resolvePromise!(mockData(2))
    expect(await screen.findByText('POS-2')).toBeInTheDocument()
    expect(screen.queryByText('POS-1')).not.toBeInTheDocument()

    // After resolve: is-fetching is removed, thead and headers are the exact same DOM nodes
    expect(tableContainer).not.toHaveClass('is-fetching')
    expect(thead?.isConnected).toBe(true)
    expect(headerCell.isConnected).toBe(true)
    expect(screen.getByRole('columnheader', { name: 'Product' })).toBe(headerCell)
  })
it('locks column header widths with fixed table layout so headers do not shift across pages of varying content sizes', async () => {
    // Page 1: 10 items with long instrument names
    const page1Holdings: api.HoldingItem[] = Array.from({ length: 10 }, (_, i) => ({
      instrument: `VERY-LONG-HOLDING-INSTRUMENT-NAME-${i + 1}`,
      sellableQuantity: 1000,
      buyAveragePrice: '1234.56',
      ltp: '2345.67',
      pnl: '111111.00',
      action: 'exit',
    }))

    // Page 2: 1 single item with short instrument name
    const page2Holdings: api.HoldingItem[] = [
      {
        instrument: 'IEX',
        sellableQuantity: 1,
        buyAveragePrice: '10.00',
        ltp: '12.00',
        pnl: '2.00',
        action: 'exit',
      },
    ]

    const getMockResponse = (page: number): api.AccountOrdersResponse => ({
      summary: {
        netQty: 100,
        openPositionsCount: 0,
        closedPositionsCount: 0,
        pendingOrdersCount: 0,
        totalMtm: 0,
        realizedPnl: 0,
        accountValue: 100000,
        status: 'online',
      },
      counts: {
        openPositions: 0,
        closedPositions: 0,
        holdings: 11,
        openOrders: 0,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'holdings',
        page,
        limit: 10,
        totalCount: 11,
        totalPages: 2,
      },
      openPositions: [],
      closedPositions: [],
      holdings: page === 1 ? page1Holdings : page2Holdings,
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    vi.spyOn(api, 'getAccountOrders').mockImplementation(async (_id, _tab, page = 1) => {
      return getMockResponse(page)
    })

    render(() => <AccountOrderDetails accountId="acc-fixed-header" />)

    // Switch to Holding tab
    fireEvent.click(await screen.findByText('Holding (11)'))
    expect(await screen.findByText('VERY-LONG-HOLDING-INSTRUMENT-NAME-1')).toBeInTheDocument()

    // Grab all column header elements and their styles
    const headers = screen.getAllByRole('columnheader')
    expect(headers).toHaveLength(6)

    // Snapshot the width style of each header on Page 1
    const page1Widths = headers.map((th) => (th as HTMLElement).style.width)
    expect(page1Widths).toEqual(['28%', '16%', '16%', '15%', '17%', '8%'])

    // Table has fixed layout class
    const table = headers[0].closest('table')
    expect(table).toHaveClass('order-subtable')

    // Navigate to Page 2
    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    expect(await screen.findByText('IEX')).toBeInTheDocument()
    expect(screen.queryByText('VERY-LONG-HOLDING-INSTRUMENT-NAME-1')).not.toBeInTheDocument()

    // Verify all header elements retained the EXACT same fixed widths on Page 2
    const page2Headers = screen.getAllByRole('columnheader')
    const page2Widths = page2Headers.map((th) => (th as HTMLElement).style.width)
    expect(page2Widths).toEqual(page1Widths)

    // Navigate back to Page 1
    fireEvent.click(screen.getByRole('button', { name: 'Previous page' }))
    expect(await screen.findByText('VERY-LONG-HOLDING-INSTRUMENT-NAME-1')).toBeInTheDocument()

    const finalHeaders = screen.getAllByRole('columnheader')
    const finalWidths = finalHeaders.map((th) => (th as HTMLElement).style.width)
    expect(finalWidths).toEqual(page1Widths)
  })

  it('keeps column headers visible when tab is empty and renders EmptyState inside table body', async () => {
    const mockData: api.AccountOrdersResponse = {
      summary: {
        netQty: 0,
        openPositionsCount: 0,
        closedPositionsCount: 0,
        pendingOrdersCount: 0,
        totalMtm: 0,
        realizedPnl: 0,
        accountValue: 0,
        status: 'online',
      },
      counts: {
        openPositions: 0,
        closedPositions: 0,
        holdings: 0,
        openOrders: 0,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'open_orders',
        page: 1,
        limit: 10,
        totalCount: 0,
        totalPages: 1,
      },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockResolvedValue(mockData)

    render(() => <AccountOrderDetails accountId="acc-empty-test" />)

    // Switch to Open Order tab (0 items)
    fireEvent.click(await screen.findByText('Open Order (0)'))

    // Headers are still rendered!
    expect(screen.getByRole('columnheader', { name: 'Product' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Time' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Instrument' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Quantity' })).toBeInTheDocument()

    // And EmptyState is rendered in the body after data resolves
    expect(await screen.findByText('No data')).toBeInTheDocument()
    const emptyCell = screen.getByText('No data').closest('td')
    expect(emptyCell).toHaveAttribute('colspan', '8')
  })
  it('stabilizes sub-tab selection: keeps tab headers mounted without reloading and only switches active highlight and internal component', async () => {
    const mockData: api.AccountOrdersResponse = {
      summary: {
        netQty: 10,
        openPositionsCount: 1,
        closedPositionsCount: 1,
        pendingOrdersCount: 0,
        totalMtm: 100,
        realizedPnl: 0,
        accountValue: 10000,
        status: 'online',
      },
      counts: {
        openPositions: 1,
        closedPositions: 1,
        holdings: 1,
        openOrders: 0,
        closedOrders: 0,
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
          product: 'CNC',
          instrument: 'CRUDEOIL17SEP26C10600',
          qty: 10,
          avgPrice: '100.00',
          ltp: '110.00',
          mtm: '100.00',
        },
      ],
      closedPositions: [
        {
          product: 'CNC',
          instrument: 'CRUDEOIL17SEP26P8200',
          qty: 0,
          avgPrice: '20.00',
          ltp: '30.00',
          mtm: '50.00',
        },
      ],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockResolvedValue(mockData)

    render(() => <AccountOrderDetails accountId="acc-tab-stable-test" />)

    // Initial tab: Open Position should be active
    const openPosBtn = await screen.findByRole('tab', { name: /Open Position/i })
    const closedPosBtn = screen.getByRole('tab', { name: /Closed Position/i })
    const holdingBtn = screen.getByRole('tab', { name: /Holding/i })

    expect(openPosBtn).toHaveClass('order-tab-active')
    expect(openPosBtn).toHaveAttribute('aria-selected', 'true')
    expect(closedPosBtn).not.toHaveClass('order-tab-active')
    expect(closedPosBtn).toHaveAttribute('aria-selected', 'false')

    // Initial internal component: Open Position table
    expect(screen.getByText('CRUDEOIL17SEP26C10600')).toBeInTheDocument()

    // Capture tab DOM node references before click to verify they are NOT remounted/reloaded
    const originalOpenBtn = openPosBtn
    const originalClosedBtn = closedPosBtn
    const tabList = screen.getByRole('tablist')

    // Click Closed Position tab
    fireEvent.click(closedPosBtn)

    // Verify tab list and buttons are the exact same DOM nodes (never remounted or reloaded)
    expect(screen.getByRole('tablist')).toBe(tabList)
    expect(screen.getByRole('tab', { name: /Open Position/i })).toBe(originalOpenBtn)
    expect(screen.getByRole('tab', { name: /Closed Position/i })).toBe(originalClosedBtn)

    // Active state shifted cleanly: Closed Position is now highlighted, Open Position is unhighlighted
    expect(closedPosBtn).toHaveClass('order-tab-active')
    expect(closedPosBtn).toHaveAttribute('aria-selected', 'true')
    expect(openPosBtn).not.toHaveClass('order-tab-active')
    expect(openPosBtn).toHaveAttribute('aria-selected', 'false')

    // Only internal component switched to Closed Position
    expect(await screen.findByText('CRUDEOIL17SEP26P8200')).toBeInTheDocument()
    expect(screen.queryByText('CRUDEOIL17SEP26C10600')).not.toBeInTheDocument()

    // Click Holding tab
    fireEvent.click(holdingBtn)
    expect(holdingBtn).toHaveClass('order-tab-active')
    expect(closedPosBtn).not.toHaveClass('order-tab-active')
  })

  it('periodically reloads orders and positions data every 3s, but skips reloading when on holdings tab', async () => {
    vi.useFakeTimers()
    const getOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, openCount: 0, closedCount: 0, pendingMetric: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 1, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'NIFTY26SEP', qty: 50, avgPrice: '100', ltp: '105', mtm: '250', action: 'exit' }],
      closedPositions: [],
      holdings: [{ instrument: 'INFY', sellableQuantity: 10, buyAveragePrice: '1500', ltp: '1550', pnl: '500', action: 'exit' }],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    try {
      render(() => <AccountOrderDetails accountId="acc-timer" />)

      // Initial fetch for open_positions
      expect(getOrdersSpy).toHaveBeenCalledTimes(1)
      expect(getOrdersSpy).toHaveBeenLastCalledWith('acc-timer', 'open_positions', 1, 10)

      // Advance by 7000ms -> should refetch open_positions
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).toHaveBeenCalledTimes(2)

      // Switch to holdings tab
      const holdingBtn = screen.getByRole('tab', { name: /Holding/i })
      fireEvent.click(holdingBtn)

      // Switching tabs triggers resource fetch for holdings
      expect(getOrdersSpy).toHaveBeenCalledTimes(3)
      expect(getOrdersSpy).toHaveBeenLastCalledWith('acc-timer', 'holdings', 1, 10)

      // Advance by 7000ms and 14000ms -> should NOT refetch while on holdings tab
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).toHaveBeenCalledTimes(3)

      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).toHaveBeenCalledTimes(3)

      // Switch back to open_positions tab
      const openPosBtn = screen.getByRole('tab', { name: /Open Position/i })
      fireEvent.click(openPosBtn)
      expect(getOrdersSpy).toHaveBeenCalledTimes(4)

      // Advance by 7000ms -> should refetch again since it's not holdings
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).toHaveBeenCalledTimes(5)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not flicker or unmount table rows on periodic poll when incoming data is identical', async () => {
    vi.useFakeTimers()
    const sampleData = (): api.AccountOrdersResponse => ({
      summary: { netQty: 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 100, realizedPnl: 0, accountValue: 10000, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'TCS', qty: 10, avgPrice: '3500.00', ltp: '3550.00', mtm: '500.00', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    const getOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockImplementation(async () => sampleData())

    try {
      render(() => <AccountOrderDetails accountId="acc-diff-identical" />)
      expect(await screen.findByText('TCS')).toBeInTheDocument()

      const row = screen.getByText('TCS').closest('tr')
      const container = row?.closest('.order-table-container')
      expect(row).toBeInTheDocument()
      expect(container).not.toHaveClass('is-fetching')

      // Trigger 3 periodic reloads
      await vi.advanceTimersByTimeAsync(7000)
      await vi.advanceTimersByTimeAsync(7000)
      await vi.advanceTimersByTimeAsync(7000)

      expect(getOrdersSpy).toHaveBeenCalledTimes(4) // 1 initial + 3 polls

      // Verify row is still the exact same DOM node (not unmounted or recreated)
      expect(row?.isConnected).toBe(true)
      expect(screen.getByText('TCS').closest('tr')).toBe(row)

      // Verify no is-fetching artifacting
      expect(container).not.toHaveClass('is-fetching')
    } finally {
      vi.useRealTimers()
    }
  })

  it('updates row content in-place without unmounting table rows when incoming data changes', async () => {
    vi.useFakeTimers()
    let currentLtp = '3550.00'
    let currentMtm = '500.00'

    const getOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockImplementation(async () => ({
      summary: { netQty: 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: Number(currentMtm), realizedPnl: 0, accountValue: 10000, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'TCS', qty: 10, avgPrice: '3500.00', ltp: currentLtp, mtm: currentMtm, action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }))

    try {
      render(() => <AccountOrderDetails accountId="acc-diff-changed" />)
      expect(await screen.findByText('TCS')).toBeInTheDocument()
      expect(screen.getByText('3550.00')).toBeInTheDocument()

      const row = screen.getByText('TCS').closest('tr')
      const container = row?.closest('.order-table-container')

      // Data changes on broker
      currentLtp = '3600.00'
      currentMtm = '1000.00'

      // Advance by 7000ms
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).toHaveBeenCalledTimes(2)

      // Cell text updated to new LTP
      expect(screen.getByText('3600.00')).toBeInTheDocument()
      expect(screen.queryByText('3550.00')).not.toBeInTheDocument()

      // The tr DOM node is STILL the exact same node (reconcile and For kept the element)
      expect(row?.isConnected).toBe(true)
      expect(screen.getByText('TCS').closest('tr')).toBe(row)

      // Container did NOT flash with is-fetching during background poll
      expect(container).not.toHaveClass('is-fetching')
    } finally {
      vi.useRealTimers()
    }
  })

  it('preserves existing DOM row nodes and only mounts new row nodes when items are added (Point 5)', async () => {
    vi.useFakeTimers()
    let positionsList = [
      { product: 'CNC', instrument: 'TCS', qty: 10, avgPrice: '3500.00', ltp: '3550.00', mtm: '500.00', action: 'exit' },
    ]

    const getOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockImplementation(async () => ({
      summary: { netQty: 10, openPositionsCount: positionsList.length, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 500, realizedPnl: 0, accountValue: 10000, status: 'online' },
      counts: { openPositions: positionsList.length, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: positionsList.length, totalPages: 1 },
      openPositions: positionsList,
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }))

    try {
      render(() => <AccountOrderDetails accountId="acc-diff-add-row" />)
      expect(await screen.findByText('TCS')).toBeInTheDocument()

      const tcsRow = screen.getByText('TCS').closest('tr')
      expect(tcsRow).toBeInTheDocument()

      // Add a new position row
      positionsList = [
        { product: 'CNC', instrument: 'TCS', qty: 10, avgPrice: '3500.00', ltp: '3550.00', mtm: '500.00', action: 'exit' },
        { product: 'CNC', instrument: 'INFOSYS', qty: 5, avgPrice: '1500.00', ltp: '1520.00', mtm: '100.00', action: 'exit' },
      ]

      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).toHaveBeenCalledTimes(2)

      // Both items are in the document
      expect(screen.getByText('TCS')).toBeInTheDocument()
      expect(screen.getByText('INFOSYS')).toBeInTheDocument()

      // Existing TCS row DOM node was preserved (NOT remounted)
      expect(tcsRow?.isConnected).toBe(true)
      expect(screen.getByText('TCS').closest('tr')).toBe(tcsRow)

      // New INFOSYS row is mounted
      const infyRow = screen.getByText('INFOSYS').closest('tr')
      expect(infyRow).toBeInTheDocument()
      expect(infyRow).not.toBe(tcsRow)
    } finally {
      vi.useRealTimers()
    }
  })

  it('strictly isolates state between multiple concurrently mounted AccountOrderDetails instances', async () => {
    vi.useFakeTimers()
    const acc1Data: api.AccountOrdersResponse = {
      summary: { netQty: 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 500, realizedPnl: 0, accountValue: 100000, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'TCS', qty: 10, avgPrice: '3500.00', ltp: '3550.00', mtm: '500.00', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    const acc2Data: api.AccountOrdersResponse = {
      summary: { netQty: 50, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 250, realizedPnl: 0, accountValue: 50000, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'INFOSYS', qty: 50, avgPrice: '1500.00', ltp: '1520.00', mtm: '250.00', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockImplementation(async (accountId) => {
      if (accountId === 'acc-1') return JSON.parse(JSON.stringify(acc1Data))
      if (accountId === 'acc-2') return JSON.parse(JSON.stringify(acc2Data))
      return JSON.parse(JSON.stringify(acc1Data))
    })

    try {
      const { container } = render(() => (
        <div>
          <div data-testid="wrapper-acc-1">
            <AccountOrderDetails accountId="acc-1" accountName="Account 1" />
          </div>
          <div data-testid="wrapper-acc-2">
            <AccountOrderDetails accountId="acc-2" accountName="Account 2" />
          </div>
        </div>
      ))

      const wrap1 = container.querySelector('[data-testid="wrapper-acc-1"]')!
      const wrap2 = container.querySelector('[data-testid="wrapper-acc-2"]')!

      // Wait for initial render of Acc 1 and Acc 2
      expect(await screen.findByText('TCS')).toBeInTheDocument()
      expect(await screen.findByText('INFOSYS')).toBeInTheDocument()

      expect(wrap1.textContent).toContain('TCS')
      expect(wrap1.textContent).not.toContain('INFOSYS')
      expect(wrap2.textContent).toContain('INFOSYS')
      expect(wrap2.textContent).not.toContain('TCS')

      // Acc 1 should show Net Qty 10
      expect(wrap1.querySelector('.summary-metric-item')?.textContent).toContain('10')
      // Acc 2 should show Net Qty 50
      expect(wrap2.querySelector('.summary-metric-item')?.textContent).toContain('50')

      // Now update Acc 1's data on subsequent fetch
      acc1Data.openPositions = [{ product: 'CNC', instrument: 'TCS-NEW', qty: 20, avgPrice: '3600.00', ltp: '3650.00', mtm: '1000.00', action: 'exit' }]
      acc1Data.summary.netQty = 20

      await vi.advanceTimersByTimeAsync(7000)

      // Acc 1 should update to TCS-NEW
      expect(wrap1.textContent).toContain('TCS-NEW')

      // Acc 2 MUST NOT be affected or overwritten by Acc 1's data
      expect(wrap2.textContent).toContain('INFOSYS')
      expect(wrap2.textContent).not.toContain('TCS-NEW')
      expect(wrap2.textContent).not.toContain('TCS')
      expect(wrap2.querySelector('.summary-metric-item')?.textContent).toContain('50')
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not wipe other tabs cached data when fetching a single tab', async () => {
    const openPosData: api.AccountOrdersResponse = {
      summary: { netQty: 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 500, realizedPnl: 0, accountValue: 100000, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 1, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'TCS-PERSIST', qty: 10, avgPrice: '3500.00', ltp: '3550.00', mtm: '500.00', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    const holdingsData: api.AccountOrdersResponse = {
      summary: { netQty: 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 500, realizedPnl: 0, accountValue: 100000, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 1, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'holdings', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [],
      closedPositions: [],
      holdings: [{ instrument: 'RELIANCE-HOLD', sellableQuantity: 15, buyAveragePrice: '2500.00', ltp: '2600.00', pnl: '1500.00', action: 'exit' }],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockImplementation(async (_accId, tab) => {
      if (tab === 'holdings') return holdingsData
      return openPosData
    })

    const { getByRole, findByText, getByText } = render(() => <AccountOrderDetails accountId="acc-tab-persist" />)
    expect(await findByText('TCS-PERSIST')).toBeInTheDocument()

    // Switch to holdings: fetch happens for holdings
    fireEvent.click(getByRole('tab', { name: /Holding/i }))
    expect(await findByText('RELIANCE-HOLD')).toBeInTheDocument()

    // Switch back to open positions: TCS-PERSIST should still be visible without blanking
    fireEvent.click(getByRole('tab', { name: /Open Position/i }))
    expect(getByText('TCS-PERSIST')).toBeInTheDocument()
  })

  it('refetches data when clicking on the currently active tab', async () => {
    let callCount = 0
    vi.spyOn(api, 'getAccountOrders').mockImplementation(async () => {
      callCount++
      return {
        accountId: 'acc-refetch-active',
        summary: { netQty: callCount * 10, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
        counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
        pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
        openPositions: [{ product: 'CNC', instrument: `INST-${callCount}`, qty: 10, avgPrice: '100', ltp: '100', mtm: '0', action: 'exit' }],
        closedPositions: [],
        holdings: [],
        openOrders: [],
        closedOrders: [],
        rejectedOrders: [],
      }
    })

    const { getByRole, findByText } = render(() => <AccountOrderDetails accountId="acc-refetch-active" />)
    expect(await findByText('INST-1')).toBeInTheDocument()
    expect(callCount).toBe(1)

    // Click on the same active tab: Open Position
    fireEvent.click(getByRole('tab', { name: /Open Position/i }))

    // Must trigger a refetch and display updated data INST-2
    expect(await findByText('INST-2')).toBeInTheDocument()
    expect(callCount).toBe(2)
  })

  it('discards responses intended for a different account ID', async () => {
    const wrongAccountData: api.AccountOrdersResponse = {
      accountId: 'DIFFERENT-ACCOUNT',
      summary: { netQty: 999, openPositionsCount: 1, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      counts: { openPositions: 1, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 1, totalPages: 1 },
      openPositions: [{ product: 'CNC', instrument: 'WRONG-ACCOUNT-POS', qty: 999, avgPrice: '100', ltp: '100', mtm: '0', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    vi.spyOn(api, 'getAccountOrders').mockResolvedValue(wrongAccountData)

    const { queryByText } = render(() => <AccountOrderDetails accountId="CORRECT-ACCOUNT" />)

    // Wait a tick for async resolution
    await new Promise((r) => setTimeout(r, 50))

    // WRONG-ACCOUNT-POS must NOT be rendered because account ID doesn't match!
    expect(queryByText('WRONG-ACCOUNT-POS')).not.toBeInTheDocument()
  })

  it('does not flash "No data" empty state while data is loading for a newly selected tab', async () => {
    let resolveTabPromise: (data: api.AccountOrdersResponse) => void
    const initialData: api.AccountOrdersResponse = {
      accountId: 'acc-empty-flash-test',
      summary: {
        netQty: 10,
        openPositionsCount: 1,
        closedPositionsCount: 0,
        pendingOrdersCount: 0,
        totalMtm: 100,
        realizedPnl: 0,
        accountValue: 10000,
        status: 'online',
      },
      counts: {
        openPositions: 1,
        closedPositions: 0,
        holdings: 0,
        openOrders: 0,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      openPositions: [{ product: 'MIS', instrument: 'NIFTY-INITIAL', qty: 50, avgPrice: '100', ltp: '105', mtm: '250', action: 'exit' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    }

    let callCount = 0
    vi.spyOn(api, 'getAccountOrders').mockImplementation(() => {
      callCount++
      if (callCount === 1) return Promise.resolve(initialData)
      return new Promise((resolve) => {
        resolveTabPromise = resolve
      })
    })

    render(() => <AccountOrderDetails accountId="acc-empty-flash-test" />)

    // Wait for initial load
    expect(await screen.findByText('NIFTY-INITIAL')).toBeInTheDocument()

    // Switch to Closed Position tab (which currently has 0 items and pending fetch)
    fireEvent.click(screen.getByRole('tab', { name: /Closed Position/i }))

    // While fetch is in flight, "No data" MUST NOT be rendered
    expect(screen.queryByText('No data')).not.toBeInTheDocument()

    // Resolve fetch with 0 closed positions
    resolveTabPromise!({
      ...initialData,
      pagination: { tab: 'closed_positions', page: 1, limit: 10, totalCount: 0, totalPages: 1 },
      closedPositions: [],
    })

    // Now that fetch completed, "No data" should be displayed
    expect(await screen.findByText('No data')).toBeInTheDocument()
  })

  it('does not unmount or flicker the "No data" empty state during periodic poll on empty tabs', async () => {
    vi.useFakeTimers()
    const emptyData = (): api.AccountOrdersResponse => ({
      accountId: 'acc-empty-poll-test',
      summary: {
        netQty: 0,
        openPositionsCount: 0,
        closedPositionsCount: 0,
        pendingOrdersCount: 0,
        totalMtm: 0,
        realizedPnl: 0,
        accountValue: 10000,
        status: 'online',
      },
      counts: {
        openPositions: 0,
        closedPositions: 0,
        holdings: 0,
        openOrders: 0,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 0, totalPages: 1 },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    const getOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockImplementation(async () => emptyData())

    try {
      render(() => <AccountOrderDetails accountId="acc-empty-poll-test" />)
      expect(await screen.findByText('No data')).toBeInTheDocument()

      const emptyEl = screen.getByText('No data')
      expect(emptyEl.isConnected).toBe(true)

      // Trigger 3 periodic reloads
      await vi.advanceTimersByTimeAsync(7000)
      await vi.advanceTimersByTimeAsync(7000)
      await vi.advanceTimersByTimeAsync(7000)

      expect(getOrdersSpy).toHaveBeenCalledTimes(4)

      // Verify the empty state node was NEVER destroyed or recreated
      expect(emptyEl.isConnected).toBe(true)
      expect(screen.getByText('No data')).toBe(emptyEl)
    } finally {
      vi.useRealTimers()
    }
  })
});



