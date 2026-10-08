import { render, screen, within } from '@solidjs/testing-library'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { GroupCard } from './GroupCard'
import type { GroupDetail } from '../api'

const mockDetail: GroupDetail = {
  id: 'g1',
  name: 'Alpha Momentum',
  masterId: 'm1',
  masterAccountId: 'ZX1234',
  masterName: 'Master Alice',
  masterActive: true,
  followers: [
    {
      accountId: 'f1',
      name: 'Follower Active Drifting',
      brokerAccountId: 'ZY1111',
      enabled: true,
      status: 'ok',
    },
    {
      accountId: 'f2',
      name: 'Follower Disabled Drifting',
      brokerAccountId: 'ZY2222',
      enabled: false,
      status: 'ok',
    },
    {
      accountId: 'f3',
      name: 'Follower Balanced',
      brokerAccountId: 'ZY3333',
      enabled: true,
      status: 'ok',
    },
  ],
}

describe('GroupCard', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders yellow status dot and notification dots on master and drifting follower when an active follower drifts', () => {
    render(() => (
      <GroupCard
        detail={mockDetail}
        status="ok"
        hasImbalance={true}
        imbalancedFollowerIds={new Set(['f1'])}
        onToggleCopy={() => {}}
        onToggleMasterActive={() => {}}
        onAction={vi.fn()}
      />
    ))

    // Group name status dot should be yellow ("warning")
    const groupHeading = screen.getByRole('heading', { level: 2 })
    const statusDot = within(groupHeading).getByTestId('status-dot')
    expect(statusDot).toHaveAttribute('data-status', 'warning')

    const rebalanceButtons = screen.getAllByRole('button', { name: 'Rebalance' })
    // Master is rebalanceButtons[0]
    expect(within(rebalanceButtons[0]).getByTestId('rebalance-notification-dot')).toBeInTheDocument()

    // Follower f1 is rebalanceButtons[1] (active drifting)
    expect(within(rebalanceButtons[1]).getByTestId('rebalance-notification-dot')).toBeInTheDocument()

    // Follower f2 is rebalanceButtons[2] (not in imbalancedFollowerIds in this test)
    expect(within(rebalanceButtons[2]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()

    // Follower f3 is rebalanceButtons[3] (balanced)
    expect(within(rebalanceButtons[3]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()
  })

  it('keeps green group status dot and hides master dot when ONLY disabled followers drift, but shows dot on disabled follower', () => {
    render(() => (
      <GroupCard
        detail={mockDetail}
        status="ok"
        hasImbalance={false} // No active followers drifting
        imbalancedFollowerIds={new Set(['f2'])} // f2 is disabled but drifted
        onToggleCopy={() => {}}
        onToggleMasterActive={() => {}}
        onAction={vi.fn()}
      />
    ))

    // Group name status dot remains green ("ok")
    const groupHeading = screen.getByRole('heading', { level: 2 })
    const statusDot = within(groupHeading).getByTestId('status-dot')
    expect(statusDot).toHaveAttribute('data-status', 'ok')

    const rebalanceButtons = screen.getAllByRole('button', { name: 'Rebalance' })
    // Master has no dot
    expect(within(rebalanceButtons[0]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()

    // Follower f1 (active, no drift) has no dot
    expect(within(rebalanceButtons[1]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()

    // Follower f2 (disabled drifting) HAS notification dot
    expect(within(rebalanceButtons[2]).getByTestId('rebalance-notification-dot')).toBeInTheDocument()

    // Follower f3 (balanced) has no dot
    expect(within(rebalanceButtons[3]).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()
  })

  it('renders green group status dot and no notification dots when group is in equilibrium', () => {
    render(() => (
      <GroupCard
        detail={mockDetail}
        status="ok"
        hasImbalance={false}
        imbalancedFollowerIds={new Set()}
        onToggleCopy={() => {}}
        onToggleMasterActive={() => {}}
        onAction={vi.fn()}
      />
    ))

    const groupHeading = screen.getByRole('heading', { level: 2 })
    const statusDot = within(groupHeading).getByTestId('status-dot')
    expect(statusDot).toHaveAttribute('data-status', 'ok')

    const rebalanceButtons = screen.getAllByRole('button', { name: 'Rebalance' })
    for (const btn of rebalanceButtons) {
      expect(within(btn).queryByTestId('rebalance-notification-dot')).not.toBeInTheDocument()
    }
  })

  it('preserves red error dot next to group name when status is "error" even if imbalance exists', () => {
    render(() => (
      <GroupCard
        detail={mockDetail}
        status="error"
        hasImbalance={true}
        imbalancedFollowerIds={new Set(['f1'])}
        onToggleCopy={() => {}}
        onToggleMasterActive={() => {}}
        onAction={vi.fn()}
      />
    ))

    const groupHeading = screen.getByRole('heading', { level: 2 })
    const statusDot = within(groupHeading).getByTestId('status-dot')
    expect(statusDot).toHaveAttribute('data-status', 'error')
  })
})
