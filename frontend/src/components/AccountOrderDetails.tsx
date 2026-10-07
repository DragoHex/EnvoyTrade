import { createSignal, createResource, createMemo, createEffect, Show, For, onCleanup, untrack } from 'solid-js'
import { createStore, reconcile } from 'solid-js/store'
import {
  getAccountOrders,
  type AccountSummaryMetrics,
  type TabCounts,
  type PaginationInfo,
  type PositionItem,
  type HoldingItem,
  type OrderDetailItem,
  type AccountOrdersResponse,
} from '../api'
import { isOrdersDataEqual } from '../utils/ordersDiff'
import { formatISTDateTime } from '../utils/time'
import {
  ExitSquareIcon,
  ProhibitIcon,
  DoorExitIcon,
} from './icons'
import { EmptyState } from './EmptyState'

export type OrderTabId =
  | 'open_positions'
  | 'closed_positions'
  | 'holdings'
  | 'open_orders'
  | 'closed_orders'
  | 'rejected_orders'

interface AccountOrderDetailsProps {
  accountId: string
  accountName?: string
  brokerAccountId?: string
  onClose?: () => void
  isExpanded?: boolean
  onSummaryChange?: (summary: AccountSummaryMetrics) => void
}

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

interface OrdersStoreState {
  summary: AccountSummaryMetrics
  counts?: TabCounts
  pagination?: PaginationInfo
  openPositions: PositionItem[]
  closedPositions: PositionItem[]
  holdings: HoldingItem[]
  openOrders: OrderDetailItem[]
  closedOrders: OrderDetailItem[]
  rejectedOrders: OrderDetailItem[]
}

function createInitialStoreState(): OrdersStoreState {
  return {
    summary: {
      netQty: 0,
      openPositionsCount: 0,
      openCount: 0,
      closedPositionsCount: 0,
      closedCount: 0,
      pendingOrdersCount: 0,
      pendingMetric: 0,
      totalMtm: 0,
      realizedPnl: 0,
      accountValue: 0,
      status: 'offline',
    },
    counts: {
      openPositions: 0,
      closedPositions: 0,
      holdings: 0,
      openOrders: 0,
      closedOrders: 0,
      rejectedOrders: 0,
    },
    pagination: {
      tab: '' as OrderTabId,
      page: 1,
      limit: 10,
      totalCount: 0,
      totalPages: 1,
    },
    openPositions: [],
    closedPositions: [],
    holdings: [],
    openOrders: [],
    closedOrders: [],
    rejectedOrders: [],
  }
}

