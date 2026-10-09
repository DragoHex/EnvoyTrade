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

  it('renders login page directly on /login without skeleton loading', async () => {
    vi.spyOn(api, 'getMe').mockRejectedValue(new Error('unauthenticated'))
    window.history.pushState({}, '', '/login')
    render(() => <App />)

    expect(await screen.findByRole('heading', { name: /sign in/i })).toBeInTheDocument()
    expect(screen.queryByTestId('auth-loading-skeleton')).not.toBeInTheDocument()
  })
})

