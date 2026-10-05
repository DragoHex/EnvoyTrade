import { createResource, For, Show, onMount, onCleanup } from 'solid-js'
import {
  getGroupDetail,
  getGroups,
  patchAccount,
  postAction,
  squareOffAccount,
  squareOffGroup,
  rebalanceGroup,
  rebalanceAccount,
  type ActionType,
} from '../api'
import { GroupCard } from '../components/GroupCard'
import { GroupCardSkeleton, GroupsSkeleton, LoadingTimeout } from '../components/Skeleton'
import { EmptyState } from '../components/EmptyState'

function GroupCardLoader(props: { id: string; status: 'ok' | 'error' }) {
  const [detail, { refetch }] = createResource(() => props.id, getGroupDetail)

  onMount(() => {
    const timer = setInterval(() => {
      refetch()
    }, 7000)
    onCleanup(() => clearInterval(timer))
  })

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

  const onSquareOffGroup = async (groupId: string, symbols?: string[]) => {
    try {
      await squareOffGroup(groupId, symbols ? { symbols } : undefined)
    } finally {
      refetch()
    }
  }

  const onSquareOffAccount = async (accountId: string, symbols?: string[]) => {
    try {
      await squareOffAccount(accountId, symbols ? { symbols } : undefined)
    } finally {
      refetch()
    }
  }

  const onRebalanceGroup = async (groupId: string, followerIds?: string[]) => {
    try {
      await rebalanceGroup(groupId, followerIds ? { follower_ids: followerIds } : undefined)
    } finally {
      refetch()
    }
  }

  const onRebalanceAccount = async (accountId: string) => {
    try {
      await rebalanceAccount(accountId)
    } finally {
      refetch()
    }
  }

  // Non-keyed Show (not keyed function child, and not Suspense) here on purpose:
  // createResource keeps the last resolved value while refetch() is in flight,
  // and non-keyed Show evaluates truthiness without disposing or remounting children,
  // so a refetch after an action updates the table in place instead of remounting
  // and closing any opened dropdowns or expansion rows.
  return (
    <Show when={detail()} fallback={<LoadingTimeout><GroupCardSkeleton /></LoadingTimeout>}>
      <GroupCard
        detail={detail()!}
        status={props.status}
        onToggleCopy={onToggleCopy}
        onToggleMasterActive={onToggleMasterActive}
        onAction={onAction}
        onSquareOffGroup={onSquareOffGroup}
        onSquareOffAccount={onSquareOffAccount}
        onRebalanceGroup={onRebalanceGroup}
        onRebalanceAccount={onRebalanceAccount}
      />
    </Show>
  )
}

export function Dashboard() {
  const [groups] = createResource(getGroups)

  return (
    <div class="page">
      <h1>Dashboard</h1>
      <Show when={groups()} fallback={<LoadingTimeout><GroupsSkeleton /></LoadingTimeout>}>
        <Show
          when={(groups()?.length ?? 0) > 0}
          fallback={
            <div class="dashboard-empty-card">
              <EmptyState>
                No group added. Please go to <a href="/accounts" class="empty-state-link">accounts</a> to add groups
              </EmptyState>
            </div>
          }
        >
          <For each={groups()}>{(g) => <GroupCardLoader id={g.id || g.masterId} status={g.status} />}</For>
        </Show>
      </Show>
    </div>
  )
}
