export function StatusDot(props: { status: 'ok' | 'error' }) {
  return (
    <span
      data-testid="status-dot"
      data-status={props.status}
      style={{
        display: 'inline-block',
        width: '0.6em',
        height: '0.6em',
        'border-radius': '50%',
        background: props.status === 'ok' ? 'var(--color-accent)' : '#e5484d',
      }}
    />
  )
}
