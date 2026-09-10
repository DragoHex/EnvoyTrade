import { render, screen, waitFor } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AccountsTable } from './AccountsTable'
import type { Account } from '../api'

const master: Account = {
  id: 'm1',
  name: 'Master Account',
  role: 'master',
  broker: 'kite',
  brokerAccountId: 'ZX1234',
  masterId: null,
  capitalRatio: null,
  maxQtyPerOrder: null,
  enabled: true,
  active: true,
  status: 'ok',
}
const follower: Account = {
  id: 'f1',
  name: 'Follower Account',
  role: 'follower',
  broker: 'kite',
  brokerAccountId: 'ZY5678',
  masterId: 'm1',
  capitalRatio: '0.5',
  maxQtyPerOrder: 100,
  enabled: true,
  active: true,
  status: 'ok',
}

describe('AccountsTable', () => {
  it('renders one row per account with role/broker/group/capitalRatio/status', () => {
    render(() => (
      <AccountsTable accounts={[master, follower]} onEdit={vi.fn()} onRemoveFromGroup={vi.fn()} onDelete={vi.fn()} />
    ))

    expect(screen.getByText('ZX1234')).toBeInTheDocument()
    expect(screen.getByText('ZY5678')).toBeInTheDocument()
    expect(screen.getByText('0.5')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()

    const masterRole = screen.getByLabelText('Master')
    expect(masterRole).toBeInTheDocument()
    expect(masterRole).toHaveAttribute('data-tooltip', 'Master')

    const followerRole = screen.getByLabelText('Follower')
    expect(followerRole).toBeInTheDocument()
    expect(followerRole).toHaveAttribute('data-tooltip', 'Follower')

    const brokerLogos = screen.getAllByLabelText('Zerodha Kite')
    expect(brokerLogos).toHaveLength(2)
    expect(brokerLogos[0]).toHaveAttribute('data-tooltip', 'Zerodha Kite')
    expect(brokerLogos[0].querySelector('svg.broker-logo-svg')).not.toBeNull()
  })

  it('renders Active copy-toggles and handles onToggleActive', async () => {
    const onToggle = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountsTable accounts={[master, follower]} onEdit={vi.fn()} onToggleActive={onToggle} />
    ))

    const toggles = screen.getAllByRole('checkbox', { name: /Toggle active for/ })
    expect(toggles).toHaveLength(2)
    expect(toggles[0]).toBeChecked()
    expect(toggles[1]).toBeChecked()

    await userEvent.click(toggles[0])
    expect(onToggle).toHaveBeenCalledWith('m1', false)
  })

  it('disables toggle button while toggling is in flight', async () => {
    let resolveToggle!: () => void
    const togglePromise = new Promise<void>((resolve) => {
      resolveToggle = resolve
    })
    const onToggle = vi.fn().mockReturnValue(togglePromise)

    render(() => (
      <AccountsTable accounts={[follower]} onEdit={vi.fn()} onToggleActive={onToggle} />
    ))

    const toggle = screen.getByRole('checkbox', { name: `Toggle active for ${follower.name}` })
    expect(toggle).not.toBeDisabled()

    await userEvent.click(toggle)
    expect(onToggle).toHaveBeenCalledWith('f1', false)
    expect(toggle).toBeDisabled()

    resolveToggle()
    await waitFor(() => expect(toggle).not.toBeDisabled())
  })

  it('renders action buttons with icons and tooltips', () => {
    render(() => (
      <AccountsTable accounts={[follower]} onEdit={vi.fn()} onRemoveFromGroup={vi.fn()} onDelete={vi.fn()} />
    ))

    const editBtn = screen.getByRole('button', { name: 'Edit' })
    expect(editBtn).toHaveAttribute('data-tooltip', 'Edit Account')
    expect(editBtn.querySelector('svg')).not.toBeNull()

    const removeBtn = screen.getByRole('button', { name: 'Remove from group' })
    expect(removeBtn).toHaveAttribute('data-tooltip', 'Remove from Group')
    expect(removeBtn.querySelector('svg')).not.toBeNull()

    const deleteBtn = screen.getByRole('button', { name: 'Delete' })
    expect(deleteBtn).toHaveAttribute('data-tooltip', 'Delete Account')
    expect(deleteBtn.querySelector('svg')).not.toBeNull()
  })

  it('clicking Edit fires onEdit with the account', async () => {
    const onEdit = vi.fn()
    render(() => (
      <AccountsTable accounts={[follower]} onEdit={onEdit} onRemoveFromGroup={vi.fn()} onDelete={vi.fn()} />
    ))

    await userEvent.click(screen.getByRole('button', { name: 'Edit' }))
    expect(onEdit).toHaveBeenCalledWith(follower)
  })

  it('shows Remove from group only for accounts attached to a group', () => {
    render(() => (
      <AccountsTable accounts={[master, follower]} onEdit={vi.fn()} onRemoveFromGroup={vi.fn()} onDelete={vi.fn()} />
    ))

    expect(screen.getAllByRole('button', { name: 'Remove from group' })).toHaveLength(1)
  })

  it('clicking Remove from group fires onRemoveFromGroup with the account', async () => {
    const onRemoveFromGroup = vi.fn()
    render(() => (
      <AccountsTable accounts={[follower]} onEdit={vi.fn()} onRemoveFromGroup={onRemoveFromGroup} onDelete={vi.fn()} />
    ))

    await userEvent.click(screen.getByRole('button', { name: 'Remove from group' }))
    expect(onRemoveFromGroup).toHaveBeenCalledWith(follower)
  })

  it('clicking Delete fires onDelete with the account', async () => {
    const onDelete = vi.fn()
    render(() => (
      <AccountsTable accounts={[master]} onEdit={vi.fn()} onRemoveFromGroup={vi.fn()} onDelete={onDelete} />
    ))

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onDelete).toHaveBeenCalledWith(master)
  })
})
