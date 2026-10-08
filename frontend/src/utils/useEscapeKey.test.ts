import { createRoot, createSignal } from 'solid-js'
import { fireEvent } from '@solidjs/testing-library'
import { describe, expect, it, vi } from 'vitest'
import { useEscapeKey } from './useEscapeKey'

describe('useEscapeKey', () => {
  it('calls onEscape when Escape key is pressed and enabled is true', () => {
    const onEscape = vi.fn()
    let disposeFn: () => void

    createRoot((dispose) => {
      disposeFn = dispose
      useEscapeKey({
        enabled: () => true,
        onEscape,
      })
    })

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).toHaveBeenCalledTimes(1)

    disposeFn!()
  })

  it('does not call onEscape when a non-Escape key is pressed', () => {
    const onEscape = vi.fn()
    let disposeFn: () => void

    createRoot((dispose) => {
      disposeFn = dispose
      useEscapeKey({
        enabled: () => true,
        onEscape,
      })
    })

    fireEvent.keyDown(document, { key: 'Enter' })
    fireEvent.keyDown(document, { key: 'Tab' })
    expect(onEscape).not.toHaveBeenCalled()

    disposeFn!()
  })

  it('does not call onEscape when enabled is false', () => {
    const onEscape = vi.fn()
    let disposeFn: () => void

    createRoot((dispose) => {
      disposeFn = dispose
      useEscapeKey({
        enabled: () => false,
        onEscape,
      })
    })

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).not.toHaveBeenCalled()

    disposeFn!()
  })

  it('does not call onEscape when canDismiss returns false', () => {
    const onEscape = vi.fn()
    let disposeFn: () => void

    createRoot((dispose) => {
      disposeFn = dispose
      useEscapeKey({
        enabled: () => true,
        canDismiss: () => false,
        onEscape,
      })
    })

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).not.toHaveBeenCalled()

    disposeFn!()
  })

  it('removes listener when enabled switches from true to false', () => {
    const onEscape = vi.fn()
    let setEnabled: (val: boolean) => void
    let disposeFn: () => void

    createRoot((dispose) => {
      disposeFn = dispose
      const [enabled, set] = createSignal(true)
      setEnabled = set
      useEscapeKey({
        enabled,
        onEscape,
      })
    })

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).toHaveBeenCalledTimes(1)

    setEnabled!(false)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onEscape).toHaveBeenCalledTimes(1) // Still 1, not called again

    disposeFn!()
  })
})
