import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AccountDetailDrawer } from './AccountDetailDrawer'
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

describe('AccountDetailDrawer', () => {
  it('create mode, role=master: does not show capitalRatio/maxQtyPerOrder/master fields, submits createAccount', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    expect(screen.queryByLabelText('Capital Ratio')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Master')).not.toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ role: 'master', broker: 'kite', brokerAccountId: 'ZX9999', apiKey: 'key', apiSecret: 'secret' }),
    )
  })

  it('create mode, role=follower: shows master dropdown, submits createAccount with masterId', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'follower')
    expect(screen.getByLabelText('Master')).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.selectOptions(screen.getByLabelText('Master'), 'm1')
    await userEvent.clear(screen.getByLabelText('Capital Ratio'))
    await userEvent.type(screen.getByLabelText('Capital Ratio'), '0.5')
    await userEvent.clear(screen.getByLabelText('Max Qty/Order'))
    await userEvent.type(screen.getByLabelText('Max Qty/Order'), '100')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'follower', brokerAccountId: 'ZY9999', masterId: 'm1', capitalRatio: '0.5', maxQtyPerOrder: 100,
      }),
    )
  })

  it('edit mode: role is locked, submits only changed fields via onSave', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={follower} masters={[master]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
    ))

    expect(screen.getByLabelText('Role')).toBeDisabled()

    await userEvent.clear(screen.getByLabelText('Capital Ratio'))
    await userEvent.type(screen.getByLabelText('Capital Ratio'), '0.75')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onSave).toHaveBeenCalledWith('f1', { capitalRatio: '0.75' })
  })
})
