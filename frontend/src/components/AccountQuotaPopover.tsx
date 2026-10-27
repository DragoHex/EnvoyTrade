import { createEffect, createSignal, For, onCleanup, Show } from 'solid-js'
import { Portal } from 'solid-js/web'
import { getBrokerConfig } from './BrokerLogo'

export interface AccountQuotaPopoverProps {
  maxAccounts: number // 0 = unlimited
  usedAccounts: number
  brokerLimits?: Record<string, number>
  brokerUsed?: Record<string, number>
  class?: string
}

export function AccountQuotaPopover(props: AccountQuotaPopoverProps) {
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

  const radius = 10
  const circumference = 2 * Math.PI * radius

  const fraction = () => {
    if (props.maxAccounts === -1) return 0
    if (props.maxAccounts === 0) return props.usedAccounts > 0 ? 1 : 0
    return Math.min(1, Math.max(0, props.usedAccounts / props.maxAccounts))
  }

  const dashOffset = () => circumference * (1 - fraction())

  const strokeColor = () => {
    if (props.maxAccounts === -1) return 'var(--color-primary, #6366f1)'
    if (props.maxAccounts >= 0 && props.usedAccounts > props.maxAccounts) {
      return 'var(--color-error, #ef4444)'
    }
    if (fraction() >= 0.8) return '#f59e0b'
    return 'var(--color-accent, #10b981)'
  }

  const brokersList = () => {
    const limits = props.brokerLimits ?? {}
    const used = props.brokerUsed ?? {}

    let calcGeneralUsed = used['general']
    if (calcGeneralUsed == null) {
      let excess = 0
      for (const [k, count] of Object.entries(used)) {
        if (k === 'general') continue
        const limit = limits[k] ?? 0
        if (count > limit) {
          excess += count - limit
        }
      }
      calcGeneralUsed = excess
    }

    const coreKeys = ['kite', 'dhan', 'general']
    const allKeys = new Set([
      ...coreKeys,
      ...Object.keys(limits),
      ...Object.keys(used),
    ])

    const result: Array<{ id: string; name: string; limit: number; used: number }> = []

    for (const key of allKeys) {
      const bKey = key.toLowerCase()
      if (bKey === 'testbroker') {
        continue // dev mock broker strictly hidden from client breakdown
      }
      const limit = limits[bKey] ?? 0
      const u = bKey === 'general' ? calcGeneralUsed : (used[bKey] ?? 0)

      if (!coreKeys.includes(bKey) && limit <= 0 && u <= 0) {
        continue
      }

      let name = bKey
      if (bKey === 'general') {
        name = 'Generalist'
      } else {
        name = getBrokerConfig(bKey).name
      }

      result.push({
        id: bKey,
        name,
        limit,
        used: u,
      })
    }

    const order = ['kite', 'dhan', 'general']
    return result.sort((a, b) => {
      const idxA = order.indexOf(a.id)
      const idxB = order.indexOf(b.id)
      if (idxA !== -1 && idxB !== -1) return idxA - idxB
      if (idxA !== -1) return -1
      if (idxB !== -1) return 1
      return a.name.localeCompare(b.name)
    })
  }

  return (
    <div class={`account-quota-anchor ${props.class ?? ''}`.trim()} style={{ position: 'relative', display: 'inline-flex' }}>
      <button
        ref={triggerBtnRef}
        type="button"
        class="account-quota-trigger-btn"
        aria-label="View Account Quota"
        title="View Account Quota Breakdown"
        onClick={toggleOpen}
        style={{
          display: 'inline-flex',
          'align-items': 'center',
          gap: '0.45rem',
          padding: '0.3rem 0.6rem',
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          'border-radius': '6px',
          cursor: 'pointer',
          color: 'var(--color-text)',
          'font-size': '0.8rem',
          'line-height': 1,
          transition: 'border-color 0.15s ease',
        }}
      >
        <svg width="24" height="24" viewBox="0 0 24 24" style={{ transform: 'rotate(-90deg)', 'flex-shrink': 0 }} aria-hidden="true">
          <circle
            cx="12"
            cy="12"
            r={radius}
            fill="none"
            stroke="var(--color-border)"
            stroke-width="3"
          />
          <Show when={props.maxAccounts !== -1}>
            <circle
              cx="12"
              cy="12"
              r={radius}
              fill="none"
              stroke={strokeColor()}
              stroke-width="3"
              stroke-dasharray={`${circumference}`}
              stroke-dashoffset={`${dashOffset()}`}
              stroke-linecap="round"
              style={{ transition: 'stroke-dashoffset 0.3s ease' }}
            />
          </Show>
          <Show when={props.maxAccounts === -1}>
            <circle
              cx="12"
              cy="12"
              r="4"
              fill="var(--color-primary, #6366f1)"
            />
          </Show>
        </svg>

        <span style={{ 'font-weight': 600 }}>
          {props.usedAccounts} / {props.maxAccounts === -1 ? '∞' : props.maxAccounts}
        </span>
        <span style={{ color: 'var(--color-text-secondary)', 'font-size': '0.75rem' }}>
          Accounts
        </span>
      </button>

      <Show when={isOpen()}>
        <Portal>
          <div
            ref={popoverRef}
            class="mtm-breakdown-popover"
            role="dialog"
            aria-label="Account Quota Breakdown"
            style={{
              position: 'fixed',
              top: `${coords().top}px`,
              left: `${coords().left}px`,
              'z-index': 1000,
              'min-width': '200px',
            }}
          >
            <div style={{ 'font-weight': 600, 'margin-bottom': '6px', 'font-size': '0.8rem', color: 'var(--color-text)' }}>
              Account Quota Breakdown
            </div>

            <For each={brokersList()}>
              {(b) => (
                <div class="mtm-popover-row">
                  <span class="mtm-popover-label">{b.name}</span>
                  <span
                    class={`mtm-popover-value font-mono ${
                      props.maxAccounts !== -1 && b.used > b.limit ? 'text-negative font-semibold' : ''
                    }`}
                  >
                    {b.used} / {props.maxAccounts === -1 ? '∞' : b.limit}
                  </span>
                </div>
              )}
            </For>

            <div class="mtm-popover-divider" />

            <div class="mtm-popover-row mtm-popover-total">
              <span class="mtm-popover-label font-semibold">Total Accounts</span>
              <span
                class={`mtm-popover-value font-mono font-semibold ${
                  props.maxAccounts !== -1 && props.usedAccounts > props.maxAccounts ? 'text-negative' : ''
                }`}
              >
                {props.usedAccounts} / {props.maxAccounts === -1 ? '∞' : props.maxAccounts}
              </span>
            </div>
          </div>
        </Portal>
      </Show>
    </div>
  )
}
