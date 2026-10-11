import { createEffect, createSignal, onCleanup, Show } from 'solid-js'
import { Portal } from 'solid-js/web'
import { ExpandArrowsIcon } from './ExpandArrowsIcon'
import type { SegmentBreakdown } from '../api'

export interface SegmentBreakdownPopoverProps {
  title?: string
  total?: number | string
  breakdown?: SegmentBreakdown | Record<string, number | string>
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

export function SegmentBreakdownPopover(props: SegmentBreakdownPopoverProps) {
  const [isOpen, setIsOpen] = createSignal(false)
  const [coords, setCoords] = createSignal<{ top: number; left: number }>({ top: 0, left: 0 })

  let triggerBtnRef: HTMLButtonElement | undefined
  let popoverRef: HTMLDivElement | undefined

  const updatePosition = () => {
    if (!triggerBtnRef) return
    const rect = triggerBtnRef.getBoundingClientRect()
    const popoverWidth = 175
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

  const equityVal = () => {
    const b = (props.breakdown || {}) as any
    return parseNum(b.equity ?? b.EQUITY)
  }

  const commodityVal = () => {
    const b = (props.breakdown || {}) as any
    return parseNum(b.commodity ?? b.COMMODITY)
  }

  const totalVal = () => {
    if (props.total !== undefined && props.total !== null && props.total !== '') {
      return parseNum(props.total)
    }
    return equityVal() + commodityVal()
  }

  const label = () => `View ${props.title || 'breakdown'}`

  return (
    <div class={`mtm-popover-anchor ${props.class ?? ''}`.trim()}>
      <button
        ref={triggerBtnRef}
        type="button"
        class="mtm-breakdown-trigger-btn"
        aria-label={label()}
        title={isOpen() ? undefined : label()}
        data-tooltip={isOpen() ? undefined : label()}
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
            aria-label={props.title || 'Segment breakdown'}
            style={{
              position: 'fixed',
              top: `${coords().top}px`,
              left: `${coords().left}px`,
              'z-index': 1000,
            }}
          >
            <div class="mtm-popover-row">
              <span class="mtm-popover-label">Equity</span>
              <span class="mtm-popover-value font-mono">
                {formatCurrency(equityVal())}
              </span>
            </div>

            <div class="mtm-popover-row">
              <span class="mtm-popover-label">Commodity</span>
              <span class="mtm-popover-value font-mono">
                {formatCurrency(commodityVal())}
              </span>
            </div>

            <div class="mtm-popover-divider" />

            <div class="mtm-popover-row mtm-popover-total">
              <span class="mtm-popover-label font-semibold">Total</span>
              <span class="mtm-popover-value font-mono font-semibold">
                {formatCurrency(totalVal())}
              </span>
            </div>
          </div>
        </Portal>
      </Show>
    </div>
  )
}
