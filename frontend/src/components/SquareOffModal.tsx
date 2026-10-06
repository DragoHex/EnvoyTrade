import { createSignal, createResource, createEffect, For, Show } from 'solid-js'
import { getAccountOrders, squareOffAccount, squareOffGroup } from '../api'
import { SyncIcon, CropSquareIcon, InfoIcon } from './icons'

export interface SquareOffModalTarget {
  type: 'group' | 'account'
  id: string // groupId or accountId
  name?: string
  brokerAccountId?: string
  masterId?: string // for group, masterId to fetch positions
}

export interface SquareOffModalProps {
  open: boolean
  target: SquareOffModalTarget | null
  onConfirm?: (symbols?: string[]) => Promise<void> | void
  onCancel: () => void
  onSuccess?: () => void
}

export function SquareOffModal(props: SquareOffModalProps) {
  const [selectedSymbols, setSelectedSymbols] = createSignal<string[]>([])
  const [submitting, setSubmitting] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)

  // Determine which account's positions to load
  const targetAccountId = () => {
    if (!props.open || !props.target) return null
    if (props.target.type === 'group') {
      return props.target.masterId || props.target.id
    }
    return props.target.id
  }

  const [ordersData] = createResource(targetAccountId, (id) =>
    getAccountOrders(id, 'open_positions')
  )

  const openPositions = () => ordersData()?.openPositions ?? []
  let headerCheckboxRef: HTMLInputElement | undefined

  createEffect(() => {
    if (headerCheckboxRef) {
      headerCheckboxRef.indeterminate =
        selectedSymbols().length > 0 && selectedSymbols().length < openPositions().length
    }
  })

  // Initialize selected symbols when positions load or modal opens
  createEffect(() => {
    if (props.open) {
      setError(null)
      setSubmitting(false)
      const positions = openPositions()
      if (positions.length > 0) {
        setSelectedSymbols(positions.map((p) => p.instrument))
      } else {
        setSelectedSymbols([])
      }
    }
  })

  const toggleSelectAll = () => {
    const all = openPositions().map((p) => p.instrument)
    if (selectedSymbols().length === all.length) {
      setSelectedSymbols([])
    } else {
      setSelectedSymbols(all)
    }
  }

  const toggleSymbol = (symbol: string) => {
    const current = selectedSymbols()
    if (current.includes(symbol)) {
      setSelectedSymbols(current.filter((s) => s !== symbol))
    } else {
      setSelectedSymbols([...current, symbol])
    }
  }

  const handleConfirm = async () => {
    if (!props.target) return
    setSubmitting(true)
    setError(null)

    const all = openPositions()
    const symbolsPayload =
      selectedSymbols().length === all.length || all.length === 0
        ? undefined
        : selectedSymbols()

    try {
      if (props.onConfirm) {
        await props.onConfirm(symbolsPayload)
      } else if (props.target.type === 'group') {
        await squareOffGroup(props.target.id, symbolsPayload ? { symbols: symbolsPayload } : undefined)
      } else {
        await squareOffAccount(props.target.id, symbolsPayload ? { symbols: symbolsPayload } : undefined)
      }
      props.onSuccess?.()
      props.onCancel()
    } catch (err: any) {
      setError(err.message || 'Square-off execution failed')
    } finally {
      setSubmitting(false)
    }
  }

  const targetDisplayName = () => {
    if (!props.target) return ''
    return props.target.name || props.target.brokerAccountId || props.target.id
  }

  return (
    <Show when={props.open && props.target}>
      <div
        role="dialog"
        aria-label="Confirm Square Off"
        class="confirm-modal-overlay"
        onClick={(e) => {
          if (e.target === e.currentTarget && !submitting()) props.onCancel()
        }}
      >
        <div class="confirm-modal square-off-modal" onClick={(e) => e.stopPropagation()}>
          <div class="square-off-modal-header">
            <div class="header-title-row">
              <h3>Square Off Positions</h3>
              <CropSquareIcon class="square-off-header-icon" />
            </div>
            <div class="target-name-row">
              <span class="target-name">
                {props.target!.type === 'group' ? `Group: ${targetDisplayName()}` : `Follower: ${targetDisplayName()}`}
              </span>
              <span
                class={`info-tooltip-trigger ${
                  props.target!.type === 'group' ? 'tooltip-danger' : 'tooltip-warning'
                }`}
                data-tooltip={
                  props.target!.type === 'group'
                    ? "Master square-off will cascade to all active followers in this group. Open limit orders will be cancelled and positions flattened."
                    : "Only this follower account will be squared off. Master and other follower positions remain untouched."
                }
                aria-label="Info"
              >
                <InfoIcon size={14} />
              </span>
            </div>
          </div>

          <Show when={error()}>
            <div class="square-off-error-banner">{error()}</div>
          </Show>

          <div class="square-off-positions-section">
            <div class="square-off-section-header">
              <span class="section-title">Open Positions</span>
              <Show when={openPositions().length > 0}>
                <span class="selection-count">
                  {selectedSymbols().length} of {openPositions().length} selected
                </span>
              </Show>
            </div>

            <Show
              when={!ordersData.loading}
              fallback={null}
            >
              <Show
                when={openPositions().length > 0}
                fallback={
                  <div class="square-off-empty">
                    <p>No open positions recorded locally.</p>
                    <small>Confirming will query the broker and square off any open positions.</small>
                  </div>
                }
              >
                <div class="square-off-table-wrapper">
                  <table class="square-off-table">
                    <thead>
                      <tr>
                        <th class="col-check">
                          <input
                            ref={headerCheckboxRef}
                            type="checkbox"
                            aria-label="Select All Symbols"
                            checked={
                              openPositions().length > 0 &&
                              selectedSymbols().length === openPositions().length
                            }
                            onChange={toggleSelectAll}
                            disabled={submitting()}
                          />
                        </th>
                        <th>Symbol</th>
                        <th>Product</th>
                        <th class="text-right">Qty</th>
                        <th class="text-right">Avg Price</th>
                        <th class="text-right">LTP</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={openPositions()}>
                        {(pos) => {
                          const isSelected = () => selectedSymbols().includes(pos.instrument)
                          return (
                            <tr
                              class={isSelected() ? 'selected-row' : ''}
                              onClick={() => !submitting() && toggleSymbol(pos.instrument)}
                            >
                              <td class="col-check" onClick={(e) => e.stopPropagation()}>
                                <input
                                  type="checkbox"
                                  aria-label={`Select ${pos.instrument}`}
                                  checked={isSelected()}
                                  onChange={() => toggleSymbol(pos.instrument)}
                                  disabled={submitting()}
                                />
                              </td>
                              <td class="font-semibold">{pos.instrument}</td>
                              <td>
                                <span class="product-badge">{pos.product}</span>
                              </td>
                              <td
                                class={`text-right ${
                                  pos.qty > 0 ? 'text-success' : pos.qty < 0 ? 'text-danger' : ''
                                }`}
                              >
                                {pos.qty > 0 ? `+${pos.qty}` : pos.qty}
                              </td>
                              <td class="text-right">{pos.avgPrice || '—'}</td>
                              <td class="text-right">{pos.ltp || '—'}</td>
                            </tr>
                          )
                        }}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
            </Show>
          </div>

          <div class="confirm-modal-actions">
            <button
              type="button"
              class="confirm-modal-cancel"
              onClick={props.onCancel}
              disabled={submitting()}
            >
              Cancel
            </button>
            <button
              type="button"
              class="confirm-modal-confirm square-off-confirm-btn"
              onClick={handleConfirm}
              disabled={submitting()}
            >
              <Show when={submitting()} fallback={<CropSquareIcon />}>
                <SyncIcon spinning={true} />
              </Show>
              <span>{submitting() ? 'Squaring off...' : 'Sq-off'}</span>
            </button>
          </div>
        </div>
      </div>
    </Show>
  )
}
