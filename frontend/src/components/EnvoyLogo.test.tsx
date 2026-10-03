import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { EnvoyLogo } from './EnvoyLogo'

describe('EnvoyLogo', () => {
  it('renders with default size 24 and accessible label', () => {
    render(() => <EnvoyLogo />)
    const logo = screen.getByTestId('envoy-logo')
    expect(logo).toBeInTheDocument()
    expect(logo.tagName.toLowerCase()).toBe('svg')
    expect(logo).toHaveAttribute('viewBox', '0 0 48 48')
    expect(logo).toHaveAttribute('aria-label', 'EnvoyTrade Logo')
    expect(logo).toHaveAttribute('role', 'img')
    expect(logo.style.width).toBe('24px')
    expect(logo.style.height).toBe('24px')
  })

  it('renders with custom size and custom aria-label', () => {
    render(() => <EnvoyLogo size={48} ariaLabel="Custom Brand Mark" />)
    const logo = screen.getByTestId('envoy-logo')
    expect(logo.style.width).toBe('48px')
    expect(logo.style.height).toBe('48px')
    expect(logo).toHaveAttribute('aria-label', 'Custom Brand Mark')
  })

  it('renders with custom colors and classes', () => {
    render(() => (
      <EnvoyLogo
        class="custom-logo"
        accentColor="#ff0055"
        secondaryColor="#00ffaa"
      />
    ))
    const logo = screen.getByTestId('envoy-logo')
    expect(logo).toHaveClass('custom-logo')
    
    // Check master and follower paths are present
    const paths = logo.querySelectorAll('path')
    expect(paths.length).toBe(6) // 3 for master (2 strokes + 1 arrow), 3 for follower
    expect(paths[0]).toHaveAttribute('stroke', '#ff0055')
    expect(paths[2]).toHaveAttribute('fill', '#ff0055')
    expect(paths[3]).toHaveAttribute('stroke', '#00ffaa')
    expect(paths[5]).toHaveAttribute('fill', '#00ffaa')
  })
})
