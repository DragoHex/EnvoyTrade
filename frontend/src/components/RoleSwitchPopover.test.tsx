import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { RoleSwitchPopover } from './RoleSwitchPopover'

describe('RoleSwitchPopover', () => {
  it('renders nothing when closed', () => {
    render(() => (
      <RoleSwitchPopover
        role="master"
        triggerRef={undefined}
        open={false}
        onClose={vi.fn()}
        onSelect={vi.fn()}
      />
    ))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders Gandalf to Hobbit when role is master', () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)

    render(() => (
      <RoleSwitchPopover
        role="master"
        triggerRef={trigger}
        open={true}
        onClose={vi.fn()}
        onSelect={vi.fn()}
      />
    ))

    const dialog = screen.getByRole('dialog', { name: 'Switch to Follower' })
    expect(dialog).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Switch to Follower' })).toBeInTheDocument()
  })

  it('renders Hobbit to Gandalf when role is follower', () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)

    render(() => (
      <RoleSwitchPopover
        role="follower"
        triggerRef={trigger}
        open={true}
        onClose={vi.fn()}
        onSelect={vi.fn()}
      />
    ))

    const dialog = screen.getByRole('dialog', { name: 'Switch to Master' })
    expect(dialog).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Switch to Master' })).toBeInTheDocument()
  })

  it('clicking button calls onSelect and onClose', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    const onSelect = vi.fn()
    const onClose = vi.fn()

    render(() => (
      <RoleSwitchPopover
        role="master"
        triggerRef={trigger}
        open={true}
        onClose={onClose}
        onSelect={onSelect}
      />
    ))

    const btn = screen.getByRole('button', { name: 'Switch to Follower' })
    await userEvent.click(btn)

    expect(onSelect).toHaveBeenCalled()
    expect(onClose).toHaveBeenCalled()
  })

  it('pressing Escape calls onClose', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    const onClose = vi.fn()

    render(() => (
      <RoleSwitchPopover
        role="master"
        triggerRef={trigger}
        open={true}
        onClose={onClose}
        onSelect={vi.fn()}
      />
    ))

    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalled()
  })
})
