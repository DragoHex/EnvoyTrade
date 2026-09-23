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
  ip: null,
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
  ip: '192.168.1.20',
}

describe('AccountDetailDrawer', () => {
  it('create mode, role=master: does not show capitalRatio/maxQtyPerOrder/master fields, submits createAccount', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
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

  it('create mode, role=follower: shows master dropdown defaulting to Select a Master, submits createAccount with masterId', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    expect(screen.getByLabelText('Role')).toHaveValue('follower')
    expect(screen.getByLabelText('Master')).toBeInTheDocument()
    expect(screen.getByLabelText('Master')).toHaveValue('')

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.selectOptions(screen.getByLabelText('Master'), 'm1')
    await userEvent.clear(screen.getByLabelText('Capital Ratio'))
    await userEvent.type(screen.getByLabelText('Capital Ratio'), '0.5')
    await userEvent.type(screen.getByLabelText(/IP Address/), '192.168.1.50')
    await userEvent.clear(screen.getByLabelText('Max Qty/Order'))
    await userEvent.type(screen.getByLabelText('Max Qty/Order'), '100')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'follower', brokerAccountId: 'ZY9999', masterId: 'm1', capitalRatio: '0.5', maxQtyPerOrder: 100, ip: '192.168.1.50',
      }),
    )
  })

  it('create mode, role=follower without master selection: shows validation error', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('A master account must be selected.')
    expect(onCreate).not.toHaveBeenCalled()
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
  it('create mode, role=follower without IP: shows validation error', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.selectOptions(screen.getByLabelText('Master'), 'm1')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('IP Address is required for follower accounts.')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('create mode, role=follower with invalid IP: shows validation error', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.selectOptions(screen.getByLabelText('Master'), 'm1')
    await userEvent.type(screen.getByLabelText(/IP Address/), 'not-an-ip')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid IP Address: must be a valid IPv4 or IPv6 address.')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('create mode, role=master with optional valid IP: submits with ip', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText(/IP Address/), '2001:db8::1')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ role: 'master', brokerAccountId: 'ZX9999', ip: '2001:db8::1' }),
    )
  })

  it('create mode, role=master with invalid IP: shows validation error', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText(/IP Address/), '999.999.999.999')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid IP Address: must be a valid IPv4 or IPv6 address.')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('edit mode: role is locked, submits changed IP and capitalRatio via onSave', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={follower} masters={[master]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
    ))

    await userEvent.clear(screen.getByLabelText(/IP Address/))
    await userEvent.type(screen.getByLabelText(/IP Address/), '10.0.0.99')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onSave).toHaveBeenCalledWith('f1', { ip: '10.0.0.99' })
  })

  it('edit mode: API Key and API Secret are editable and prefilled with existing values', async () => {
    const accountWithCreds: Account = {
      ...master,
      apiKey: 'current-key',
      apiSecret: 'current-secret',
    }
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={accountWithCreds} masters={[master]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
    ))

    const apiKeyInput = screen.getByLabelText('API Key')
    const apiSecretInput = screen.getByLabelText('API Secret')

    expect(apiKeyInput).not.toBeDisabled()
    expect(apiSecretInput).not.toBeDisabled()
    expect(apiKeyInput).toHaveValue('current-key')
    expect(apiSecretInput).toHaveValue('current-secret')

    await userEvent.clear(apiKeyInput)
    await userEvent.type(apiKeyInput, 'new-api-key')
    await userEvent.clear(apiSecretInput)
    await userEvent.type(apiSecretInput, 'new-api-secret')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onSave).toHaveBeenCalledWith('m1', {
      apiKey: 'new-api-key',
      apiSecret: 'new-api-secret',
    })
  })

  it('create mode: submits password and totpSecret', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} masters={[master]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.type(screen.getByLabelText('Password (Kite Login)'), 'secret_pass_123')
    await userEvent.type(screen.getByLabelText('TOTP Secret Key'), 'TOTPSECRET123')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        password: 'secret_pass_123',
        totpSecret: 'TOTPSECRET123',
      }),
    )
  })

  it('edit mode: displays auth-error banner when authStatus is error', () => {
    const authErrorAccount: Account = {
      ...master,
      authStatus: 'error',
      authError: 'Invalid 2FA TOTP code',
    }
    render(() => (
      <AccountDetailDrawer open account={authErrorAccount} masters={[]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    expect(screen.getByRole('alert')).toHaveTextContent('Authentication required: Invalid 2FA TOTP code')
  })
})
