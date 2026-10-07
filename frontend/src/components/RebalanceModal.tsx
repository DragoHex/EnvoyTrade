import { createSignal, createResource, createEffect, createMemo, For, Show } from 'solid-js'
import {
  getGroupRebalanceDiff,
  getAccountRebalanceDiff,
  rebalanceGroup,
  rebalanceAccount,
  type RebalanceResult,
  type FollowerDrift,
  type SymbolDrift,
} from '../api'
import { ThanosBalanceIcon, ChevronDownIcon, InfoIcon, AlertTriangleIcon } from './icons'
import { StatusDot } from './StatusDot'

export interface RebalanceModalTarget {
  type: 'group' | 'account'
  id: string // groupId or accountId
  name?: string
  brokerAccountId?: string
}

export interface RebalanceModalProps {
  open: boolean
  target: RebalanceModalTarget | null
  onConfirm?: (followerIds?: string[]) => Promise<void> | void
  onCancel: () => void
  onSuccess?: () => void
}

export function humanizeError(err: unknown): string {
  if (!err) return 'An unexpected error occurred while communicating with the broker.'
  const msg = typeof err === 'string' ? err : (err as any)?.message || String(err)
  const trimmed = msg.trim()
  if (!trimmed || trimmed.toLowerCase() === 'error') {
    return 'Unable to communicate with the broker. Please check the broker status or network connection.'
  }
  const lower = trimmed.toLowerCase()
  if (
    lower.includes('tokenexception') ||
    lower.includes('api_key') ||
    lower.includes('access_token') ||
    lower.includes('invalid token') ||
    lower.includes('login has expired') ||
    lower.includes('unauthorized') ||
    lower.includes('401')
  ) {
    return 'Broker account login has expired. Please re-authenticate the account.'
  }
  if (
    lower.includes('connection refused') ||
    lower.includes('no such host') ||
    lower.includes('timeout') ||
    lower.includes('broker is not reachable') ||
    lower.includes('broker not reachable') ||
    lower.includes('502') ||
    lower.includes('503') ||
    lower.includes('504') ||
    lower.includes('failed to fetch')
  ) {
    return 'Broker is not reachable. Please check your network connection or broker status.'
  }
  if (lower.includes('not found') || lower.includes('404')) {
    return 'Account or portfolio group could not be found.'
  }
  return trimmed
}

function isFollowerDriftEqual(a: FollowerDrift, b: FollowerDrift): boolean {
  if (
    a.account_id !== b.account_id ||
    a.account_name !== b.account_name ||
    a.broker_account_id !== b.broker_account_id ||
    a.enabled !== b.enabled ||
    a.clone_factor !== b.clone_factor
  ) {
    return false
  }
  const sA = a.symbols ?? []
  const sB = b.symbols ?? []
  if (sA.length !== sB.length) return false
  for (let i = 0; i < sA.length; i++) {
    const s1 = sA[i]
    const s2 = sB[i]
    if (
      s1.exchange !== s2.exchange ||
      s1.tradingsymbol !== s2.tradingsymbol ||
      s1.product !== s2.product ||
      s1.lot_size !== s2.lot_size ||
      s1.master_qty !== s2.master_qty ||
      s1.target_qty !== s2.target_qty ||
      s1.follower_qty !== s2.follower_qty ||
      s1.drift_qty !== s2.drift_qty ||
      s1.action !== s2.action
    ) {
      return false
    }
  }
  return true
}

function isSymbolDriftEqual(s1: SymbolDrift, s2: SymbolDrift): boolean {
  return (
    s1.exchange === s2.exchange &&
    s1.tradingsymbol === s2.tradingsymbol &&
    s1.product === s2.product &&
    s1.lot_size === s2.lot_size &&
    s1.master_qty === s2.master_qty &&
    s1.target_qty === s2.target_qty &&
    s1.follower_qty === s2.follower_qty &&
    s1.drift_qty === s2.drift_qty &&
    s1.action === s2.action
  )
}

