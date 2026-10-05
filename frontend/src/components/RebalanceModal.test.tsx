import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { RebalanceModal, type RebalanceModalTarget } from './RebalanceModal'
import * as api from '../api'

describe('RebalanceModal', () => {
  const groupTarget: RebalanceModalTarget = {
    type: 'group',
    id: 'g1',
    name: 'Master Alpha Group',
  }

  const accountTarget: RebalanceModalTarget = {
    type: 'account',
    id: 'f1',
    name: 'Follower Beta',
    brokerAccountId: 'FOLLOW01A',
  }

  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('does not render when open is false', () => {
    render(() => (
      <RebalanceModal open={false} target={groupTarget} onCancel={() => {}} />
    ))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders equilibrium state when no drift exists in group', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 2,
      followers_with_drift: 0,
      drifts: [
        {
          account_id: 'f1',
          account_name: 'Follower 1',
          broker_account_id: 'FOLLOW01A',
          enabled: true,
          clone_factor: '1',
          symbols: [],
        },
      ],
    })

    render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByTestId('rebalance-equilibrium')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 3 })).toHaveTextContent('Rebalance')
    expect(screen.queryByText('Portfolio Rebalance')).not.toBeInTheDocument()
    const infoTrigger = screen.getByLabelText('Info')
    expect(infoTrigger).toHaveClass('tooltip-accent')
    expect(infoTrigger).toHaveAttribute('data-tooltip', expect.stringContaining("Compares active followers' open positions"))
    expect(screen.getByText('All positions are in equilibrium.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /In Equilibrium/i })).toBeDisabled()
  })

  it('renders drifting followers checklist and accordion symbols breakdown in group mode', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 2,
      followers_with_drift: 2,
      drifts: [
        {
          account_id: 'f1',
          account_name: 'Follower 1',
          broker_account_id: 'FOLLOW01A',
          enabled: true,
          clone_factor: '1',
          symbols: [
            {
              exchange: 'NFO',
              tradingsymbol: 'NIFTY26OCTFUT',
              product: 'NRML',
              lot_size: 75,
              master_qty: 75,
              target_qty: 75,
              follower_qty: 0,
              drift_qty: 75,
              action: 'BUY',
            },
          ],
        },
        {
          account_id: 'f2',
          account_name: 'Follower 2',
          broker_account_id: 'FOLLOW01B',
          enabled: true,
          clone_factor: '1',
          symbols: [
            {
              exchange: 'NSE',
              tradingsymbol: 'RELIANCE',
              product: 'CNC',
              lot_size: 1,
              master_qty: 0,
              target_qty: 0,
              follower_qty: 10,
              drift_qty: -10,
              action: 'SELL',
            },
          ],
        },
      ],
    })

    const onConfirm = vi.fn().mockResolvedValue(undefined)

    render(() => (
      <RebalanceModal
        open={true}
        target={groupTarget}
        onConfirm={onConfirm}
        onCancel={() => {}}
      />
    ))

    expect(await screen.findByText('Follower 1')).toBeInTheDocument()
    expect(screen.getByText('Follower 2')).toBeInTheDocument()
    expect(screen.getByText('2 of 2 selected')).toBeInTheDocument()
    // All dropdowns are folded by default
    expect(screen.queryByText('NIFTY26OCTFUT')).not.toBeInTheDocument()

    // Expand Follower 1 accordion
    const expandButtons = screen.getAllByLabelText('Expand symbols')
    await userEvent.click(expandButtons[0])

    // Now Follower 1 symbol table is visible
    expect(screen.getByText('NIFTY26OCTFUT')).toBeInTheDocument()
    expect(screen.getByText('BUY')).toBeInTheDocument()

    // Deselect Follower 2 by clicking its checkbox
    const f2Checkbox = screen.getByLabelText('Select Follower 2')
    await userEvent.click(f2Checkbox)

    expect(screen.getByText('1 of 2 selected')).toBeInTheDocument()
    expect(screen.getByText('Rebalance (1)')).toBeInTheDocument()

    // Click confirm rebalance
    await userEvent.click(screen.getByText('Rebalance (1)'))
    expect(onConfirm).toHaveBeenCalledWith(['f1'])
  })

  it('supports Select All toggle in group mode', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 2,
      followers_with_drift: 1,
      drifts: [
        {
          account_id: 'f1',
          account_name: 'Follower 1',
          broker_account_id: 'FOLLOW01A',
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
        },
      ],
    })

    render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByText('Follower 1')).toBeInTheDocument()
    const selectAllCheckbox = screen.getByLabelText('Select All Followers')
    expect(screen.getByText('1 of 1 selected')).toBeInTheDocument()

    // Uncheck all
    await userEvent.click(selectAllCheckbox)
    expect(screen.getByText('0 of 1 selected')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Rebalance \(0\)/i })).toBeDisabled()

    // Check all again
    await userEvent.click(selectAllCheckbox)
    expect(screen.getByText('1 of 1 selected')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Rebalance \(1\)/i })).not.toBeDisabled()
  })

  it('renders single follower symbols table in account mode and executes rebalance', async () => {
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower 1',
      broker_account_id: 'FOLLOW01A',
      enabled: true,
      clone_factor: '1',
      symbols: [
        {
          exchange: 'NSE',
          tradingsymbol: 'RELIANCE',
          product: 'CNC',
          lot_size: 1,
          master_qty: 0,
          target_qty: 0,
          follower_qty: 25,
          drift_qty: -25,
          action: 'SELL',
        },
      ],
    })

    const onConfirm = vi.fn().mockResolvedValue(undefined)

    render(() => (
      <RebalanceModal
        open={true}
        target={accountTarget}
        onConfirm={onConfirm}
        onCancel={() => {}}
      />
    ))

    expect(await screen.findByText('RELIANCE')).toBeInTheDocument()
    expect(screen.getByText('SELL')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Rebalance$/i })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /^Rebalance$/i }))
    expect(onConfirm).toHaveBeenCalled()
  })

  it('calls onCancel when Cancel button is clicked', async () => {
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower 1',
      broker_account_id: 'FOLLOW01A',
      enabled: true,
      clone_factor: '1',
      symbols: [],
    })

    const onCancel = vi.fn()

    render(() => (
      <RebalanceModal open={true} target={accountTarget} onCancel={onCancel} />
    ))

    await userEvent.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })
})
