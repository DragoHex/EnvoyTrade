import { render, screen, waitFor, fireEvent } from '@solidjs/testing-library'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AdminUsersPage } from './AdminUsersPage'
import * as api from '../../api'

function renderUsersPage() {
  window.history.pushState({}, '', '/admin/users')
  return render(() => (
    <Router>
      <Route path="/admin/status" component={() => <div>status</div>} />
      <Route path="/admin/ips" component={() => <div>ips</div>} />
      <Route path="/admin/users" component={AdminUsersPage} />
    </Router>
  ))
}

const mockUsersResponse: api.AdminUsersResponse = {
  users: [
    {
      id: '11111111-1111-1111-1111-111111111111',
      email: 'admin@envoytrade.com',
      username: 'adminuser',
      name: 'Super Admin',
      role: 'admin',
      active: true,
      accountCount: 2,
      sessionCount: 1,
      createdAt: '2026-10-01T00:00:00Z',
      updatedAt: '2026-10-01T00:00:00Z',
    },
    {
      id: '22222222-2222-2222-2222-222222222222',
      email: 'trader@envoytrade.com',
      username: 'traderuser',
      name: 'Lead Trader',
      role: 'user',
      active: true,
      accountCount: 5,
      sessionCount: 2,
      createdAt: '2026-10-02T00:00:00Z',
      updatedAt: '2026-10-02T00:00:00Z',
    },
  ],
  total: 2,
  limit: 20,
  offset: 0,
}

