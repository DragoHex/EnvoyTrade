import { Show, createSignal, createEffect, onCleanup } from 'solid-js'
import { Portal } from 'solid-js/web'
import { GandalfIcon, HobbitIcon } from './icons'

export function RoleSwitchPopover(props: {
  role: 'master' | 'follower'
  triggerRef: HTMLElement | undefined
  open: boolean
  onClose: () => void
  onSelect: () => void
}) {
  const [coords, setCoords] = createSignal<{
    top: number
    left: number
    arrowLeft: number
    placement: 'top' | 'bottom'
  }>({
    top: 0,
    left: 0,
    arrowLeft: 42,
    placement: 'top',
  })
  let popoverRef: HTMLDivElement | undefined

  const updatePosition = () => {
    if (!props.triggerRef) return
    const rect = props.triggerRef.getBoundingClientRect()
    const popoverEl = popoverRef
    const popoverWidth = popoverEl ? popoverEl.offsetWidth : 84
    const popoverHeight = popoverEl ? popoverEl.offsetHeight : 34

    // Center horizontally over the role icon trigger
    let left = rect.left + rect.width / 2 - popoverWidth / 2
    if (left < 8) left = 8
    if (typeof window !== 'undefined' && window.innerWidth > 0 && left + popoverWidth > window.innerWidth - 8) {
      left = window.innerWidth - popoverWidth - 8
    }

    // Anchor directly above the role icon
    let top = rect.top - popoverHeight - 6
    let placement: 'top' | 'bottom' = 'top'
    if (top < 8) {
      top = rect.bottom + 6
      placement = 'bottom'
    }

    const arrowLeft = Math.max(8, Math.min(popoverWidth - 8, Math.round(rect.left + rect.width / 2 - left)))

    setCoords({
      top: Math.round(top),
      left: Math.round(left),
      arrowLeft,
      placement,
    })
  }

  createEffect(() => {
    if (!props.open) return
    updatePosition()

    let rafId: number | undefined
    if (typeof requestAnimationFrame !== 'undefined') {
      rafId = requestAnimationFrame(() => {
        updatePosition()
      })
    }

    const handlePointerDown = (e: MouseEvent) => {
      const target = e.target as Node | null
      if (!target) return
      if (props.triggerRef?.contains(target)) return
      if (popoverRef?.contains(target)) return
      props.onClose()
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        props.onClose()
      }
    }

    const handleScrollOrResize = () => {
      updatePosition()
    }

    window.addEventListener('mousedown', handlePointerDown)
    window.addEventListener('keydown', handleKeyDown)
    window.addEventListener('scroll', handleScrollOrResize, true)
    window.addEventListener('resize', handleScrollOrResize)

    onCleanup(() => {
      if (rafId) cancelAnimationFrame(rafId)
      window.removeEventListener('mousedown', handlePointerDown)
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('scroll', handleScrollOrResize, true)
      window.removeEventListener('resize', handleScrollOrResize)
    })
  })

  const targetRole = () => (props.role === 'master' ? 'follower' : 'master')
  const label = () => `Switch to ${targetRole() === 'master' ? 'Master' : 'Follower'}`

  return (
    <Show when={props.open}>
      <Portal>
        <div
          ref={popoverRef}
          role="dialog"
          aria-label={label()}
          class="role-switch-popover"
          data-placement={coords().placement}
          style={{
            position: 'fixed',
            top: `${coords().top}px`,
            left: `${coords().left}px`,
            '--arrow-left': `${coords().arrowLeft}px`,
            'z-index': 9999,
          }}
        >
          <button
            type="button"
            class="role-switch-btn"
            title={label()}
            aria-label={label()}
            onClick={() => {
              props.onSelect()
              props.onClose()
            }}
          >
            <span class="role-switch-row">
              <Show
                when={props.role === 'master'}
                fallback={
                  <>
                    <span class="role-switch-icon">
                      <HobbitIcon size={20} />
                    </span>
                    <span class="role-switch-arrow" aria-hidden="true">→</span>
                    <span class="role-switch-icon">
                      <GandalfIcon size={20} />
                    </span>
                  </>
                }
              >
                <span class="role-switch-icon">
                  <GandalfIcon size={20} />
                </span>
                <span class="role-switch-arrow" aria-hidden="true">→</span>
                <span class="role-switch-icon">
                  <HobbitIcon size={20} />
                </span>
              </Show>
            </span>
          </button>
        </div>
      </Portal>
    </Show>
  )
}
