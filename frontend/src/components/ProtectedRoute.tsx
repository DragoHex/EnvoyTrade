import { type JSX, Show } from 'solid-js'
import { Navigate } from '@solidjs/router'
import { useAuth } from '../context/AuthContext'
import { TableSkeleton } from './Skeleton'

export function ProtectedRoute(props: { children?: JSX.Element }) {
  const { isAuthenticated, isLoading } = useAuth()

  return (
    <Show
      when={!isLoading()}
      fallback={
        <div data-testid="auth-loading-skeleton" style={{ padding: '2rem' }}>
          <TableSkeleton />
        </div>
      }
    >
      <Show when={isAuthenticated()} fallback={<Navigate href="/login" />}>
        {props.children}
      </Show>
    </Show>
  )
}
