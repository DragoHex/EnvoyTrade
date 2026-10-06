import type { ActionType, GroupDetail } from '../api'
import { AccountTable } from './AccountTable'
import { StatusDot } from './StatusDot'

export function GroupCard(props: {
  detail: GroupDetail
  status: 'ok' | 'error'
  onToggleCopy: (accountId: string, next: boolean) => Promise<void> | void
  onToggleMasterActive: (next: boolean) => Promise<void> | void
  onAction: (accountId: string, type: ActionType) => Promise<void>
  onSquareOffGroup?: (groupId: string, symbols?: string[]) => Promise<void>
  onSquareOffAccount?: (accountId: string, symbols?: string[]) => Promise<void>
  onRebalanceGroup?: (groupId: string, followerIds?: string[]) => Promise<void>
  onRebalanceAccount?: (accountId: string) => Promise<void>
}) {
  return (
    <section data-testid="group-card">
      <h2>
        {props.detail.name || props.detail.masterAccountId} <StatusDot status={props.status} />
      </h2>
      <AccountTable
        master={{
          masterId: props.detail.masterId,
          groupId: props.detail.id,
          name: props.detail.masterName || props.detail.name || '',
          groupName: props.detail.name,
          brokerAccountId: props.detail.masterAccountId,
          status: props.status,
          active: props.detail.masterActive,
          netQty: props.detail.masterNetQty,
          openPositionsCount: props.detail.masterOpenPositionsCount,
          closedPositionsCount: props.detail.masterClosedPositionsCount,
          openOrdersCount: props.detail.masterOpenOrdersCount,
          totalMtm: props.detail.masterTotalMtm,
          availableCash: props.detail.masterAvailableCash,
          availableMargin: props.detail.masterAvailableMargin,
        }}
        followers={props.detail.followers}
        onToggleCopy={props.onToggleCopy}
        onToggleMasterActive={props.onToggleMasterActive}
        onAction={props.onAction}
        onSquareOffGroup={props.onSquareOffGroup}
        onSquareOffAccount={props.onSquareOffAccount}
        onRebalanceGroup={props.onRebalanceGroup}
        onRebalanceAccount={props.onRebalanceAccount}
      />
    </section>
  )
}
