import type { ActionType, GroupDetail } from '../api'
import { AccountTable } from './AccountTable'
import { StatusDot } from './StatusDot'

export function GroupCard(props: {
  detail: GroupDetail
  status: 'ok' | 'error'
  onToggleCopy: (accountId: string, next: boolean) => Promise<void> | void
  onToggleMasterActive: (next: boolean) => Promise<void> | void
  onAction: (accountId: string, type: ActionType) => Promise<void>
}) {
  return (
    <section data-testid="group-card">
      <h2>
        {props.detail.name || props.detail.masterAccountId} <StatusDot status={props.status} />
      </h2>
      <AccountTable
        master={{
          masterId: props.detail.masterId,
          name: props.detail.masterName || props.detail.name || '',
          brokerAccountId: props.detail.masterAccountId,
          status: props.status,
          active: props.detail.masterActive,
        }}
        followers={props.detail.followers}
        onToggleCopy={props.onToggleCopy}
        onToggleMasterActive={props.onToggleMasterActive}
        onAction={props.onAction}
      />
    </section>
  )
}
