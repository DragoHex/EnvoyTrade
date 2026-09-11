export function StatusDot(props: { status: 'ok' | 'error' | string }) {
  const isHealthy = () => props.status === 'ok' || props.status === 'active'
  return (
    <span
      data-testid="status-dot"
      data-status={props.status}
      style={{
        display: 'inline-block',
        width: '0.6em',
        height: '0.6em',
        'border-radius': '50%',
        background: isHealthy() ? 'var(--color-accent)' : '#e5484d',
      }}
    />
  )
}
