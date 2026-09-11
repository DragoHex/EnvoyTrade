export function CopyToggle(props: {
  enabled: boolean
  disabled?: boolean
  ariaLabel?: string
  title?: string
  onToggle: (next: boolean) => void
}) {
  return (
    <label class="copy-toggle" title={props.title} data-tooltip={props.title || undefined}>
      <input
        type="checkbox"
        aria-label={props.ariaLabel || 'copy trading enabled'}
        checked={props.enabled}
        disabled={props.disabled}
        onChange={(e) => props.onToggle(e.currentTarget.checked)}
      />
      <span class="copy-toggle-track">
        <span class="copy-toggle-knob" />
      </span>
    </label>
  )
}
