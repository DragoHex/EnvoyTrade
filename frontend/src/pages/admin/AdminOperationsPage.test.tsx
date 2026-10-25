import { render, screen, waitFor, fireEvent } from '@solidjs/testing-library'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AdminOperationsPage } from './AdminOperationsPage'
import * as api from '../../api'

function renderOperationsPage() {
  window.history.pushState({}, '', '/admin/operations')
  return render(() => (
    <Router>
      <Route path="/admin/status" component={() => <div>status</div>} />
      <Route path="/admin/operations" component={AdminOperationsPage} />
      <Route path="/admin/ips" component={() => <div>ips</div>} />
      <Route path="/admin/users" component={() => <div>users</div>} />
    </Router>
  ))
}

const mockReadiness: api.BrokerReadinessSummary = {
  masterReady: true,
  totalFollowers: 5,
  followersReady: 4,
  attentionAccounts: [
    {
      id: 'acc-1',
      name: 'Attention Follower',
      brokerAccountId: 'ZW1234',
      role: 'follower',
      authStatus: 'error',
      authError: 'Token expired',
    },
  ],
}

const mockExecutionFeed: api.ExecutionEvent[] = [
  {
    timestamp: new Date().toISOString(),
    tradingsymbol: 'INFY',
    exchange: 'NSE',
    transactionType: 'BUY',
    masterQty: 100,
    price: 1540.25,
    followersCount: 4,
    skippedCount: 1,
    latencyMs: 8,
  },
]

const mockOrderIssuesResponse: api.AdminOrderIssuesResponse = {
  issues: [
    {
      id: 99,
      createdAt: new Date().toISOString(),
      followerName: 'Follower Alpha',
      brokerAccountId: 'FA9999',
      tradingsymbol: 'BANKNIFTY26OCTFUT',
      exchange: 'NFO',
      intendedQty: 15,
      placedQty: 0,
      sizingReason: 'below_one_lot',
      terminalStatus: 'SKIPPED',
      lastError: 'below_one_lot',
    },
  ],
  total: 1,
  limit: 50,
  offset: 0,
}

