import { render, screen, waitFor, fireEvent } from '@solidjs/testing-library'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AdminUserAccountsPage } from './AdminUserAccountsPage'
import * as api from '../../api'

function renderUserAccountsPage(userId: string) {
  window.history.pushState({}, '', `/admin/users/${userId}/accounts`)
  return render(() => (
    <Router>
      <Route path="/admin/users" component={() => <div>users page</div>} />
      <Route path="/admin/users/:id/accounts" component={AdminUserAccountsPage} />
    </Router>
  ))
}

const mockUser: api.User = {
  id: 'user-1234',
  email: 'trader@envoytrade.com',
  username: 'traderbob',
  name: 'Bob Trader',
  role: 'user',
  active: true,
  phone: '+91 9876543210',
  address: 'Mumbai, Maharashtra',
  gstNumber: '27AABCU9603R1ZM',
}

const mockAccounts: api.Account[] = [
  {
    id: 'acct-1',
    name: 'Primary Zerodha Master',
    role: 'master',
    broker: 'kite',
    brokerAccountId: 'ZB1234',
    enabled: true,
    active: true,
    status: 'ok',
    groupName: 'Alpha Scalpers',
    ip: '103.21.58.10',
    authStatus: 'authenticated',
    masterId: null,
    cloneFactor: null,
    maxQtyPerOrder: null,
  },
  {
    id: 'acct-2',
    name: 'Follower Safe',
    role: 'follower',
    broker: 'kite',
    brokerAccountId: 'ZB9999',
    enabled: true,
    active: true,
    status: 'ok',
    groupName: 'Alpha Scalpers',
    ip: null,
    authStatus: 'error',
    authError: 'Token expired at 06:00',
    masterId: 'acct-1',
    cloneFactor: '1.5',
    maxQtyPerOrder: 100,
  },
]

