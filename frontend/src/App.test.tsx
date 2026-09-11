import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import App from './App'
import * as api from './api'

describe('App navigation', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(api, 'getGroups').mockResolvedValue([])
    vi.spyOn(api, 'getAccounts').mockResolvedValue([])
  })

  it('highlights only Dashboard when on /', async () => {
    window.history.pushState({}, '', '/')
    render(() => <App />)

    const dashboardLink = screen.getByRole('link', { name: 'Dashboard' })
    const accountsLink = screen.getByRole('link', { name: 'Accounts' })
    const analyticsLink = screen.getByRole('link', { name: 'Analytics' })

    expect(dashboardLink).toHaveClass('active')
    expect(accountsLink).not.toHaveClass('active')
    expect(analyticsLink).not.toHaveClass('active')
  })

  it('highlights only Accounts when on /accounts', async () => {
    window.history.pushState({}, '', '/accounts')
    render(() => <App />)

    const dashboardLink = screen.getByRole('link', { name: 'Dashboard' })
    const accountsLink = screen.getByRole('link', { name: 'Accounts' })
    const analyticsLink = screen.getByRole('link', { name: 'Analytics' })

    expect(dashboardLink).not.toHaveClass('active')
    expect(accountsLink).toHaveClass('active')
    expect(analyticsLink).not.toHaveClass('active')
  })

  it('highlights only Analytics when on /analytics', async () => {
    window.history.pushState({}, '', '/analytics')
    render(() => <App />)

    const dashboardLink = screen.getByRole('link', { name: 'Dashboard' })
    const accountsLink = screen.getByRole('link', { name: 'Accounts' })
    const analyticsLink = screen.getByRole('link', { name: 'Analytics' })

    expect(dashboardLink).not.toHaveClass('active')
    expect(accountsLink).not.toHaveClass('active')
    expect(analyticsLink).toHaveClass('active')
  })
})
