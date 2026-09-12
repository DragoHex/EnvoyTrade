import type { JSX } from 'solid-js'
import { EmptyBoxIcon } from './icons'

export interface EmptyStateProps {
  message?: string
  children?: JSX.Element
}

export function EmptyState(props: EmptyStateProps) {
  return (
    <div class="order-empty-state" data-testid="order-empty-state">
      <div class="empty-icon-wrapper">
        <EmptyBoxIcon />
      </div>
      <div class="empty-text">{props.children ?? props.message}</div>
    </div>
  )
}
