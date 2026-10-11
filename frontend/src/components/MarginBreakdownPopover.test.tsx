import { render, screen, fireEvent } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { MarginBreakdownPopover } from './MarginBreakdownPopover'
import type { MarginBreakdown } from '../api'

describe('MarginBreakdownPopover', () => {
  it('renders trigger button and toggles popover dialog on click', async () => {
    const breakdown: MarginBreakdown = {
      net: 250000,
      cash: -437945.1,
      openingCash: 120769.9,
      collateral: 2300911.09,
      utilised: 1117430,
    }

    render(() => (
      <MarginBreakdownPopover
        availableMargin={250000}
        availableCash={-437945.1}
        breakdown={breakdown}
      />
    ))

    const trigger = screen.getByRole('button', { name: /view margin breakdown/i })
    expect(trigger).toBeDefined()
    expect(screen.queryByRole('dialog', { name: /margin breakdown/i })).toBeNull()

    // Open popover
    await fireEvent.click(trigger)
    const dialog = screen.getByRole('dialog', { name: /margin breakdown/i })
    expect(dialog).toBeDefined()

    // Assert key labels and values are displayed
    expect(screen.getByText('Live Cash')).toBeDefined()
    expect(screen.getByText('Collateral')).toBeDefined()
    expect(screen.getByText('Utilised')).toBeDefined()
    expect(screen.getByText('Total Margin')).toBeDefined()

    // Close on click outside or Escape
    await fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: /margin breakdown/i })).toBeNull()
  })

  it('handles fallback when breakdown is not fully populated', async () => {
    render(() => (
      <MarginBreakdownPopover
        availableMargin={100000}
        availableCash={50000}
      />
    ))

    const trigger = screen.getByRole('button', { name: /view margin breakdown/i })
    await fireEvent.click(trigger)
    expect(screen.getByRole('dialog', { name: /margin breakdown/i })).toBeDefined()
    expect(screen.getByText('Live Cash')).toBeDefined()
  })
})
