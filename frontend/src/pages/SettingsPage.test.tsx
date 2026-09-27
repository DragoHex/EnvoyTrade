import { render, screen, fireEvent } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { SettingsPage } from './SettingsPage'
import type { AuthContextValue } from '../context/AuthContext'
import * as api from '../api'

// Mock useAuth directly or wrap with mock context
const mockUser: api.User = {
  id: 'u-123',
  email: 'trader@envoytrade.com',
  username: 'trader',
  name: 'Demo Trader',
  role: 'admin',
  phone: '+919876543210',
  address: '123 Dalal Street, Mumbai',
  gstNumber: '27AABCU9603R1ZM',
}

let authMock: AuthContextValue

vi.mock('../context/AuthContext', () => ({
  useAuth: () => authMock,
}))

describe('SettingsPage', () => {
  let updateProfileMock = vi.fn()
  let updatePasswordMock = vi.fn()

  beforeEach(() => {
    vi.restoreAllMocks()
    updateProfileMock = vi.fn()
    updatePasswordMock = vi.fn()

    authMock = {
      user: () => mockUser,
      isLoading: () => false,
      isAuthenticated: () => true,
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: updateProfileMock,
      updatePassword: updatePasswordMock,
    }
  })

  it('renders current profile information into inputs', () => {
    render(() => <SettingsPage />)

    expect(screen.getByTestId('input-email')).toHaveValue('trader@envoytrade.com')
    expect(screen.getByTestId('input-username')).toHaveValue('trader')
    expect(screen.getByTestId('input-name')).toHaveValue('Demo Trader')
    // Phone prefix +91 is separate prefix; input shows 9876543210
    expect(screen.getByTestId('input-phone')).toHaveValue('9876543210')
    expect(screen.getByTestId('input-address')).toHaveValue('123 Dalal Street, Mumbai')
    expect(screen.getByTestId('input-gst')).toHaveValue('27AABCU9603R1ZM')
  })

  it('submits updated profile successfully and shows success message', async () => {
    updateProfileMock.mockResolvedValue({
      ...mockUser,
      name: 'New Name',
    })

    render(() => <SettingsPage />)

    const nameInput = screen.getByTestId('input-name')
    fireEvent.input(nameInput, { target: { value: 'New Name' } })

    const saveBtn = screen.getByTestId('btn-save-profile')
    fireEvent.click(saveBtn)

    expect(updateProfileMock).toHaveBeenCalledWith({
      email: 'trader@envoytrade.com',
      username: 'trader',
      name: 'New Name',
      phone: '9876543210',
      address: '123 Dalal Street, Mumbai',
      gstNumber: '27AABCU9603R1ZM',
    })

    expect(await screen.findByTestId('profile-success')).toHaveTextContent(
      'Profile updated successfully.',
    )
  })

  it('displays error alert on profile update failure', async () => {
    updateProfileMock.mockRejectedValue(new Error('user with this email or username already exists'))

    render(() => <SettingsPage />)

    const saveBtn = screen.getByTestId('btn-save-profile')
    fireEvent.click(saveBtn)

    expect(await screen.findByTestId('profile-error')).toHaveTextContent(
      'user with this email or username already exists',
    )
  })

  it('validates password length on client side before submitting', async () => {
    render(() => <SettingsPage />)

    fireEvent.input(screen.getByTestId('input-old-password'), { target: { value: 'password123' } })
    fireEvent.input(screen.getByTestId('input-new-password'), { target: { value: 'short' } })
    fireEvent.input(screen.getByTestId('input-confirm-password'), { target: { value: 'short' } })

    fireEvent.click(screen.getByTestId('btn-update-password'))

    expect(updatePasswordMock).not.toHaveBeenCalled()
    expect(await screen.findByTestId('password-error')).toHaveTextContent(
      'New password must be at least 8 characters long.',
    )
  })

  it('validates matching confirm password before submitting', async () => {
    render(() => <SettingsPage />)

    fireEvent.input(screen.getByTestId('input-old-password'), { target: { value: 'password123' } })
    fireEvent.input(screen.getByTestId('input-new-password'), { target: { value: 'NewPassword123!' } })
    fireEvent.input(screen.getByTestId('input-confirm-password'), { target: { value: 'DifferentPassword123!' } })

    fireEvent.click(screen.getByTestId('btn-update-password'))

    expect(updatePasswordMock).not.toHaveBeenCalled()
    expect(await screen.findByTestId('password-error')).toHaveTextContent(
      'New passwords do not match.',
    )
  })

  it('submits password reset successfully, clears password inputs, and displays confirmation', async () => {
    updatePasswordMock.mockResolvedValue(undefined)

    render(() => <SettingsPage />)

    const oldInput = screen.getByTestId('input-old-password')
    const newInput = screen.getByTestId('input-new-password')
    const confirmInput = screen.getByTestId('input-confirm-password')

    fireEvent.input(oldInput, { target: { value: 'password123' } })
    fireEvent.input(newInput, { target: { value: 'NewPassword123!' } })
    fireEvent.input(confirmInput, { target: { value: 'NewPassword123!' } })

    fireEvent.click(screen.getByTestId('btn-update-password'))

    expect(updatePasswordMock).toHaveBeenCalledWith({
      oldPassword: 'password123',
      newPassword: 'NewPassword123!',
    })

    expect(await screen.findByTestId('password-success')).toHaveTextContent(
      'Password changed successfully. All other sessions have been signed out.',
    )

    // Inputs should be cleared
    expect(oldInput).toHaveValue('')
    expect(newInput).toHaveValue('')
    expect(confirmInput).toHaveValue('')
  })

  it('displays error alert on password change API failure', async () => {
    updatePasswordMock.mockRejectedValue(new Error('incorrect current password'))

    render(() => <SettingsPage />)

    fireEvent.input(screen.getByTestId('input-old-password'), { target: { value: 'wrongpass' } })
    fireEvent.input(screen.getByTestId('input-new-password'), { target: { value: 'NewPassword123!' } })
    fireEvent.input(screen.getByTestId('input-confirm-password'), { target: { value: 'NewPassword123!' } })

    fireEvent.click(screen.getByTestId('btn-update-password'))

    expect(await screen.findByTestId('password-error')).toHaveTextContent(
      'incorrect current password',
    )
  })
})
