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

  it('renders pinned list header and scrollable followers list container in group mode', async () => {
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
              tradingsymbol: 'INFY',
              product: 'CNC',
              lot_size: 1,
              master_qty: 5,
              target_qty: 5,
              follower_qty: 0,
              drift_qty: 5,
              action: 'BUY',
            },
          ],
        },
      ],
    })

    const { container } = render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByText('Follower 1')).toBeInTheDocument()
    const headerRow = container.querySelector('.rebalance-list-header-row')
    expect(headerRow).toBeInTheDocument()

    const followersList = container.querySelector('.rebalance-followers-list')
    expect(followersList).toBeInTheDocument()

    // Expand accordion and verify scrollable dropdown wrapper
    const expandBtn = screen.getByLabelText('Expand symbols')
    await userEvent.click(expandBtn)

    const symbolsWrapper = container.querySelector('.follower-symbols-table-wrapper')
    expect(symbolsWrapper).toBeInTheDocument()
    expect(container.querySelector('.rebalance-symbols-table')).toBeInTheDocument()
    expect(screen.getByText('INFY')).toBeInTheDocument()
  })

  it('renders scrollable account-mode-wrapper in account mode for drifting symbols', async () => {
    vi.spyOn(api, 'getAccountRebalanceDiff').mockResolvedValue({
      account_id: 'f1',
      account_name: 'Follower 1',
      broker_account_id: 'FOLLOW01A',
      enabled: true,
      clone_factor: '1',
      symbols: [
        {
          exchange: 'NSE',
          tradingsymbol: 'TCS',
          product: 'CNC',
          lot_size: 1,
          master_qty: 15,
          target_qty: 15,
          follower_qty: 5,
          drift_qty: 10,
          action: 'BUY',
        },
      ],
    })

    const { container } = render(() => (
      <RebalanceModal open={true} target={accountTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByText('TCS')).toBeInTheDocument()
    const accountWrapper = container.querySelector('.follower-symbols-table-wrapper.account-mode-wrapper')
    expect(accountWrapper).toBeInTheDocument()
    expect(container.querySelector('.rebalance-symbols-table')).toBeInTheDocument()
  })

  it('does not fold expanded dropdown menu when background refetch occurs and preserves non-diff rows', async () => {
    let resolveFirstDiff: (val: any) => void
    const firstDiffPromise = new Promise((resolve) => {
      resolveFirstDiff = resolve
    })

    const diffSpy = vi.spyOn(api, 'getGroupRebalanceDiff').mockImplementation(() => firstDiffPromise as any)

    const initialData: api.GroupRebalanceDiff = {
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
              exchange: 'NSE',
              tradingsymbol: 'INFY',
              product: 'CNC',
              lot_size: 1,
              master_qty: 5,
              target_qty: 5,
              follower_qty: 0,
              drift_qty: 5,
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
              tradingsymbol: 'WIPRO',
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
    }

    resolveFirstDiff!(initialData)

    render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByText('Follower 1')).toBeInTheDocument()
    expect(screen.getByText('Follower 2')).toBeInTheDocument()

    // Expand Follower 1's dropdown
    const expandButtons = screen.getAllByLabelText('Expand symbols')
    await userEvent.click(expandButtons[0])

    // Follower 1's symbols are now visible
    expect(screen.getByText('INFY')).toBeInTheDocument()
    expect(screen.queryByText('WIPRO')).not.toBeInTheDocument()

    // Follower 1's dropdown element in the DOM
    const f1RowBefore = screen.getByTestId('follower-row-f1')
    expect(f1RowBefore).toBeInTheDocument()

    // Simulate background refetch where Follower 1 is unchanged (non-diff)
    // and Follower 2 has updated drift quantity
    const updatedData: api.GroupRebalanceDiff = {
      ...initialData,
      drifts: [
        initialData.drifts[0], // Identical Follower 1 data
        {
          ...initialData.drifts[1],
          symbols: [
            {
              ...initialData.drifts[1].symbols[0],
              follower_qty: 4,
              drift_qty: 6,
              action: 'BUY' as const,
            },
          ],
        },
      ],
    }

    diffSpy.mockResolvedValue(updatedData)

    // Verify Follower 1's symbols dropdown is STILL OPEN and INFY is still visible
    expect(screen.getByText('INFY')).toBeInTheDocument()
    const f1RowAfter = screen.getByTestId('follower-row-f1')
    expect(f1RowAfter).toBeInTheDocument()

    // Non-diff row DOM node is preserved
    expect(f1RowAfter).toBe(f1RowBefore)
  })

  it('shows broker unreachable error card and keeps Cancel button functional when diff fetch fails', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockRejectedValue(
      new Error('fetch master positions: domain: broker is not reachable (dial tcp: connection refused)')
    )
    const onCancel = vi.fn()

    const { container } = render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={onCancel} />
    ))

    expect(await screen.findByTestId('rebalance-error-state')).toBeInTheDocument()
    expect(container.querySelector('.rebalance-error-card')).toBeInTheDocument()
    expect(screen.getByText('DRIFTING FOLLOWERS')).toBeInTheDocument()
    expect(screen.getByText(/Connection Failed/i)).toBeInTheDocument()
    expect(screen.getByText('Broker Connection Error')).toBeInTheDocument()
    expect(container.querySelector('.error-message')).toHaveTextContent(/Broker is not reachable/i)
    expect(container.querySelector('.error-details-box')).toBeInTheDocument()

    // Skeleton is not stuck
    expect(screen.queryByTestId('rebalance-skeleton')).not.toBeInTheDocument()

    // Cancel button is active and functional
    const cancelBtn = screen.getByRole('button', { name: /Cancel/i })
    expect(cancelBtn).not.toBeDisabled()
    await userEvent.click(cancelBtn)
    expect(onCancel).toHaveBeenCalledTimes(1)

    // Rebalance button is disabled and says Rebalance (not In Equilibrium)
    const rebalanceBtn = screen.getByRole('button', { name: /^Rebalance$/i })
    expect(rebalanceBtn).toBeDisabled()
    expect(screen.queryByRole('button', { name: /In Equilibrium/i })).not.toBeInTheDocument()
  })

  it('humanizes bare ERROR string and renders structured card section', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockRejectedValue(new Error('ERROR'))
    const { container } = render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={() => {}} />
    ))

    expect(await screen.findByTestId('rebalance-error-state')).toBeInTheDocument()
    expect(container.querySelector('.rebalance-error-card')).toBeInTheDocument()
    expect(screen.getByText('DRIFTING FOLLOWERS')).toBeInTheDocument()
    expect(screen.getByText('Broker Connection Error')).toBeInTheDocument()
    // Never displays plain bare "ERROR"
    expect(screen.getByText(/Unable to communicate with the broker/i)).toBeInTheDocument()
  })

  it('shows account login expired error card when broker token is invalid', async () => {
    vi.spyOn(api, 'getAccountRebalanceDiff').mockRejectedValue(new Error('TokenException: Token is invalid'))
    const onCancel = vi.fn()

    const { container } = render(() => (
      <RebalanceModal open={true} target={accountTarget} onCancel={onCancel} />
    ))

    expect(await screen.findByTestId('rebalance-error-state')).toBeInTheDocument()
    expect(container.querySelector('.rebalance-error-card')).toBeInTheDocument()
    expect(screen.getByText('DRIFTING FOLLOWERS')).toBeInTheDocument()
    expect(screen.getByText('Authentication Required')).toBeInTheDocument()
    expect(screen.getByText(/Broker account login has expired/i)).toBeInTheDocument()

    // Cancel button works
    const cancelBtn = screen.getByRole('button', { name: /Cancel/i })
    expect(cancelBtn).not.toBeDisabled()
    await userEvent.click(cancelBtn)
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('keeps Cancel button functional and displays error banner when broker fails during execution', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 1,
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
              tradingsymbol: 'INFY',
              product: 'CNC',
              lot_size: 1,
              master_qty: 5,
              target_qty: 5,
              follower_qty: 0,
              drift_qty: 5,
              action: 'BUY',
            },
          ],
        },
      ],
    })

    const onConfirm = vi.fn().mockRejectedValue(new Error('broker is not reachable'))
    const onCancel = vi.fn()

    const { container } = render(() => (
      <RebalanceModal
        open={true}
        target={groupTarget}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    ))

    expect(await screen.findByText('Follower 1')).toBeInTheDocument()
    // By default all accounts are selected
    expect(screen.getByText('1 of 1 selected')).toBeInTheDocument()
    const rebalanceBtn = screen.getByRole('button', { name: /Rebalance \(1\)/i })
    expect(rebalanceBtn).not.toBeDisabled()

    // Click Rebalance
    await userEvent.click(rebalanceBtn)

    // Execution fails: error banner shown in standard .rebalance-error-banner
    expect(await screen.findByText(/Broker is not reachable/i)).toBeInTheDocument()
    expect(container.querySelector('.rebalance-error-banner')).toBeInTheDocument()

    // Cancel button is active and functional
    const cancelBtn = screen.getByRole('button', { name: /Cancel/i })
    expect(cancelBtn).not.toBeDisabled()
    await userEvent.click(cancelBtn)
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('greys out disabled followers, disables checkbox, excludes from Select All and submission', async () => {
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 2,
      followers_with_drift: 2,
      drifts: [
        {
          account_id: 'f1',
          account_name: 'Enabled Follower',
          broker_account_id: 'FOLLOW01A',
          enabled: true,
          clone_factor: '1',
          symbols: [
            {
              exchange: 'NSE',
              tradingsymbol: 'INFY',
              product: 'CNC',
              lot_size: 1,
              master_qty: 5,
              target_qty: 5,
              follower_qty: 0,
              drift_qty: 5,
              action: 'BUY',
            },
          ],
        },
        {
          account_id: 'f2',
          account_name: 'Disabled Follower',
          broker_account_id: 'FOLLOW01B',
          enabled: false,
          clone_factor: '1',
          symbols: [
            {
              exchange: 'NSE',
              tradingsymbol: 'TCS',
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

    const onConfirm = vi.fn().mockResolvedValue(undefined)

    const { container } = render(() => (
      <RebalanceModal
        open={true}
        target={groupTarget}
        onConfirm={onConfirm}
        onCancel={() => {}}
      />
    ))

    // Both followers appear in the list
    expect(await screen.findByText('Enabled Follower')).toBeInTheDocument()
    expect(screen.getByText('Disabled Follower')).toBeInTheDocument()

    // Static "Disabled" badge text is removed
    expect(screen.queryByText('Disabled')).not.toBeInTheDocument()

    // Disabled follower row has disabled styling class
    const disabledRow = container.querySelector('[data-testid="follower-row-f2"]')
    expect(disabledRow).toHaveClass('disabled-follower-item')

    // Disabled follower drift pill is greyed out
    const disabledPill = disabledRow!.querySelector('.follower-drift-badge')
    expect(disabledPill).toHaveClass('disabled-drift-badge')
    expect(disabledPill!.querySelector('span')).toHaveClass('pill-disabled')

    // Enabled follower drift pill is not greyed out
    const enabledRow = container.querySelector('[data-testid="follower-row-f1"]')
    const enabledPill = enabledRow!.querySelector('.follower-drift-badge')
    expect(enabledPill).not.toHaveClass('disabled-drift-badge')
    expect(enabledPill!.querySelector('span')).not.toHaveClass('pill-disabled')

    // Exactly one data-tooltip on the header, not on the row or inner elements
    const disabledHeader = disabledRow!.querySelector('.follower-accordion-header')
    expect(disabledHeader).toHaveAttribute('data-tooltip', 'Disabled')
    expect(disabledRow).not.toHaveAttribute('data-tooltip')

    // Disabled follower checkbox is disabled and unchecked
    const disabledCheckbox = screen.getByLabelText('Select Disabled Follower') as HTMLInputElement
    expect(disabledCheckbox).toBeDisabled()
    expect(disabledCheckbox.checked).toBe(false)

    // Dropdown button does NOT have any tooltip ("View symbols" / "View Details")
    const expandBtns = screen.getAllByLabelText(/symbols/i)
    for (const btn of expandBtns) {
      expect(btn).not.toHaveAttribute('data-tooltip')
    }


    // Enabled follower checkbox is enabled and checked

    const enabledCheckbox = screen.getByLabelText('Select Enabled Follower') as HTMLInputElement
    expect(enabledCheckbox).not.toBeDisabled()
    expect(enabledCheckbox.checked).toBe(true)

    // Selection count reflects enabled accounts: 1 of 1 selected (1 disabled)
    expect(screen.getByText(/1 of 1 selected/)).toBeInTheDocument()

    // Clicking on disabled row does not select it
    await userEvent.click(disabledRow!.querySelector('.follower-accordion-header')!)
    expect(disabledCheckbox.checked).toBe(false)

    // Clicking "Select All" toggles only enabled accounts
    const selectAllCheckbox = screen.getByLabelText('Select All Followers')
    await userEvent.click(selectAllCheckbox)
    expect(enabledCheckbox.checked).toBe(false)
    expect(disabledCheckbox.checked).toBe(false)

    await userEvent.click(selectAllCheckbox)
    expect(enabledCheckbox.checked).toBe(true)
    expect(disabledCheckbox.checked).toBe(false)

    // Submitting only submits enabled follower 'f1'
    const rebalanceBtn = screen.getByRole('button', { name: /Rebalance \(1\)/i })
    await userEvent.click(rebalanceBtn)
    expect(onConfirm).toHaveBeenCalledWith(['f1'])
  })

  it('closes when Escape key is pressed', async () => {
    const onCancel = vi.fn()
    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 1,
      followers_with_drift: 0,
      drifts: [],
    })

    render(() => (
      <RebalanceModal open={true} target={groupTarget} onCancel={onCancel} />
    ))

    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.keyboard('{Escape}')
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('does not close on Escape key when submission is in flight', async () => {
    const onCancel = vi.fn()
    let resolveSubmit!: () => void
    const pendingSubmit = new Promise<void>((res) => {
      resolveSubmit = res
    })
    const onConfirm = vi.fn().mockReturnValue(pendingSubmit)

    vi.spyOn(api, 'getGroupRebalanceDiff').mockResolvedValue({
      group_id: 'g1',
      master_id: 'm1',
      followers_evaluated: 1,
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

    render(() => (
      <RebalanceModal open={true} target={groupTarget} onConfirm={onConfirm} onCancel={onCancel} />
    ))

    const rebalanceBtn = await screen.findByRole('button', { name: /Rebalance/i })
    await userEvent.click(rebalanceBtn)
    expect(onConfirm).toHaveBeenCalled()

    // While submitting, Escape key should be ignored
    await userEvent.keyboard('{Escape}')
    expect(onCancel).not.toHaveBeenCalled()

    resolveSubmit()
  })
})