export function RebalanceModal(props: RebalanceModalProps) {
  const [selectedFollowers, setSelectedFollowers] = createSignal<string[]>([])
  const [expandedFollowers, setExpandedFollowers] = createSignal<string[]>([])
  const [submitting, setSubmitting] = createSignal(false)
  const [error, setError] = createSignal<string | null>(null)
  const [receipt, setReceipt] = createSignal<RebalanceResult | null>(null)

  // Resource for Group diff
  const [groupDiff] = createResource(
    () => (props.open && props.target?.type === 'group' ? props.target.id : null),
    (groupId) => getGroupRebalanceDiff(groupId)
  )

  // Resource for Account diff
  const [accountDiff] = createResource(
    () => (props.open && props.target?.type === 'account' ? props.target.id : null),
    (accountId) => getAccountRebalanceDiff(accountId)
  )

  const fetchError = () => {
    if (props.target?.type === 'group' && groupDiff.error) {
      return humanizeError(groupDiff.error)
    }
    if (props.target?.type === 'account' && accountDiff.error) {
      return humanizeError(accountDiff.error)
    }
    return null
  }

  const safeGroupDiff = () => {
    if (groupDiff.error) return undefined
    return groupDiff()
  }

  const safeAccountDiff = () => {
    if (accountDiff.error) return undefined
    return accountDiff()
  }

  const isLoading = () =>
    (props.target?.type === 'group' && groupDiff.loading) ||
    (props.target?.type === 'account' && accountDiff.loading)

  const hasData = () => {
    if (fetchError()) return false
    if (props.target?.type === 'group') return safeGroupDiff() !== undefined
    if (props.target?.type === 'account') return safeAccountDiff() !== undefined
    return false
  }

  // Stable memoized follower drift list to avoid remounting non-diff rows
  let prevFollowersMap = new Map<string, FollowerDrift>()
  const driftingFollowers = createMemo(() => {
    if (props.target?.type !== 'group') return []
    const diff = safeGroupDiff()
    if (!diff?.drifts) return []
    const newDrifters = diff.drifts.filter((d) => d.symbols && d.symbols.length > 0)

    const nextMap = new Map<string, FollowerDrift>()
    const result: FollowerDrift[] = []

    for (const item of newDrifters) {
      const prev = prevFollowersMap.get(item.account_id)
      if (prev && isFollowerDriftEqual(prev, item)) {
        result.push(prev)
        nextMap.set(item.account_id, prev)
      } else {
        result.push(item)
        nextMap.set(item.account_id, item)
      }
    }

    prevFollowersMap = nextMap
    return result
  })

  // Stable memoized single account symbols to avoid remounting non-diff symbol rows
  let prevAccountSymbolsMap = new Map<string, SymbolDrift>()
  const accountSymbols = createMemo(() => {
    if (props.target?.type !== 'account') return []
    const acc = safeAccountDiff()
    if (!acc?.symbols) return []

    const nextMap = new Map<string, SymbolDrift>()
    const result: SymbolDrift[] = []

    for (const s of acc.symbols) {
      const key = `${s.exchange}:${s.tradingsymbol}:${s.product}`
      const prev = prevAccountSymbolsMap.get(key)
      if (prev && isSymbolDriftEqual(prev, s)) {
        result.push(prev)
        nextMap.set(key, prev)
      } else {
        result.push(s)
        nextMap.set(key, s)
      }
    }

    prevAccountSymbolsMap = nextMap
    return result
  })

  let headerCheckboxRef: HTMLInputElement | undefined

  // Indeterminate state for master checkbox
  createEffect(() => {
    if (headerCheckboxRef) {
      const total = driftingFollowers().length
      const selected = selectedFollowers().length
      headerCheckboxRef.indeterminate = selected > 0 && selected < total
    }
  })

  // Reset state on modal open or target change
  let lastOpenedTargetKey: string | null = null
  createEffect(() => {
    const isOpen = props.open
    const targetKey = isOpen && props.target ? `${props.target.type}:${props.target.id}` : null

    if (targetKey && targetKey !== lastOpenedTargetKey) {
      setError(null)
      setSubmitting(false)
      setReceipt(null)
      setExpandedFollowers([])
      setSelectedFollowers([])
      lastOpenedTargetKey = targetKey
    } else if (!isOpen) {
      lastOpenedTargetKey = null
    }
  })

  // By default, select all drifting followers like in SquareOffModal
  createEffect(() => {
    if (!props.open || props.target?.type !== 'group') return
    const drifters = driftingFollowers()
    setSelectedFollowers(drifters.map((d) => d.account_id))
  })

  const toggleSelectAll = () => {
    const allIds = driftingFollowers().map((d) => d.account_id)
    if (selectedFollowers().length === allIds.length) {
      setSelectedFollowers([])
    } else {
      setSelectedFollowers(allIds)
    }
  }

  const toggleFollower = (accId: string) => {
    const current = selectedFollowers()
    if (current.includes(accId)) {
      setSelectedFollowers(current.filter((id) => id !== accId))
    } else {
      setSelectedFollowers([...current, accId])
    }
  }

  const toggleExpand = (accId: string) => {
    const current = expandedFollowers()
    if (current.includes(accId)) {
      setExpandedFollowers(current.filter((id) => id !== accId))
    } else {
      setExpandedFollowers([...current, accId])
    }
  }

  const handleConfirm = async () => {
    if (!props.target) return
    setSubmitting(true)
    setError(null)

    try {
      if (props.onConfirm) {
        await props.onConfirm(props.target.type === 'group' ? selectedFollowers() : undefined)
        props.onSuccess?.()
        props.onCancel()
      } else if (props.target.type === 'group') {
        const res = await rebalanceGroup(props.target.id, { follower_ids: selectedFollowers() })
        setReceipt(res)
        props.onSuccess?.()
      } else {
        const res = await rebalanceAccount(props.target.id)
        setReceipt(res)
        props.onSuccess?.()
      }
    } catch (err: any) {
      setError(humanizeError(err))
    } finally {
      setSubmitting(false)
    }
  }

  const targetDisplayName = () => {
    if (!props.target) return ''
    return props.target.name || props.target.brokerAccountId || 'Account'
  }

  const hasAnyDrift = () => {
    if (fetchError()) return false
    if (props.target?.type === 'group') {
      return driftingFollowers().length > 0
    }
    const acc = safeAccountDiff()
    return (acc?.symbols?.length ?? 0) > 0
  }

  const errorTitle = () => {
    const err = fetchError()
    if (!err) return 'Broker Connection Error'
    const lower = err.toLowerCase()
    if (lower.includes('login has expired') || lower.includes('authenticate') || lower.includes('unauthorized') || lower.includes('token')) {
      return 'Authentication Required'
    }
    if (
      lower.includes('not reachable') ||
      lower.includes('connection refused') ||
      lower.includes('timeout') ||
      lower.includes('502') ||
      lower.includes('503') ||
      lower.includes('504') ||
      lower.includes('broker') ||
      lower.includes('communicate')
    ) {
      return 'Broker Connection Error'
    }
    if (lower.includes('not found') || lower.includes('404')) {
      return 'Portfolio Not Found'
    }
    return 'Unable to Load Portfolio Diff'
  }

  const rawErrorDetails = () => {
    const err = props.target?.type === 'group' ? groupDiff.error : accountDiff.error
    if (!err) return null
    const raw = typeof err === 'string' ? err : (err as any)?.message || String(err)
    if (!raw || raw.trim().toLowerCase() === 'error') return null
    const humanized = fetchError()
    if (raw === humanized) return null
    return raw
  }

  return (
    <Show when={props.open && props.target}>
      <div
        role="dialog"
        aria-label="Portfolio Rebalance"
        class="confirm-modal-overlay"
        onClick={(e) => {
          if (e.target === e.currentTarget && !submitting()) props.onCancel()
        }}
      >
        <div class="confirm-modal rebalance-modal" onClick={(e) => e.stopPropagation()}>
          <div class="rebalance-modal-header">
            <div class="header-title-row">
              <h3>Rebalance</h3>
              <ThanosBalanceIcon class="rebalance-header-icon" />
            </div>
            <div class="target-name-row">
              <span class="target-name">
                {props.target!.type === 'group' ? `Group: ${targetDisplayName()}` : `Follower: ${targetDisplayName()}`}
              </span>
              <span
                class="info-tooltip-trigger tooltip-accent"
                data-tooltip={
                  props.target!.type === 'group'
                    ? "Compares active followers' open positions against the master's positions. Select which drifting followers to bring back into equilibrium."
                    : "Rebalances this follower's positions to match its master allocation. Open limit orders for drifting symbols will be cancelled before placing market balancing orders."
                }
                aria-label="Info"
              >
                <InfoIcon size={14} />
              </span>
            </div>
          </div>

          {/* Order execution error banner */}
          <Show when={error()}>
            <div class="rebalance-error-banner" role="alert">
              <AlertTriangleIcon size={16} />
              <span>{error()}</span>
            </div>
          </Show>

          {/* Receipt View on Successful Execution */}
          <Show when={receipt()}>
            <div class="rebalance-receipt">
              <div class="receipt-header">
                <span class="receipt-status-badge status-completed">
                  Status: {receipt()!.status.toUpperCase()}
                </span>
                <span class="receipt-summary-text">
                  {receipt()!.orders_placed} order(s) placed, {receipt()!.cancelled_orders} existing order(s) cancelled.
                </span>
              </div>
              <Show when={receipt()!.orders && receipt()!.orders.length > 0}>
                <div class="receipt-orders-table-wrapper">
                  <table class="receipt-orders-table">
                    <thead>
                      <tr>
                        <th>Symbol</th>
                        <th>Side</th>
                        <th class="text-right">Qty</th>
                        <th>Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={receipt()!.orders}>
                        {(ord) => (
                          <tr>
                            <td class="font-semibold">{ord.tradingsymbol}</td>
                            <td>
                              <span class={`action-badge action-${ord.side.toLowerCase()}`}>
                                {ord.side}
                              </span>
                            </td>
                            <td class="text-right">{ord.quantity}</td>
                            <td>
                              <span class="order-status-tag">{ord.status}</span>
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
              <div class="confirm-modal-actions">
                <button
                  type="button"
                  class="confirm-modal-confirm rebalance-confirm-btn"
                  onClick={props.onCancel}
                >
                  Done
                </button>
              </div>
            </div>
          </Show>

          {/* Diff Content View */}
          <Show when={!receipt()}>
            <div class="rebalance-diff-section">
              <Show when={!hasData() && isLoading()}>
                <div class="rebalance-skeleton" data-testid="rebalance-skeleton">
                  <div class="skeleton-line skeleton-header-line" />
                  <div class="skeleton-line" />
                  <div class="skeleton-line" />
                </div>
              </Show>

              {/* Fetch Error Card - mirrors table section layout and theming */}
              <Show when={fetchError()}>
                <div class="rebalance-error-card" data-testid="rebalance-error-state">
                  <div class="rebalance-section-header">
                    <span>DRIFTING FOLLOWERS</span>
                    <span
                      class="error-status-badge"
                      style={{
                        display: 'inline-flex',
                        'align-items': 'center',
                        gap: '0.35rem',
                        'font-size': '0.75rem',
                        color: '#e5484d',
                        'font-weight': '500',
                      }}
                    >
                      <StatusDot status="error" /> Connection Failed
                    </span>
                  </div>
                  <div class="rebalance-error-body">
                    <div class="error-icon-badge">
                      <AlertTriangleIcon size={20} />
                    </div>
                    <h4 class="error-title">{errorTitle()}</h4>
                    <p class="error-message">{fetchError()}</p>
                    <Show when={rawErrorDetails()}>
                      <div class="error-details-box">{rawErrorDetails()}</div>
                    </Show>
                  </div>
                </div>
              </Show>

              <Show when={hasData() && !fetchError()}>
                {/* Clean Equilibrium State */}
                <Show when={!hasAnyDrift()}>
                  <div class="rebalance-equilibrium-state" data-testid="rebalance-equilibrium">
                    <span class="equilibrium-icon">✓</span>
                    <p class="font-semibold">All positions are in equilibrium.</p>
                    <small>No drift detected between master and follower portfolio allocations.</small>
                  </div>
                </Show>

                {/* Group Mode: Drifting Followers List */}
                <Show when={props.target!.type === 'group' && hasAnyDrift()}>
                  <div class="rebalance-followers-section">
                    <div class="rebalance-section-header">
                      <span class="section-title">Drifting Followers</span>
                      <span class="selection-count">
                        {selectedFollowers().length} of {driftingFollowers().length} selected
                      </span>
                    </div>

                    <div class="rebalance-list-header-row">
                      <div class="col-check">
                        <input
                          ref={headerCheckboxRef}
                          type="checkbox"
                          aria-label="Select All Followers"
                          checked={
                            driftingFollowers().length > 0 &&
                            selectedFollowers().length === driftingFollowers().length
                          }
                          onChange={toggleSelectAll}
                          disabled={submitting()}
                        />
                      </div>
                      <span class="col-follower-header">Follower Account</span>
                      <span class="col-drift-header">Drift Details</span>
                      <span class="col-expand-header" />
                    </div>

                    <div class="rebalance-followers-list">
                      <For each={driftingFollowers()}>
                        {(f) => {
                          const isSelected = () => selectedFollowers().includes(f.account_id)
                          const isExpanded = () => expandedFollowers().includes(f.account_id)

                          return (
                            <div
                              class={`follower-accordion-item ${
                                isSelected() ? 'selected-accordion' : ''
                              }`}
                              data-testid={`follower-row-${f.account_id}`}
                            >
                              <div
                                class="follower-accordion-header"
                                onClick={() => !submitting() && toggleFollower(f.account_id)}
                              >
                                <div class="col-check" onClick={(e) => e.stopPropagation()}>
                                  <input
                                    type="checkbox"
                                    aria-label={`Select ${f.account_name || f.broker_account_id}`}
                                    checked={isSelected()}
                                    onChange={() => toggleFollower(f.account_id)}
                                    disabled={submitting()}
                                  />
                                </div>
                                <div class="follower-info">
                                  <span class="follower-name font-semibold">
                                    {f.account_name || f.broker_account_id}
                                  </span>
                                  <span class="follower-broker-id">{f.broker_account_id}</span>
                                </div>
                                <div class="follower-drift-badge">
                                  <span>{f.symbols.length} drifting</span>
                                </div>
                                <button
                                  type="button"
                                  class="accordion-expand-btn"
                                  aria-label={isExpanded() ? 'Collapse symbols' : 'Expand symbols'}
                                  data-tooltip={isExpanded() ? 'Hide symbols' : 'View symbols'}
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    toggleExpand(f.account_id)
                                  }}
                                >
                                  <ChevronDownIcon
                                    class={`chevron-icon ${isExpanded() ? 'chevron-rotated' : ''}`}
                                  />
                                </button>
                              </div>

                              <Show when={isExpanded()}>
                                <div class="follower-symbols-table-wrapper">
                                  <table class="rebalance-symbols-table">
                                    <thead>
                                      <tr>
                                        <th>Symbol</th>
                                        <th>Product</th>
                                        <th class="text-right">Current</th>
                                        <th class="text-right">Target</th>
                                        <th class="text-right">Drift</th>
                                        <th>Action</th>
                                      </tr>
                                    </thead>
                                    <tbody>
                                      <For each={f.symbols}>
                                        {(s) => (
                                          <tr>
                                            <td class="font-semibold">{s.tradingsymbol}</td>
                                            <td>
                                              <span class="product-badge">{s.product}</span>
                                            </td>
                                            <td class="text-right">{s.follower_qty}</td>
                                            <td class="text-right">{s.target_qty}</td>
                                            <td
                                              class={`text-right font-medium ${
                                                s.drift_qty > 0 ? 'text-success' : 'text-danger'
                                              }`}
                                            >
                                              {s.drift_qty > 0 ? `+${s.drift_qty}` : s.drift_qty}
                                            </td>
                                            <td>
                                              <span
                                                class={`action-badge action-${s.action.toLowerCase()}`}
                                              >
                                                {s.action}
                                              </span>
                                            </td>
                                          </tr>
                                        )}
                                      </For>
                                    </tbody>
                                  </table>
                                </div>
                              </Show>
                            </div>
                          )
                        }}
                      </For>
                    </div>
                  </div>
                </Show>

                {/* Account Mode: Single Follower Drift Table */}
                <Show when={props.target!.type === 'account' && hasAnyDrift()}>
                  <div class="follower-symbols-table-wrapper account-mode-wrapper">
                    <table class="rebalance-symbols-table">
                      <thead>
                        <tr>
                          <th>Symbol</th>
                          <th>Product</th>
                          <th class="text-right">Current</th>
                          <th class="text-right">Target</th>
                          <th class="text-right">Drift</th>
                          <th>Action</th>
                        </tr>
                      </thead>
                      <tbody>
                        <For each={accountSymbols()}>
                          {(s) => (
                            <tr>
                              <td class="font-semibold">{s.tradingsymbol}</td>
                              <td>
                                <span class="product-badge">{s.product}</span>
                              </td>
                              <td class="text-right">{s.follower_qty}</td>
                              <td class="text-right">{s.target_qty}</td>
                              <td
                                class={`text-right font-medium ${
                                  s.drift_qty > 0 ? 'text-success' : 'text-danger'
                                }`}
                              >
                                {s.drift_qty > 0 ? `+${s.drift_qty}` : s.drift_qty}
                              </td>
                              <td>
                                <span class={`action-badge action-${s.action.toLowerCase()}`}>
                                  {s.action}
                                </span>
                              </td>
                            </tr>
                          )}
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
                class="confirm-modal-confirm rebalance-confirm-btn"
                onClick={handleConfirm}
                disabled={
                  submitting() ||
                  isLoading() ||
                  !!fetchError() ||
                  !hasAnyDrift() ||
                  (props.target!.type === 'group' && selectedFollowers().length === 0)
                }
              >
                <ThanosBalanceIcon spinning={submitting()} />
                <span>
                  {submitting()
                    ? 'Rebalancing...'
                    : fetchError()
                    ? 'Rebalance'
                    : !hasAnyDrift()
                    ? 'In Equilibrium'
                    : props.target!.type === 'group'
                    ? `Rebalance (${selectedFollowers().length})`
                    : 'Rebalance'}
                </span>
              </button>
            </div>
          </Show>
        </div>
      </div>
    </Show>
  )
}
