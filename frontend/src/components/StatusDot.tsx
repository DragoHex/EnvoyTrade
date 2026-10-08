export function StatusDot(props: { status: 'ok' | 'error' | 'warning' | string }) {
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
