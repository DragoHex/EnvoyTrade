import { Show } from 'solid-js'

export function ConfirmActionModal(props: {
  open: boolean
  label: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <Show when={props.open}>
      <div role="dialog" aria-label={`Confirm ${props.label}`} class="confirm-modal-overlay">
        <div class="confirm-modal">
          <h3>{props.label}</h3>
          <p>Are you sure you want to {props.label.toLowerCase()}?</p>
          <div class="confirm-modal-actions">
            <button type="button" onClick={props.onCancel} class="confirm-modal-cancel">
              Cancel
            </button>
            <button type="button" onClick={props.onConfirm} class="confirm-modal-confirm">
              Confirm
            </button>
          </div>
        </div>
      </div>
    </Show>
  )
}