describe('AdminUsersPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders user directory and role badges', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })

    expect(screen.getByText('traderuser')).toBeInTheDocument()
    expect(screen.getByText('Total Users: 2')).toBeInTheDocument()
    expect(screen.getByText('ADMIN')).toBeInTheDocument()
    expect(screen.getByText('USER')).toBeInTheDocument()
  })

  it('disables active status switch for admin accounts and allows toggling for regular users', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)
    const patchSpy = vi.spyOn(api, 'patchAdminUser').mockResolvedValue({ status: 'ok' })

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })

    const checkboxes = screen.getAllByRole('checkbox')
    // First user is admin - checkbox should be disabled
    expect(checkboxes[0]).toBeDisabled()
    fireEvent.click(checkboxes[0])
    expect(patchSpy).not.toHaveBeenCalled()

    // Second user is regular trader - checkbox should be enabled
    expect(checkboxes[1]).not.toBeDisabled()
    fireEvent.click(checkboxes[1])

    await waitFor(() => {
      expect(patchSpy).toHaveBeenCalledWith('22222222-2222-2222-2222-222222222222', { active: false })
    })
  })

  it('renders links to user accounts page for each user', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })

    const userLink = screen.getByRole('link', { name: 'adminuser' })
    expect(userLink).toHaveAttribute('href', '/admin/users/11111111-1111-1111-1111-111111111111/accounts')
  })

  it('opens sessions inspection modal and revokes sessions', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)
    const getSessionsSpy = vi.spyOn(api, 'getAdminUserSessions').mockResolvedValue({
      sessions: [
        {
          id: 1,
          tokenHash: 'hash1',
          userId: '22222222-2222-2222-2222-222222222222',
          ipAddress: '192.168.1.50',
          userAgent: 'Mozilla/5.0 Chrome/120',
          createdAt: new Date().toISOString(),
          lastSeenAt: new Date().toISOString(),
          expiresAt: new Date().toISOString(),
        },
      ],
    })
    const revokeSpy = vi.spyOn(api, 'deleteAdminUserSessions').mockResolvedValue({
      status: 'ok',
      revokedCount: 1,
    })

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('traderuser')).toBeInTheDocument()
    })

    // Click sessions button
    const sessionBtns = screen.getAllByRole('button', { name: /Inspect Sessions/i })
    fireEvent.click(sessionBtns[1])

    await waitFor(() => {
      expect(getSessionsSpy).toHaveBeenCalledWith('22222222-2222-2222-2222-222222222222')
      expect(screen.getByText(/192.168.1.50/i)).toBeInTheDocument()
    })

    // Revoke all sessions
    const revokeBtn = screen.getByRole('button', { name: /Revoke All Sessions/i })
    fireEvent.click(revokeBtn)

    await waitFor(() => {
      expect(revokeSpy).toHaveBeenCalledWith('22222222-2222-2222-2222-222222222222')
    })
  })

  it('renders top metric cards calculating active, suspended, admins, and accounts count', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })

    // Metric cards should display aggregate stats
    expect(screen.getByText('Active Traders')).toBeInTheDocument()
    expect(screen.getByText('Suspended Users')).toBeInTheDocument()
    expect(screen.getByText('Admins')).toBeInTheDocument()
    expect(screen.getByText('Accounts Managed')).toBeInTheDocument()
  })

  it('filters users in-memory by search input across username, email, and name without re-fetching API', async () => {
    const getSpy = vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })
    expect(screen.getByText('traderuser')).toBeInTheDocument()
    expect(getSpy).toHaveBeenCalledTimes(1)

    // Type in search bar
    const searchInput = screen.getByPlaceholderText(/search users/i)
    fireEvent.input(searchInput, { target: { value: 'lead trader' } })

    // Lead Trader should remain, adminuser should be filtered out
    expect(screen.getByText('traderuser')).toBeInTheDocument()
    expect(screen.queryByText('adminuser')).not.toBeInTheDocument()

    // No extra network calls
    expect(getSpy).toHaveBeenCalledTimes(1)

    // Search by username
    fireEvent.input(searchInput, { target: { value: 'adminuser' } })
    expect(screen.getByText('adminuser')).toBeInTheDocument()
    expect(screen.queryByText('traderuser')).not.toBeInTheDocument()

    // Clear search
    fireEvent.input(searchInput, { target: { value: '' } })
    expect(screen.getByText('adminuser')).toBeInTheDocument()
    expect(screen.getByText('traderuser')).toBeInTheDocument()
  })

  it('filters users in-memory by role and status dropdowns', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })

    const roleSelect = screen.getByLabelText(/filter by role/i)
    fireEvent.change(roleSelect, { target: { value: 'admin' } })

    expect(screen.getByText('adminuser')).toBeInTheDocument()
    expect(screen.queryByText('traderuser')).not.toBeInTheDocument()

    fireEvent.change(roleSelect, { target: { value: 'user' } })
    expect(screen.queryByText('adminuser')).not.toBeInTheDocument()
    expect(screen.getByText('traderuser')).toBeInTheDocument()
  })

  it('shows empty filter state when no user matches search query', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('adminuser')).toBeInTheDocument()
    })

    const searchInput = screen.getByPlaceholderText(/search users/i)
    fireEvent.input(searchInput, { target: { value: 'nonexistent_trader_xyz' } })

    expect(screen.getByText(/no platform users match the current filter/i)).toBeInTheDocument()
    expect(screen.queryByText('adminuser')).not.toBeInTheDocument()
  })

  it('opens reset password modal and validates password confirmation', async () => {
    vi.spyOn(api, 'getAdminUsers').mockResolvedValue(mockUsersResponse)

    renderUsersPage()

    await waitFor(() => {
      expect(screen.getByText('traderuser')).toBeInTheDocument()
    })

    const resetBtns = screen.getAllByRole('button', { name: /Reset Password/i })
    expect(resetBtns.length).toBeGreaterThan(0)
    fireEvent.click(resetBtns[0])

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /Reset Password/i })).toBeInTheDocument()
    })

    const newPassInput = screen.getByLabelText(/^New Password/i)
    const confirmPassInput = screen.getByLabelText(/^Confirm Password/i)
    const submitBtn = screen.getByRole('button', { name: /^Set Password$/i })

    // Mismatched passwords
    fireEvent.input(newPassInput, { target: { value: 'SecretPassword123' } })
    fireEvent.input(confirmPassInput, { target: { value: 'DifferentPassword' } })
    fireEvent.click(submitBtn)

    expect(screen.getByText(/passwords do not match/i)).toBeInTheDocument()

    // Matching passwords
    fireEvent.input(confirmPassInput, { target: { value: 'SecretPassword123' } })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      expect(screen.getByText(/password updated successfully/i)).toBeInTheDocument()
    })
  })
})
