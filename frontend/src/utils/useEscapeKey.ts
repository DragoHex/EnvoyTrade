import { createEffect, onCleanup, type Accessor } from 'solid-js'

export interface UseEscapeKeyOptions {
  enabled: Accessor<boolean>
  onEscape: () => void
  canDismiss?: Accessor<boolean>
}

/**
 * Reusable SolidJS hook to listen for the Escape key and invoke a callback.
 * 
 * Efficiency guarantees:
 * 1. Attaches event listener to `document` ONLY when `enabled()` is true.
 * 2. Cleans up listener immediately via `onCleanup` when `enabled()` becomes false or component unmounts.
 * 3. Prevents action if `canDismiss()` returns false (e.g. during active in-flight submissions).
 */
export function useEscapeKey(options: UseEscapeKeyOptions): void {
  createEffect(() => {
    if (!options.enabled()) return

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (!options.canDismiss || options.canDismiss()) {
          e.preventDefault()
          options.onEscape()
        }
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    onCleanup(() => {
      document.removeEventListener('keydown', handleKeyDown)
    })
  })
}
