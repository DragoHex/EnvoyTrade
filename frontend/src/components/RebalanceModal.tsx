import { createSignal, createResource, createEffect, For, Show } from 'solid-js'
import {
  getGroupRebalanceDiff,
  getAccountRebalanceDiff,
  rebalanceGroup,
  rebalanceAccount,
  type RebalanceResult,
} from '../api'
import { ThanosBalanceIcon, ChevronDownIcon, InfoIcon } from './icons'

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

  const isLoading = () =>
    (props.target?.type === 'group' && groupDiff.loading) ||
    (props.target?.type === 'account' && accountDiff.loading)

  // Extract drifting followers for group mode
  const driftingFollowers = () => {
    if (props.target?.type !== 'group') return []
    const diff = groupDiff()
    if (!diff?.drifts) return []
    return diff.drifts.filter((d) => d.symbols && d.symbols.length > 0)
  }

  let headerCheckboxRef: HTMLInputElement | undefined

  // Indeterminate state for master checkbox
  createEffect(() => {
    if (headerCheckboxRef) {
      const total = driftingFollowers().length
      const selected = selectedFollowers().length
      headerCheckboxRef.indeterminate = selected > 0 && selected < total
    }
  })

  // Reset state and select all drifting followers when diff loads
  createEffect(() => {
    if (props.open) {
      setError(null)
      setSubmitting(false)
      setReceipt(null)

      if (props.target?.type === 'group') {
        const drifters = driftingFollowers()
        const ids = drifters.map((d) => d.account_id)
        setSelectedFollowers(ids)
        // Keep all dropdowns folded by default
        setExpandedFollowers([])
      }
    }
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
      setError(err.message || 'Rebalance execution failed')
    } finally {
      setSubmitting(false)
    }
  }

  const targetDisplayName = () => {
    if (!props.target) return ''
    return props.target.name || props.target.brokerAccountId || 'Account'
  }

  const hasAnyDrift = () => {
    if (props.target?.type === 'group') {
      return driftingFollowers().length > 0
    }
    const acc = accountDiff()
    return (acc?.symbols?.length ?? 0) > 0
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

          <Show when={error()}>
            <div class="rebalance-error-banner">{error()}</div>
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
              <Show when={isLoading()}>
                <div class="rebalance-skeleton" data-testid="rebalance-skeleton">
                  <div class="skeleton-line skeleton-header-line" />
                  <div class="skeleton-line" />
                  <div class="skeleton-line" />
                </div>
              </Show>

              <Show when={!isLoading()}>
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

                    <div class="rebalance-followers-list">
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
                        <For each={accountDiff()?.symbols ?? []}>
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
                  !hasAnyDrift() ||
                  (props.target!.type === 'group' && selectedFollowers().length === 0)
                }
              >
                <ThanosBalanceIcon spinning={submitting()} />
                <span>
                  {submitting()
                    ? 'Rebalancing...'
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
