import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AccountsTable } from './AccountsTable'
import type { Account } from '../api'

const master: Account = {
  id: 'm1', role: 'master', broker: 'kite', brokerAccountId: 'ZX1234', masterId: null,
  capitalRatio: null, maxQtyPerOrder: null, enabled: true, active: true, status: 'ok',
}
const follower: Account = {
  id: 'f1', role: 'follower', broker: 'kite', brokerAccountId: 'ZY5678', masterId: 'm1',
  capitalRatio: '0.5', maxQtyPerOrder: 100, enabled: true, active: true, status: 'ok',
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
