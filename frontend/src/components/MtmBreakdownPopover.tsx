import { createEffect, createSignal, For, onCleanup, Show } from 'solid-js'
import { Portal } from 'solid-js/web'
import { ExpandArrowsIcon } from './ExpandArrowsIcon'

export interface MtmBreakdownPopoverProps {
  totalMtm?: number | string
  breakdown?: Record<string, number | string>
  class?: string
}

function parseNum(val: unknown): number {
  if (val === null || val === undefined || val === '') return 0
  const n = typeof val === 'number' ? val : Number(val)
  return Number.isNaN(n) ? 0 : n
}

function formatCurrency(val: unknown): string {
  const n = parseNum(val)
  const sign = n < 0 ? '-' : ''
  const abs = Math.abs(n)
  return `${sign}₹${abs.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export function MtmBreakdownPopover(props: MtmBreakdownPopoverProps) {
  const [isOpen, setIsOpen] = createSignal(false)
  const [coords, setCoords] = createSignal<{ top: number; left: number }>({ top: 0, left: 0 })

  let triggerBtnRef: HTMLButtonElement | undefined
  let popoverRef: HTMLDivElement | undefined

  const updatePosition = () => {
    if (!triggerBtnRef) return
    const rect = triggerBtnRef.getBoundingClientRect()
    const popoverWidth = 170
    let left = rect.left + rect.width / 2 - popoverWidth / 2
    if (left < 8) left = 8
    if (typeof window !== 'undefined' && window.innerWidth > 0 && left + popoverWidth > window.innerWidth - 8) {
      left = window.innerWidth - popoverWidth - 8
    }
    setCoords({
      top: Math.round(rect.bottom + 4),
      left: Math.round(left),
    })
  }

  const toggleOpen = (e: MouseEvent) => {
    e.stopPropagation()
    const next = !isOpen()
    if (next) {
      updatePosition()
    }
    setIsOpen(next)
  }

  createEffect(() => {
    if (!isOpen()) return

    updatePosition()

    const handlePointerDown = (e: MouseEvent) => {
      const target = e.target as Node | null
      if (!target) return
      if (triggerBtnRef?.contains(target)) return
      if (popoverRef?.contains(target)) return
      setIsOpen(false)
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }

    const handleScrollOrResize = () => {
      updatePosition()
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    window.addEventListener('resize', handleScrollOrResize)
    window.addEventListener('scroll', handleScrollOrResize, true)

    onCleanup(() => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('resize', handleScrollOrResize)
      window.removeEventListener('scroll', handleScrollOrResize, true)
    })
  })

  const misVal = () => {
    const b = props.breakdown || {}
    return parseNum(b['MIS'] ?? b['mis'])
  }

  const nrmlVal = () => {
    const b = props.breakdown || {}
    return parseNum(b['NRML'] ?? b['nrml'])
  }

  const totalVal = () => parseNum(props.totalMtm)

  const otherProducts = () => {
    const b = props.breakdown || {}
    const items: { product: string; value: number }[] = []
    for (const [prod, val] of Object.entries(b)) {
      const pUpper = prod.toUpperCase()
      if (pUpper !== 'MIS' && pUpper !== 'NRML' && pUpper !== 'CNC') {
        const num = parseNum(val)
        if (num !== 0) {
          items.push({ product: pUpper, value: num })
        }
      }
    }
    return items
  }

  return (
    <div class={`mtm-popover-anchor ${props.class ?? ''}`.trim()}>
      <button
        ref={triggerBtnRef}
        type="button"
        class="mtm-breakdown-trigger-btn"
        aria-label="View MTM breakdown"
        title="View MTM breakdown"
        data-tooltip="View MTM breakdown"
        data-active={isOpen() ? 'true' : 'false'}
        onClick={toggleOpen}
      >
        <ExpandArrowsIcon size={12} />
      </button>

      <Show when={isOpen()}>
        <Portal>
          <div
            ref={popoverRef}
            class="mtm-breakdown-popover"
            role="dialog"
            aria-label="MTM breakdown"
            style={{
              position: 'fixed',
              top: `${coords().top}px`,
              left: `${coords().left}px`,
              'z-index': 1000,
            }}
          >
            <div class="mtm-popover-row">
              <span class="mtm-popover-label">MIS</span>
              <span
                class={`mtm-popover-value font-mono ${
                  misVal() > 0 ? 'text-positive' : misVal() < 0 ? 'text-negative' : ''
                }`}
              >
                {formatCurrency(misVal())}
              </span>
            </div>

            <div class="mtm-popover-row">
              <span class="mtm-popover-label">NRML</span>
              <span
                class={`mtm-popover-value font-mono ${
                  nrmlVal() > 0 ? 'text-positive' : nrmlVal() < 0 ? 'text-negative' : ''
                }`}
              >
                {formatCurrency(nrmlVal())}
              </span>
            </div>

            <For each={otherProducts()}>
              {(item) => (
                <div class="mtm-popover-row">
                  <span class="mtm-popover-label">{item.product}</span>
                  <span
                    class={`mtm-popover-value font-mono ${
                      item.value > 0 ? 'text-positive' : item.value < 0 ? 'text-negative' : ''
                    }`}
                  >
                    {formatCurrency(item.value)}
                  </span>
                </div>
              )}
            </For>

            <div class="mtm-popover-divider" />

            <div class="mtm-popover-row mtm-popover-total">
              <span class="mtm-popover-label font-semibold">Total</span>
              <span
                class={`mtm-popover-value font-mono font-semibold ${
                  totalVal() > 0 ? 'text-positive' : totalVal() < 0 ? 'text-negative' : ''
                }`}
              >
                {formatCurrency(totalVal())}
              </span>
            </div>
          </div>
        </Portal>
      </Show>
    </div>
  )
}
