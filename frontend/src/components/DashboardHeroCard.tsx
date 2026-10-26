import { createMemo, createSignal, onMount, onCleanup, Show, Switch, Match } from 'solid-js'
import { useAuth } from '../context/AuthContext'
import { EnvoyLogo } from './EnvoyLogo'
import { ShieldCheckIcon } from './icons'
import { ThemedSelect, type ThemedSelectOption } from './ThemedSelect'
import { formatHeroTimestamp } from '../utils/time'
import type { GroupSummary, GroupDetail } from '../api'

export interface DashboardHeroCardProps {
  groups: GroupSummary[]
  groupDetails: Record<string, GroupDetail>
  isLoading?: boolean
  timestamp?: Date | string
}

const STORAGE_KEY = 'envoy_dashboard_mtm_scope'

const toNumber = (val: unknown): number => {
  if (val === null || val === undefined || val === '') return 0
  const n = typeof val === 'number' ? val : Number(val)
  return Number.isNaN(n) ? 0 : n
}

function formatCurrency(val: number, showSign = true): string {
  const sign = showSign ? (val > 0 ? '+' : val < 0 ? '-' : '') : (val < 0 ? '-' : '')
  const abs = Math.abs(val)
  return `${sign}₹${abs.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

export const isGroupActive = (g: GroupSummary, detail?: GroupDetail): boolean => {
  if (detail && typeof detail.masterActive === 'boolean') {
    return detail.masterActive
  }
  if (typeof g.active === 'boolean') {
    return g.active
  }
  return g.status === 'ok'
}

export function DashboardHeroCard(props: DashboardHeroCardProps) {
  let userFn: () => any = () => null
  try {
    const auth = useAuth()
    userFn = auth.user
  } catch {
    userFn = () => null
  }

  const user = userFn

  const initialScope = () => {
    if (typeof window !== 'undefined' && window.localStorage) {
      return localStorage.getItem(STORAGE_KEY) || 'all_masters'
    }
    return 'all_masters'
  }

  const [scope, setScopeSignal] = createSignal<string>(initialScope())

  const setScope = (val: string) => {
    setScopeSignal(val)
    if (typeof window !== 'undefined' && window.localStorage) {
      localStorage.setItem(STORAGE_KEY, val)
    }
  }

  const fullName = () => user()?.name || user()?.username || 'Trader'

  const initials = () => {
    const n = user()?.name || user()?.username
    if (!n) return 'ET'
    const parts = n.trim().split(/\s+/)
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
    }
    return n.slice(0, 2).toUpperCase()
  }

  const activeGroupsCount = createMemo(() => {
    return props.groups.filter((g) => isGroupActive(g, props.groupDetails[g.id])).length
  })

  const totalFollowersCount = createMemo(() => {
    return props.groups.reduce((acc, g) => acc + (g.followerCount ?? 0), 0)
  })

  const options = createMemo<ThemedSelectOption[]>(() => {
    const opts: ThemedSelectOption[] = [
      {
        value: 'all_masters',
        label: 'All Masters (Aggregate)',
        description: 'Aggregate for active master accounts',
        badge: { text: 'Default', variant: 'emerald' },
      },
      {
        value: 'all_groups',
        label: 'All Groups (Combined)',
        description: 'Combined total for active masters and followers',
        badge: { text: 'All', variant: 'blue' },
      },
    ]

    for (const g of props.groups) {
      const active = isGroupActive(g)
      const groupName = g.name || g.masterAccountId
      const masterName = g.masterName || groupName

      opts.push({
        value: `group_agg:${g.id}`,
        label: `${groupName} (Total)`,
        description: active ? 'Master + followers combined' : 'Group is inactive (paused)',
        badge: active
          ? { text: 'Group Total', variant: 'slate' }
          : { text: 'Paused', variant: 'amber' },
      })
      opts.push({
        value: `group_master:${g.id}`,
        label: `${masterName} (${groupName})`,
        description: `Master account only (${g.masterAccountId})`,
        badge: active
          ? { text: 'Master', variant: 'slate' }
          : { text: 'Inactive', variant: 'amber' },
      })
    }

    return opts
  })

  interface MtmBreakdown {
    scope: 'all_masters' | 'all_groups' | 'group_agg' | 'group_master'
    totalMtm: number
    mastersMtm: number
    followersMtm: number
    masterMtm: number
    followerMtm: number
    masterName: string
    hasData: boolean
  }

  const breakdown = createMemo<MtmBreakdown>(() => {
    const currentScope = scope()
    const details = props.groupDetails
    const groups = props.groups

    let totalMastersMtm = 0
    let totalFollowersMtm = 0

    // Only aggregate active groups
    for (const g of groups) {
      const d = details[g.id]
      if (!isGroupActive(g, d)) {
        continue
      }
      if (d) {
        totalMastersMtm += toNumber(d.masterTotalMtm)
        if (d.followers) {
          for (const f of d.followers) {
            totalFollowersMtm += toNumber(f.totalMtm)
          }
        }
      }
    }

    if (currentScope === 'all_masters') {
      return {
        scope: 'all_masters',
        totalMtm: totalMastersMtm,
        mastersMtm: totalMastersMtm,
        followersMtm: totalFollowersMtm,
        masterMtm: 0,
        followerMtm: 0,
        masterName: '',
        hasData: true,
      }
    }

    if (currentScope === 'all_groups') {
      return {
        scope: 'all_groups',
        totalMtm: totalMastersMtm + totalFollowersMtm,
        mastersMtm: totalMastersMtm,
        followersMtm: totalFollowersMtm,
        masterMtm: 0,
        followerMtm: 0,
        masterName: '',
        hasData: true,
      }
    }

    if (currentScope.startsWith('group_agg:')) {
      const gId = currentScope.replace('group_agg:', '')
      const d = details[gId]
      if (!d) {
        return {
          scope: 'group_agg',
          totalMtm: 0,
          mastersMtm: 0,
          followersMtm: 0,
          masterMtm: 0,
          followerMtm: 0,
          masterName: '',
          hasData: false,
        }
      }
      const masterMtm = toNumber(d.masterTotalMtm)
      const followerMtm = (d.followers ?? []).reduce((acc, f) => acc + toNumber(f.totalMtm), 0)
      return {
        scope: 'group_agg',
        totalMtm: masterMtm + followerMtm,
        mastersMtm: 0,
        followersMtm: 0,
        masterMtm,
        followerMtm,
        masterName: '',
        hasData: true,
      }
    }

    if (currentScope.startsWith('group_master:')) {
      const gId = currentScope.replace('group_master:', '')
      const d = details[gId]
      const g = groups.find((item) => item.id === gId)
      const masterMtm = d ? toNumber(d.masterTotalMtm) : 0
      const masterName = d?.masterName || g?.masterName || g?.name || d?.masterAccountId || '—'
      return {
        scope: 'group_master',
        totalMtm: masterMtm,
        mastersMtm: 0,
        followersMtm: 0,
        masterMtm,
        followerMtm: 0,
        masterName,
        hasData: !!d,
      }
    }

    return {
      scope: 'all_masters',
      totalMtm: totalMastersMtm,
      mastersMtm: totalMastersMtm,
      followersMtm: totalFollowersMtm,
      masterMtm: 0,
      followerMtm: 0,
      masterName: '',
      hasData: true,
    }
  })

  const rawMtmValue = createMemo(() => breakdown().totalMtm)

  const formattedMtm = createMemo(() => {
    const val = rawMtmValue()
    if (val === 0) return '₹0.00'
    return formatCurrency(val, true)
  })

  const mtmClass = createMemo(() => {
    const val = rawMtmValue()
    if (val > 0) return 'positive text-positive'
    if (val < 0) return 'negative text-negative'
    return 'neutral'
  })

  const [currentDate, setCurrentDate] = createSignal(new Date())

  onMount(() => {
    const timer = setInterval(() => {
      setCurrentDate(new Date())
    }, 30000)
    onCleanup(() => clearInterval(timer))
  })

  const timestampText = () => {
    return formatHeroTimestamp(props.timestamp ?? currentDate())
  }

  return (
    <div class="dashboard-hero-container" data-testid="dashboard-hero-card">
      {/* Left Partition: Social Shareable Card (ideal 16:9 dimensions for Twitter/X) */}
      <div class="hero-shareable-card">
        <div class="shareable-card-header">
          <div class="hero-brand-wrap">
            <EnvoyLogo size={24} />
            <span class="hero-brand-name">EnvoyTrade</span>
          </div>

          <div class="hero-user-cluster">
            <div class="hero-user-avatar" aria-hidden="true">
              {initials()}
            </div>
            <div class="hero-user-stack">
              <span class="hero-user-fullname">{fullName()}</span>
              <span class="hero-user-handle">@envoytrade</span>
            </div>
          </div>
        </div>

        <div class="shareable-card-body">
          <div class="hero-mtm-pill">
            <span class="hero-mtm-pill-dot" />
            <span>TOTAL MTM TODAY</span>
          </div>
          <div class={`hero-mtm-value ${mtmClass()}`}>{formattedMtm()}</div>
          <div class="hero-mtm-subtext" data-testid="hero-mtm-subtext">
            <Switch>
              <Match when={breakdown().scope === 'all_masters'}>
                For all active Primary Accounts
              </Match>
              <Match when={breakdown().scope === 'all_groups'}>
                <span>
                  Masters:{' '}
                  <span class={breakdown().mastersMtm > 0 ? 'text-positive' : breakdown().mastersMtm < 0 ? 'text-negative' : ''}>
                    {formatCurrency(breakdown().mastersMtm, true)}
                  </span>
                  {' '}• Followers:{' '}
                  <span class={breakdown().followersMtm > 0 ? 'text-positive' : breakdown().followersMtm < 0 ? 'text-negative' : ''}>
                    {formatCurrency(breakdown().followersMtm, true)}
                  </span>
                </span>
              </Match>
              <Match when={breakdown().scope === 'group_agg'}>
                <Show when={breakdown().hasData} fallback="No data available for this group">
                  <span>
                    Master:{' '}
                    <span class={breakdown().masterMtm > 0 ? 'text-positive' : breakdown().masterMtm < 0 ? 'text-negative' : ''}>
                      {formatCurrency(breakdown().masterMtm, true)}
                    </span>
                    {' '}(Followers:{' '}
                    <span class={breakdown().followerMtm > 0 ? 'text-positive' : breakdown().followerMtm < 0 ? 'text-negative' : ''}>
                      {formatCurrency(breakdown().followerMtm, true)}
                    </span>
                    )
                  </span>
                </Show>
              </Match>
              <Match when={breakdown().scope === 'group_master'}>
                {breakdown().masterName}
              </Match>
            </Switch>
          </div>
        </div>

        <div class="shareable-card-footer">
          <div class="hero-verified-badge">
            <ShieldCheckIcon size={14} class="hero-verified-icon" />
            <span>Verified Trades</span>
          </div>
          <span class="hero-timestamp" data-testid="hero-timestamp">{timestampText()}</span>
        </div>
      </div>

      {/* Right Partition: Controls Panel (Filter + Telemetry) */}
      <div class="hero-controls-panel">
        <div class="hero-controls-field">
          <label class="hero-controls-label" for="dashboard-mtm-scope-select">Scope:</label>
          <div class="hero-select-container">
            <ThemedSelect
              id="dashboard-mtm-scope-select"
              value={scope()}
              options={options()}
              onChange={setScope}
              aria-label="Select MTM Scope"
            />
          </div>
        </div>

        <div class="hero-telemetry-section">
          <div class="hero-stat-pill">
            <span class="hero-pulse-dot" />
            <span>{activeGroupsCount()} Groups Active</span>
          </div>
          <Show when={totalFollowersCount() > 0}>
            <div class="hero-stat-pill">
              <span>{totalFollowersCount()} Followers Synced</span>
            </div>
          </Show>
        </div>
      </div>
    </div>
  )
}
