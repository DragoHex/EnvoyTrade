import { render, screen, fireEvent, waitFor } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { AccountQuotaPopover } from './AccountQuotaPopover'

describe('AccountQuotaPopover', () => {
  it('renders trigger with used and total accounts', () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={5}
        usedAccounts={2}
        brokerLimits={{ kite: 3, dhan: 2 }}
        brokerUsed={{ kite: 2, dhan: 0 }}
      />
    ))

    expect(screen.getByText('2 / 5')).toBeInTheDocument()
    expect(screen.getByText('Accounts')).toBeInTheDocument()
  })

  it('renders infinity symbol when maxAccounts is -1 (unlimited)', () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={-1}
        usedAccounts={4}
      />
    ))

    expect(screen.getByText('4 / ∞')).toBeInTheDocument()
  })

  it('renders 0 quota when maxAccounts is 0 (0 allocation / exhausted)', () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={0}
        usedAccounts={0}
      />
    ))

    expect(screen.getByText('0 / 0')).toBeInTheDocument()
  })

  it('opens breakdown popover on click and closes on Escape', async () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={5}
        usedAccounts={3}
        brokerLimits={{ kite: 2, dhan: 3 }}
        brokerUsed={{ kite: 2, dhan: 1 }}
      />
    ))

    const triggerBtn = screen.getByRole('button', { name: /View Account Quota/i })
    expect(screen.queryByRole('dialog', { name: /Account Quota Breakdown/i })).not.toBeInTheDocument()

    fireEvent.click(triggerBtn)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Account Quota Breakdown/i })).toBeInTheDocument()
    })

    expect(screen.getByText('Account Quota Breakdown')).toBeInTheDocument()
    expect(screen.getByText('Zerodha Kite')).toBeInTheDocument()
    expect(screen.getByText('2 / 2')).toBeInTheDocument()
    expect(screen.getByText('Dhan')).toBeInTheDocument()
    expect(screen.getByText('1 / 3')).toBeInTheDocument()
    expect(screen.getByText('Total Accounts')).toBeInTheDocument()

    // Press Escape
    fireEvent.keyDown(document, { key: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: /Account Quota Breakdown/i })).not.toBeInTheDocument()
    })
  })

  it('renders breakdown rows for Generalist and strictly excludes Test Broker from accounts page breakdown', async () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={6}
        usedAccounts={4}
        brokerLimits={{ general: 2, kite: 2, dhan: 1, testbroker: 1 }}
        brokerUsed={{ kite: 2, dhan: 1, testbroker: 0 }}
      />
    ))

    const triggerBtn = screen.getByRole('button', { name: /View Account Quota/i })
    fireEvent.click(triggerBtn)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Account Quota Breakdown/i })).toBeInTheDocument()
    })

    expect(screen.getByText('Zerodha Kite')).toBeInTheDocument()
    expect(screen.getByText('2 / 2')).toBeInTheDocument()
    expect(screen.getByText('Dhan')).toBeInTheDocument()
    expect(screen.getByText('1 / 1')).toBeInTheDocument()
    // Test Broker is strictly for dev use and MUST be hidden from the client-facing breakdown
    expect(screen.queryByText('Test Broker')).not.toBeInTheDocument()
    expect(screen.getByText('Generalist')).toBeInTheDocument()
    // excess used across other brokers is 0 (kite 2/2, dhan 1/1)
    expect(screen.getByText('0 / 2')).toBeInTheDocument()
    expect(screen.getByText('Total Accounts')).toBeInTheDocument()
    expect(screen.getAllByText('4 / 6')).toHaveLength(2)
  })

  it('does not turn quota red on complete usage (e.g. 6/6), turning red only when exceeding (e.g. 7/6)', async () => {
    const { container: container6 } = render(() => (
      <AccountQuotaPopover
        maxAccounts={6}
        usedAccounts={6}
        brokerLimits={{ kite: 3, dhan: 3 }}
        brokerUsed={{ kite: 3, dhan: 3 }}
      />
    ))

    // Donut ring stroke should be amber (#f59e0b) on complete usage, NOT red error
    const circle6 = container6.querySelectorAll('circle')[1]
    expect(circle6).toHaveAttribute('stroke', '#f59e0b')

    const triggerBtn6 = screen.getByRole('button', { name: /View Account Quota/i })
    fireEvent.click(triggerBtn6)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Account Quota Breakdown/i })).toBeInTheDocument()
    })

    // Neither 3/3 nor 6/6 should have text-negative red class
    const negativeElements6 = document.querySelectorAll('.text-negative')
    expect(negativeElements6.length).toBe(0)

    // Press Escape to close
    fireEvent.keyDown(document, { key: 'Escape' })
  })

  it('turns quota red when usage exceeds capacity (e.g. 7/6 or 1/0)', async () => {
    const { container: container7 } = render(() => (
      <AccountQuotaPopover
        maxAccounts={6}
        usedAccounts={7}
        brokerLimits={{ kite: 3, dhan: 3 }}
        brokerUsed={{ kite: 4, dhan: 3 }}
      />
    ))

    // Donut ring stroke should be red (var(--color-error, #ef4444)) when exceeding
    const circle7 = container7.querySelectorAll('circle')[1]
    expect(circle7).toHaveAttribute('stroke', 'var(--color-error, #ef4444)')

    const triggerBtn7 = screen.getAllByRole('button', { name: /View Account Quota/i })[0]
    fireEvent.click(triggerBtn7)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Account Quota Breakdown/i })).toBeInTheDocument()
    })

    // Exceeding broker (4/3) and exceeding total (7/6) must have text-negative red class
    const negativeElements7 = document.querySelectorAll('.text-negative')
    expect(negativeElements7.length).toBeGreaterThan(0)
  })

  it('unrestricts all individual categories when maxAccounts is -1', async () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={-1}
        usedAccounts={2}
        brokerLimits={{ kite: 0, dhan: 0, general: 0 }}
        brokerUsed={{ kite: 1, dhan: 1 }}
      />
    ))

    const triggerBtn = screen.getByRole('button', { name: /View Account Quota/i })
    fireEvent.click(triggerBtn)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Account Quota Breakdown/i })).toBeInTheDocument()
    })

    expect(screen.getByText('Zerodha Kite')).toBeInTheDocument()
    expect(screen.getByText('Dhan')).toBeInTheDocument()
    expect(screen.getAllByText('1 / ∞')).toHaveLength(2)
    expect(screen.getByText('Generalist')).toBeInTheDocument()
    expect(screen.getAllByText('2 / ∞')).toHaveLength(3)
    expect(screen.getByText('Total Accounts')).toBeInTheDocument()
  })

  it('shows 0 for individual types when limit is 0 (never infinity)', async () => {
    render(() => (
      <AccountQuotaPopover
        maxAccounts={5}
        usedAccounts={1}
        brokerLimits={{ kite: 3, dhan: 0, general: 2 }}
        brokerUsed={{ kite: 1, dhan: 0 }}
      />
    ))

    const triggerBtn = screen.getByRole('button', { name: /View Account Quota/i })
    fireEvent.click(triggerBtn)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Account Quota Breakdown/i })).toBeInTheDocument()
    })

    expect(screen.getByText('Zerodha Kite')).toBeInTheDocument()
    expect(screen.getByText('1 / 3')).toBeInTheDocument()
    // Dhan limit is 0 -> MUST show 0 / 0, NEVER 0 / ∞
    expect(screen.getByText('Dhan')).toBeInTheDocument()
    expect(screen.getByText('0 / 0')).toBeInTheDocument()
    expect(screen.getByText('Generalist')).toBeInTheDocument()
    expect(screen.getByText('0 / 2')).toBeInTheDocument()
    expect(screen.getByText('Total Accounts')).toBeInTheDocument()
    expect(screen.getAllByText('1 / 5')).toHaveLength(2)
  })
})
