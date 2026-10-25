import { render, screen, waitFor } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AccountsTable } from './AccountsTable'
import * as api from '../api'
import type { Account } from '../api'

const master: Account = {
  id: 'm1',
  name: 'Master Account',
  role: 'master',
  broker: 'kite',
  brokerAccountId: 'ZX1234',
  masterId: null,
  cloneFactor: null,
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
  cloneFactor: '0.5',
  maxQtyPerOrder: 100,
  enabled: true,
  active: true,
  status: 'ok',
}

describe('AccountsTable', () => {
  it('renders one row per account with role/broker/group/cloneFactor/status', () => {
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

  it('displays groupName directly when present on account', () => {
    const acct: Account = { ...follower, groupName: 'Alpha Momentum' }
    render(() => <AccountsTable accounts={[acct]} onEdit={vi.fn()} />)
    expect(screen.getByText('Alpha Momentum')).toBeInTheDocument()
  })

  it('resolves group name from groups prop and never displays raw UUID', () => {
    const rawUuid = 'b0000000-0000-0000-0000-000000000001'
    const acctWithGroup: Account = { ...follower, groupId: rawUuid, groupName: undefined }
    const groups: api.GroupSummary[] = [
      { id: rawUuid, name: 'Beta Scalping', masterId: 'm1', masterAccountId: 'ZX1234', broker: 'kite', followerCount: 1, status: 'ok' },
    ]
    render(() => <AccountsTable accounts={[acctWithGroup]} groups={groups} onEdit={vi.fn()} />)
    expect(screen.getByText('Beta Scalping')).toBeInTheDocument()
    expect(screen.queryByText(rawUuid)).not.toBeInTheDocument()
  })

  it('renders em-dash and never displays raw UUID when group cannot be resolved', () => {
    const rawUuid = 'b0000000-0000-0000-0000-000000000002'
    const unresolvableAcct: Account = { ...follower, groupId: rawUuid, masterId: 'm999', groupName: undefined }
    render(() => <AccountsTable accounts={[unresolvableAcct]} groups={[]} onEdit={vi.fn()} />)
    expect(screen.queryByText(rawUuid)).not.toBeInTheDocument()
    expect(screen.queryByText('m999')).not.toBeInTheDocument()
  })

  it('renders emptyMessage spanning all columns when accounts is empty', () => {
    render(() => (
      <AccountsTable accounts={[]} emptyMessage="No master accounts." onEdit={vi.fn()} />
    ))

    const emptyCell = screen.getByText('No master accounts.').closest('td')
    expect(emptyCell).toBeInTheDocument()
    expect(emptyCell).toHaveAttribute('colspan', '11')
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

  it('clicking expand button toggles holdings dropdown row', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: {
        netQty: 0,
        totalMtm: '0.00',
        realizedPnl: '0.00',
        accountValue: '0.00',
        status: 'online',
      },
      holdings: [
        {
          instrument: 'INFY',
          sellableQuantity: 10,
          buyAveragePrice: '1400.00',
          ltp: '1450.00',
          pnl: '500.00',
        },
      ],
      openPositions: [],
      closedPositions: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })

    render(() => (
      <AccountsTable accounts={[follower]} onEdit={vi.fn()} />
    ))

    const expandBtn = screen.getByTestId(`expand-holdings-btn-${follower.id}`)
    expect(screen.queryByTestId('account-holdings-expansion-row')).not.toBeInTheDocument()

    await userEvent.click(expandBtn)
    expect(screen.getByTestId('account-holdings-expansion-row')).toBeInTheDocument()
    expect(await screen.findByText('INFY')).toBeInTheDocument()

    await userEvent.click(expandBtn)
    expect(screen.queryByTestId('account-holdings-expansion-row')).not.toBeInTheDocument()
  })

  it('clicking Sync from Kite calls postAction and refreshes holdings', async () => {
    vi.spyOn(api, 'getAccountOrders').mockResolvedValue({
      summary: {
        netQty: 0,
        totalMtm: '0.00',
        realizedPnl: '0.00',
        accountValue: '0.00',
        status: 'online',
      },
      holdings: [],
      openPositions: [],
      closedPositions: [],
      openOrders: [],
      closedOrders: [],
      rejectedOrders: [],
    })
    const postActionSpy = vi.spyOn(api, 'postAction').mockResolvedValue({ type: 'sync_positions', status: 'accepted' })

    render(() => (
      <AccountsTable accounts={[follower]} onEdit={vi.fn()} />
    ))

    const expandBtn = screen.getByTestId(`expand-holdings-btn-${follower.id}`)
    await userEvent.click(expandBtn)

    const syncBtn = await screen.findByRole('button', { name: /Sync from Kite/i })
    await userEvent.click(syncBtn)

    expect(postActionSpy).toHaveBeenCalledWith(follower.id, 'sync_positions')
  })
})
