import { render, screen, waitFor, within } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { DashboardHeroCard } from './DashboardHeroCard'
import * as AuthContextModule from '../context/AuthContext'
import type { GroupSummary, GroupDetail } from '../api'

describe('DashboardHeroCard', () => {
  const mockUser = {
    id: 'u1',
    email: 'trader@envoytrade.com',
    username: 'trader',
    name: 'Jane Doe',
    role: 'trader',
  }

  const mockGroups: GroupSummary[] = [
    {
      id: 'g1',
      name: 'Alpha Growth',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Rajesh Sharma',
      broker: 'zerodha',
      followerCount: 2,
      status: 'ok',
      active: true,
    },
    {
      id: 'g2',
      name: 'Beta Hedged',
      masterId: 'm2',
      masterAccountId: 'ZX5678',
      masterName: 'Sunita Rao',
      broker: 'zerodha',
      followerCount: 1,
      status: 'ok',
      active: true,
    },
  ]

  const mockGroupDetails: Record<string, GroupDetail> = {
    g1: {
      id: 'g1',
      name: 'Alpha Growth',
      masterId: 'm1',
      masterAccountId: 'ZX1234',
      masterName: 'Rajesh Sharma',
      masterActive: true,
      masterTotalMtm: 5000,
      followers: [
        {
          accountId: 'f1',
          name: 'Follower One',
          brokerAccountId: 'FO111',
          enabled: true,
          status: 'ok',
          totalMtm: 1500,
        },
        {
          accountId: 'f2',
          name: 'Follower Two',
          brokerAccountId: 'FO222',
          enabled: true,
          status: 'ok',
          totalMtm: 2500,
        },
      ],
    },
    g2: {
      id: 'g2',
      name: 'Beta Hedged',
      masterId: 'm2',
      masterAccountId: 'ZX5678',
      masterName: 'Sunita Rao',
      masterActive: true,
      masterTotalMtm: 3000,
      followers: [
        {
          accountId: 'f3',
          name: 'Follower Three',
          brokerAccountId: 'FO333',
          enabled: true,
          status: 'ok',
          totalMtm: -1000,
        },
      ],
    },
  }

  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    vi.spyOn(AuthContextModule, 'useAuth').mockReturnValue({
      user: () => mockUser,
      isLoading: () => false,
      isAuthenticated: () => true,
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })
  })

  // 1. Positive: Two-Partition Layout, Branding, User Full Name, and @envoytrade
  it('renders two partitions: left shareable card with @envoytrade handle and right controls panel', () => {
    const { container } = render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    const shareableCard = container.querySelector('.hero-shareable-card')
    const controlsPanel = container.querySelector('.hero-controls-panel')
    expect(shareableCard).toBeInTheDocument()
    expect(controlsPanel).toBeInTheDocument()

    // Brand and User Info on the Left Shareable Card
    expect(within(shareableCard as HTMLElement).getByTestId('envoy-logo')).toBeInTheDocument()
    expect(within(shareableCard as HTMLElement).getByText('EnvoyTrade')).toBeInTheDocument()
    expect(within(shareableCard as HTMLElement).getByText('Jane Doe')).toBeInTheDocument()
    // Displays @envoytrade, not @trader
    expect(within(shareableCard as HTMLElement).getByText('@envoytrade')).toBeInTheDocument()
    expect(within(shareableCard as HTMLElement).queryByText('@trader')).not.toBeInTheDocument()
    expect(within(shareableCard as HTMLElement).getByText('JD')).toBeInTheDocument()

    // Right Controls Panel has Scope select and telemetry
    expect(within(controlsPanel as HTMLElement).getByLabelText('Select MTM Scope')).toBeInTheDocument()
    expect(within(controlsPanel as HTMLElement).getByText('2 Groups Active')).toBeInTheDocument()
  })

  // 2. Positive: "All Masters" displays "For all active Primary Accounts"
  it('defaults to All Masters showing "For all active Primary Accounts" as subtext', () => {
    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    // Total MTM for masters is 5000 + 3000 = +₹8,000.00
    expect(screen.getByText('+₹8,000.00')).toBeInTheDocument()
    const subtextEl = screen.getByTestId('hero-mtm-subtext')
    expect(subtextEl).toHaveTextContent('For all active Primary Accounts')
  })

  // 3. Positive & Ordering: "All Groups" keeps Masters before Followers in breakdown
  it('switches to All Groups and keeps Masters before Followers in the breakdown', async () => {
    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    // Total: Masters (8000) + Followers (1500 + 2500 - 1000 = 3000) = 11,000
    const select = screen.getByLabelText('Select MTM Scope')
    await userEvent.selectOptions(select, 'all_groups')

    await waitFor(() => {
      expect(screen.getByText('+₹11,000.00')).toBeInTheDocument()
    })

    const subtextEl = screen.getByTestId('hero-mtm-subtext')
    // Must list Masters before Followers: "Masters: +₹8,000.00 • Followers: +₹3,000.00"
    const text = subtextEl.textContent || ''
    const mastersIndex = text.indexOf('Masters:')
    const followersIndex = text.indexOf('Followers:')
    expect(mastersIndex).toBeGreaterThanOrEqual(0)
    expect(followersIndex).toBeGreaterThanOrEqual(0)
    expect(mastersIndex).toBeLessThan(followersIndex)
    expect(subtextEl).toHaveTextContent(/Masters:\s*\+₹8,000\.00\s*•\s*Followers:\s*\+₹3,000\.00/)
  })

  // 4. Positive & Ordering: Individual Group keeps Master before Follower
  it('switches to Individual Group Total and keeps Master before Follower in breakdown', async () => {
    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    // Alpha Growth: Master = 5000, Followers = 4000. Total = 9000.
    const select = screen.getByLabelText('Select MTM Scope')
    await userEvent.selectOptions(select, 'group_agg:g1')

    await waitFor(() => {
      expect(screen.getByText('+₹9,000.00')).toBeInTheDocument()
    })

    const subtextEl = screen.getByTestId('hero-mtm-subtext')
    // Must list Master before Follower: "Master: +₹5,000.00 (Followers: +₹4,000.00)"
    const text = subtextEl.textContent || ''
    const masterIndex = text.indexOf('Master:')
    const followersIndex = text.indexOf('Followers:')
    expect(masterIndex).toBeGreaterThanOrEqual(0)
    expect(followersIndex).toBeGreaterThanOrEqual(0)
    expect(masterIndex).toBeLessThan(followersIndex)
    expect(subtextEl).toHaveTextContent(/Master:\s*\+₹5,000\.00\s*\(Followers:\s*\+₹4,000\.00\)/)
  })

  // 5. Positive: Master Only shows Master's name (not account ID) in dropdown and on card
  it('shows Master name with group name in brackets in dropdown, and Master name on card', async () => {
    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    // Dropdown option should be "Rajesh Sharma (Alpha Growth)"
    expect(screen.getByRole('option', { name: 'Rajesh Sharma (Alpha Growth)' })).toBeInTheDocument()

    const select = screen.getByLabelText('Select MTM Scope')
    await userEvent.selectOptions(select, 'group_master:g1')

    await waitFor(() => {
      expect(screen.getByText('+₹5,000.00')).toBeInTheDocument()
    })

    // On the card, show Master's name (Rajesh Sharma), NOT account ID (ZX1234), and no "Master:" prefix
    const subtextEl = screen.getByTestId('hero-mtm-subtext')
    expect(subtextEl).toHaveTextContent('Rajesh Sharma')
    expect(subtextEl).not.toHaveTextContent('Master:')
    expect(subtextEl).not.toHaveTextContent('ZX1234')
  })

  // 6. Negative Case: Negative MTM formats with minus and red styling
  it('formats negative overall MTM with minus sign and negative class', () => {
    const negativeDetails: Record<string, GroupDetail> = {
      g1: {
        ...mockGroupDetails.g1,
        masterTotalMtm: -4250.75,
      },
    }

    render(() => (
      <DashboardHeroCard
        groups={[mockGroups[0]]}
        groupDetails={negativeDetails}
      />
    ))

    const mtmEl = screen.getByText('-₹4,250.75')
    expect(mtmEl).toBeInTheDocument()
    expect(mtmEl).toHaveClass('negative')
  })

  // 7. Negative & Mixed Case: Mixed P&L with positive Master and negative Followers
  it('formats mixed P&L with appropriate text-positive and text-negative classes in breakdown', async () => {
    const mixedDetails: Record<string, GroupDetail> = {
      g1: {
        ...mockGroupDetails.g1,
        masterTotalMtm: 6000,
        followers: [
          {
            accountId: 'f1',
            name: 'Follower One',
            brokerAccountId: 'FO111',
            enabled: true,
            status: 'ok',
            totalMtm: -2500, // Negative followers
          },
        ],
      },
    }

    render(() => (
      <DashboardHeroCard
        groups={[mockGroups[0]]}
        groupDetails={mixedDetails}
      />
    ))

    const select = screen.getByLabelText('Select MTM Scope')
    await userEvent.selectOptions(select, 'group_agg:g1')

    await waitFor(() => {
      expect(screen.getByText('+₹3,500.00')).toBeInTheDocument()
    })

    const subtextEl = screen.getByTestId('hero-mtm-subtext')
    expect(subtextEl.querySelector('.text-positive')).toHaveTextContent('+₹6,000.00')
    expect(subtextEl.querySelector('.text-negative')).toHaveTextContent('-₹2,500.00')
  })

  // 8. Edge Case: Missing user name falls back gracefully but retains @envoytrade
  it('handles missing user name gracefully with fallback to Trader and displays @envoytrade', () => {
    vi.spyOn(AuthContextModule, 'useAuth').mockReturnValue({
      user: () => ({ id: 'u2', email: 'test@example.com', username: '', name: '', role: 'trader' }),
      isLoading: () => false,
      isAuthenticated: () => true,
      login: vi.fn(),
      register: vi.fn(),
      logout: vi.fn(),
      refetchUser: vi.fn(),
      updateProfile: vi.fn(),
      updatePassword: vi.fn(),
    })

    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    expect(screen.getByText('Trader')).toBeInTheDocument()
    expect(screen.getByText('@envoytrade')).toBeInTheDocument()
  })

  // 9. Edge Case: Master has no masterName set (falls back to group name or ID without crashing)
  it('handles master without masterName by falling back to group name', async () => {
    const unnamedMasterGroups: GroupSummary[] = [
      {
        id: 'g3',
        name: 'Delta Neutral',
        masterId: 'm3',
        masterAccountId: 'ZX9999',
        broker: 'zerodha',
        followerCount: 0,
        status: 'ok',
        active: true,
      },
    ]

    const unnamedMasterDetails: Record<string, GroupDetail> = {
      g3: {
        id: 'g3',
        name: 'Delta Neutral',
        masterId: 'm3',
        masterAccountId: 'ZX9999',
        masterActive: true,
        masterTotalMtm: 1200,
        followers: [],
      },
    }

    render(() => (
      <DashboardHeroCard
        groups={unnamedMasterGroups}
        groupDetails={unnamedMasterDetails}
      />
    ))

    const select = screen.getByLabelText('Select MTM Scope')
    await userEvent.selectOptions(select, 'group_master:g3')

    await waitFor(() => {
      expect(screen.getByText('+₹1,200.00')).toBeInTheDocument()
    })
    expect(screen.getByTestId('hero-mtm-subtext')).toHaveTextContent('Delta Neutral')
    expect(screen.getByTestId('hero-mtm-subtext')).not.toHaveTextContent('Master:')
  })

  // 10. Edge Case: Empty groups & details render ₹0.00
  it('handles empty groups and details gracefully with ₹0.00', () => {
    render(() => (
      <DashboardHeroCard
        groups={[]}
        groupDetails={{}}
      />
    ))

    expect(screen.getByText('₹0.00')).toBeInTheDocument()
    expect(screen.getByText('0 Groups Active')).toBeInTheDocument()
  })

  // 11. Edge Case: Only active groups are aggregated
  it('only aggregates active groups, ignoring inactive groups from the aggregate total', () => {
    const mixedGroups: GroupSummary[] = [
      {
        id: 'g1',
        name: 'Active Group',
        masterId: 'm1',
        masterAccountId: 'ZX111',
        masterName: 'Active Master',
        broker: 'zerodha',
        followerCount: 1,
        status: 'ok',
        active: true,
      },
      {
        id: 'g2',
        name: 'Inactive Group',
        masterId: 'm2',
        masterAccountId: 'ZX222',
        masterName: 'Inactive Master',
        broker: 'zerodha',
        followerCount: 1,
        status: 'ok',
        active: false,
      },
    ]

    const mixedDetails: Record<string, GroupDetail> = {
      g1: {
        id: 'g1',
        name: 'Active Group',
        masterId: 'm1',
        masterAccountId: 'ZX111',
        masterName: 'Active Master',
        masterActive: true,
        masterTotalMtm: 5000,
        followers: [{ accountId: 'f1', name: 'Follower 1', brokerAccountId: 'FO1', enabled: true, status: 'ok', totalMtm: 1000 }],
      },
      g2: {
        id: 'g2',
        name: 'Inactive Group',
        masterId: 'm2',
        masterAccountId: 'ZX222',
        masterName: 'Inactive Master',
        masterActive: false,
        masterTotalMtm: 99000, // Should be ignored!
        followers: [{ accountId: 'f2', name: 'Follower 2', brokerAccountId: 'FO2', enabled: true, status: 'ok', totalMtm: 88000 }],
      },
    }

    render(() => (
      <DashboardHeroCard
        groups={mixedGroups}
        groupDetails={mixedDetails}
      />
    ))

    // Default All Masters aggregate should only include Active Group: 5000, NOT 5000 + 99000
    expect(screen.getByText('+₹5,000.00')).toBeInTheDocument()
    expect(screen.queryByText('+₹104,000.00')).not.toBeInTheDocument()
    expect(screen.getByText('1 Groups Active')).toBeInTheDocument()
    expect(screen.getByTestId('hero-mtm-subtext')).toHaveTextContent('For all active Primary Accounts')
  })

  // 12. Positive: Time and Date at the end like "Taken @ 25 Oct 2026, 11:50 AM" and centered MTM
  it('renders time and date at the end of the left card like "Taken @ 25 Oct 2026, 11:50 AM" with MTM in the center', () => {
    const fixedTime = new Date('2026-10-25T11:50:00+05:30')
    const { container } = render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
        timestamp={fixedTime}
      />
    ))

    const shareableCard = container.querySelector('.hero-shareable-card') as HTMLElement
    expect(shareableCard).toBeInTheDocument()

    // 1. Header has Brand on top-left and User profile on top-right
    const header = shareableCard.querySelector('.shareable-card-header') as HTMLElement
    expect(header).toBeInTheDocument()
    expect(within(header).getByText('EnvoyTrade')).toBeInTheDocument()
    expect(within(header).getByText('Jane Doe')).toBeInTheDocument()
    expect(within(header).getByText('@envoytrade')).toBeInTheDocument()

    // 2. MTM is in the center with capsule pill label
    const body = shareableCard.querySelector('.shareable-card-body') as HTMLElement
    expect(body).toBeInTheDocument()
    expect(within(body).getByText('TOTAL MTM TODAY')).toBeInTheDocument()
    expect(within(body).getByText('+₹8,000.00')).toBeInTheDocument()
    expect(within(body).getByTestId('hero-mtm-subtext')).toBeInTheDocument()

    // 3. Footer has Verified Trades badge on bottom-left and timestamp on bottom-right
    const footer = shareableCard.querySelector('.shareable-card-footer') as HTMLElement
    expect(footer).toBeInTheDocument()
    expect(within(footer).getByText('Verified Trades')).toBeInTheDocument()
    const timestampElem = within(footer).getByTestId('hero-timestamp')
    expect(timestampElem).toHaveTextContent('Taken @ 25 Oct 2026, 11:50 AM')
  })

  // 13. Edge: Automatically formats current time when timestamp prop is omitted
  it('formats current time as per IST when no explicit timestamp prop is provided', () => {
    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={mockGroupDetails}
      />
    ))

    const timestampElem = screen.getByTestId('hero-timestamp')
    expect(timestampElem.textContent).toMatch(/^Taken @ \d{1,2} [A-Z][a-z]{2} \d{4}, \d{1,2}:\d{2} (AM|PM)$/)
  })

  // 14. Performance/Optimization: Dropdown options array and DOM elements remain stable across groupDetails updates
  it('preserves select options DOM elements across groupDetails updates when groups are unchanged', async () => {
    const { createSignal } = await import('solid-js')
    const [details, setDetails] = createSignal<Record<string, GroupDetail>>(mockGroupDetails)

    const { container } = render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={details()}
      />
    ))

    const selectEl = container.querySelector('#dashboard-mtm-scope-select') as HTMLSelectElement
    expect(selectEl).toBeInTheDocument()
    const initialFirstOption = selectEl.options[0]

    // Simulate 1.5s polling update where MTM changes but groups do not
    setDetails({
      ...mockGroupDetails,
      g1: {
        ...mockGroupDetails.g1,
        masterTotalMtm: 9999,
      },
    })

    await waitFor(() => {
      expect(screen.getByText('+₹12,999.00')).toBeInTheDocument()
    })

    // The option element in the DOM must be the exact same reference (not torn down and recreated)
    const updatedFirstOption = selectEl.options[0]
    expect(updatedFirstOption).toBe(initialFirstOption)
  })

  // 15. Performance/Optimization: Subtext in All Groups updates text in-place without discarding DOM container
  it('updates All Groups subtext values in-place without replacing the subtext container', async () => {
    const { createSignal } = await import('solid-js')
    const [details, setDetails] = createSignal<Record<string, GroupDetail>>(mockGroupDetails)

    render(() => (
      <DashboardHeroCard
        groups={mockGroups}
        groupDetails={details()}
      />
    ))

    const select = screen.getByLabelText('Select MTM Scope')
    await userEvent.selectOptions(select, 'all_groups')

    await waitFor(() => {
      expect(screen.getByText('+₹11,000.00')).toBeInTheDocument()
    })

    const subtextContainer = screen.getByTestId('hero-mtm-subtext')

    // Simulate 1.5s polling update with new MTM
    setDetails({
      ...mockGroupDetails,
      g1: {
        ...mockGroupDetails.g1,
        masterTotalMtm: 8000, // Masters was 8000, now 8000 + 3000 = 11000
      },
    })

    await waitFor(() => {
      expect(screen.getByText('+₹14,000.00')).toBeInTheDocument()
    })

    // Subtext container should remain the exact same element in DOM
    expect(screen.getByTestId('hero-mtm-subtext')).toBe(subtextContainer)
    expect(subtextContainer).toHaveTextContent(/Masters:\s*\+₹11,000\.00\s*•\s*Followers:\s*\+₹3,000\.00/)
  })

  // 16. Edge Case: Neutral styling when MTM is exactly 0
  it('applies neutral styling and formats as ₹0.00 when MTM is exactly 0', () => {
    const zeroDetails: Record<string, GroupDetail> = {
      g1: {
        ...mockGroupDetails.g1,
        masterTotalMtm: 0,
        followers: [],
      },
    }

    render(() => (
      <DashboardHeroCard
        groups={[mockGroups[0]]}
        groupDetails={zeroDetails}
      />
    ))

    const mtmEl = screen.getByText('₹0.00')
    expect(mtmEl).toBeInTheDocument()
    expect(mtmEl).toHaveClass('neutral')
    expect(mtmEl).not.toHaveClass('positive')
    expect(mtmEl).not.toHaveClass('negative')
  })
})
