import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AccountRow } from './AccountRow'
import type { GroupFollower } from '../api'

const follower: GroupFollower = {
  accountId: 'f1',
  brokerAccountId: 'ZY5678',
  enabled: true,
  status: 'ok',
}

describe('AccountRow', () => {
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

  it('calls onRebalance immediately, without a confirm modal', async () => {
    const onRebalance = vi.fn()
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
    expect(onRebalance).toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('gates Square Off behind a confirm modal', async () => {
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

    await userEvent.click(screen.getByText('Confirm'))
    expect(onSquareOff).toHaveBeenCalled()
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
    await userEvent.click(screen.getByText('Confirm'))
    expect(onExitOpenOrders).toHaveBeenCalled()
  })

  it('master row: renders Stop/Start instead of the copy toggle', async () => {
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

  it('spins the Rebalance icon while pending and shows a success toast when it resolves', async () => {
    let resolveRebalance: () => void = () => {}
    const onRebalance = vi.fn(() => new Promise<void>((resolve) => (resolveRebalance = resolve)))
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

    expect(rebalanceButton.querySelector('svg')).toHaveClass('icon-spin')
    expect(rebalanceButton).toBeDisabled()

    resolveRebalance()
    await screen.findByText(/rebalance triggered successfully/i)
    expect(rebalanceButton.querySelector('svg')).not.toHaveClass('icon-spin')
    expect(rebalanceButton).not.toBeDisabled()
  })

  it('shows an error toast when Rebalance fails', async () => {
    const onRebalance = vi.fn(() => Promise.reject(new Error('no master fill to rebalance from')))
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
    const toast = await screen.findByText('no master fill to rebalance from')
    expect(toast.closest('.result-toast')).toHaveClass('result-toast-error')
  })

  it('greys out the row and disables its actions when actionsDisabled', () => {
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
})
