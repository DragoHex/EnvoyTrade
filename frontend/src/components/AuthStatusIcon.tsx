import { Show } from 'solid-js'

export interface AuthStatusIconProps {
  status?: 'authenticated' | 'error' | 'unauthenticated' | null | string
  error?: string | null
  size?: number
  class?: string
}

export function AuthStatusIcon(props: AuthStatusIconProps) {
  const size = () => props.size ?? 20
  const status = () => props.status || 'unauthenticated'

  const label = () => {
    if (status() === 'authenticated') return 'Authenticated - Active Broker Session'
    if (status() === 'error') {
      return props.error
        ? `Authentication Error: ${props.error}`
        : 'Authentication Error - Token Expired'
    }
    return 'Unauthenticated - No Active Broker Session'
  }

  return (
    <span
      class={`auth-status-icon-wrap ${props.class ?? ''}`.trim()}
      data-tooltip={label()}
      title={label()}
      aria-label={label()}
      role="img"
    >
      <Show when={status() === 'authenticated'}>
        <svg
          viewBox="0 0 24 24"
          width={size()}
          height={size()}
          class="auth-status-icon-svg auth-status-icon-authenticated"
          style={{ color: 'var(--color-accent)' }}
          aria-hidden="true"
        >
          {/* Shutterstock status set - Outer Ring */}
          <circle cx="12" cy="12" r="9.25" stroke="currentColor" stroke-width="2.5" fill="none" />
          {/* Rounded Checkmark */}
          <path
            d="M7.75 12.25L10.5 15L16.25 9"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            fill="none"
          />
        </svg>
      </Show>

      <Show when={status() === 'error'}>
        <svg
          viewBox="0 0 24 24"
          width={size()}
          height={size()}
          class="auth-status-icon-svg auth-status-icon-error"
          style={{ color: '#ef4444' }}
          aria-hidden="true"
        >
          {/* Shutterstock status set - Outer Ring */}
          <circle cx="12" cy="12" r="9.25" stroke="currentColor" stroke-width="2.5" fill="none" />
          {/* Rounded Cross */}
          <path
            d="M8.5 8.5L15.5 15.5M15.5 8.5L8.5 15.5"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            fill="none"
          />
        </svg>
      </Show>

      <Show when={status() !== 'authenticated' && status() !== 'error'}>
        <svg
          viewBox="0 0 24 24"
          width={size()}
          height={size()}
          class="auth-status-icon-svg auth-status-icon-unauthenticated"
          style={{ color: 'var(--color-text-secondary)', opacity: '0.8' }}
          aria-hidden="true"
        >
          {/* Shutterstock status set - Outer Ring */}
          <circle cx="12" cy="12" r="9.25" stroke="currentColor" stroke-width="2.5" fill="none" />
          {/* Rounded Exclamation */}
          <path
            d="M12 7.75V12.75"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            fill="none"
          />
          <circle cx="12" cy="16.25" r="1.35" fill="currentColor" />
        </svg>
      </Show>
    </span>
  )
}
