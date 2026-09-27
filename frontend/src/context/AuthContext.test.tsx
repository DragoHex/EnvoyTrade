import { render, screen, fireEvent } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AuthProvider, useAuth } from './AuthContext'
import * as api from '../api'

function TestConsumer() {
  const { user, isAuthenticated, isLoading, login, register, logout, updateProfile, updatePassword } = useAuth()
  return (
    <div>
      <div data-testid="loading">{isLoading() ? 'loading' : 'ready'}</div>
      <div data-testid="auth-state">{isAuthenticated() ? 'authenticated' : 'unauthenticated'}</div>
      <div data-testid="user-email">{user()?.email ?? 'none'}</div>
      <div data-testid="user-phone">{user()?.phone ?? 'none'}</div>
      <button
        onClick={() =>
          login({ email: 'alice@example.com', password: 'password123' })
        }
      >
        Trigger Login
      </button>
      <button
        onClick={() =>
          register({
            email: 'bob@example.com',
            username: 'bob',
            password: 'password123',
            name: 'Bob',
          })
        }
      >
        Trigger Register
      </button>
      <button onClick={() => logout()}>Trigger Logout</button>
      <button
        onClick={() =>
          updateProfile({
            email: 'alice_updated@example.com',
            username: 'alice',
            phone: '9876543210',
          })
        }
      >
        Trigger Update Profile
      </button>
      <button
        onClick={() =>
          updatePassword({
            oldPassword: 'oldPassword123',
            newPassword: 'newPassword123',
          })
        }
      >
        Trigger Update Password
      </button>
    </div>
  )
}

describe('AuthContext', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('checks session on mount and sets authenticated state if user returned', async () => {
    const fakeUser: api.User = {
      id: 'u-1',
      email: 'alice@example.com',
      username: 'alice',
      name: 'Alice Smith',
      role: 'user',
    }
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    expect(await screen.findByText('ready')).toBeInTheDocument()
    expect(screen.getByTestId('auth-state')).toHaveTextContent('authenticated')
    expect(screen.getByTestId('user-email')).toHaveTextContent('alice@example.com')
  })

  it('handles unauthenticated state on mount', async () => {
    vi.spyOn(api, 'getMe').mockRejectedValue(new Error('unauthenticated'))

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    expect(await screen.findByText('ready')).toBeInTheDocument()
    expect(screen.getByTestId('auth-state')).toHaveTextContent('unauthenticated')
    expect(screen.getByTestId('user-email')).toHaveTextContent('none')
  })

  it('login updates user and authenticated state', async () => {
    vi.spyOn(api, 'getMe').mockRejectedValue(new Error('unauthenticated'))
    const fakeUser: api.User = {
      id: 'u-1',
      email: 'alice@example.com',
      username: 'alice',
      name: 'Alice Smith',
      role: 'user',
    }
    vi.spyOn(api, 'login').mockResolvedValue({ user: fakeUser })

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    await screen.findByText('ready')
    expect(screen.getByTestId('auth-state')).toHaveTextContent('unauthenticated')

    fireEvent.click(screen.getByRole('button', { name: 'Trigger Login' }))

    expect(await screen.findByText('alice@example.com')).toBeInTheDocument()
    expect(screen.getByTestId('auth-state')).toHaveTextContent('authenticated')
  })

  it('register updates user and authenticated state', async () => {
    vi.spyOn(api, 'getMe').mockRejectedValue(new Error('unauthenticated'))
    const fakeUser: api.User = {
      id: 'u-2',
      email: 'bob@example.com',
      username: 'bob',
      name: 'Bob',
      role: 'user',
    }
    vi.spyOn(api, 'register').mockResolvedValue({ user: fakeUser })

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    await screen.findByText('ready')
    expect(screen.getByTestId('auth-state')).toHaveTextContent('unauthenticated')

    fireEvent.click(screen.getByRole('button', { name: 'Trigger Register' }))

    expect(await screen.findByText('bob@example.com')).toBeInTheDocument()
    expect(screen.getByTestId('auth-state')).toHaveTextContent('authenticated')
  })

  it('logout clears user and authenticated state', async () => {
    const fakeUser: api.User = {
      id: 'u-1',
      email: 'alice@example.com',
      username: 'alice',
      name: 'Alice Smith',
      role: 'user',
    }
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    vi.spyOn(api, 'logout').mockResolvedValue({ status: 'ok' })

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    expect(await screen.findByText('alice@example.com')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Trigger Logout' }))

    expect(await screen.findByText('none')).toBeInTheDocument()
    expect(screen.getByTestId('auth-state')).toHaveTextContent('unauthenticated')
  })

  it('updateProfile updates user state', async () => {
    const fakeUser: api.User = {
      id: 'u-1',
      email: 'alice@example.com',
      username: 'alice',
      name: 'Alice Smith',
      role: 'user',
    }
    const updatedUser: api.User = {
      ...fakeUser,
      email: 'alice_updated@example.com',
      phone: '+919876543210',
    }
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    vi.spyOn(api, 'updateProfile').mockResolvedValue({ user: updatedUser })

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    expect(await screen.findByText('alice@example.com')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Trigger Update Profile' }))

    expect(await screen.findByText('alice_updated@example.com')).toBeInTheDocument()
    expect(screen.getByTestId('user-phone')).toHaveTextContent('+919876543210')
  })

  it('updatePassword calls api.updatePassword', async () => {
    const fakeUser: api.User = {
      id: 'u-1',
      email: 'alice@example.com',
      username: 'alice',
      name: 'Alice Smith',
      role: 'user',
    }
    vi.spyOn(api, 'getMe').mockResolvedValue({ user: fakeUser })
    const updateSpy = vi.spyOn(api, 'updatePassword').mockResolvedValue({ status: 'ok' })

    render(() => (
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    ))

    expect(await screen.findByText('alice@example.com')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Trigger Update Password' }))

    expect(updateSpy).toHaveBeenCalledWith({
      oldPassword: 'oldPassword123',
      newPassword: 'newPassword123',
    })
  })
})
