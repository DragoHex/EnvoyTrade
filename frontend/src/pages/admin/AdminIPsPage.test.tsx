import { render, screen, waitFor, fireEvent } from '@solidjs/testing-library'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AdminIPsPage } from './AdminIPsPage'
import * as api from '../../api'

function renderIPsPage() {
  window.history.pushState({}, '', '/admin/ips')
  return render(() => (
    <Router>
      <Route path="/admin/status" component={() => <div>status</div>} />
      <Route path="/admin/ips" component={AdminIPsPage} />
      <Route path="/admin/users" component={() => <div>users</div>} />
    </Router>
  ))
}

const mockIPsResponse: api.AdminProxyIPsResponse = {
  ips: [
    {
      ipAddress: '148.113.41.42',
      ipType: 'ipv4',
      host: 'dc46-mum-01.algoip.in',
      port: 443,
      username: 'algo_user_1',
      validFrom: '2026-10-06T00:00:00Z',
      validUntil: '2027-01-06T00:00:00Z',
      plan: 'QUARTERLY',
      isAssigned: false,
      daysRemaining: 86,
      expiryStatus: 'active',
    },
    {
      ipAddress: '2401:c080::1',
      ipType: 'ipv6',
      host: 'dc6-mum-01.algoip.in',
      port: 443,
      validFrom: '2026-10-06T00:00:00Z',
      validUntil: '2026-10-18T00:00:00Z',
      plan: 'QUARTERLY',
      isAssigned: true,
      assignedAccountId: 'acc-1',
      assignedAccountName: 'Master Account',
      daysRemaining: 6,
      expiryStatus: 'expiring_soon',
    },
  ],
  stats: {
    totalCount: 2,
    availableCount: 1,
    assignedCount: 1,
    ipv4Count: 1,
    ipv6Count: 1,
    expiringSoonCount: 1,
    expiredCount: 0,
    assignedExpiring: 1,
  },
}

