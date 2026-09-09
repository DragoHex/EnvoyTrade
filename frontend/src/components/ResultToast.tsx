import { Show } from 'solid-js'

export type ToastResult = { kind: 'success' | 'error'; message: string }

export function ResultToast(props: { result: ToastResult | null; onDismiss: () => void }) {
  return (
    <Show when={props.result}>
      {(r) => (
        <div role="status" class={`result-toast result-toast-${r().kind}`}>
          <span>{r().message}</span>
          <button type="button" aria-label="Dismiss" onClick={props.onDismiss}>
            ×
          </button>
        </div>
      )}
    </Show>
  )
}
