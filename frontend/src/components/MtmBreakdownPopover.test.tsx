import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { MtmBreakdownPopover } from './MtmBreakdownPopover'

describe('MtmBreakdownPopover', () => {
  it('renders nothing by default and opens on button click with breakdown and total', async () => {
    render(() => (
      <MtmBreakdownPopover
        totalMtm={1500}
        breakdown={{
          MIS: 500,
          NRML: 1000,
        }}
      />
    ))

    // Popover dialog is not rendered initially
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    // Click trigger button
    const triggerBtn = screen.getByLabelText('View MTM breakdown')
    expect(triggerBtn).toBeInTheDocument()
    await userEvent.click(triggerBtn)

    // Popover is now open
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('MIS')).toBeInTheDocument()
    expect(screen.getByText('₹500.00')).toBeInTheDocument()
    expect(screen.getByText('NRML')).toBeInTheDocument()
    expect(screen.getByText('₹1,000.00')).toBeInTheDocument()
    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByText('₹1,500.00')).toBeInTheDocument()
  })

  it('renders additional non-zero derivative products such as BFO, while filtering out CNC holdings', async () => {
    render(() => (
      <MtmBreakdownPopover
        totalMtm={1450}
        breakdown={{
          MIS: 500,
          NRML: 1000,
          BFO: -50,
          CNC: 200,
        }}
      />
    ))

    await userEvent.click(screen.getByLabelText('View MTM breakdown'))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('BFO')).toBeInTheDocument()
    expect(screen.getByText('-₹50.00')).toBeInTheDocument()
    expect(screen.queryByText('CNC')).not.toBeInTheDocument()
  })

  it('closes on Escape key press', async () => {
    render(() => (
      <MtmBreakdownPopover
        totalMtm={200}
        breakdown={{ MIS: 200, NRML: 0 }}
      />
    ))

    await userEvent.click(screen.getByLabelText('View MTM breakdown'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('closes on outside click', async () => {
    render(() => (
      <div>
        <button type="button" data-testid="outside-el">Outside</button>
        <MtmBreakdownPopover
          totalMtm={200}
          breakdown={{ MIS: 200, NRML: 0 }}
        />
      </div>
    ))

    await userEvent.click(screen.getByLabelText('View MTM breakdown'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.click(screen.getByTestId('outside-el'))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('toggles closed when trigger button is clicked again', async () => {
    render(() => (
      <MtmBreakdownPopover
        totalMtm={200}
        breakdown={{ MIS: 200, NRML: 0 }}
      />
    ))

    const triggerBtn = screen.getByLabelText('View MTM breakdown')
    await userEvent.click(triggerBtn)
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.click(triggerBtn)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('hides hover help text (data-tooltip and title) once popover window appears', async () => {
    render(() => (
      <MtmBreakdownPopover
        totalMtm={200}
        breakdown={{ MIS: 200, NRML: 0 }}
      />
    ))

    const triggerBtn = screen.getByLabelText('View MTM breakdown')
    expect(triggerBtn).toHaveAttribute('data-tooltip', 'View MTM breakdown')
    expect(triggerBtn).toHaveAttribute('title', 'View MTM breakdown')

    await userEvent.click(triggerBtn)
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    // Hover help text is removed while popover is open
    expect(triggerBtn).not.toHaveAttribute('data-tooltip')
    expect(triggerBtn).not.toHaveAttribute('title')

    await userEvent.click(triggerBtn)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    // Hover help text is restored when closed
    expect(triggerBtn).toHaveAttribute('data-tooltip', 'View MTM breakdown')
    expect(triggerBtn).toHaveAttribute('title', 'View MTM breakdown')
  })
})

