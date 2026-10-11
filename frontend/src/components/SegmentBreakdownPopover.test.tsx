import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { SegmentBreakdownPopover } from './SegmentBreakdownPopover'

describe('SegmentBreakdownPopover', () => {
  it('renders trigger button and toggles open with Equity, Commodity and Total rows', async () => {
    render(() => (
      <SegmentBreakdownPopover
        title="Margin breakdown"
        total={1500000}
        breakdown={{
          EQUITY: 1200000,
          COMMODITY: 300000,
        }}
      />
    ))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    const triggerBtn = screen.getByLabelText('View Margin breakdown')
    expect(triggerBtn).toBeInTheDocument()
    await userEvent.click(triggerBtn)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Equity')).toBeInTheDocument()
    expect(screen.getByText('₹12,00,000.00')).toBeInTheDocument()
    expect(screen.getByText('Commodity')).toBeInTheDocument()
    expect(screen.getByText('₹3,00,000.00')).toBeInTheDocument()
    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByText('₹15,00,000.00')).toBeInTheDocument()
  })

  it('handles negative cash ledger balance properly', async () => {
    render(() => (
      <SegmentBreakdownPopover
        title="Cash breakdown"
        total={-437945.1}
        breakdown={{
          EQUITY: -437945.1,
          COMMODITY: 0,
        }}
      />
    ))

    await userEvent.click(screen.getByLabelText('View Cash breakdown'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getAllByText('-₹4,37,945.10')).toHaveLength(2)
    expect(screen.getByText('₹0.00')).toBeInTheDocument()
  })

  it('closes on Escape key press', async () => {
    render(() => (
      <SegmentBreakdownPopover
        title="Margin breakdown"
        total={50000}
        breakdown={{ EQUITY: 50000 }}
      />
    ))

    await userEvent.click(screen.getByLabelText('View Margin breakdown'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('closes on outside click', async () => {
    render(() => (
      <div>
        <button type="button" data-testid="outside-el">Outside</button>
        <SegmentBreakdownPopover
          title="Margin breakdown"
          total={50000}
          breakdown={{ EQUITY: 50000 }}
        />
      </div>
    ))

    await userEvent.click(screen.getByLabelText('View Margin breakdown'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.click(screen.getByTestId('outside-el'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
