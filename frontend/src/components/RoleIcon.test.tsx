import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { RoleIcon } from './RoleIcon'

describe('RoleIcon', () => {
  it('renders Gandalf icon for "master" with tooltip, accessible label, and black pen strokes', () => {
    render(() => <RoleIcon role="master" />)
    const iconEl = screen.getByLabelText('Master')
    expect(iconEl).toBeInTheDocument()
    expect(iconEl).toHaveAttribute('data-tooltip', 'Master')
    expect(iconEl).toHaveAttribute('title', 'Master')
    const svg = iconEl.querySelector('svg.role-icon-gandalf')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('viewBox', '0 0 512 512')
    const path = svg?.querySelector('path')
    expect(path).not.toBeNull()
    expect(path).toHaveAttribute('stroke', '#000000')
    expect(path).toHaveAttribute('stroke-width', '14')
  })

  it('renders Hobbit icon for "follower" with tooltip, accessible label, and hover filter', () => {
    render(() => <RoleIcon role="follower" />)
    const iconEl = screen.getByLabelText('Follower')
    expect(iconEl).toBeInTheDocument()
    expect(iconEl).toHaveAttribute('data-tooltip', 'Follower')
    expect(iconEl).toHaveAttribute('title', 'Follower')
    const svg = iconEl.querySelector('svg.role-icon-hobbit')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('viewBox', '0 0 920 940')
    const img = svg?.querySelector('image')
    expect(img).not.toBeNull()
    const filter = svg?.querySelector('filter#hobbit-green-hover')
    expect(filter).not.toBeNull()
  })

  it('supports custom size and custom class', () => {
    render(() => <RoleIcon role="master" size={32} class="custom-role-class" />)
    const iconEl = screen.getByLabelText('Master')
    expect(iconEl).toHaveClass('role-icon-wrap')
    expect(iconEl).toHaveClass('custom-role-class')
    const svg = iconEl.querySelector('svg')
    expect(svg).toHaveAttribute('width', '32')
    expect(svg).toHaveAttribute('height', '32')
  })
})
