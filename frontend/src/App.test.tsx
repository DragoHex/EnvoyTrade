import { render, screen, fireEvent, waitFor } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import App from './App'
import * as api from './api'

describe('App authentication & navigation', () => {
  const fakeUser: api.User = {
    id: 'u-1',
    email: 'admin@envoytrade.com',
    username: 'admin',
    name: 'Admin User',
    role: 'user',
  }

  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    vi.spyOn(api, 'getAccounts').mockResolvedValue([])
    vi.spyOn(api, 'logout').mockResolvedValue({ status: 'ok' })
  })

  it('renders login page and hides navigation links when unauthenticated', async () => {
    vi.spyOn(api, 'getMe').mockRejectedValue(new Error('unauthenticated'))
    window.history.pushState({}, '', '/')
    render(() => <App />)

    // Heading "Sign In" should appear
    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()

    // Nav links must NOT be present
    expect(screen.queryByRole('link', { name: 'Dashboard' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Accounts' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Analytics' })).not.toBeInTheDocument()
    expect(screen.queryByTestId('user-badge')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Sign Out' })).not.toBeInTheDocument()
  })

  it('highlights only Dashboard when authenticated on /', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    window.history.pushState({}, '', '/')
    render(() => <App />)

    const dashboardLink = await screen.findByRole('link', { name: 'Dashboard' })
    const accountsLink = screen.getByRole('link', { name: 'Accounts' })
    const analyticsLink = screen.getByRole('link', { name: 'Analytics' })

    expect(dashboardLink).toHaveClass('active')
    expect(accountsLink).not.toHaveClass('active')
    expect(analyticsLink).not.toHaveClass('active')

    // Shows user badge as settings link and logout button
    const userBadge = screen.getByTestId('user-badge')
    expect(userBadge).toHaveTextContent('Admin User')
    expect(userBadge).toHaveAttribute('href', '/settings')
    expect(userBadge).not.toHaveAttribute('data-tooltip')

    // Verify semantic navigation layout containers
    const nav = screen.getByRole('navigation')
    expect(nav).toHaveClass('app-nav')
    expect(nav.querySelector('.nav-brand-bar')).toBeInTheDocument()
    expect(nav.querySelector('.nav-links')).toBeInTheDocument()
    expect(nav.querySelector('.nav-actions')).toBeInTheDocument()

    const signOutBtn = screen.getByRole('button', { name: 'Sign Out' })
    expect(signOutBtn).toBeInTheDocument()
    expect(signOutBtn).toHaveAttribute('data-tooltip', 'Sign Out')
    expect(signOutBtn).toHaveAttribute('data-tooltip-pos', 'bottom')
  })

  it('renders settings page when authenticated on /settings and marks badge active', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    window.history.pushState({}, '', '/settings')
    render(() => <App />)

    expect(await screen.findByRole('heading', { name: 'Account Management' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Profile Information' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Security & Password' })).toBeInTheDocument()

    const userBadge = screen.getByTestId('user-badge')
    expect(userBadge).toHaveClass('active')
  })

  it('highlights only Accounts when authenticated on /accounts', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    window.history.pushState({}, '', '/accounts')
    render(() => <App />)

    const dashboardLink = await screen.findByRole('link', { name: 'Dashboard' })
    const accountsLink = screen.getByRole('link', { name: 'Accounts' })
    const analyticsLink = screen.getByRole('link', { name: 'Analytics' })

    expect(dashboardLink).not.toHaveClass('active')
    expect(accountsLink).toHaveClass('active')
    expect(analyticsLink).not.toHaveClass('active')
  })

  it('highlights only Analytics when authenticated on /analytics', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    window.history.pushState({}, '', '/analytics')
    render(() => <App />)

    const dashboardLink = await screen.findByRole('link', { name: 'Dashboard' })
    const accountsLink = screen.getByRole('link', { name: 'Accounts' })
    const analyticsLink = screen.getByRole('link', { name: 'Analytics' })

    expect(dashboardLink).not.toHaveClass('active')
    expect(accountsLink).not.toHaveClass('active')
    expect(analyticsLink).toHaveClass('active')
  })

  it('clicking sign out logs user out and shows login wall', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    window.history.pushState({}, '', '/')
    render(() => <App />)

    const signOutBtn = await screen.findByRole('button', { name: 'Sign Out' })
    fireEvent.click(signOutBtn)

    await waitFor(() => {
      expect(api.logout).toHaveBeenCalledTimes(1)
    })

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Dashboard' })).not.toBeInTheDocument()
    expect(screen.queryByTestId('user-badge')).not.toBeInTheDocument()
  })

  it('renders 404 Not Found page with themed cartoon when visiting an unknown route and removes top pane', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    window.history.pushState({}, '', '/non-existent-route-xyz')
    render(() => <App />)

    expect(await screen.findByRole('heading', { level: 1, name: '404' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 2, name: 'Page Not Found' })).toBeInTheDocument()
    expect(screen.getByTestId('not-found-image')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Return to Dashboard' })).toBeInTheDocument()

    // Top nav pane must be removed on 404 page
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
    expect(screen.queryByText('EnvoyTrade')).not.toBeInTheDocument()
  })

  it('renders persistent emergency notification banner when kill switch is engaged', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    vi.spyOn(api, 'getKillSwitchStatus').mockResolvedValue({
      engaged: true,
      reason: 'Broker maintenance window',
      engagedBy: 'admin',
    })
    window.history.pushState({}, '', '/')
    render(() => <App />)

    const banner = await screen.findByTestId('killswitch-banner')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveTextContent(/EMERGENCY KILL SWITCH ENGAGED/i)
    expect(banner).toHaveTextContent(/Broker maintenance window/i)
  })

  it('does not render emergency banner when kill switch is disengaged', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    vi.spyOn(api, 'getKillSwitchStatus').mockResolvedValue({
      engaged: false,
    })
    window.history.pushState({}, '', '/')
    render(() => <App />)

    await screen.findByRole('link', { name: 'Dashboard' })
    expect(screen.queryByTestId('killswitch-banner')).not.toBeInTheDocument()
  })

  it('updates emergency banner immediately upon receiving killswitch:change event', async () => {
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    vi.spyOn(api, 'getKillSwitchStatus').mockResolvedValue({
      engaged: false,
    })
    window.history.pushState({}, '', '/')
    render(() => <App />)

    await screen.findByRole('link', { name: 'Dashboard' })
    expect(screen.queryByTestId('killswitch-banner')).not.toBeInTheDocument()

    // Dispatch killswitch:change event
    window.dispatchEvent(
      new CustomEvent('killswitch:change', {
        detail: { engaged: true, reason: 'Instant circuit breaker tripped' },
      }),
    )

    const banner = await screen.findByTestId('killswitch-banner')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveTextContent(/Instant circuit breaker tripped/i)
  })

  it('does not poll kill switch periodically on admin routes', async () => {
    vi.useFakeTimers()
    const adminUser = { ...fakeUser, role: 'admin' }
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: adminUser })
    const ksSpy = vi.spyOn(api, 'getKillSwitchStatus').mockResolvedValue({ engaged: false })
    vi.spyOn(api, 'getAdminServiceStatus').mockResolvedValue({
      killSwitch: { engaged: false },
      masterTickers: [],
      queues: { masterFillDepth: 0, masterFillCap: 100, orderUpdateDepth: 0, orderUpdateCap: 100 },
      workerCount: 0,
      dbPool: { totalConns: 1, idleConns: 1, acquiredConns: 0, maxConns: 10 },
      systemResources: { cpuPercent: 1, heapAllocMb: 20, sysMemoryMb: 50, goroutineCount: 15, gcCycles: 5 },
    })

    window.history.pushState({}, '', '/admin/status')
    render(() => <App />)

    // Initial check on mount
    await vi.advanceTimersByTimeAsync(100)
    expect(ksSpy).toHaveBeenCalledTimes(1)

    // Advance 60 seconds (2 intervals of 30s) — should NOT poll because route is /admin/*
    await vi.advanceTimersByTimeAsync(60_000)
    expect(ksSpy).toHaveBeenCalledTimes(1)

    vi.useRealTimers()
  })

  it('highlights Admin link across all admin sub-tabs for admin users', async () => {
    const adminUser = { ...fakeUser, role: 'admin' }
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: adminUser })
    vi.spyOn(api, 'getAdminServiceStatus').mockResolvedValue({
      killSwitch: { engaged: false },
      masterTickers: [],
      queues: { masterFillDepth: 0, masterFillCap: 100, orderUpdateDepth: 0, orderUpdateCap: 100 },
      workerCount: 0,
      dbPool: { totalConns: 1, idleConns: 1, acquiredConns: 0, maxConns: 10 },
      systemResources: { cpuPercent: 1, heapAllocMb: 20, sysMemoryMb: 50, goroutineCount: 15, gcCycles: 5 },
    })

    const adminRoutes = ['/admin/status', '/admin/operations', '/admin/ips', '/admin/users']
    for (const route of adminRoutes) {
      window.history.pushState({}, '', route)
      const { unmount } = render(() => <App />)

      const adminLink = await screen.findByRole('link', { name: 'Admin' })
      const dashboardLink = screen.getByRole('link', { name: 'Dashboard' })
      const accountsLink = screen.getByRole('link', { name: 'Accounts' })

      expect(adminLink).toHaveClass('active')
      expect(dashboardLink).not.toHaveClass('active')
      expect(accountsLink).not.toHaveClass('active')

      unmount()
    }
  })
})

