import { Index, createSignal } from 'solid-js'
import { AccountRow } from './AccountRow'
import type { ActionType, GroupFollower } from '../api'

export function AccountTable(props: {
  master: {
    masterId: string
    name?: string
    brokerAccountId: string
    status: 'ok' | 'error'
    active: boolean
    netQty?: number
    openPositionsCount?: number
    closedPositionsCount?: number
    openOrdersCount?: number
    totalMtm?: number | string
    availableCash?: number | string
    availableMargin?: number | string
    groupId?: string
    groupName?: string
  }
  followers: GroupFollower[]
  onToggleCopy: (accountId: string, next: boolean) => Promise<void> | void
  onToggleMasterActive: (next: boolean) => Promise<void> | void
  onAction: (accountId: string, type: ActionType) => Promise<void>
  onSquareOffGroup?: (groupId: string, symbols?: string[]) => Promise<void>
  onSquareOffAccount?: (accountId: string, symbols?: string[]) => Promise<void>
  onRebalanceGroup?: (groupId: string, followerIds?: string[]) => Promise<void>
  onRebalanceAccount?: (accountId: string) => Promise<void>
}) {
  const [togglingId, setTogglingId] = createSignal<string | null>(null)

  const handleToggleMaster = async (next: boolean) => {
    setTogglingId(props.master.masterId)
    try {
      await props.onToggleMasterActive(next)
    } finally {
      setTogglingId(null)
    }
  }

  const handleToggleFollower = async (accountId: string, next: boolean) => {
    setTogglingId(accountId)
    try {
      await props.onToggleCopy(accountId, next)
    } finally {
      setTogglingId(null)
    }
  }

  return (
    <div class="account-table-container">
      <table class="account-table">
        <thead>
          <tr>
            <th class="col-toggle"></th>
            <th class="col-name">Name</th>
            <th class="col-account-id">Account ID</th>
            <th class="col-net-qty">Net Qty</th>
            <th class="col-positions">Positions (O/C)</th>
            <th class="col-open-orders">Open Orders</th>
            <th class="col-mtm">Total MTM</th>
            <th class="col-cash">Available Cash</th>
            <th class="col-margin">Available Margin</th>
            <th class="col-status">Status</th>
            <th class="col-actions"></th>
            <th class="col-expand th-expand-col"></th>
          </tr>
        </thead>
      <tbody>
        <AccountRow
          follower={{
            accountId: props.master.masterId,
            name: props.master.name ?? '',
            brokerAccountId: `${props.master.brokerAccountId} (Master)`,
            enabled: props.master.active,
            status: props.master.status,
            netQty: props.master.netQty,
            openPositionsCount: props.master.openPositionsCount,
            closedPositionsCount: props.master.closedPositionsCount,
            openOrdersCount: props.master.openOrdersCount,
            totalMtm: props.master.totalMtm,
            availableCash: props.master.availableCash,
            availableMargin: props.master.availableMargin,
          }}
          isMaster
          targetGroupId={props.master.groupId || props.master.masterId}
          targetGroupName={props.master.groupName}
          actionsDisabled={!props.master.active}
          toggleDisabled={togglingId() === props.master.masterId}
          onToggleCopy={handleToggleMaster}
          onRebalance={(followerIds) => {
            if (props.onRebalanceGroup) {
              return props.onRebalanceGroup(props.master.groupId || props.master.masterId, followerIds)
            }
            return props.onAction(props.master.masterId, 'rebalance')
          }}
          onSquareOff={(symbols) => {
            if (props.onSquareOffGroup) {
              return props.onSquareOffGroup(props.master.groupId || props.master.masterId, symbols)
            }
            return props.onAction(props.master.masterId, 'square_off')
          }}
          onExitOpenOrders={() => props.onAction(props.master.masterId, 'exit_open_orders')}
        />
        {/* Index (not For): a refetch after a button click resolves to a
            new array of new follower objects even though the account list
            itself hasn't changed. For keys by item reference and would
            unmount/remount every row on each click; Index keys by position,
            so rows update in place instead of flashing/reloading. */}
        <Index each={props.followers}>
          {(f) => (
            <AccountRow
              follower={f()}
              actionsDisabled={!props.master.active || !f().enabled}
              toggleDisabled={!props.master.active || togglingId() === f().accountId}
              onToggleCopy={(next) => handleToggleFollower(f().accountId, next)}
              onRebalance={() => {
                if (props.onRebalanceAccount) {
                  return props.onRebalanceAccount(f().accountId)
                }
                return props.onAction(f().accountId, 'rebalance')
              }}
              onSquareOff={(symbols) => {
                if (props.onSquareOffAccount) {
                  return props.onSquareOffAccount(f().accountId, symbols)
                }
                return props.onAction(f().accountId, 'square_off')
              }}
              onExitOpenOrders={() => props.onAction(f().accountId, 'exit_open_orders')}
            />
          )}
        </Index>
      </tbody>
    </table>
  </div>
  )
}
