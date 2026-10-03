import { render, screen, fireEvent, waitFor } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { Router, Route } from '@solidjs/router'
import { LoginPage } from './LoginPage'
import * as AuthContextModule from '../context/AuthContext'
import * as SolidRouter from '@solidjs/router'

describe('LoginPage', () => {
  const mockLogin = vi.fn()
  const mockRegister = vi.fn()
  const mockNavigate = vi.fn()

  const renderLogin = () =>
    render(() => (
      <Router>
        <Route path="/login" component={LoginPage} />
      </Router>
    ))

  beforeEach(() => {
    vi.restoreAllMocks()
    window.history.pushState({}, '', '/login')
    vi.spyOn(SolidRouter, 'useNavigate').mockReturnValue(mockNavigate)
    vi.spyOn(AuthContextModule, 'useAuth').mockReturnValue({
      user: () => null,
      isLoading: () => false,
      isAuthenticated: () => false,
      login: mockLogin,
      register: mockRegister,
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })
  })

  it('renders Sign In form by default without any skeleton loading', () => {
    renderLogin()

    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument()
    expect(screen.queryByTestId('auth-loading-skeleton')).not.toBeInTheDocument()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^password/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign In' })).toBeInTheDocument()
  })

  it('can toggle between Sign In and Create Account modes', async () => {
    renderLogin()

    const switchBtn = screen.getByRole('button', { name: /create an account|create account/i })
    fireEvent.click(switchBtn)

    expect(screen.getByRole('heading', { name: /create account/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create Account' })).toBeInTheDocument()

    const backToSignIn = screen.getByRole('button', { name: /already have an account\? sign in|sign in/i })
    fireEvent.click(backToSignIn)

    expect(screen.getByRole('heading', { name: /sign in/i })).toBeInTheDocument()
  })

  it('submits login when email and password are provided and navigates to /', async () => {
    mockLogin.mockResolvedValueOnce(undefined)

    renderLogin()

    fireEvent.input(screen.getByLabelText(/email/i), { target: { value: 'trader@example.com' } })
    fireEvent.input(screen.getByLabelText(/^password/i), { target: { value: 'password123' } })

    const submitBtn = screen.getByRole('button', { name: 'Sign In' })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith({
        email: 'trader@example.com',
        password: 'password123',
      })
      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
    })
  })

  it('displays error banner if login fails', async () => {
    mockLogin.mockRejectedValueOnce(new Error('invalid email or password'))

    renderLogin()

    fireEvent.input(screen.getByLabelText(/email/i), { target: { value: 'trader@example.com' } })
    fireEvent.input(screen.getByLabelText(/^password/i), { target: { value: 'wrongpass' } })

    fireEvent.click(screen.getByRole('button', { name: 'Sign In' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('invalid email or password')
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('submits registration and navigates to / on success', async () => {
    mockRegister.mockResolvedValueOnce(undefined)

    renderLogin()

    // Switch to register tab
    fireEvent.click(screen.getByRole('button', { name: /create an account|create account/i }))

    fireEvent.input(screen.getByLabelText(/email/i), { target: { value: 'bob@example.com' } })
    fireEvent.input(screen.getByLabelText(/username/i), { target: { value: 'bobtrader' } })
    fireEvent.input(screen.getByLabelText(/full name/i), { target: { value: 'Bob Trader' } })
    fireEvent.input(screen.getByLabelText(/^password/i), { target: { value: 'securepwd123' } })

    fireEvent.click(screen.getByRole('button', { name: 'Create Account' }))

    await waitFor(() => {
      expect(mockRegister).toHaveBeenCalledWith({
        email: 'bob@example.com',
        username: 'bobtrader',
        name: 'Bob Trader',
        password: 'securepwd123',
      })
      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
    })
  })

  it('redirects to / if user is already authenticated', async () => {
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
      login: mockLogin,
      register: mockRegister,
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })

    renderLogin()

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true })
    })
  })
})
