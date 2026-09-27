import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { Router, Route } from '@solidjs/router'
import { ProtectedRoute } from './ProtectedRoute'
import * as AuthContextModule from '../context/AuthContext'

describe('ProtectedRoute', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.history.pushState({}, '', '/')
  })

  it('renders loading placeholder while authentication check is loading', () => {
    vi.spyOn(AuthContextModule, 'useAuth').mockReturnValue({
      user: () => null,
      isLoading: () => true,
      isAuthenticated: () => false,
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })

    render(() => (
      <Router>
        <Route
          path="/"
          component={() => (
            <ProtectedRoute>
              <div>Secret Protected Content</div>
            </ProtectedRoute>
          )}
        />
      </Router>
    ))

    expect(screen.queryByText('Secret Protected Content')).not.toBeInTheDocument()
    expect(screen.getByTestId('auth-loading-skeleton')).toBeInTheDocument()
  })

  it('redirects to /login if user is not authenticated', async () => {
    vi.spyOn(AuthContextModule, 'useAuth').mockReturnValue({
      user: () => null,
      isLoading: () => false,
      isAuthenticated: () => false,
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })

    render(() => (
      <Router>
        <Route
          path="/"
          component={() => (
            <ProtectedRoute>
              <div>Secret Protected Content</div>
            </ProtectedRoute>
          )}
        />
        <Route path="/login" component={() => <div>Login Destination</div>} />
      </Router>
    ))

    expect(screen.queryByText('Secret Protected Content')).not.toBeInTheDocument()
    expect(await screen.findByText('Login Destination')).toBeInTheDocument()
  })

  it('renders children if user is authenticated', () => {
    vi.spyOn(AuthContextModule, 'useAuth').mockReturnValue({
      user: () => ({
        id: 'u-1',
        email: 'alice@example.com',
        username: 'alice',
        name: 'Alice',
        role: 'user',
      }),
      isLoading: () => false,
      isAuthenticated: () => true,
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })

    render(() => (
      <Router>
        <Route
          path="/"
          component={() => (
            <ProtectedRoute>
              <div>Secret Protected Content</div>
            </ProtectedRoute>
          )}
        />
      </Router>
    ))

    expect(screen.getByText('Secret Protected Content')).toBeInTheDocument()
  })
})
