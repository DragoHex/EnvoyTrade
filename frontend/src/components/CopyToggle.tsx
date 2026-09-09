export function CopyToggle(props: { enabled: boolean; disabled?: boolean; onToggle: (next: boolean) => void }) {
  return (
    <label class="copy-toggle">
      <input
        type="checkbox"
        aria-label="copy trading enabled"
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
