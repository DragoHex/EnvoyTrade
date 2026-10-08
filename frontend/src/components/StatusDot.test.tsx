import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { StatusDot } from './StatusDot'

describe('StatusDot', () => {
  it('renders green dot when status is "ok"', () => {
    render(() => <StatusDot status="ok" />)
    const dot = screen.getByTestId('status-dot')
    expect(dot).toHaveAttribute('data-status', 'ok')
    expect(dot.style.background).toBe('var(--color-accent)')
  })

  it('renders green dot when status is "active"', () => {
    render(() => <StatusDot status="active" />)
    const dot = screen.getByTestId('status-dot')
    expect(dot).toHaveAttribute('data-status', 'active')
    expect(dot.style.background).toBe('var(--color-accent)')
  })

  it('renders yellow dot when status is "warning"', () => {
    render(() => <StatusDot status="warning" />)
    const dot = screen.getByTestId('status-dot')
    expect(dot).toHaveAttribute('data-status', 'warning')
    expect(dot.style.background).toBe('rgb(245, 159, 0)') // #f59f00 in rgb
  })

  it('renders red dot when status is "error"', () => {
    render(() => <StatusDot status="error" />)
    const dot = screen.getByTestId('status-dot')
    expect(dot).toHaveAttribute('data-status', 'error')
    expect(dot.style.background).toBe('rgb(229, 72, 77)') // #e5484d in rgb
  })
})
