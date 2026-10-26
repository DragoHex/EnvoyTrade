import { SyncIcon } from './icons'

export function StatusDot(props: { status: 'ok' | 'error' | 'warning' | 'pending' | 'syncing' | string }) {
  if (props.status === 'pending' || props.status === 'syncing') {
    return (
      <span
        data-testid="status-dot"
        data-status={props.status}
        title="Connecting / Syncing..."
        style={{
          display: 'inline-flex',
          'align-items': 'center',
          'justify-content': 'center',
          width: '0.85em',
          height: '0.85em',
          color: 'var(--color-accent, #6366f1)',
          'vertical-align': 'middle',
        }}
      >
        <SyncIcon spinning={true} />
      </span>
    )
  }

  const color = () => {
    if (props.status === 'warning') return '#f59f00'
    if (props.status === 'ok' || props.status === 'active') return 'var(--color-accent)'
    return '#e5484d'
  }
  return (
    <span
      data-testid="status-dot"
      data-status={props.status}
      style={{
        display: 'inline-block',
        width: '0.6em',
        height: '0.6em',
        'border-radius': '50%',
        background: color(),
      }}
    />
  )
}
