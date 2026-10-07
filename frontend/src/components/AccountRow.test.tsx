import { render, screen, fireEvent, within } from '@solidjs/testing-library'
import { createSignal } from 'solid-js'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AccountRow } from './AccountRow'
import * as api from '../api'
import type { GroupFollower } from '../api'

const follower: GroupFollower = {
  accountId: 'f1',
  name: 'Follower Account',
  brokerAccountId: 'ZY5678',
  enabled: true,
  status: 'ok',
}

describe('AccountRow', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower Account',
      broker_account_id: 'ZY5678',
      enabled: true,
      clone_factor: '1',
      symbols: [],
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
  })
  it('calls onToggleCopy when the CopyToggle changes', async () => {
    const onToggleCopy = vi.fn()
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={onToggleCopy}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    await userEvent.click(screen.getByRole('checkbox'))
    expect(onToggleCopy).toHaveBeenCalledWith(false)
  })

  it('does not render a Stop/Start button for a follower row', () => {
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.queryByLabelText('Stop Copy')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Start Copy')).not.toBeInTheDocument()
  })

  it('gates Square Off behind a confirm modal', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const onSquareOff = vi.fn()
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={onSquareOff}
        onExitOpenOrders={() => {}}
      />
    ))
    await userEvent.click(screen.getByLabelText('Square Off'))
    expect(onSquareOff).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.click(screen.getByText('Sq-off'))
    expect(onSquareOff).toHaveBeenCalled()
  })

  it('master row: loads positions using master accountId instead of targetGroupId for Square Off', async () => {
    const getAccountOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      openPositions: [
        { product: 'NRML', instrument: 'CRUDEOIL26OCT10200CE', qty: -3, avgPrice: '69.6', ltp: '76.4', mtm: '-3090' },
      ],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const masterFollower: GroupFollower = {
      accountId: 'master-acc-uuid-1234',
      name: 'Rohan Singhal',
      brokerAccountId: 'KU2675',
      enabled: true,
      status: 'ok',
    }
    render(() => (
      <AccountRow
        follower={masterFollower}
        isMaster
        targetGroupId="group-uuid-5678"
        targetGroupName="Test Group"
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    await userEvent.click(screen.getByLabelText('Square Off'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(getAccountOrdersSpy).toHaveBeenCalledWith('master-acc-uuid-1234', 'open_positions')
    expect(getAccountOrdersSpy).not.toHaveBeenCalledWith('group-uuid-5678', 'open_positions')
  })

  it('gates Exit Open Orders behind a confirm modal', async () => {
    const onExitOpenOrders = vi.fn()
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={onExitOpenOrders}
      />
    ))
    await userEvent.click(screen.getByLabelText('Exit Open Orders'))
    expect(onExitOpenOrders).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.click(screen.getByText('Confirm'))
    expect(onExitOpenOrders).toHaveBeenCalled()
  })

  it('master row: renders a Stop Copy button instead of a toggle and stops copy when active', async () => {
    const onToggleCopy = vi.fn()
    render(() => (
      <AccountRow
        follower={follower}
        isMaster
        onToggleCopy={onToggleCopy}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Stop Copy'))
    expect(onToggleCopy).toHaveBeenCalledWith(false)
  })

  it('master row: relabels to Start Copy and reactivates when already stopped', async () => {
    const onToggleCopy = vi.fn()
    render(() => (
      <AccountRow
        follower={{ ...follower, enabled: false }}
        isMaster
        onToggleCopy={onToggleCopy}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.queryByLabelText('Stop Copy')).not.toBeInTheDocument()
    await userEvent.click(screen.getByLabelText('Start Copy'))
    expect(onToggleCopy).toHaveBeenCalledWith(true)
  })

  it('gates Rebalance behind a confirm modal for follower row and executes on confirmation', async () => {
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower Account',
      broker_account_id: 'ZY5678',
      enabled: true,
      clone_factor: '1',
      symbols: [
        {
          exchange: 'NSE',
          tradingsymbol: 'RELIANCE',
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
    const onRebalance = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={onRebalance}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    const rebalanceButton = screen.getByLabelText('Rebalance')
    await userEvent.click(rebalanceButton)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Follower: Follower Account')).toBeInTheDocument()
    expect(await screen.findByText('RELIANCE')).toBeInTheDocument()

    const modal = within(screen.getByRole('dialog'))
    const confirmBtn = modal.getByRole('button', { name: /Rebalance/i })
    await userEvent.click(confirmBtn)
    expect(onRebalance).toHaveBeenCalled()
  })

  it('shows an error toast when Rebalance confirmation fails', async () => {
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower Account',
      broker_account_id: 'ZY5678',
      enabled: true,
      clone_factor: '1',
      symbols: [
        {
          exchange: 'NSE',
          tradingsymbol: 'RELIANCE',
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
    const onRebalance = vi.fn().mockRejectedValue(new Error('no master fill to rebalance from'))
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={onRebalance}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    await userEvent.click(screen.getByLabelText('Rebalance'))
    expect(await screen.findByText('RELIANCE')).toBeInTheDocument()

    const modal = within(screen.getByRole('dialog'))
    const confirmBtn = modal.getByRole('button', { name: /Rebalance/i })
    await userEvent.click(confirmBtn)
    expect(await screen.findByRole('status')).toHaveTextContent(/no master fill to rebalance from/i)
  })

  it('marks the row disabled and disables actions when actionsDisabled is true', () => {
    render(() => (
      <AccountRow
        follower={follower}
        actionsDisabled
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.getByTestId('account-row')).toHaveAttribute('data-disabled', 'true')
    expect(screen.getByLabelText('Rebalance')).toBeDisabled()
    expect(screen.getByLabelText('Square Off')).toBeDisabled()
    expect(screen.getByLabelText('Exit Open Orders')).toBeDisabled()
  })

  it('does not disable actions by default', () => {
    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.getByTestId('account-row')).toHaveAttribute('data-disabled', 'false')
    expect(screen.getByLabelText('Rebalance')).not.toBeDisabled()
  })

  it('disables the follower toggle when toggleDisabled, without disabling its own actions', () => {
    render(() => (
      <AccountRow
        follower={follower}
        toggleDisabled
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.getByRole('checkbox')).toBeDisabled()
    expect(screen.getByLabelText('Rebalance')).not.toBeDisabled()
  })

  it('renders the master Stop/Start control in the leading cell, same as the follower toggle', () => {
    render(() => (
      <AccountRow
        follower={follower}
        isMaster
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    const row = screen.getByTestId('master-row')
    const leadingCell = row.querySelectorAll('td')[0]
    expect(leadingCell.querySelector('[aria-label="Stop Copy"]')).not.toBeNull()
  })

  it('never disables the master Stop/Start button via actionsDisabled', () => {
    render(() => (
      <AccountRow
        follower={follower}
        isMaster
        actionsDisabled
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))
    expect(screen.getByLabelText('Stop Copy')).not.toBeDisabled()
    expect(screen.getByLabelText('Rebalance')).toBeDisabled()
  })

  it('expands details and loads live summary metrics on expand button click', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: {
        netQty: 150,
        openPositionsCount: 2,
        closedPositionsCount: 1,
        pendingOrdersCount: 3,
        totalMtm: '1250.50',
        realizedPnl: '500.00',
        accountValue: '250000.00',
        availableCash: '75000.00',
        availableMargin: '180000.00',
        status: 'online',
      },
      counts: {
        openPositions: 2,
        closedPositions: 1,
        holdings: 0,
        openOrders: 3,
        closedOrders: 0,
        rejectedOrders: 0,
      },
      pagination: {
        tab: 'open_positions',
        page: 1,
        limit: 10,
        totalCount: 2,
        totalPages: 1,
      },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    render(() => (
      <AccountRow
        follower={follower}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))

    const expandBtn = screen.getByTestId('expand-row-btn')
    await userEvent.click(expandBtn)

    expect(screen.getByTestId('account-details-expansion-row')).toBeInTheDocument()
  })

  it('polls orders only when expanded, not when folded', async () => {
    vi.useFakeTimers()
    const getOrdersSpy = vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: { netQty: 0, openPositionsCount: 0, closedPositionsCount: 0, pendingOrdersCount: 0, totalMtm: 0, realizedPnl: 0, accountValue: 0, status: 'online' },
      counts: { openPositions: 0, closedPositions: 0, holdings: 0, openOrders: 0, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 0, totalPages: 0 },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    try {
      render(() => (
        <AccountRow
          follower={follower}
          onToggleCopy={() => {}}
          onRebalance={() => Promise.resolve()}
          onSquareOff={() => {}}
          onExitOpenOrders={() => {}}
        />
      ))

      // 1. Folded by default: advancing timers should NOT trigger getAccountOrders
      await vi.advanceTimersByTimeAsync(7000)
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy).not.toHaveBeenCalled()

      // 2. Expand the row
      const expandBtn = screen.getByTestId('expand-row-btn')
      fireEvent.click(expandBtn)

      // Initial fetch on expand
      expect(getOrdersSpy).toHaveBeenCalled()
      const callsAfterExpand = getOrdersSpy.mock.calls.length

      // Advance by 7000ms -> polling occurs while expanded
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy.mock.calls.length).toBeGreaterThan(callsAfterExpand)

      // 3. Fold/collapse the row again
      fireEvent.click(expandBtn)
      const callsAfterFold = getOrdersSpy.mock.calls.length

      // Advance by 7000ms and 14000ms -> NO further calls while folded
      await vi.advanceTimersByTimeAsync(7000)
      await vi.advanceTimersByTimeAsync(7000)
      expect(getOrdersSpy.mock.calls.length).toBe(callsAfterFold)
    } finally {
      vi.useRealTimers()
    }
  })

  it('resets live summary when row is collapsed so it displays updated follower props', async () => {
    const initialFollower: GroupFollower = {
      accountId: 'f-fold',
      name: 'Fold Test Follower',
      brokerAccountId: 'FLD123',
      enabled: true,
      status: 'ok',
      netQty: 5,
      openPositionsCount: 1,
      closedPositionsCount: 0,
      openOrdersCount: 0,
      totalMtm: 100,
    }

    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: {
        netQty: 99,
        openPositionsCount: 5,
        closedPositionsCount: 2,
        pendingOrdersCount: 4,
        totalMtm: '999.00',
        realizedPnl: '100.00',
        accountValue: '50000.00',
        status: 'online',
      },
      counts: { openPositions: 5, closedPositions: 2, holdings: 0, openOrders: 4, closedOrders: 0, rejectedOrders: 0 },
      pagination: { tab: 'open_positions', page: 1, limit: 10, totalCount: 5, totalPages: 1 },
      openPositions: [],
      closedPositions: [],
      holdings: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    const [followerSig, setFollowerSig] = createSignal(initialFollower)

    render(() => (
      <AccountRow
        follower={followerSig()}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))

    // Initially folded: shows follower's netQty: 5
    const netQtyCell = document.querySelector('tr[data-testid="account-row"] .col-net-qty')!
    expect(netQtyCell.textContent).toBe('5')

    // Expand row
    const expandBtn = screen.getByTestId('expand-row-btn')
    fireEvent.click(expandBtn)

    // Wait for live metrics to load: Net Qty becomes 99
    await vi.waitFor(() => {
      expect(netQtyCell.textContent).toBe('99')
    })

    // Now collapse/fold the row
    fireEvent.click(expandBtn)

    // Update follower prop to simulate GroupDetail update while folded (e.g. netQty becomes 12)
    setFollowerSig({
      ...initialFollower,
      netQty: 12,
      totalMtm: 250,
    })

    // It should now revert to follower's updated netQty 12, NOT remain stuck on 99
    await vi.waitFor(() => {
      expect(netQtyCell.textContent).toBe('12')
    })
  })

  it('disables Rebalance and Square-off buttons when follower is disabled', () => {
    const disabledFollower: GroupFollower = {
      ...follower,
      enabled: false,
    }
    render(() => (
      <AccountRow
        follower={disabledFollower}
        onToggleCopy={() => {}}
        onRebalance={() => Promise.resolve()}
        onSquareOff={() => {}}
        onExitOpenOrders={() => {}}
      />
    ))

    const rebalanceBtn = screen.getByRole('button', { name: 'Rebalance' })
    expect(rebalanceBtn).toBeDisabled()
    expect(rebalanceBtn).toHaveAttribute('data-tooltip', 'Account is disabled')

    const sqOffBtn = screen.getByRole('button', { name: 'Square Off' })
    expect(sqOffBtn).toBeDisabled()
    expect(sqOffBtn).toHaveAttribute('data-tooltip', 'Account is disabled')
  })
})