describe('AdminOperationsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders sub-tabs and defaults to Pre-Market Broker Readiness', async () => {
    vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(mockExecutionFeed)
    vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    renderOperationsPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Pre-Market Broker Readiness/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Live Execution Feed/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /Order Issues & Rejections Audit/i })).toBeInTheDocument()
      expect(screen.getByText('4 / 5 Ready')).toBeInTheDocument()
    })

    expect(screen.getByText('Attention Follower (ZW1234)')).toBeInTheDocument()
    expect(screen.getByText('Token expired')).toBeInTheDocument()
    expect(screen.getByText('Master Broker Session')).toBeInTheDocument()
    expect(screen.queryByText('Master Zerodha Token')).not.toBeInTheDocument()

    // Role Icon (Follower Hobbit) and Auth Status Icon (Red Cross Error)
    expect(screen.getByRole('img', { name: 'Follower' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /Authentication Error: Token expired/i })).toBeInTheDocument()
  })

  it('renders RoleIcon, AuthStatusIcon in error state, and corrects raw "authenticated" status on attention accounts', async () => {
    vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue({
      masterReady: false,
      totalFollowers: 2,
      followersReady: 1,
      attentionAccounts: [
        {
          id: 'acc-raw-auth',
          name: 'Master Account',
          brokerAccountId: 'MST001',
          role: 'master',
          authStatus: 'authenticated', // Raw DB state where token expired
          authError: '',
        },
      ],
    })

    renderOperationsPage()

    await waitFor(() => {
      expect(screen.getByText('Master Account (MST001)')).toBeInTheDocument()
    })

    // Role icon is Gandalf for Master
    expect(screen.getByRole('img', { name: 'Master' })).toBeInTheDocument()

    // Auth status icon must be Error (red cross), NOT active session
    expect(screen.getByRole('img', { name: /Authentication Error: Token expired/i })).toBeInTheDocument()
    expect(screen.queryByRole('img', { name: /Active Broker Session/i })).not.toBeInTheDocument()
  })

  it('switches to Live Execution Feed tab on click', async () => {
    vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(mockExecutionFeed)
    vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    renderOperationsPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Live Execution Feed/i })).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: /Live Execution Feed/i }))

    await waitFor(() => {
      expect(screen.getByText('INFY')).toBeInTheDocument()
      expect(screen.getByText('8 ms')).toBeInTheDocument()
      expect(screen.getByText('4 placed')).toBeInTheDocument()
      expect(screen.getByText('1 skipped')).toBeInTheDocument()
    })
  })

  it('switches to Order Issues & Rejections Audit tab on click and fetches issues', async () => {
    vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(mockExecutionFeed)
    const issuesSpy = vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    renderOperationsPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Order Issues & Rejections Audit/i })).toBeInTheDocument()
    })

    issuesSpy.mockClear()
    fireEvent.click(screen.getByRole('button', { name: /Order Issues & Rejections Audit/i }))

    await waitFor(() => {
      expect(issuesSpy).toHaveBeenCalledWith(50, 0)
      expect(screen.getByText('Follower Alpha (FA9999)')).toBeInTheDocument()
      expect(screen.getByText('BANKNIFTY26OCTFUT')).toBeInTheDocument()
      expect(screen.getByText('below_one_lot')).toBeInTheDocument()
      expect(screen.getByText('SKIPPED')).toBeInTheDocument()
    })
  })

  it('paginates order issues when total exceeds 50', async () => {
    vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue([])
    
    // Page 1 mock response with total = 75
    const page1Issues: api.OrderIssue[] = Array.from({ length: 50 }, (_, i) => ({
      id: i + 1,
      createdAt: new Date().toISOString(),
      followerName: `Follower ${i + 1}`,
      brokerAccountId: `ACC${i + 1}`,
      tradingsymbol: 'TATASTEEL',
      exchange: 'NSE',
      intendedQty: 100,
      placedQty: 0,
      sizingReason: 'margin_insufficient',
      terminalStatus: 'REJECTED',
      lastError: 'Margin Insufficient',
    }))

    const page2Issues: api.OrderIssue[] = Array.from({ length: 25 }, (_, i) => ({
      id: 50 + i + 1,
      createdAt: new Date().toISOString(),
      followerName: `Follower ${50 + i + 1}`,
      brokerAccountId: `ACC${50 + i + 1}`,
      tradingsymbol: 'TATASTEEL',
      exchange: 'NSE',
      intendedQty: 100,
      placedQty: 0,
      sizingReason: 'margin_insufficient',
      terminalStatus: 'REJECTED',
      lastError: 'Margin Insufficient',
    }))

    const issuesSpy = vi.spyOn(api, 'getAdminOrderIssues').mockImplementation((_limit, offset) => {
      if (offset === 50) {
        return Promise.resolve({
          issues: page2Issues,
          total: 75,
          limit: 50,
          offset: 50,
        })
      }
      return Promise.resolve({
        issues: page1Issues,
        total: 75,
        limit: 50,
        offset: 0,
      })
    })

    renderOperationsPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Order Issues & Rejections Audit/i })).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: /Order Issues & Rejections Audit/i }))

    await waitFor(() => {
      expect(screen.getByText(/Showing 1 – 50 of 75 issues/i)).toBeInTheDocument()
    })

    const nextBtn = screen.getByRole('button', { name: /^Next$/i })
    expect(nextBtn).toBeEnabled()

    fireEvent.click(nextBtn)

    await waitFor(() => {
      expect(issuesSpy).toHaveBeenCalledWith(50, 50)
      expect(screen.getByText(/Showing 51 – 75 of 75 issues/i)).toBeInTheDocument()
    })
  })

  it('paginates execution feed when events exceed 50', async () => {
    vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    const manyEvents: api.ExecutionEvent[] = Array.from({ length: 60 }, (_, i) => ({
      timestamp: new Date().toISOString(),
      tradingsymbol: `SYM${i + 1}`,
      exchange: 'NSE',
      transactionType: 'BUY',
      masterQty: 10,
      price: 100,
      followersCount: 2,
      skippedCount: 0,
      latencyMs: 5,
    }))
    vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(manyEvents)

    renderOperationsPage()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Live Execution Feed/i })).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: /Live Execution Feed/i }))

    await waitFor(() => {
      expect(screen.getByText('SYM1')).toBeInTheDocument()
      expect(screen.getByText(/Showing 1 – 50 of 60 events/i)).toBeInTheDocument()
    })

    const nextBtn = screen.getByRole('button', { name: /^Next$/i })
    fireEvent.click(nextBtn)

    await waitFor(() => {
      expect(screen.getByText('SYM51')).toBeInTheDocument()
      expect(screen.getByText(/Showing 51 – 60 of 60 events/i)).toBeInTheDocument()
    })
  })

  it('lazy loads tabs: only queries broker readiness on mount, not feed or issues', async () => {
    const readySpy = vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    const feedSpy = vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(mockExecutionFeed)
    const issuesSpy = vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    renderOperationsPage()

    await waitFor(() => {
      expect(readySpy).toHaveBeenCalledTimes(1)
    })

    // Feed and issues must NOT be called on mount
    expect(feedSpy).not.toHaveBeenCalled()
    expect(issuesSpy).not.toHaveBeenCalled()
  })

  it('switches tabs without re-fetching already loaded tab data', async () => {
    const readySpy = vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    const feedSpy = vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(mockExecutionFeed)
    const issuesSpy = vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    renderOperationsPage()

    await waitFor(() => {
      expect(readySpy).toHaveBeenCalledTimes(1)
    })

    // Switch to Feed
    fireEvent.click(screen.getByRole('button', { name: /Live Execution Feed/i }))
    await waitFor(() => {
      expect(feedSpy).toHaveBeenCalledTimes(1)
    })

    // Switch back to Readiness — should use cache, not call readiness API again
    fireEvent.click(screen.getByRole('button', { name: /Pre-Market Broker Readiness/i }))
    expect(readySpy).toHaveBeenCalledTimes(1)

    // Switch back to Feed — should use cache, not call feed API again
    fireEvent.click(screen.getByRole('button', { name: /Live Execution Feed/i }))
    expect(feedSpy).toHaveBeenCalledTimes(1)
    expect(issuesSpy).not.toHaveBeenCalled()
  })

  it('refresh button only queries the active tab', async () => {
    const readySpy = vi.spyOn(api, 'getAdminBrokerReadiness').mockResolvedValue(mockReadiness)
    const feedSpy = vi.spyOn(api, 'getAdminExecutionFeed').mockResolvedValue(mockExecutionFeed)
    const issuesSpy = vi.spyOn(api, 'getAdminOrderIssues').mockResolvedValue(mockOrderIssuesResponse)

    renderOperationsPage()

    await waitFor(() => {
      expect(readySpy).toHaveBeenCalledTimes(1)
    })

    // Switch to Feed tab
    fireEvent.click(screen.getByRole('button', { name: /Live Execution Feed/i }))
    await waitFor(() => {
      expect(feedSpy).toHaveBeenCalledTimes(1)
    })

    // Click Refresh while on Feed tab
    const refreshBtn = screen.getByRole('button', { name: /Refresh/i })
    fireEvent.click(refreshBtn)

    await waitFor(() => {
      expect(feedSpy).toHaveBeenCalledTimes(2)
    })

    // Readiness and Issues should NOT be re-queried
    expect(readySpy).toHaveBeenCalledTimes(1)
    expect(issuesSpy).not.toHaveBeenCalled()
  })
})

