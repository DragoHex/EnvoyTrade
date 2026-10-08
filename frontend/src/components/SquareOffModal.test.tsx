import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { SquareOffModal, type SquareOffModalTarget } from './SquareOffModal'
import * as api from '../api'

describe('SquareOffModal', () => {
  const groupTarget: SquareOffModalTarget = {
    type: 'group',
    id: 'g1',
    name: 'Group 1',
    masterId: 'm1',
  }

  const accountTarget: SquareOffModalTarget = {
    type: 'account',
    id: 'f1',
    name: 'Follower 1',
    brokerAccountId: 'FOLLOW01A',
  }

  it('does not render when open is false', () => {
    render(() => (
      <SquareOffModal open={false} target={groupTarget} onCancel={() => {}} />
    ))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders cluster info tooltip when target is group', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    render(() => (
      <SquareOffModal open={true} target={groupTarget} onCancel={() => {}} />
    ))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Group: Group 1')).toBeInTheDocument()
    const infoTrigger = screen.getByLabelText('Info')
    expect(infoTrigger).toHaveClass('tooltip-danger')
    expect(infoTrigger).toHaveAttribute(
      'data-tooltip',
      expect.stringContaining('Master square-off will cascade to all active followers')
    )
  })

  it('renders isolation info tooltip when target is follower account', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    render(() => (
      <SquareOffModal open={true} target={accountTarget} onCancel={() => {}} />
    ))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Follower: Follower 1')).toBeInTheDocument()
    const infoTrigger = screen.getByLabelText('Info')
    expect(infoTrigger).toHaveClass('tooltip-warning')
    expect(infoTrigger).toHaveAttribute(
      'data-tooltip',
      expect.stringContaining('Only this follower account will be squared off')
    )
  })

  it('renders open positions checklist and supports selective symbol square off', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 175, totalMtm: 500, realizedPnl: 0, accountValue: 100000, status: 'online' },
      openPositions: [
        { product: 'NRML', instrument: 'NIFTY26OCTFUT', qty: 75, avgPrice: '25000', ltp: '25100', mtm: '7500' },
        { product: 'CNC', instrument: 'RELIANCE', qty: 10, avgPrice: '2400', ltp: '2450', mtm: '500' },
      ],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    const onConfirm = vi.fn().mockResolvedValue(undefined)

    render(() => (
      <SquareOffModal
        open={true}
        target={accountTarget}
        onConfirm={onConfirm}
        onCancel={() => {}}
      />
    ))

    expect(await screen.findByText('NIFTY26OCTFUT')).toBeInTheDocument()
    expect(screen.getByText('RELIANCE')).toBeInTheDocument()
    expect(screen.getByText('2 of 2 selected')).toBeInTheDocument()

    // Deselect RELIANCE by unchecking its checkbox
    const relianceCheckbox = screen.getByLabelText('Select RELIANCE')
    await userEvent.click(relianceCheckbox)

    expect(screen.getByText('1 of 2 selected')).toBeInTheDocument()

    // Click Sq-off button
    await userEvent.click(screen.getByText('Sq-off'))
    expect(onConfirm).toHaveBeenCalledWith(['NIFTY26OCTFUT'])
  })

  it('toggles Select All and Deselect All via table header checkbox', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 75, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [
        { product: 'NRML', instrument: 'NIFTY26OCTFUT', qty: 75, avgPrice: '25000', ltp: '25100', mtm: '0' },
      ],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    render(() => (
      <SquareOffModal open={true} target={accountTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByText('NIFTY26OCTFUT')).toBeInTheDocument()
    const headerCheckbox = screen.getByLabelText('Select All Symbols')
    expect(screen.getByText('1 of 1 selected')).toBeInTheDocument()

    // Uncheck all
    await userEvent.click(headerCheckbox)
    expect(screen.getByText('0 of 1 selected')).toBeInTheDocument()

    // Check all again
    await userEvent.click(headerCheckbox)
    expect(screen.getByText('1 of 1 selected')).toBeInTheDocument()
  })

  it('calls onCancel when Cancel button is clicked', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const onCancel = vi.fn()

    render(() => (
      <SquareOffModal open={true} target={accountTarget} onCancel={onCancel} />
    ))

    await userEvent.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })

  it('closes when Escape key is pressed', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const onCancel = vi.fn()

    render(() => (
      <SquareOffModal open={true} target={accountTarget} onCancel={onCancel} />
    ))

    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.keyboard('{Escape}')
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('does not close on Escape key when submission is in flight', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 10, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [{ product: 'MIS', instrument: 'INFY', qty: 10, avgPrice: '1500', ltp: '1510', mtm: '100' }],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    let resolveSubmit!: () => void
    const pendingSubmit = new Promise<void>((res) => {
      resolveSubmit = res
    })
    const onConfirm = vi.fn().mockReturnValue(pendingSubmit)
    const onCancel = vi.fn()

    render(() => (
      <SquareOffModal open={true} target={accountTarget} onConfirm={onConfirm} onCancel={onCancel} />
    ))

    const confirmBtn = await screen.findByRole('button', { name: /Sq-off/i })
    await userEvent.click(confirmBtn)
    expect(onConfirm).toHaveBeenCalled()

    // While submitting, Escape key should be ignored
    await userEvent.keyboard('{Escape}')
    expect(onCancel).not.toHaveBeenCalled()

    resolveSubmit()
  })
})