describe('AdminIPsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders top metric cards and IP inventory table', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('148.113.41.42')).toBeInTheDocument()
    })

    expect(screen.getByText('2401:c080::1')).toBeInTheDocument()
    expect(screen.getByText('Available IPs')).toBeInTheDocument()
    expect(screen.getByText('Expires in 86d')).toBeInTheDocument()
    expect(screen.getByText('Expires in 6d')).toBeInTheDocument()
  })

  it('probes a single IP on test button click', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const testSpy = vi.spyOn(api, 'testAdminProxyIP').mockResolvedValue({
      ipAddress: '148.113.41.42',
      success: true,
      latencyMs: 38,
      egressIp: '148.113.41.42',
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('148.113.41.42')).toBeInTheDocument()
    })

    const probeButtons = screen.getAllByRole('button', { name: /Test connectivity/i })
    fireEvent.click(probeButtons[0])

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith('148.113.41.42')
    })
  })

  it('opens bulk ingest modal and submits parsed JSON', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const bulkSpy = vi.spyOn(api, 'postAdminProxyIPsBulk').mockResolvedValue({
      total: 1,
      inserted: 1,
      updated: 0,
      errors: [],
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Bulk Ingest' })).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Bulk Ingest' }))

    const textarea = screen.getByPlaceholderText(/\[ { "ip": "2401:.../i)
    fireEvent.input(textarea, {
      target: {
        value: JSON.stringify([
          {
            ip: '103.21.58.1',
            host: 'dc6.algoip.in',
            port: 443,
            user_id: 'u1',
            password: 'p1',
          },
        ]),
      },
    })

    const submitBtn = screen.getByRole('button', { name: 'Ingest Batch' })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      expect(bulkSpy).toHaveBeenCalled()
    })
  })

  it('probes all assigned proxies on Test All Proxies button click', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const testAllSpy = vi.spyOn(api, 'testAllAdminProxyIPs').mockResolvedValue({
      totalTested: 1,
      successCount: 1,
      failureCount: 0,
      avgLatencyMs: 45,
      results: [
        {
          ipAddress: '2401:c080::1',
          success: true,
          latencyMs: 45,
          egressIp: '2401:c080::1',
        },
      ],
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('⚡ Test All Proxies')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('⚡ Test All Proxies'))

    await waitFor(() => {
      expect(testAllSpy).toHaveBeenCalled()
      expect(screen.getByText(/Tested 1 proxies — 1 succeeded, 0 failed/i)).toBeInTheDocument()
    })
  })

  it('paginates proxy IPs table at 20 items per page and resets on filter change', async () => {
    const manyIPs: api.AdminProxyIP[] = Array.from({ length: 25 }, (_, i) => ({
      ipAddress: `10.0.0.${i + 1}`,
      ipType: 'ipv4',
      host: 'dc1.algoip.in',
      port: 443,
      validFrom: '2026-10-01T00:00:00Z',
      validUntil: '2027-01-01T00:00:00Z',
      plan: 'QUARTERLY',
      isAssigned: false,
      daysRemaining: 70,
      expiryStatus: 'active',
    }))

    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue({
      ips: manyIPs,
      stats: {
        totalCount: 25,
        availableCount: 25,
        assignedCount: 0,
        ipv4Count: 25,
        ipv6Count: 0,
        expiringSoonCount: 0,
        expiredCount: 0,
        assignedExpiring: 0,
      },
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
      expect(screen.getByText('10.0.0.20')).toBeInTheDocument()
      expect(screen.queryByText('10.0.0.21')).not.toBeInTheDocument()
      expect(screen.getByText(/Showing 1 – 20 of 25 IPs/i)).toBeInTheDocument()
    })

    const nextBtn = screen.getByRole('button', { name: /^Next$/i })
    expect(nextBtn).toBeEnabled()

    fireEvent.click(nextBtn)

    await waitFor(() => {
      expect(screen.getByText('10.0.0.21')).toBeInTheDocument()
      expect(screen.getByText('10.0.0.25')).toBeInTheDocument()
      expect(screen.queryByText('10.0.0.1')).not.toBeInTheDocument()
      expect(screen.getByText(/Showing 21 – 25 of 25 IPs/i)).toBeInTheDocument()
    })

    // Filter change resets page to 1
    const filterBtn = screen.getByRole('button', { name: /Active/i })
    fireEvent.click(filterBtn)

    await waitFor(() => {
      expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
      expect(screen.getByText(/Showing 1 – 20 of 25 IPs/i)).toBeInTheDocument()
    })
  })

  it('filters IPs in-memory on filter pill click without re-fetching from API', async () => {
    const mixedIPs: api.AdminProxyIP[] = [
      {
        ipAddress: '10.0.0.1',
        ipType: 'ipv4',
        host: 'dc1.algoip.in',
        port: 443,
        validFrom: '2026-10-01T00:00:00Z',
        validUntil: '2027-01-01T00:00:00Z',
        plan: 'QUARTERLY',
        isAssigned: false,
        daysRemaining: 70,
        expiryStatus: 'active',
      },
      {
        ipAddress: '10.0.0.2',
        ipType: 'ipv4',
        host: 'dc1.algoip.in',
        port: 443,
        validFrom: '2026-08-01T00:00:00Z',
        validUntil: '2026-09-01T00:00:00Z',
        plan: 'MONTHLY',
        isAssigned: false,
        daysRemaining: 0,
        expiryStatus: 'expired',
      },
      {
        ipAddress: '2401:c080::1',
        ipType: 'ipv6',
        host: 'dc2.algoip.in',
        port: 443,
        validFrom: '2026-10-01T00:00:00Z',
        validUntil: '2026-10-25T00:00:00Z',
        plan: 'QUARTERLY',
        isAssigned: false,
        daysRemaining: 4,
        expiryStatus: 'expiring_soon',
      },
    ]

    const ipsSpy = vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue({
      ips: mixedIPs,
      stats: {
        totalCount: 3,
        availableCount: 3,
        assignedCount: 0,
        ipv4Count: 2,
        ipv6Count: 1,
        expiringSoonCount: 1,
        expiredCount: 1,
        assignedExpiring: 0,
      },
    })

    renderIPsPage()

    await waitFor(() => {
      expect(ipsSpy).toHaveBeenCalledTimes(1)
      expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
      expect(screen.getByText('10.0.0.2')).toBeInTheDocument()
      expect(screen.getByText('2401:c080::1')).toBeInTheDocument()
    })

    // Click "Active" filter — should filter in memory without calling getAdminProxyIPs again
    const activeBtn = screen.getByRole('button', { name: /^Active$/i })
    fireEvent.click(activeBtn)

    await waitFor(() => {
      expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
      expect(screen.queryByText('10.0.0.2')).not.toBeInTheDocument()
      expect(screen.queryByText('2401:c080::1')).not.toBeInTheDocument()
    })
    expect(ipsSpy).toHaveBeenCalledTimes(1)

    // Click "Expired" filter — should filter in memory without calling getAdminProxyIPs again
    const expiredBtn = screen.getByRole('button', { name: /^Expired$/i })
    fireEvent.click(expiredBtn)

    await waitFor(() => {
      expect(screen.getByText('10.0.0.2')).toBeInTheDocument()
      expect(screen.queryByText('10.0.0.1')).not.toBeInTheDocument()
      expect(screen.queryByText('2401:c080::1')).not.toBeInTheDocument()
    })
    expect(ipsSpy).toHaveBeenCalledTimes(1)

    // Click "IPv6" filter — should filter in memory without calling getAdminProxyIPs again
    const ipv6Btn = screen.getByRole('button', { name: /^IPv6$/i })
    fireEvent.click(ipv6Btn)

    await waitFor(() => {
      expect(screen.getByText('2401:c080::1')).toBeInTheDocument()
      expect(screen.queryByText('10.0.0.1')).not.toBeInTheDocument()
      expect(screen.queryByText('10.0.0.2')).not.toBeInTheDocument()
    })
    expect(ipsSpy).toHaveBeenCalledTimes(1)

    // Click Refresh button — should re-query API once
    const refreshBtn = screen.getByRole('button', { name: /Refresh/i })
    fireEvent.click(refreshBtn)

    await waitFor(() => {
      expect(ipsSpy).toHaveBeenCalledTimes(2)
    })
  })

  it('allows editing an IP by pasting individual IP JSON and validates matching IP', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const putSpy = vi.spyOn(api, 'putAdminProxyIP').mockResolvedValue({
      status: 'ok',
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('148.113.41.42')).toBeInTheDocument()
    })

    // Click edit button for first IP
    const editButtons = screen.getAllByRole('button', { name: /Edit \/ Renew IP/i })
    fireEvent.click(editButtons[0])

    // Verify Edit Modal opened
    await waitFor(() => {
      expect(screen.getByText(/Edit Proxy IP: 148.113.41.42/i)).toBeInTheDocument()
    })

    // Click "JSON Text" tab
    const pasteJsonTab = screen.getByRole('button', { name: /JSON Text|Paste JSON/i })
    fireEvent.click(pasteJsonTab)

    const textarea = screen.getByRole('textbox', { name: /Individual IP JSON/i }) as HTMLTextAreaElement
    const saveBtn = screen.getByRole('button', { name: 'Save Changes' })

    // No placeholder text should be shown
    expect(textarea.placeholder).toBe('')
    // Auto-filled payload should be populated initially
    expect(textarea.value).toContain('"ip": "148.113.41.42"')
    // Helper text is removed from the modal
    expect(screen.queryByText(/Paste JSON object for this proxy/i)).not.toBeInTheDocument()
    // Match badge is active and save button is enabled
    expect(screen.getByText(/IP matches \(148.113.41.42\)/i)).toBeInTheDocument()
    expect(saveBtn).not.toBeDisabled()

    // Edge case: when textarea is cleared to empty, no match/error badges and submit is disabled
    fireEvent.input(textarea, { target: { value: '   ' } })
    expect(screen.queryByText(/Paste JSON object for this proxy/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/IP matches/i)).not.toBeInTheDocument()
    expect(saveBtn).toBeDisabled()

    // Test negative: paste mismatched IP
    fireEvent.input(textarea, {
      target: {
        value: JSON.stringify({
          ip: '10.99.99.99',
          host: 'dc1.algoip.in',
          port: 443,
        }),
      },
    })

    // Should display mismatch warning and disable submit
    expect(screen.queryByText(/Paste JSON object for this proxy/i)).not.toBeInTheDocument()
    expect(screen.getByText(/IP in JSON \(10.99.99.99\) does not match editing IP \(148.113.41.42\)/i)).toBeInTheDocument()
    expect(saveBtn).toBeDisabled()

    // Test positive: paste updated matching IP JSON
    fireEvent.input(textarea, {
      target: {
        value: JSON.stringify({
          ip: '148.113.41.42',
          host: 'new-host.algoip.in',
          port: 8443,
          plan: 'ANNUAL',
          type: 'IPV4',
        }),
      },
    })

    // Should display match checkmark and enable submit
    expect(screen.getByText(/IP matches \(148.113.41.42\)/i)).toBeInTheDocument()
    expect(saveBtn).not.toBeDisabled()

    // Submit
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(putSpy).toHaveBeenCalledWith('148.113.41.42', expect.objectContaining({
        ip: '148.113.41.42',
        host: 'new-host.algoip.in',
        port: 8443,
        plan: 'ANNUAL',
      }))
    })
  })

  it('allows adding a new proxy IP by pasting individual IP JSON in Add modal', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const postSpy = vi.spyOn(api, 'postAdminProxyIP').mockResolvedValue({
      status: 'ok',
      ip: mockIPsResponse.ips[0],
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('+ Add IP')).toBeInTheDocument()
    })

    // Click "+ Add IP"
    fireEvent.click(screen.getByText('+ Add IP'))

    const addDialog = screen.getByRole('dialog', { name: 'Add Proxy IP' })
    expect(addDialog).toBeInTheDocument()

    // Find and click the JSON Text tab in Add modal
    const jsonTab = screen.getByRole('button', { name: /JSON Text|Paste JSON/i })
    fireEvent.click(jsonTab)

    // The individual IP JSON textarea in Add modal
    const textarea = screen.getByRole('textbox', { name: /Individual IP JSON/i }) as HTMLTextAreaElement
    const addBtn = screen.getByRole('button', { name: 'Add Proxy IP' })

    // Negative case: empty textarea disables Add button
    fireEvent.input(textarea, { target: { value: '' } })
    expect(addBtn).toBeDisabled()

    // Negative case: invalid JSON syntax disables Add button and shows warning
    fireEvent.input(textarea, { target: { value: '{ invalid: json ' } })
    expect(screen.getByText(/Invalid JSON syntax/i)).toBeInTheDocument()
    expect(addBtn).toBeDisabled()

    // Negative case: array instead of single object
    fireEvent.input(textarea, { target: { value: '[{ "ip": "1.2.3.4" }]' } })
    expect(screen.getByText(/JSON must be an individual proxy IP object/i)).toBeInTheDocument()
    expect(addBtn).toBeDisabled()

    // Positive case: valid individual IP JSON
    fireEvent.input(textarea, {
      target: {
        value: JSON.stringify({
          ip: '103.21.58.99',
          host: 'dc6-mum-05.algoip.in',
          port: 8443,
          user_id: 'aip_json_user',
          password: 'secret_json_password',
          valid_until: '2027-12-31 23:59:59 (IST)',
          plan: 'YEARLY',
          type: 'IPV4',
        }),
      },
    })

    // Validation badge appears and button enables
    expect(screen.getByText(/Valid JSON \(103.21.58.99\)/i)).toBeInTheDocument()
    expect(addBtn).not.toBeDisabled()

    // Submit
    fireEvent.click(addBtn)

    await waitFor(() => {
      expect(postSpy).toHaveBeenCalledWith(expect.objectContaining({
        ip: '103.21.58.99',
        host: 'dc6-mum-05.algoip.in',
        port: 8443,
        user_id: 'aip_json_user',
        plan: 'YEARLY',
      }))
    })
  })

  it('submits new proxy IP with selected subscription plan in Add modal', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const postSpy = vi.spyOn(api, 'postAdminProxyIP').mockResolvedValue({
      status: 'ok',
      ip: mockIPsResponse.ips[0],
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('+ Add IP')).toBeInTheDocument()
    })

    // Click "+ Add IP"
    fireEvent.click(screen.getByText('+ Add IP'))

    // Verify modal is open
    expect(screen.getByRole('dialog', { name: 'Add Proxy IP' })).toBeInTheDocument()

    // Fill form fields
    const ipInput = screen.getByPlaceholderText(/e\.g\. 103\.21\.58\.10/i)
    fireEvent.input(ipInput, { target: { value: '103.21.58.50' } })

    const hostInput = screen.getByLabelText(/^Gateway Host$/i)
    fireEvent.input(hostInput, { target: { value: 'dc6-mum-02.algoip.in' } })

    const portInput = screen.getByLabelText(/^Port$/i)
    fireEvent.input(portInput, { target: { value: '8443' } })

    const userInput = screen.getByPlaceholderText('aip_live_...')
    fireEvent.input(userInput, { target: { value: 'aip_live_newuser' } })

    const passInput = screen.getByPlaceholderText('aip_sec_...')
    fireEvent.input(passInput, { target: { value: 'secretpass123' } })

    const validUntilInput = screen.getByLabelText(/^Valid Until$/i)
    fireEvent.input(validUntilInput, { target: { value: '2027-10-21' } })

    // Change Subscription Plan dropdown from default QUARTERLY to YEARLY
    const planSelect = screen.getByLabelText(/^Subscription Plan$/i)
    fireEvent.change(planSelect, { target: { value: 'YEARLY' } })

    // Submit
    const submitBtn = screen.getByRole('button', { name: 'Add Proxy IP' })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      expect(postSpy).toHaveBeenCalledWith(expect.objectContaining({
        ip: '103.21.58.50',
        host: 'dc6-mum-02.algoip.in',
        port: 8443,
        user_id: 'aip_live_newuser',
        password: 'secretpass123',
        plan: 'YEARLY',
        type: 'IPV4',
      }))
    })
  })

  it('edits proxy IP host, port, username, plan and preserves password in Form Fields mode', async () => {
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(mockIPsResponse)
    const putSpy = vi.spyOn(api, 'putAdminProxyIP').mockResolvedValue({
      status: 'ok',
    })

    renderIPsPage()

    await waitFor(() => {
      expect(screen.getByText('148.113.41.42')).toBeInTheDocument()
    })

    // Click edit button for first IP
    const editButtons = screen.getAllByRole('button', { name: /Edit \/ Renew IP/i })
    fireEvent.click(editButtons[0])

    await waitFor(() => {
      expect(screen.getByText(/Edit Proxy IP: 148.113.41.42/i)).toBeInTheDocument()
    })

    // Active tab is Form Fields by default
    const formTab = screen.getByRole('button', { name: 'Form Fields' })
    expect(formTab).toHaveClass('active')

    // Check pre-populated values
    const hostInput = screen.getByLabelText(/^Gateway Host$/i) as HTMLInputElement
    expect(hostInput.value).toBe('dc46-mum-01.algoip.in')

    const portInput = screen.getByLabelText(/^Port$/i) as HTMLInputElement
    expect(portInput.value).toBe('443')

    const userInput = screen.getByLabelText(/^Username$/i) as HTMLInputElement
    expect(userInput.value).toBe('algo_user_1')

    const validUntilInput = screen.getByLabelText(/^Valid Until$/i) as HTMLInputElement
    expect(validUntilInput.value).toBe('2027-01-06')

    const planSelect = screen.getByLabelText(/^Subscription Plan$/i) as HTMLSelectElement
    expect(planSelect.value).toBe('QUARTERLY')

    const passInput = screen.getByPlaceholderText(/Leave blank to keep existing password/i) as HTMLInputElement
    expect(passInput.value).toBe('')

    // Modify fields
    fireEvent.input(hostInput, { target: { value: 'dc46-mum-02.algoip.in' } })
    fireEvent.input(portInput, { target: { value: '8443' } })
    fireEvent.input(userInput, { target: { value: 'algo_user_2' } })
    fireEvent.change(planSelect, { target: { value: 'YEARLY' } })

    // Save changes
    const saveBtn = screen.getByRole('button', { name: 'Save Changes' })
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(putSpy).toHaveBeenCalledWith('148.113.41.42', expect.objectContaining({
        ip: '148.113.41.42',
        host: 'dc46-mum-02.algoip.in',
        port: 8443,
        user_id: 'algo_user_2',
        plan: 'YEARLY',
        type: 'IPV4',
      }))
    })
  })

  it('renders Whitelisted Brokers column and handles whitelisted_brokers in Add and Edit modals', async () => {
    const customIPsResponse: api.AdminProxyIPsResponse = {
      ips: [
        {
          ...mockIPsResponse.ips[0],
          whitelistedBrokers: 'Zerodha',
        },
        {
          ...mockIPsResponse.ips[1],
          whitelistedBrokers: 'TestBroker',
        },
      ],
      stats: mockIPsResponse.stats,
    }
    vi.spyOn(api, 'getAdminProxyIPs').mockResolvedValue(customIPsResponse)
    const postSpy = vi.spyOn(api, 'postAdminProxyIP').mockResolvedValue({
      status: 'ok',
      ip: customIPsResponse.ips[0],
    })
    const putSpy = vi.spyOn(api, 'putAdminProxyIP').mockResolvedValue({
      status: 'ok',
      ip: customIPsResponse.ips[0],
    })

    renderIPsPage()

    // 1. Verify table header and broker badges
    await waitFor(() => {
      expect(screen.getByText('Whitelisted Brokers')).toBeInTheDocument()
    })
    expect(screen.getByText('Zerodha')).toBeInTheDocument()
    expect(screen.getByText('TestBroker')).toBeInTheDocument()

    // 2. Open Add IP modal and check Whitelisted Brokers field
    const addBtn = screen.getByRole('button', { name: '+ Add IP' })
    fireEvent.click(addBtn)

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Add Proxy IP' })).toBeInTheDocument()
    })

    const addBrokerSelect = screen.getByLabelText(/^Whitelisted Brokers$/i) as HTMLSelectElement
    expect(addBrokerSelect.value).toBe('Zerodha') // default
    fireEvent.change(addBrokerSelect, { target: { value: 'TestBroker' } })

    const ipInput = screen.getByPlaceholderText(/e\.g\. 103\.21\.58\.10/i)
    fireEvent.input(ipInput, { target: { value: '148.113.41.99' } })

    const userInput = screen.getByPlaceholderText('aip_live_...')
    fireEvent.input(userInput, { target: { value: 'user_test' } })

    const passInput = screen.getByPlaceholderText('aip_sec_...')
    fireEvent.input(passInput, { target: { value: 'secretpass123' } })

    const validUntilInput = screen.getByLabelText(/^Valid Until$/i)
    fireEvent.input(validUntilInput, { target: { value: '2027-10-21' } })

    const submitAddBtn = screen.getByRole('button', { name: 'Add Proxy IP' })
    fireEvent.click(submitAddBtn)

    await waitFor(() => {
      expect(postSpy).toHaveBeenCalledWith(expect.objectContaining({
        ip: '148.113.41.99',
        whitelisted_brokers: 'TestBroker',
      }))
    })

    // 3. Open Edit modal and verify pre-populated Whitelisted Broker
    const editButtons = screen.getAllByRole('button', { name: /Edit \/ Renew IP/i })
    fireEvent.click(editButtons[0])

    await waitFor(() => {
      expect(screen.getByText(/Edit Proxy IP: 148.113.41.42/i)).toBeInTheDocument()
    })

    const editBrokerSelect = screen.getByLabelText(/^Whitelisted Brokers$/i) as HTMLSelectElement
    expect(editBrokerSelect.value).toBe('Zerodha')
    fireEvent.change(editBrokerSelect, { target: { value: 'Zerodha, TestBroker' } })

    const saveEditBtn = screen.getByRole('button', { name: 'Save Changes' })
    fireEvent.click(saveEditBtn)

    await waitFor(() => {
      expect(putSpy).toHaveBeenCalledWith('148.113.41.42', expect.objectContaining({
        whitelisted_brokers: 'Zerodha, TestBroker',
      }))
    })
  })
})

