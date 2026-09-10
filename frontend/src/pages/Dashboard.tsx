import { createResource, For, Show } from 'solid-js'
import { getGroupDetail, getGroups, patchAccount, postAction, type ActionType } from '../api'
import { GroupCard } from '../components/GroupCard'
import { GroupCardSkeleton, GroupsSkeleton, LoadingTimeout } from '../components/Skeleton'

function GroupCardLoader(props: { id: string; status: 'ok' | 'error' }) {
  const [detail, { refetch }] = createResource(() => props.id, getGroupDetail)

  const onToggleCopy = async (accountId: string, next: boolean) => {
    await patchAccount(accountId, { enabled: next })
    refetch()
  }

  const onToggleMasterActive = async (next: boolean) => {
    const masterId = detail()?.masterId || props.id
    await patchAccount(masterId, { active: next })
    refetch()
  }

  const onAction = async (accountId: string, type: ActionType) => {
    try {
      await postAction(accountId, type)
    } finally {
      refetch()
    }
  }

  // Show (not Suspense) here on purpose: createResource keeps the last
  // resolved value while refetch() is in flight, and Show only cares about
  // that value being truthy — so a refetch after a button click updates the
  // table in place instead of re-suspending and flashing the whole card.
  return (
    <Show when={detail()} fallback={<LoadingTimeout><GroupCardSkeleton /></LoadingTimeout>}>
      {(d) => (
        <GroupCard
          detail={d()}
          status={props.status}
          onToggleCopy={onToggleCopy}
          onToggleMasterActive={onToggleMasterActive}
          onAction={onAction}
        />
      )}
    </Show>
  )
}

export function Dashboard() {
  const [groups] = createResource(getGroups)

  // Show (not Suspense) here for the same reason as GroupCardLoader below:
  // a <Suspense> boundary re-suspends — hiding everything inside it, not
  // just the one card whose resource refetched — for ANY resource read in
  // its subtree, including every GroupCardLoader's per-group resource. That
  // turned one button's refetch into a flash of the whole dashboard.
  return (
    <div class="page">
      <h1>Dashboard</h1>
      <Show when={groups()} fallback={<LoadingTimeout><GroupsSkeleton /></LoadingTimeout>}>
        {(gs) => <For each={gs()}>{(g) => <GroupCardLoader id={g.id || g.masterId} status={g.status} />}</For>}
      </Show>
    </div>
  )
}
