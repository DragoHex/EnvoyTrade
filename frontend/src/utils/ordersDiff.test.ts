import { describe, it, expect } from 'vitest'
import { isOrdersDataEqual } from './ordersDiff'
import type { AccountOrdersResponse } from '../api'

function createSampleResponse(): AccountOrdersResponse {
  return {
    summary: {
      netQty: 100,
      totalMtm: 1500.5,
      realizedPnl: 500,
      accountValue: 100000,
      status: 'active',
      openPositionsCount: 1,
      closedPositionsCount: 1,
      pendingOrdersCount: 1,
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
      page: 1,
      limit: 10,
      totalCount: 1,
      totalPages: 1,
      tab: 'open_positions',
    },
    openPositions: [
      {
        instrument: 'NIFTY24OCTFUT',
        product: 'NRML',
        qty: 100,
        avgPrice: '24500.00',
        ltp: 24515,
        mtm: 1500,
        action: 'BUY',
      },
    ],
    closedPositions: [
      {
        instrument: 'BANKNIFTY24OCTFUT',
        product: 'MIS',
        qty: 0,
        avgPrice: '51200.00',
        ltp: 51200,
        mtm: 500,
        action: 'SELL',
      },
    ],
    holdings: [
      {
        instrument: 'RELIANCE',
        sellableQuantity: 50,
        buyAveragePrice: 2800,
        ltp: 2900,
        pnl: 5000,
        action: 'HOLD',
      },
    ],
    openOrders: [
      {
        id: 'ord-1',
        instrument: 'TCS',
        product: 'CNC',
        quantity: 10,
        type: 'LIMIT',
        triggerPrice: 0,
        limitPrice: 4200,
        time: '10:00:00',
        status: 'OPEN',
      },
    ],
    closedOrders: [
      {
        id: 'ord-2',
        instrument: 'INFY',
        product: 'MIS',
        quantity: 25,
        type: 'MARKET',
        price: 1850,
        time: '09:30:00',
        status: 'COMPLETE',
      },
    ],
    rejectedOrders: [
      {
        id: 'ord-3',
        instrument: 'HDFCBANK',
        product: 'NRML',
        quantity: 50,
        type: 'LIMIT',
        time: '09:15:00',
        status: 'REJECTED',
        reason: 'Insufficient funds',
      },
    ],
  }
}

describe('isOrdersDataEqual', () => {
  it('returns true when references are identical', () => {
    const data = createSampleResponse()
    expect(isOrdersDataEqual(data, data, 'open_positions')).toBe(true)
    expect(isOrdersDataEqual(undefined, undefined, 'open_positions')).toBe(true)
  })

  it('returns false when one is undefined', () => {
    const data = createSampleResponse()
    expect(isOrdersDataEqual(data, undefined, 'open_positions')).toBe(false)
    expect(isOrdersDataEqual(undefined, data, 'open_positions')).toBe(false)
  })

  it('returns true for deeply equal new object clone', () => {
    const prev = createSampleResponse()
    const next = JSON.parse(JSON.stringify(prev)) as AccountOrdersResponse
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(true)
    expect(isOrdersDataEqual(prev, next, 'closed_positions')).toBe(true)
    expect(isOrdersDataEqual(prev, next, 'holdings')).toBe(true)
    expect(isOrdersDataEqual(prev, next, 'open_orders')).toBe(true)
    expect(isOrdersDataEqual(prev, next, 'closed_orders')).toBe(true)
    expect(isOrdersDataEqual(prev, next, 'rejected_orders')).toBe(true)
  })

  it('detects changes in summary metrics', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.summary.totalMtm = 2000
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)

    next.summary.totalMtm = prev.summary.totalMtm
    next.summary.status = 'error'
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)

    next.summary.status = prev.summary.status
    next.summary.netQty = 150
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)
  })

  it('detects changes in tab counts', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.counts!.rejectedOrders = 5
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)
  })

  it('detects changes in pagination', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.pagination!.page = 2
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)

    next.pagination!.page = 1
    next.pagination!.totalCount = 15
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)
  })

  it('detects row changes in open_positions', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.openPositions[0].ltp = 24600
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(false)

    // Should ignore closed_positions changes when activeTab is open_positions
    next.openPositions[0].ltp = prev.openPositions[0].ltp
    next.closedPositions[0].ltp = 51500
    expect(isOrdersDataEqual(prev, next, 'open_positions')).toBe(true)
  })

  it('detects row changes in closed_positions', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.closedPositions[0].mtm = 600
    expect(isOrdersDataEqual(prev, next, 'closed_positions')).toBe(false)
  })

  it('detects row changes in holdings', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.holdings[0].ltp = 2950
    expect(isOrdersDataEqual(prev, next, 'holdings')).toBe(false)
  })

  it('detects row changes in open_orders', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.openOrders[0].limitPrice = 4250
    expect(isOrdersDataEqual(prev, next, 'open_orders')).toBe(false)
  })

  it('detects row changes in closed_orders', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.closedOrders[0].price = 1860
    expect(isOrdersDataEqual(prev, next, 'closed_orders')).toBe(false)
  })

  it('detects row changes in rejected_orders', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.rejectedOrders[0].reason = 'RMS margin breach'
    expect(isOrdersDataEqual(prev, next, 'rejected_orders')).toBe(false)
  })

  it('detects row length differences', () => {
    const prev = createSampleResponse()
    const next = createSampleResponse()

    next.rejectedOrders = []
    expect(isOrdersDataEqual(prev, next, 'rejected_orders')).toBe(false)
  })
})
