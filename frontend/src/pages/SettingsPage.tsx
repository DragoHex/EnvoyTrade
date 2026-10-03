import { createSignal, createEffect, Show } from 'solid-js'
import { useAuth } from '../context/AuthContext'
import { SyncIcon } from '../components/icons'

export function SettingsPage() {
  const { user, updateProfile, updatePassword } = useAuth()

  // Profile Form state
  const [email, setEmail] = createSignal('')
  const [username, setUsername] = createSignal('')
  const [name, setName] = createSignal('')
  const [phone, setPhone] = createSignal('')
  const [address, setAddress] = createSignal('')
  const [gstNumber, setGstNumber] = createSignal('')

  const [profileSaving, setProfileSaving] = createSignal(false)
  const [profileSuccess, setProfileSuccess] = createSignal<string | null>(null)
  const [profileError, setProfileError] = createSignal<string | null>(null)

  // Password Form state
  const [oldPassword, setOldPassword] = createSignal('')
  const [newPassword, setNewPassword] = createSignal('')
  const [confirmPassword, setConfirmPassword] = createSignal('')

  const [passwordSaving, setPasswordSaving] = createSignal(false)
  const [passwordSuccess, setPasswordSuccess] = createSignal<string | null>(null)
  const [passwordError, setPasswordError] = createSignal<string | null>(null)

  // Populate profile fields from current user
  createEffect(() => {
    const u = user()
    if (u) {
      setEmail(u.email || '')
      setUsername(u.username || '')
      setName(u.name || '')

      // Strip +91 prefix for the 10-digit input field
      let p = u.phone || ''
      if (p.startsWith('+91')) {
        p = p.slice(3)
      }
      setPhone(p)

      setAddress(u.address || '')
      setGstNumber(u.gstNumber || '')
    }
  })

  const handleProfileSubmit = async (e: Event) => {
    e.preventDefault()
    setProfileError(null)
    setProfileSuccess(null)
    setProfileSaving(true)

    try {
      await updateProfile({
        email: email().trim(),
        username: username().trim(),
        name: name().trim() || undefined,
        phone: phone().trim() || undefined,
        address: address().trim() || undefined,
        gstNumber: gstNumber().trim() || undefined,
      })
      setProfileSuccess('Profile updated successfully.')
    } catch (err) {
      setProfileError(err instanceof Error ? err.message : 'Failed to update profile')
    } finally {
      setProfileSaving(false)
    }
  }

  const handlePasswordSubmit = async (e: Event) => {
    e.preventDefault()
    setPasswordError(null)
    setPasswordSuccess(null)

    if (newPassword().length < 8) {
      setPasswordError('New password must be at least 8 characters long.')
      return
    }

    if (newPassword() !== confirmPassword()) {
      setPasswordError('New passwords do not match.')
      return
    }

    setPasswordSaving(true)

    try {
      await updatePassword({
        oldPassword: oldPassword(),
        newPassword: newPassword(),
      })
      setPasswordSuccess('Password changed successfully. All other sessions have been signed out.')
      setOldPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (err) {
      setPasswordError(err instanceof Error ? err.message : 'Failed to update password')
    } finally {
      setPasswordSaving(false)
    }
  }

  return (
    <div class="settings-page">
      <div class="settings-header">
        <h1>Account Management</h1>
        <p>Manage your account profile, contact details, and security settings.</p>
      </div>

      <div class="settings-grid">
        {/* Profile Card */}
        <section class="settings-card" aria-labelledby="profile-title">
          <h2 id="profile-title" class="settings-card-title">Profile Information</h2>
          <p class="settings-card-desc">Update your personal and business contact details.</p>

          <Show when={profileSuccess()}>
            {(msg) => (
              <div role="alert" class="settings-alert settings-alert-success" data-testid="profile-success">
                {msg()}
              </div>
            )}
          </Show>

          <Show when={profileError()}>
            {(err) => (
              <div role="alert" class="settings-alert settings-alert-error" data-testid="profile-error">
                {err()}
              </div>
            )}
          </Show>

          <form onSubmit={handleProfileSubmit} class="settings-form">
            <div class="settings-field">
              <label for="email" class="settings-label">
                Email Address
              </label>
              <input
                id="email"
                type="email"
                required
                class="settings-input"
                value={email()}
                onInput={(e) => setEmail(e.currentTarget.value)}
                placeholder="name@company.com"
                disabled={profileSaving()}
                data-testid="input-email"
              />
            </div>

            <div class="settings-field">
              <label for="username" class="settings-label">
                Username
              </label>
              <input
                id="username"
                type="text"
                required
                class="settings-input"
                value={username()}
                onInput={(e) => setUsername(e.currentTarget.value)}
                placeholder="username"
                disabled={profileSaving()}
                data-testid="input-username"
              />
            </div>

            <div class="settings-field">
              <label for="name" class="settings-label">
                Full Name
                <span class="settings-label-optional">Optional</span>
              </label>
              <input
                id="name"
                type="text"
                class="settings-input"
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="e.g. Rajesh Sharma"
                disabled={profileSaving()}
                data-testid="input-name"
              />
            </div>

            <div class="settings-field">
              <label for="phone" class="settings-label">
                Phone Number
                <span class="settings-label-optional">Optional</span>
              </label>
              <div class="settings-phone-wrap">
                <span class="settings-phone-prefix">+91</span>
                <input
                  id="phone"
                  type="tel"
                  class="settings-input"
                  value={phone()}
                  onInput={(e) => setPhone(e.currentTarget.value)}
                  placeholder="9876543210"
                  disabled={profileSaving()}
                  data-testid="input-phone"
                />
              </div>
            </div>

            <div class="settings-field">
              <label for="address" class="settings-label">
                Address
                <span class="settings-label-optional">Optional</span>
              </label>
              <textarea
                id="address"
                class="settings-textarea"
                value={address()}
                onInput={(e) => setAddress(e.currentTarget.value)}
                placeholder="e.g. 123 Dalal Street, Fort, Mumbai"
                disabled={profileSaving()}
                data-testid="input-address"
              />
            </div>

            <div class="settings-field">
              <label for="gstNumber" class="settings-label">
                GST Number (GSTIN)
                <span class="settings-label-optional">Optional</span>
              </label>
              <input
                id="gstNumber"
                type="text"
                class="settings-input"
                value={gstNumber()}
                onInput={(e) => setGstNumber(e.currentTarget.value.toUpperCase())}
                placeholder="27AABCU9603R1ZM"
                disabled={profileSaving()}
                maxLength={15}
                data-testid="input-gst"
              />
            </div>

            <div class="settings-actions">
              <button
                type="submit"
                class="btn-primary"
                disabled={profileSaving()}
                data-testid="btn-save-profile"
              >
                <Show when={profileSaving()}>
                  <SyncIcon spinning />
                </Show>
                <span>{profileSaving() ? 'Saving…' : 'Save Changes'}</span>
              </button>
            </div>
          </form>
        </section>

        {/* Security & Password Card */}
        <section class="settings-card" aria-labelledby="security-title">
          <h2 id="security-title" class="settings-card-title">Security & Password</h2>
          <p class="settings-card-desc">Change your password and secure your account.</p>

          <Show when={passwordSuccess()}>
            {(msg) => (
              <div role="alert" class="settings-alert settings-alert-success" data-testid="password-success">
                {msg()}
              </div>
            )}
          </Show>

          <Show when={passwordError()}>
            {(err) => (
              <div role="alert" class="settings-alert settings-alert-error" data-testid="password-error">
                {err()}
              </div>
            )}
          </Show>

          <form onSubmit={handlePasswordSubmit} class="settings-form">
            <div class="settings-field">
              <label for="oldPassword" class="settings-label">
                Current Password
              </label>
              <input
                id="oldPassword"
                type="password"
                required
                class="settings-input"
                value={oldPassword()}
                onInput={(e) => setOldPassword(e.currentTarget.value)}
                placeholder="••••••••"
                disabled={passwordSaving()}
                data-testid="input-old-password"
              />
            </div>

            <div class="settings-field">
              <label for="newPassword" class="settings-label">
                New Password
              </label>
              <input
                id="newPassword"
                type="password"
                required
                class="settings-input"
                value={newPassword()}
                onInput={(e) => setNewPassword(e.currentTarget.value)}
                placeholder="At least 8 characters"
                disabled={passwordSaving()}
                minLength={8}
                data-testid="input-new-password"
              />
            </div>

            <div class="settings-field">
              <label for="confirmPassword" class="settings-label">
                Confirm New Password
              </label>
              <input
                id="confirmPassword"
                type="password"
                required
                class="settings-input"
                value={confirmPassword()}
                onInput={(e) => setConfirmPassword(e.currentTarget.value)}
                placeholder="Repeat new password"
                disabled={passwordSaving()}
                minLength={8}
                data-testid="input-confirm-password"
              />
            </div>

            <div class="settings-actions">
              <button
                type="submit"
                class="btn-primary"
                disabled={passwordSaving()}
                data-testid="btn-update-password"
              >
                <Show when={passwordSaving()}>
                  <SyncIcon spinning />
                </Show>
                <span>{passwordSaving() ? 'Updating…' : 'Update Password'}</span>
              </button>
            </div>
          </form>
        </section>
      </div>
    </div>
  )
}