export function AccountOrderDetails(props: AccountOrderDetailsProps) {
  const [activeTab, setActiveTab] = createSignal<OrderTabId>('open_positions')
  const [page, setPage] = createSignal<number>(1)
  const [holdingSortField, setHoldingSortField] = createSignal<'instrument' | 'sellableQuantity'>('instrument')
  const [holdingSortDir, setHoldingSortDir] = createSignal<'asc' | 'desc'>('asc')
  const [isPolling, setIsPolling] = createSignal<boolean>(false)
  const [hasLoaded, setHasLoaded] = createSignal<boolean>(false)

  const [store, setStore] = createStore<OrdersStoreState>(createInitialStoreState())

  createEffect((prevId) => {
    if (prevId !== undefined && prevId !== props.accountId) {
      setHasLoaded(false)
      setStore(reconcile(createInitialStoreState()))
    }
    return props.accountId
  })

  const isExpanded = () => props.isExpanded ?? true
  const isTableFetching = () => ordersData.loading && !isPolling()

  const [ordersData, { mutate, refetch }] = createResource(
    () => {
      if (!isExpanded()) return null
      return {
        accountId: props.accountId,
        tab: activeTab(),
        page: page(),
        limit: 10,
      }
    },
    ({ accountId, tab, page, limit }) => getAccountOrders(accountId, tab, page, limit),
  )

  const isTabReady = (tab: OrderTabId) => store.pagination?.tab === tab

  function normalizeOrders<T extends OrderDetailItem>(items: T[] | undefined): T[] {
    if (!items) return []
    return items.map((item, idx) => ({
      ...item,
      id: item.id || `${item.instrument}-${item.time ?? ''}-${item.type}-${idx}`,
    }))
  }

  function applyData(fresh: AccountOrdersResponse) {
    if (!fresh) return
    // Strict isolation guard: discard responses intended for any other account
    if (fresh.accountId && fresh.accountId !== props.accountId) return

    if (fresh.summary) {
      setStore('summary', reconcile(fresh.summary))
    }
    if (fresh.counts) {
      setStore('counts', reconcile(fresh.counts))
    }
    const responseTab = fresh.pagination?.tab ?? activeTab()

    if (fresh.pagination) {
      setStore('pagination', reconcile(fresh.pagination))
    } else {
      setStore('pagination', 'tab', responseTab)
    }

    if (responseTab === 'open_positions' || (fresh.openPositions && fresh.openPositions.length > 0)) {
      setStore('openPositions', reconcile(fresh.openPositions ?? [], { key: 'instrument' }))
    }
    if (responseTab === 'closed_positions' || (fresh.closedPositions && fresh.closedPositions.length > 0)) {
      setStore('closedPositions', reconcile(fresh.closedPositions ?? [], { key: 'instrument' }))
    }
    if (responseTab === 'holdings' || (fresh.holdings && fresh.holdings.length > 0)) {
      setStore('holdings', reconcile(fresh.holdings ?? [], { key: 'instrument' }))
    }
    if (responseTab === 'open_orders' || (fresh.openOrders && fresh.openOrders.length > 0)) {
      setStore('openOrders', reconcile(normalizeOrders(fresh.openOrders), { key: 'id' }))
    }
    if (responseTab === 'closed_orders' || (fresh.closedOrders && fresh.closedOrders.length > 0)) {
      setStore('closedOrders', reconcile(normalizeOrders(fresh.closedOrders), { key: 'id' }))
    }
    if (responseTab === 'rejected_orders' || (fresh.rejectedOrders && fresh.rejectedOrders.length > 0)) {
      setStore('rejectedOrders', reconcile(normalizeOrders(fresh.rejectedOrders), { key: 'id' }))
    }

    if (fresh.summary) {
      props.onSummaryChange?.(fresh.summary)
    }
  }

  createEffect(() => {
    const data = ordersData()
    if (data) {
      setHasLoaded(true)
      untrack(() => applyData(data))
    }
  })

  createEffect(() => {
    if (!isExpanded()) return
    const timer = setInterval(async () => {
      if (activeTab() === 'holdings') return
      try {
        setIsPolling(true)
        const currentTab = activeTab()
        const currentPage = page()
        const fresh = await getAccountOrders(props.accountId, currentTab, currentPage, 10)
        if (activeTab() === currentTab && page() === currentPage) {
          if (!isOrdersDataEqual(ordersData(), fresh, currentTab)) {
            mutate(fresh)
          }
        }
      } catch (err) {
        // Silently ignore background poll errors
      } finally {
        setIsPolling(false)
      }
    }, 7000)
    onCleanup(() => clearInterval(timer))
  })

  function switchTab(tab: OrderTabId) {
    if (tab === activeTab()) {
      refetch()
      return
    }
    setActiveTab(tab)
    setPage(1)
  }

  const summary = () => {
    const s = store.summary
    return {
      netQty: toNumber(s.netQty),
      openCount: s.openPositionsCount ?? s.openCount ?? 0,
      closedCount: s.closedPositionsCount ?? s.closedCount ?? 0,
      pendingMetric: s.pendingOrdersCount ?? s.pendingMetric ?? 0,
      totalMtm: toNumber(s.totalMtm),
      realizedPnl: toNumber(s.realizedPnl),
      accountValue: toNumber(s.accountValue),
      status: s.status ?? 'offline',
    }
  }

  const openPositions = () => store.openPositions
  const closedPositions = () => store.closedPositions
  const holdings = () => store.holdings
  const openOrders = () => store.openOrders
  const closedOrders = () => store.closedOrders
  const rejectedOrders = () => store.rejectedOrders

  const counts = () => ({
    openPositions: store.counts?.openPositions ?? store.openPositions.length,
    closedPositions: store.counts?.closedPositions ?? store.closedPositions.length,
    holdings: store.counts?.holdings ?? store.holdings.length,
    openOrders: store.counts?.openOrders ?? store.openOrders.length,
    closedOrders: store.counts?.closedOrders ?? store.closedOrders.length,
    rejectedOrders: store.counts?.rejectedOrders ?? store.rejectedOrders.length,
  })

  const pagination = () => {
    const p = store.pagination
    const currentTab = activeTab()
    let totalCount = 0
    switch (currentTab) {
      case 'open_positions':
        totalCount = counts().openPositions
        break
      case 'closed_positions':
        totalCount = counts().closedPositions
        break
      case 'holdings':
        totalCount = counts().holdings
        break
      case 'open_orders':
        totalCount = counts().openOrders
        break
      case 'closed_orders':
        totalCount = counts().closedOrders
        break
      case 'rejected_orders':
        totalCount = counts().rejectedOrders
        break
    }
    const curPage = p?.page ?? page()
    const limit = p?.limit ?? 10
    const totalPages = p?.totalPages ?? (totalCount > 0 ? Math.ceil(totalCount / limit) : 1)
    const start = totalCount > 0 ? (curPage - 1) * limit + 1 : 0
    const end = Math.min(curPage * limit, totalCount)
    return {
      page: curPage,
      totalPages: Math.max(1, totalPages),
      totalCount,
      start,
      end,
    }
  }

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
    <div class="account-order-drawer" data-testid="account-order-details">
      {/* 1. Global Persistent Summary Header */}
      <div class="order-drawer-header">
        <div class="order-summary-metrics">
          <div class="summary-metric-item">
            <span class="metric-label">Net Qty</span>
            <span class="metric-value font-mono">{formatNumber(summary().netQty)}</span>
          </div>

          <div class="summary-metric-divider" />

          <div class="summary-metric-item">
            <span class="metric-label">Open / Closed</span>
            <span class="metric-value font-mono">
              {summary().openCount} / {summary().closedCount}
            </span>
          </div>

          <div class="summary-metric-divider" />

          <div class="summary-metric-item">
            <span class="metric-label">Pending</span>
            <span class="metric-value font-mono">{summary().pendingMetric}</span>
          </div>

          <div class="summary-metric-divider" />

          <div class="summary-metric-item">
            <span class="metric-label">Total MTM</span>
            <span
              class={`metric-value font-mono font-semibold ${
                summary().totalMtm > 0
                  ? 'text-positive'
                  : summary().totalMtm < 0
                  ? 'text-negative'
                  : ''
              }`}
            >
              {formatCurrency(summary().totalMtm)}
            </span>
          </div>

          <div class="summary-metric-divider" />

          <div class="summary-metric-item">
            <span class="metric-label">Realized P&L</span>
            <span
              class={`metric-value font-mono ${
                summary().realizedPnl > 0
                  ? 'text-positive'
                  : summary().realizedPnl < 0
                  ? 'text-negative'
                  : ''
              }`}
            >
              {formatCurrency(summary().realizedPnl)}
            </span>
          </div>

          <div class="summary-metric-divider" />

          <div class="summary-metric-item">
            <span class="metric-label">Margin / Value</span>
            <span class="metric-value font-mono font-semibold">
              {formatCurrency(summary().accountValue)}
            </span>
          </div>

          <div class="summary-metric-divider" />

          <div class="summary-status-item">
            <span
              class={`status-indicator-dot ${
                summary().status === 'online' || summary().status === 'ok'
                  ? 'status-dot-online'
                  : 'status-dot-offline'
              }`}
              title={summary().status === 'online' || summary().status === 'ok' ? 'Connected / Online' : 'Offline'}
            />
            <span class="status-text capitalize">{summary().status}</span>
          </div>
        </div>

        <div class="order-header-actions">
          <button
            type="button"
            class="header-action-btn"
            title="Prohibit Trading"
            aria-label="Prohibit Trading"
          >
            <ProhibitIcon />
          </button>
          <button
            type="button"
            class="header-action-btn"
            title="Exit / Logout"
            aria-label="Exit / Logout"
          >
            <DoorExitIcon />
          </button>
        </div>
      </div>

      {/* 2. Six Navigation Tabs */}
      <div class="order-drawer-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab() === 'open_positions'}
          class={`order-tab-btn ${activeTab() === 'open_positions' ? 'order-tab-active' : ''}`}
          onClick={() => switchTab('open_positions')}
        >
          Open Position ({counts().openPositions})
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab() === 'closed_positions'}
          class={`order-tab-btn ${activeTab() === 'closed_positions' ? 'order-tab-active' : ''}`}
          onClick={() => switchTab('closed_positions')}
        >
          Closed Position ({counts().closedPositions})
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab() === 'holdings'}
          class={`order-tab-btn ${activeTab() === 'holdings' ? 'order-tab-active' : ''}`}
          onClick={() => switchTab('holdings')}
        >
          Holding ({counts().holdings})
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab() === 'open_orders'}
          class={`order-tab-btn ${activeTab() === 'open_orders' ? 'order-tab-active' : ''}`}
          onClick={() => switchTab('open_orders')}
        >
          Open Order ({counts().openOrders})
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab() === 'closed_orders'}
          class={`order-tab-btn ${activeTab() === 'closed_orders' ? 'order-tab-active' : ''}`}
          onClick={() => switchTab('closed_orders')}
        >
          Closed Order ({counts().closedOrders})
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={activeTab() === 'rejected_orders'}
          class={`order-tab-btn ${activeTab() === 'rejected_orders' ? 'order-tab-active' : ''}`}
          onClick={() => switchTab('rejected_orders')}
        >
          Rejected Order ({counts().rejectedOrders})
        </button>
      </div>

      {/* 3. Tab Content Panels */}
      <div class="order-drawer-body">
        <Show when={!hasLoaded() && ordersData.loading}>
          <div class="order-loading-skeleton">
            <div class="skeleton-row" />
            <div class="skeleton-row" />
            <div class="skeleton-row" />
          </div>
        </Show>

        <Show when={hasLoaded()}>
          {/* Tab 1: Open Position */}
          <Show when={activeTab() === 'open_positions'}>
            <div class={`order-table-container ${isTableFetching() ? 'is-fetching' : ''}`}>
              <table class="order-subtable">
                <thead>
                  <tr>
                    <th style={{ width: '9%' }}>Product</th>
                    <th style={{ width: '27%' }}>Instrument</th>
                    <th class="text-right" style={{ width: '11%' }}>QTY</th>
                    <th style={{ width: '16%' }}>Avg Price</th>
                    <th class="text-right" style={{ width: '12%' }}>LTP</th>
                    <th class="text-right" style={{ width: '17%' }}>MTM</th>
                    <th class="text-center" style={{ width: '8%' }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={openPositions().length > 0}
                    fallback={
                      <tr>
                        <td colspan={7} class="table-empty-cell">
                          <Show when={isTabReady('open_positions')}>
                            <EmptyState message="No data" />
                          </Show>
                        </td>
                      </tr>
                    }
                  >
                    <For each={openPositions()}>
                      {(pos) => (
                        <tr>
                          <td>
                            <span class="product-badge">{pos.product}</span>
                          </td>
                          <td class="font-medium">{pos.instrument}</td>
                          <td
                            class={`text-right font-mono ${
                              pos.qty > 0 ? 'text-positive' : pos.qty < 0 ? 'text-negative' : ''
                            }`}
                          >
                            {pos.qty}
                          </td>
                          <td class="font-mono text-muted">{pos.avgPrice}</td>
                          <td class="text-right font-mono">{formatPrice(pos.ltp)}</td>
                          <td
                            class={`text-right font-mono font-medium ${
                              toNumber(pos.mtm) > 0
                                ? 'text-positive'
                                : toNumber(pos.mtm) < 0
                                ? 'text-negative'
                                : ''
                            }`}
                          >
                            {formatCurrency(pos.mtm)}
                          </td>
                          <td class="text-center">
                            <button
                              type="button"
                              class="table-action-icon-btn text-negative"
                              title="Exit Position"
                              aria-label="Exit Position"
                            >
                              <ExitSquareIcon />
                            </button>
                          </td>
                        </tr>
                      )}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Tab 2: Closed Position */}
          <Show when={activeTab() === 'closed_positions'}>
            <div class={`order-table-container ${isTableFetching() ? 'is-fetching' : ''}`}>
              <table class="order-subtable">
                <thead>
                  <tr>
                    <th style={{ width: '12%' }}>Product</th>
                    <th style={{ width: '36%' }}>Instrument</th>
                    <th style={{ width: '20%' }}>Avg Price (B/S)</th>
                    <th class="text-right" style={{ width: '14%' }}>LTP</th>
                    <th class="text-right" style={{ width: '18%' }}>P&L</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={closedPositions().length > 0}
                    fallback={
                      <tr>
                        <td colspan={5} class="table-empty-cell">
                          <Show when={isTabReady('closed_positions')}>
                            <EmptyState message="No data" />
                          </Show>
                        </td>
                      </tr>
                    }
                  >
                    <For each={closedPositions()}>
                      {(pos) => {
                        const val = () => pos.pnl !== undefined ? pos.pnl : pos.mtm
                        return (
                          <tr>
                            <td>
                              <span class="product-badge">{pos.product}</span>
                            </td>
                            <td class="font-medium">{pos.instrument}</td>
                            <td class="font-mono text-muted">{pos.avgPrice}</td>
                            <td class="text-right font-mono">{formatPrice(pos.ltp)}</td>
                            <td
                              class={`text-right font-mono font-medium ${
                                toNumber(val()) > 0
                                  ? 'text-positive'
                                  : toNumber(val()) < 0
                                  ? 'text-negative'
                                  : ''
                              }`}
                            >
                              {formatCurrency(val())}
                            </td>
                          </tr>
                        )
                      }}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Tab 3: Holding */}
          <Show when={activeTab() === 'holdings'}>
            <div class={`order-table-container ${isTableFetching() ? 'is-fetching' : ''}`}>
              <table class="order-subtable">
                <thead>
                  <tr>
                    <th
                      class="cursor-pointer select-none sortable-header"
                      style={{ width: '28%' }}
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
                      class="text-right cursor-pointer select-none sortable-header"
                      style={{ width: '16%' }}
                      onClick={() => toggleHoldingSort('sellableQuantity')}
                    >
                      Sellable Quantity{' '}
                      {holdingSortField() === 'sellableQuantity'
                        ? holdingSortDir() === 'asc'
                          ? '▲'
                          : '▼'
                        : '↕'}
                    </th>
                    <th class="text-right" style={{ width: '16%' }}>Buy Average Price</th>
                    <th class="text-right" style={{ width: '15%' }}>LTP</th>
                    <th class="text-right" style={{ width: '17%' }}>P&L</th>
                    <th class="text-center" style={{ width: '8%' }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={sortedHoldings().length > 0}
                    fallback={
                      <tr>
                        <td colspan={6} class="table-empty-cell">
                          <Show when={isTabReady('holdings')}>
                            <EmptyState message="No data" />
                          </Show>
                        </td>
                      </tr>
                    }
                  >
                    <For each={sortedHoldings()}>
                      {(h) => (
                        <tr>
                          <td class="font-medium">{h.instrument}</td>
                          <td class="text-right font-mono">{formatNumber(h.sellableQuantity)}</td>
                          <td class="text-right font-mono">{formatPrice(h.buyAveragePrice)}</td>
                          <td class="text-right font-mono">{formatPrice(h.ltp)}</td>
                          <td
                            class={`text-right font-mono font-medium ${
                              toNumber(h.pnl) > 0
                                ? 'text-positive'
                                : toNumber(h.pnl) < 0
                                ? 'text-negative'
                                : ''
                            }`}
                          >
                            {formatCurrency(h.pnl)}
                          </td>
                          <td class="text-center">
                            <button
                              type="button"
                              class="table-action-icon-btn text-negative"
                              title="Exit Holding"
                              aria-label="Exit Holding"
                            >
                              <ExitSquareIcon />
                            </button>
                          </td>
                        </tr>
                      )}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Tab 4: Open Order */}
          <Show when={activeTab() === 'open_orders'}>
            <div class={`order-table-container ${isTableFetching() ? 'is-fetching' : ''}`}>
              <table class="order-subtable">
                <thead>
                  <tr>
                    <th style={{ width: '9%' }}>Product</th>
                    <th style={{ width: '16%' }}>Time (IST)</th>
                    <th style={{ width: '23%' }}>Instrument</th>
                    <th class="text-right" style={{ width: '10%' }}>Quantity</th>
                    <th class="text-right" style={{ width: '12%' }}>Trigger Price</th>
                    <th class="text-right" style={{ width: '12%' }}>Limit Price</th>
                    <th class="text-center" style={{ width: '10%' }}>Type</th>
                    <th class="text-center" style={{ width: '8%' }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={openOrders().length > 0}
                    fallback={
                      <tr>
                        <td colspan={8} class="table-empty-cell">
                          <Show when={isTabReady('open_orders')}>
                            <EmptyState message="No data" />
                          </Show>
                        </td>
                      </tr>
                    }
                  >
                    <For each={openOrders()}>
                      {(ord) => (
                        <tr>
                          <td>
                            <span class="product-badge">{ord.product ?? 'CNC'}</span>
                          </td>
                          <td class="font-mono text-sm text-muted">{formatISTDateTime(ord.time)}</td>
                          <td class="font-medium">{ord.instrument}</td>
                          <td class="text-right font-mono">{ord.quantity}</td>
                          <td class="text-right font-mono">{formatPrice(ord.triggerPrice)}</td>
                          <td class="text-right font-mono">{formatPrice(ord.limitPrice)}</td>
                          <td class="text-center">
                            <span
                              class={`type-badge ${
                                ord.type === 'B' ? 'type-badge-buy' : 'type-badge-sell'
                              }`}
                            >
                              {ord.type}
                            </span>
                          </td>
                          <td class="text-center">
                            <button
                              type="button"
                              class="table-action-icon-btn text-negative"
                              title="Cancel / Exit Order"
                              aria-label="Cancel / Exit Order"
                            >
                              <ExitSquareIcon />
                            </button>
                          </td>
                        </tr>
                      )}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Tab 5: Closed Order */}
          <Show when={activeTab() === 'closed_orders'}>
            <div class={`order-table-container ${isTableFetching() ? 'is-fetching' : ''}`}>
              <table class="order-subtable">
                <thead>
                  <tr>
                    <th style={{ width: '11%' }}>Product</th>
                    <th style={{ width: '17%' }}>Time (IST)</th>
                    <th style={{ width: '28%' }}>Instrument</th>
                    <th class="text-right" style={{ width: '12%' }}>Quantity</th>
                    <th class="text-right" style={{ width: '18%' }}>Price</th>
                    <th class="text-center" style={{ width: '14%' }}>Type</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={closedOrders().length > 0}
                    fallback={
                      <tr>
                        <td colspan={6} class="table-empty-cell">
                          <Show when={isTabReady('closed_orders')}>
                            <EmptyState message="No data" />
                          </Show>
                        </td>
                      </tr>
                    }
                  >
                    <For each={closedOrders()}>
                      {(ord) => (
                        <tr>
                          <td>
                            <span class="product-badge">{ord.product ?? 'CNC'}</span>
                          </td>
                          <td class="font-mono text-sm text-muted">{formatISTDateTime(ord.time)}</td>
                          <td class="font-medium">{ord.instrument}</td>
                          <td class="text-right font-mono">{ord.quantity}</td>
                          <td class="text-right font-mono">{formatPrice(ord.price)}</td>
                          <td class="text-center">
                            <span
                              class={`type-badge ${
                                ord.type === 'B' ? 'type-badge-buy' : 'type-badge-sell'
                              }`}
                            >
                              {ord.type}
                            </span>
                          </td>
                        </tr>
                      )}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </Show>

          {/* Tab 6: Rejected Order */}
          <Show when={activeTab() === 'rejected_orders'}>
            <div class={`order-table-container ${isTableFetching() ? 'is-fetching' : ''}`}>
              <table class="order-subtable">
                <thead>
                  <tr>
                    <th style={{ width: '16%' }}>Time (IST)</th>
                    <th style={{ width: '24%' }}>Instrument</th>
                    <th class="text-right" style={{ width: '12%' }}>Quantity</th>
                    <th class="text-center" style={{ width: '12%' }}>Type</th>
                    <th style={{ width: '36%' }}>Reason</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={rejectedOrders().length > 0}
                    fallback={
                      <tr>
                        <td colspan={5} class="table-empty-cell">
                          <Show when={isTabReady('rejected_orders')}>
                            <EmptyState message="No data" />
                          </Show>
                        </td>
                      </tr>
                    }
                  >
                    <For each={rejectedOrders()}>
                      {(ord) => (
                        <tr>
                          <td class="font-mono text-sm text-muted">{formatISTDateTime(ord.time)}</td>
                          <td class="font-medium">{ord.instrument}</td>
                          <td class="text-right font-mono">{ord.quantity}</td>
                          <td class="text-center">
                            <span
                              class={`type-badge ${
                                ord.type === 'B' ? 'type-badge-buy' : 'type-badge-sell'
                              }`}
                            >
                              {ord.type}
                            </span>
                          </td>
                          <td class="text-negative font-medium">{ord.reason ?? 'Rejected'}</td>
                        </tr>
                      )}
                    </For>
                  </Show>
                </tbody>
              </table>
            </div>
          </Show>

          {/* 4. Sub-tab Pagination Navigator */}
          <Show when={pagination().totalCount > 0}>
            <div class="order-pagination-navigator" data-testid="order-pagination-navigator">
              <div class="pagination-info">
                Showing <span class="font-mono font-medium">{pagination().start}</span>–
                <span class="font-mono font-medium">{pagination().end}</span> of{' '}
                <span class="font-mono font-medium">{pagination().totalCount}</span> items
              </div>
              <div class="pagination-controls">
                <button
                  type="button"
                  class="pagination-btn"
                  disabled={pagination().page <= 1 || isTableFetching()}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  aria-label="Previous page"
                >
                  ‹ Prev
                </button>
                <span class="pagination-page-indicator">
                  Page <span class="font-mono font-semibold">{pagination().page}</span> of{' '}
                  <span class="font-mono font-semibold">{pagination().totalPages}</span>
                </span>
                <button
                  type="button"
                  class="pagination-btn"
                  disabled={pagination().page >= pagination().totalPages || isTableFetching()}
                  onClick={() => setPage((p) => Math.min(pagination().totalPages, p + 1))}
                  aria-label="Next page"
                >
                  Next ›
                </button>
              </div>
            </div>
          </Show>
        </Show>
      </div>
    </div>
  )
}
