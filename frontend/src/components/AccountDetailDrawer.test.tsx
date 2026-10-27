import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AccountDetailDrawer } from './AccountDetailDrawer'
import type { Account } from '../api'
import * as api from '../api'

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
  ip: null,
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
  ip: '192.168.1.20',
}

const mockGroup: api.GroupSummary = {
  id: 'g1',
  name: 'Alpha Group',
  masterId: 'm1',
  masterAccountId: 'ZX1234',
  masterName: 'Master Account',
  broker: 'kite',
  followerCount: 0,
  status: 'ok',
}

describe('AccountDetailDrawer', () => {
  it('create mode, role=master: does not show group/cloneFactor/maxQtyPerOrder fields, submits createAccount', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    expect(screen.queryByLabelText('Clone Factor')).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/Group/)).not.toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ role: 'master', broker: 'kite', brokerAccountId: 'ZX9999', apiKey: 'key', apiSecret: 'secret' }),
    )
  })

  it('create mode, role=follower: defaults to None (Unassigned) group, allows creation without group', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    expect(screen.getByLabelText('Role')).toHaveValue('follower')
    const groupSelect = screen.getByLabelText(/Group/)
    expect(groupSelect).toBeInTheDocument()
    expect(groupSelect).toHaveValue('') // None (Unassigned)

    // When unassigned, cloneFactor and maxQty are hidden
    expect(screen.queryByLabelText('Clone Factor')).not.toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.type(screen.getByLabelText(/IP Address/), '192.168.1.50')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'follower', brokerAccountId: 'ZY9999', ip: '192.168.1.50',
      }),
    )
  })

  it('create mode, role=follower with group: shows GroupName (MasterName) format and submits groupId and optional maxQty', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    const groupSelect = screen.getByLabelText(/Group/)
    expect(screen.getByText('Alpha Group (Master Account)')).toBeInTheDocument()

    await userEvent.selectOptions(groupSelect, 'g1')
    expect(screen.getByLabelText('Clone Factor')).toHaveValue('1')

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key')
    await userEvent.type(screen.getByLabelText('API Secret'), 'secret')
    await userEvent.type(screen.getByLabelText(/IP Address/), '192.168.1.50')
    await userEvent.clear(screen.getByLabelText('Clone Factor'))
    await userEvent.type(screen.getByLabelText('Clone Factor'), '0.5')
    await userEvent.type(screen.getByLabelText(/Max Qty\/Order/), '100')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'follower', brokerAccountId: 'ZY9999', groupId: 'g1', cloneFactor: '0.5', maxQtyPerOrder: 100, ip: '192.168.1.50',
      }),
    )
  })

  it('edit mode: role is locked, submits only changed fields via onSave', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const followerWithGroup: Account = { ...follower, groupId: 'g1' }
    render(() => (
      <AccountDetailDrawer open account={followerWithGroup} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
    ))

    expect(screen.getByLabelText('Role')).toBeDisabled()
    expect(screen.queryByLabelText('Status')).not.toBeInTheDocument()

    await userEvent.clear(screen.getByLabelText('Clone Factor'))
    await userEvent.type(screen.getByLabelText('Clone Factor'), '0.75')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onSave).toHaveBeenCalledWith('f1', { cloneFactor: '0.75' })
  })

  it('edit mode: allows changing or detaching group', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    const followerWithGroup: Account = { ...follower, groupId: 'g1' }
    render(() => (
      <AccountDetailDrawer open account={followerWithGroup} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
    ))

    const groupSelect = screen.getByLabelText(/Group/)
    await userEvent.selectOptions(groupSelect, '') // detach
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onSave).toHaveBeenCalledWith('f1', { groupId: '' })
  })
  it('create mode, role=follower without IP: shows validation error', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('IP Address is required for follower accounts.')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('create mode, role=follower with invalid IP: shows validation error', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZY9999')
    await userEvent.type(screen.getByLabelText(/IP Address/), 'not-an-ip')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid IP Address: must be a valid IPv4 or IPv6 address.')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('create mode, role=master with optional valid IP: submits with ip', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
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
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText(/IP Address/), '999.999.999.999')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid IP Address: must be a valid IPv4 or IPv6 address.')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('create mode, broker=kite: shows validation error when API secret is 31 characters', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    await userEvent.type(screen.getByLabelText('Broker User ID'), 'ZX9999')
    await userEvent.type(screen.getByLabelText('API Key'), 'key1234567890123')
    await userEvent.type(screen.getByLabelText('API Secret'), '1234567890123456789012345678901') // 31 chars
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Zerodha API Secret must be exactly 32 characters (currently 31 characters).')
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('edit mode: role is locked, submits changed IP and cloneFactor via onSave', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={follower} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
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
      <AccountDetailDrawer open account={accountWithCreds} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={onSave} />
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
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
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
      <AccountDetailDrawer open account={authErrorAccount} groups={[]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    expect(screen.getByRole('alert')).toHaveTextContent('Authentication required: Invalid 2FA TOTP code')
  })

  it('create mode, role=follower with available proxy IPs: auto-assigns IPv4, and switches to IPv6 on dropdown select', async () => {
    vi.spyOn(api, 'fetchAvailableProxyIPs').mockResolvedValue({
      ipv4: [
        {
          ipAddress: '148.113.41.42',
          ipType: 'ipv4',
          host: 'dc46-mum-01.algoip.in',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'QUARTERLY',
        },
      ],
      ipv6: [
        {
          ipAddress: '2402:1f00:8302:91e6:6d08:9249:eca8:8252',
          ipType: 'ipv6',
          host: 'dc46-mum-01.algoip.in',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'QUARTERLY',
        },
      ],
    })

    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    // Wait for async fetch to populate IP
    const ipInput = await screen.findByLabelText(/IP Address/)
    expect(ipInput).toHaveValue('148.113.41.42')

    // Switch to IPv6
    const ipTypeSelect = screen.getByLabelText('IP Type')
    await userEvent.selectOptions(ipTypeSelect, 'ipv6')
    expect(ipInput).toHaveValue('2402:1f00:8302:91e6:6d08:9249:eca8:8252')
  })

  it('create mode, role=master: shows NA option in IP Type and sets IP to empty', async () => {
    vi.spyOn(api, 'fetchAvailableProxyIPs').mockResolvedValue({
      ipv4: [{ ipAddress: '148.113.41.42', ipType: 'ipv4', host: 'dc46-mum-01.algoip.in', port: 443, validFrom: '', validUntil: '', plan: 'QUARTERLY' }],
      ipv6: [],
    })

    render(() => (
      <AccountDetailDrawer open account={null} groups={[]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    const ipTypeSelect = screen.getByLabelText('IP Type')
    expect(ipTypeSelect).toHaveValue('na')

    const ipInput = screen.getByLabelText(/IP Address/)
    expect(ipInput).toHaveValue('')

    // Selecting IPv4 sets the available IPv4
    // Selecting NA clears it
    await userEvent.selectOptions(ipTypeSelect, 'na')
    expect(ipInput).toHaveValue('')
  })

  it('create mode, broker=dhan: does not show API Key or API Secret, shows Dhan Client ID and Access Token, submits without apiKey/apiSecret', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={onCreate} onSave={vi.fn()} />
    ))

    // Select Dhan broker
    await userEvent.selectOptions(screen.getByLabelText('Broker'), 'dhan')
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('API Secret')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Password (Kite Login)')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('TOTP Secret Key')).not.toBeInTheDocument()

    expect(screen.getByLabelText('Dhan Client ID')).toBeInTheDocument()
    expect(screen.getByLabelText('Access Token (JWT)')).toBeInTheDocument()

    await userEvent.selectOptions(screen.getByLabelText('Role'), 'master')
    await userEvent.type(screen.getByLabelText('Dhan Client ID'), '1000000001')
    await userEvent.type(screen.getByLabelText('Access Token (JWT)'), 'dhan-jwt-token-123')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(onCreate).toHaveBeenCalledWith({
      name: '1000000001',
      role: 'master',
      broker: 'dhan',
      brokerAccountId: '1000000001',
      accessToken: 'dhan-jwt-token-123',
    })
  })

  it('filters and prioritizes broker-specialized proxy IPs over unrestricted IPs and allows pool selection', async () => {
    vi.spyOn(api, 'fetchAvailableProxyIPs').mockResolvedValue({
      ipv4: [
        {
          ipAddress: '10.0.0.1',
          ipType: 'ipv4',
          host: 'host1',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'MONTHLY',
          whitelistedBrokers: '', // All brokers / unrestricted (listed first in API)
        },
        {
          ipAddress: '10.0.0.2',
          ipType: 'ipv4',
          host: 'host2',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'MONTHLY',
          whitelistedBrokers: 'Zerodha', // Zerodha specialized
        },
        {
          ipAddress: '10.0.0.3',
          ipType: 'ipv4',
          host: 'host3',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'MONTHLY',
          whitelistedBrokers: 'Dhan', // Dhan specialized
        },
      ],
      ipv6: [],
    })

    // 1. By default broker is 'kite'. Should prioritize 10.0.0.2 (Zerodha specialized) over 10.0.0.1 (unrestricted)
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    const ipInput = await screen.findByLabelText(/IP Address/)
    expect(ipInput).toHaveValue('10.0.0.2')
    expect(ipInput).not.toHaveAttribute('list')

    // Confirm NO separate selector is rendered and NO native datalist
    expect(screen.queryByLabelText('Available Proxy IPs')).not.toBeInTheDocument()
    expect(document.getElementById('available-proxy-ips')).toBeNull()

    // 2. Open custom themed dropdown via toggle button
    const toggleBtn = screen.getByRole('button', { name: 'Toggle IP dropdown' })
    expect(toggleBtn).toBeInTheDocument()
    await userEvent.click(toggleBtn)

    const listbox = screen.getByRole('listbox', { name: 'Compatible Proxy IPs' })
    expect(listbox).toBeInTheDocument()

    const options = Array.from(listbox.querySelectorAll('[role="option"]'))
    expect(options.length).toBe(2)
    expect(options[0]).toHaveTextContent('10.0.0.2')
    expect(options[0]).toHaveTextContent('Zerodha Specialized')
    expect(options[1]).toHaveTextContent('10.0.0.1')
    expect(options[1]).toHaveTextContent('All Brokers')

    // Click 10.0.0.1 from the custom dropdown
    await userEvent.click(options[1])
    expect(ipInput).toHaveValue('10.0.0.1')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()

    // 3. Switch broker to Dhan: should auto-switch to 10.0.0.3 (Dhan specialized), excluding 10.0.0.2 (Zerodha)
    await userEvent.selectOptions(screen.getByLabelText('Broker'), 'dhan')
    expect(ipInput).toHaveValue('10.0.0.3')

    // Reopen dropdown to verify Dhan options
    await userEvent.click(toggleBtn)
    const dhanListbox = screen.getByRole('listbox', { name: 'Compatible Proxy IPs' })
    const dhanOptions = Array.from(dhanListbox.querySelectorAll('[role="option"]'))
    expect(dhanOptions.length).toBe(2)
    expect(dhanOptions[0]).toHaveTextContent('10.0.0.3')
    expect(dhanOptions[0]).toHaveTextContent('Dhan Specialized')
    expect(dhanOptions[1]).toHaveTextContent('10.0.0.1')
    expect(dhanOptions[1]).toHaveTextContent('All Brokers')

    // Press Escape to close dropdown
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('falls back to unrestricted IP when no specialized IP exists for the broker', async () => {
    vi.spyOn(api, 'fetchAvailableProxyIPs').mockResolvedValue({
      ipv4: [
        {
          ipAddress: '10.0.0.1',
          ipType: 'ipv4',
          host: 'host1',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'MONTHLY',
          whitelistedBrokers: '', // Only unrestricted IP available
        },
      ],
      ipv6: [],
    })

    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    const ipInput = await screen.findByLabelText(/IP Address/)
    expect(ipInput).toHaveValue('10.0.0.1')
  })

  it('strictly excludes other brokers specialized IPs and shows pool warning when none compatible', async () => {
    vi.spyOn(api, 'fetchAvailableProxyIPs').mockResolvedValue({
      ipv4: [
        {
          ipAddress: '10.0.0.3',
          ipType: 'ipv4',
          host: 'host3',
          port: 443,
          validFrom: '',
          validUntil: '',
          plan: 'MONTHLY',
          whitelistedBrokers: 'Dhan', // Only Dhan specialized IP available
        },
      ],
      ipv6: [],
    })

    // Create mode with broker='kite'
    render(() => (
      <AccountDetailDrawer open account={null} groups={[mockGroup]} onClose={vi.fn()} onCreate={vi.fn()} onSave={vi.fn()} />
    ))

    const ipInput = await screen.findByLabelText(/IP Address/)
    expect(ipInput).toHaveValue('')
    expect(screen.getByText('No available IPV4 address in proxy pool.')).toBeInTheDocument()
  })
})

