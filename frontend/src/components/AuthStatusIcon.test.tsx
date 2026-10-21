import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { AuthStatusIcon } from './AuthStatusIcon'

describe('AuthStatusIcon', () => {
  it('renders authenticated ring checkmark with accent color and active session tooltip', () => {
    render(() => <AuthStatusIcon status="authenticated" />)

    const el = screen.getByRole('img', { name: /Authenticated - Active Broker Session/i })
    expect(el).toBeInTheDocument()
    expect(el).toHaveAttribute('data-tooltip', 'Authenticated - Active Broker Session')
    expect(el).toHaveClass('auth-status-icon-wrap')

    const svg = el.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('auth-status-icon-authenticated')
  })

  it('renders error ring cross with red color and specific auth error tooltip', () => {
    render(() => <AuthStatusIcon status="error" error="Token expired at 06:00" />)

    const el = screen.getByRole('img', { name: /Authentication Error: Token expired at 06:00/i })
    expect(el).toBeInTheDocument()
    expect(el).toHaveAttribute('data-tooltip', 'Authentication Error: Token expired at 06:00')

    const svg = el.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('auth-status-icon-error')
  })

  it('renders unauthenticated ring exclamation mark when status is null or unauthenticated', () => {
    render(() => <AuthStatusIcon status="unauthenticated" />)

    const el = screen.getByRole('img', { name: /Unauthenticated - No Active Broker Session/i })
    expect(el).toBeInTheDocument()
    expect(el).toHaveAttribute('data-tooltip', 'Unauthenticated - No Active Broker Session')

    const svg = el.querySelector('svg')
    expect(svg).toBeInTheDocument()
    expect(svg).toHaveClass('auth-status-icon-unauthenticated')
  })

  it('supports custom size and class overrides', () => {
    render(() => <AuthStatusIcon status="authenticated" size={28} class="custom-auth-class" />)

    const el = screen.getByRole('img')
    expect(el).toHaveClass('custom-auth-class')

    const svg = el.querySelector('svg')
    expect(svg).toHaveAttribute('width', '28')
    expect(svg).toHaveAttribute('height', '28')
  })
})