describe('AdminUserAccountsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders user details and their trading accounts table', async () => {
    const apiSpy = vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(apiSpy).toHaveBeenCalledWith('user-1234')
      expect(screen.getByText('Bob Trader')).toBeInTheDocument()
    })

    expect(screen.getByText(/@traderbob/)).toBeInTheDocument()
    expect(screen.getByText(/trader@envoytrade.com/)).toBeInTheDocument()
    expect(screen.getByText("traderbob's Profile & Accounts")).toBeInTheDocument()
    expect(screen.getByText('Primary Zerodha Master')).toBeInTheDocument()
    expect(screen.getByText('ZB1234')).toBeInTheDocument()
    expect(screen.getByText('Follower Safe')).toBeInTheDocument()
    expect(screen.getByText('ZB9999')).toBeInTheDocument()
    expect(screen.getByText('103.21.58.10')).toBeInTheDocument()
    expect(screen.getByText('Direct')).toBeInTheDocument()

    const backLink = screen.getByRole('link', { name: /Back to Platform Users/i })
    expect(backLink).toHaveAttribute('href', '/admin/users')
  })

  it('renders enriched user profile metadata (phone, address, GSTIN)', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('Bob Trader')).toBeInTheDocument()
    })

    expect(screen.getByText('+91 9876543210')).toBeInTheDocument()
    expect(screen.getByText('Mumbai, Maharashtra')).toBeInTheDocument()
    expect(screen.getByText('27AABCU9603R1ZM')).toBeInTheDocument()
  })

  it('renders role icons (Master Gandalf / Follower Hobbit), auth status icons, and sizing terms', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('Primary Zerodha Master')).toBeInTheDocument()
    })

    // Role Icons
    expect(screen.getByRole('img', { name: 'Master' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Follower' })).toBeInTheDocument()

    // Auth Status Icons
    expect(screen.getByRole('img', { name: /Authenticated - Active Broker Session/i })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /Authentication Error: Token expired at 06:00/i })).toBeInTheDocument()

    // Sizing Terms
    expect(screen.getByText('1.5x')).toBeInTheDocument()
    expect(screen.getByText('100 qty')).toBeInTheDocument()
  })

  it('allows toggling copy enabled status on follower account', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })
    const patchSpy = vi.spyOn(api, 'patchAccount').mockResolvedValue({ status: 'ok' })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('Follower Safe')).toBeInTheDocument()
    })

    const copyToggle = screen.getByRole('checkbox', { name: /Toggle copy trading for Follower Safe/i })
    expect(copyToggle).toBeChecked()

    fireEvent.click(copyToggle)

    await waitFor(() => {
      expect(patchSpy).toHaveBeenCalledWith('acct-2', { enabled: false })
    })
  })

  it('probes proxy IP connectivity when clicking Test button next to assigned IP', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })
    const testSpy = vi.spyOn(api, 'testAdminProxyIP').mockResolvedValue({
      ipAddress: '103.21.58.10',
      success: true,
      latencyMs: 42,
    })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('103.21.58.10')).toBeInTheDocument()
    })

    const testBtn = screen.getByRole('button', { name: /Test Proxy 103.21.58.10/i })
    fireEvent.click(testBtn)

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith('103.21.58.10')
      expect(screen.getByText(/42ms/i)).toBeInTheDocument()
    })
  })

  it('renders "Failed" with error tooltip and nowrap styling on proxy probe failure', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })
    const testSpy = vi.spyOn(api, 'testAdminProxyIP').mockResolvedValue({
      ipAddress: '103.21.58.10',
      success: false,
      latencyMs: 0,
      error: 'connection timed out after 3000ms',
    })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('103.21.58.10')).toBeInTheDocument()
    })

    const testBtn = screen.getByRole('button', { name: /Test Proxy 103.21.58.10/i })
    fireEvent.click(testBtn)

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith('103.21.58.10')
      const failedEl = screen.getByText('Failed')
      expect(failedEl).toBeInTheDocument()
      expect(failedEl).toHaveAttribute('title', 'connection timed out after 3000ms')
      expect(failedEl).toHaveStyle({ 'white-space': 'nowrap' })
    })
  })

  it('switches to Active Sessions tab and allows revoking sessions', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: mockAccounts,
    })
    const sessionsSpy = vi.spyOn(api, 'getAdminUserSessions').mockResolvedValue({
      sessions: [
        {
          id: 101,
          tokenHash: 'tok123',
          userId: 'user-1234',
          ipAddress: '203.0.113.15',
          userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X)',
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

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('Bob Trader')).toBeInTheDocument()
    })

    // Click Active Sessions tab
    const sessionsTab = screen.getByRole('tab', { name: /Active Sessions/i })
    fireEvent.click(sessionsTab)

    await waitFor(() => {
      expect(sessionsSpy).toHaveBeenCalledWith('user-1234')
      expect(screen.getByText('203.0.113.15')).toBeInTheDocument()
      expect(screen.getByText(/Mozilla\/5.0/i)).toBeInTheDocument()
    })

    // Revoke sessions
    const revokeBtn = screen.getByRole('button', { name: /Revoke All Sessions/i })
    fireEvent.click(revokeBtn)

    await waitFor(() => {
      expect(revokeSpy).toHaveBeenCalledWith('user-1234')
      expect(screen.getByText(/No active sessions found/i)).toBeInTheDocument()
    })
  })

  it('renders empty state when user has no trading accounts', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockResolvedValue({
      user: mockUser,
      accounts: [],
    })

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('No trading accounts associated with this user.')).toBeInTheDocument()
    })
  })

  it('renders error banner when loading accounts fails', async () => {
    vi.spyOn(api, 'getAdminUserAccounts').mockRejectedValue(new Error('Network error loading user accounts'))

    renderUserAccountsPage('user-1234')

    await waitFor(() => {
      expect(screen.getByText('Network error loading user accounts')).toBeInTheDocument()
    })
  })
})
