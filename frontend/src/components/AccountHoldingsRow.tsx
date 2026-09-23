import { createMemo, createResource, createSignal, For, Show } from 'solid-js'
import { getAccountOrders, postAction } from '../api'
import { EmptyState } from './EmptyState'
import { SyncIcon } from './icons'

function toNumber(val: unknown, fallback = 0): number {
  if (val === null || val === undefined || val === '') return fallback
  const n = typeof val === 'number' ? val : Number(val)
  return Number.isFinite(n) ? n : fallback
}

function formatPrice(val: unknown): string {
  if (val === null || val === undefined || val === '') return '—'
  const n = toNumber(val, NaN)
  if (Number.isNaN(n)) return '—'
  return n.toFixed(2)
}

function formatCurrency(val: unknown): string {
  const n = toNumber(val, 0)
  const sign = n < 0 ? '-' : ''
  const abs = Math.abs(n)
  return `${sign}₹${abs.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function formatNumber(val: unknown): string {
  const n = toNumber(val, 0)
  return n.toLocaleString('en-IN')
}

export function AccountHoldingsRow(props: {
  accountId: string
  accountName?: string
  brokerAccountId?: string
  colspan: number
}) {
  const [holdingSortField, setHoldingSortField] = createSignal<'instrument' | 'sellableQuantity'>('instrument')
  const [holdingSortDir, setHoldingSortDir] = createSignal<'asc' | 'desc'>('asc')
  const [syncing, setSyncing] = createSignal(false)
  const [syncError, setSyncError] = createSignal<string | null>(null)

  const [ordersData, { refetch }] = createResource(
    () => props.accountId,
    (accId) => getAccountOrders(accId, 'holdings'),
  )

  const handleSync = async () => {
    setSyncing(true)
    setSyncError(null)
    try {
      await postAction(props.accountId, 'sync_positions')
      await refetch()
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : 'Sync failed')
    } finally {
      setSyncing(false)
    }
  }

  const holdings = () => ordersData()?.holdings ?? []

  const sortedHoldings = createMemo(() => {
    const list = [...holdings()]
    const field = holdingSortField()
    const dir = holdingSortDir() === 'asc' ? 1 : -1
    return list.sort((a, b) => {
      if (field === 'instrument') {
        return a.instrument.localeCompare(b.instrument) * dir
      }
      return (toNumber(a.sellableQuantity) - toNumber(b.sellableQuantity)) * dir
    })
  })

  function toggleHoldingSort(field: 'instrument' | 'sellableQuantity') {
    if (holdingSortField() === field) {
      setHoldingSortDir((prev) => (prev === 'asc' ? 'desc' : 'asc'))
    } else {
      setHoldingSortField(field)
      setHoldingSortDir('asc')
    }
  }

  return (
    <tr class="account-details-expansion-row" data-testid="account-holdings-expansion-row">
      <td colspan={props.colspan} class="account-details-expansion-cell">
        <div class="account-order-drawer">
          <div class="order-drawer-header">
            <div style={{ display: 'flex', 'align-items': 'center', gap: '8px' }}>
              <span style={{ 'font-size': '13px', 'font-weight': 600, color: 'var(--color-text)' }}>
                Current Holdings
              </span>
              <span style={{ 'font-size': '12px', color: 'var(--color-text-secondary)' }}>
                ({holdings().length} {holdings().length === 1 ? 'item' : 'items'})
              </span>
            </div>
            <div style={{ display: 'flex', 'align-items': 'center', gap: '8px' }}>
              <button
                type="button"
                class="icon-button"
                disabled={syncing() || ordersData.loading}
                onClick={handleSync}
                aria-label="Sync from Kite"
                data-tooltip="Sync from Kite"
                title="Sync from Kite"
              >
                <SyncIcon spinning={syncing()} />
              </button>
            </div>
          </div>

          <Show when={syncError()}>
            {(msg) => (
              <div
                role="alert"
                style={{
                  padding: '8px 14px',
                  margin: '10px 16px 0 16px',
                  'background-color': 'rgba(239, 68, 68, 0.1)',
                  border: '1px solid #ef4444',
                  'border-radius': '4px',
                  color: '#ef4444',
                  'font-size': '12px',
                }}
              >
                {msg()}
              </div>
            )}
          </Show>

          <div class="order-drawer-body" style={{ padding: '12px 16px' }}>
            <div class={`order-table-container ${ordersData.loading ? 'is-fetching' : ''}`}>
              <table class="order-subtable" style={{ width: '100%', 'table-layout': 'fixed' }}>
                <thead>
                  <tr>
                    <th
                      class="cursor-pointer select-none sortable-header"
                      style={{ width: '28%', 'text-align': 'left' }}
                      onClick={() => toggleHoldingSort('instrument')}
                    >
                      Instrument{' '}
                      {holdingSortField() === 'instrument'
                        ? holdingSortDir() === 'asc'
                          ? '▲'
                          : '▼'
                        : '↕'}
                    </th>
                    <th
                      class="col-center cursor-pointer select-none sortable-header"
                      style={{ width: '18%', 'text-align': 'center' }}
                      onClick={() => toggleHoldingSort('sellableQuantity')}
                    >
                      Sellable Quantity{' '}
                      {holdingSortField() === 'sellableQuantity'
                        ? holdingSortDir() === 'asc'
                          ? '▲'
                          : '▼'
                        : '↕'}
                    </th>
                    <th class="col-center" style={{ width: '18%', 'text-align': 'center' }}>
                      Buy Average Price
                    </th>
                    <th class="col-center" style={{ width: '18%', 'text-align': 'center' }}>
                      LTP
                    </th>
                    <th class="col-center" style={{ width: '18%', 'text-align': 'center' }}>
                      P&L
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={sortedHoldings().length > 0}
                    fallback={
                      <tr>
                        <td colspan={5} class="table-empty-cell" style={{ 'text-align': 'center', padding: '32px 16px' }}>
                          <EmptyState message="No holdings found" />
                        </td>
                      </tr>
                    }
                  >
                    <For each={sortedHoldings()}>
                      {(h) => (
                        <tr>
                          <td class="font-medium" style={{ 'text-align': 'left' }}>
                            {h.instrument}
                          </td>
                          <td class="col-center font-mono" style={{ 'text-align': 'center' }}>
                            {formatNumber(h.sellableQuantity)}
                          </td>
                          <td class="col-center font-mono" style={{ 'text-align': 'center' }}>
                            {formatPrice(h.buyAveragePrice)}
                          </td>
                          <td class="col-center font-mono" style={{ 'text-align': 'center' }}>
                            {formatPrice(h.ltp)}
                          </td>
                          <td
                            class={`col-center font-mono font-medium ${
                              toNumber(h.pnl) > 0
                                ? 'text-positive'
                                : toNumber(h.pnl) < 0
                                ? 'text-negative'
                                : ''
                            }`}
                            style={{ 'text-align': 'center' }}
                          >
                            {formatCurrency(h.pnl)}
                          </td>
                        </tr>
                      )}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </td>
    </tr>
  )
}
