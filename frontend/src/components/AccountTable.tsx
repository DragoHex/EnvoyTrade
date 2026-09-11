import { Index } from 'solid-js'
import { AccountRow } from './AccountRow'
import type { ActionType, GroupFollower } from '../api'

export function AccountTable(props: {
  master: { masterId: string; name?: string; brokerAccountId: string; status: 'ok' | 'error'; active: boolean }
  followers: GroupFollower[]
  onToggleCopy: (accountId: string, next: boolean) => void
  onToggleMasterActive: (next: boolean) => void
  onAction: (accountId: string, type: ActionType) => Promise<void>
}) {
  return (
    <table>
      <thead>
        <tr>
          <th></th>
          <th>Name</th>
          <th>Account ID</th>
          <th>Net Qty</th>
          <th>Positions (O/C)</th>
          <th>Open Orders</th>
          <th>Total MTM</th>
          <th>Available Cash</th>
          <th>Available Margin</th>
          <th>Status</th>
          <th></th>
          <th class="th-expand-col"></th>
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
          }}
          isMaster
          actionsDisabled={!props.master.active}
          onToggleCopy={props.onToggleMasterActive}
          onRebalance={() => props.onAction(props.master.masterId, 'rebalance')}
          onSquareOff={() => props.onAction(props.master.masterId, 'square_off')}
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
              toggleDisabled={!props.master.active}
              onToggleCopy={(next) => props.onToggleCopy(f().accountId, next)}
              onRebalance={() => props.onAction(f().accountId, 'rebalance')}
              onSquareOff={() => props.onAction(f().accountId, 'square_off')}
              onExitOpenOrders={() => props.onAction(f().accountId, 'exit_open_orders')}
            />
          )}
        </Index>
      </tbody>
    </table>
  )
}
