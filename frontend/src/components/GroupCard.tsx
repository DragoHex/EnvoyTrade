import type { ActionType, GroupDetail } from '../api'
import { AccountTable } from './AccountTable'
import { StatusDot } from './StatusDot'

export function GroupCard(props: {
  detail: GroupDetail
  status: 'ok' | 'error'
  onToggleCopy: (accountId: string, next: boolean) => void
  onToggleMasterActive: (next: boolean) => void
  onAction: (accountId: string, type: ActionType) => Promise<void>
}) {
  return (
    <section data-testid="group-card">
      <h2>
        {props.detail.masterAccountId} <StatusDot status={props.status} />
      </h2>
      <AccountTable
        master={{
          masterId: props.detail.masterId,
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
