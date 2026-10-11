import { createEffect, createSignal, onCleanup, Show } from 'solid-js'
import { Portal } from 'solid-js/web'
import { ExpandArrowsIcon } from './ExpandArrowsIcon'
import type { MarginBreakdown } from '../api'

export interface MarginBreakdownPopoverProps {
  availableMargin?: number | string
  availableCash?: number | string
  breakdown?: MarginBreakdown
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

export function MarginBreakdownPopover(props: MarginBreakdownPopoverProps) {
  const [isOpen, setIsOpen] = createSignal(false)
  const [coords, setCoords] = createSignal<{ top: number; left: number }>({ top: 0, left: 0 })

  let triggerBtnRef: HTMLButtonElement | undefined
  let popoverRef: HTMLDivElement | undefined

  const updatePosition = () => {
    if (!triggerBtnRef) return
    const rect = triggerBtnRef.getBoundingClientRect()
    const popoverWidth = 200
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

  const cashVal = () => {
    if (props.breakdown && props.breakdown.cash !== undefined) {
      return parseNum(props.breakdown.cash)
    }
    return parseNum(props.availableCash)
  }

  const openingCashVal = () => {
    if (props.breakdown && props.breakdown.openingCash !== undefined) {
      return parseNum(props.breakdown.openingCash)
    }
    return undefined
  }

  const collateralVal = () => {
    if (props.breakdown && props.breakdown.collateral !== undefined) {
      return parseNum(props.breakdown.collateral)
    }
    return 0
  }

  const utilisedVal = () => {
    if (props.breakdown && props.breakdown.utilised !== undefined) {
      return parseNum(props.breakdown.utilised)
    }
    return 0
  }

  const netVal = () => {
    if (props.breakdown && props.breakdown.net !== undefined) {
      return parseNum(props.breakdown.net)
    }
    return parseNum(props.availableMargin)
  }

  return (
    <div class={`margin-popover-anchor ${props.class ?? ''}`.trim()}>
      <button
        ref={triggerBtnRef}
        type="button"
        class="margin-breakdown-trigger-btn"
        aria-label="View Margin Breakdown"
        title={isOpen() ? undefined : 'View Margin Breakdown'}
        data-tooltip={isOpen() ? undefined : 'View Margin Breakdown'}
        data-active={isOpen() ? 'true' : 'false'}
        onClick={toggleOpen}
      >
        <ExpandArrowsIcon size={12} />
      </button>

      <Show when={isOpen()}>
        <Portal>
          <div
            ref={popoverRef}
            class="margin-breakdown-popover"
            role="dialog"
            aria-label="Margin breakdown"
            style={{
              position: 'fixed',
              top: `${coords().top}px`,
              left: `${coords().left}px`,
              'z-index': 1000,
            }}
          >
            <div class="margin-popover-row">
              <span class="margin-popover-label">Live Cash</span>
              <span
                class={`margin-popover-value font-mono ${
                  cashVal() < 0 ? 'text-negative' : ''
                }`}
              >
                {formatCurrency(cashVal())}
              </span>
            </div>

            <Show when={openingCashVal() !== undefined}>
              <div class="margin-popover-row">
                <span class="margin-popover-label">Opening Cash</span>
                <span class="margin-popover-value font-mono">
                  {formatCurrency(openingCashVal())}
                </span>
              </div>
            </Show>

            <div class="margin-popover-row">
              <span class="margin-popover-label">Collateral</span>
              <span class="margin-popover-value font-mono text-positive">
                {formatCurrency(collateralVal())}
              </span>
            </div>

            <Show when={utilisedVal() > 0}>
              <div class="margin-popover-row">
                <span class="margin-popover-label">Utilised</span>
                <span class="margin-popover-value font-mono text-negative">
                  {formatCurrency(utilisedVal())}
                </span>
              </div>
            </Show>

            <div class="margin-popover-divider" />

            <div class="margin-popover-row margin-popover-total">
              <span class="margin-popover-label font-semibold">Total Margin</span>
              <span class="margin-popover-value font-mono font-semibold">
                {formatCurrency(netVal())}
              </span>
            </div>
          </div>
        </Portal>
      </Show>
    </div>
  )
}
