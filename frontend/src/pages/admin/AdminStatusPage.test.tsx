import { render, screen, waitFor, fireEvent, within } from '@solidjs/testing-library'
import { Router, Route } from '@solidjs/router'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { AdminStatusPage } from './AdminStatusPage'
import * as api from '../../api'

function renderStatusPage() {
  window.history.pushState({}, '', '/admin/status')
  return render(() => (
    <Router>
      <Route path="/admin/status" component={AdminStatusPage} />
      <Route path="/admin/ips" component={() => <div>ips</div>} />
      <Route path="/admin/users" component={() => <div>users</div>} />
    </Router>
  ))
}

const mockStatus: api.PipelineStatus = {
  killSwitch: {
    engaged: false,
  },
  masterTickers: [
    { masterId: '11111111-1111-1111-1111-111111111111', active: true },
  ],
  queues: {
    masterFillDepth: 2,
    masterFillCap: 1024,
    orderUpdateDepth: 0,
    orderUpdateCap: 512,
  },
  workerCount: 4,
  dbPool: {
    acquiredConns: 1,
    idleConns: 9,
    totalConns: 10,
    maxConns: 20,
  },
  systemResources: {
    cpuPercent: 3.5,
    heapAllocMb: 18.2,
    sysMemoryMb: 42.0,
    goroutineCount: 38,
    gcCycles: 15,
  },
}

describe('AdminStatusPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('renders telemetry metrics and healthy pipeline state', async () => {
    vi.spyOn(api, 'getAdminServiceStatus').mockResolvedValue(mockStatus)

    renderStatusPage()

    await waitFor(() => {
      expect(screen.getByText('Copy Engine Operational')).toBeInTheDocument()
    })

    expect(screen.getByText('Master WebSocket Tickers')).toBeInTheDocument()
    expect(screen.getByText('1 Active')).toBeInTheDocument()
    expect(screen.getByText('2 / 1024')).toBeInTheDocument()
    expect(screen.getByText('4 Workers')).toBeInTheDocument()
    expect(screen.getByText('10 / 20')).toBeInTheDocument()
    expect(screen.getByText('System Resources (Process)')).toBeInTheDocument()
    expect(screen.getByText('3.5%')).toBeInTheDocument()
    expect(screen.getByText('18.2 MB / 42.0 MB')).toBeInTheDocument()
    expect(screen.getByText('38')).toBeInTheDocument()
    expect(screen.getByText('15')).toBeInTheDocument()
  })

  it('engages kill switch with reason prompt and disengages cleanly', async () => {
    vi.spyOn(api, 'getAdminServiceStatus').mockResolvedValue(mockStatus)
    const postKillSpy = vi.spyOn(api, 'postAdminKillSwitch').mockResolvedValue({
      status: 'ok',
      killSwitch: {
        engaged: true,
        reason: 'Circuit breaker volatility',
        engagedBy: 'admin@envoytrade.com',
        engagedAt: new Date().toISOString(),
      },
    })

    renderStatusPage()

    await waitFor(() => {
      expect(screen.getByText('Engage Kill Switch')).toBeInTheDocument()
    })

    // Click Engage Kill Switch
    fireEvent.click(screen.getByText('Engage Kill Switch'))

    // Dialog opens
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    const input = screen.getByPlaceholderText(/Market volatility breaker/i)
    fireEvent.input(input, { target: { value: 'Circuit breaker volatility' } })

    // Submit form
    const dialog = screen.getByRole('dialog')
    const submitBtn = within(dialog).getByRole('button', { name: 'Engage Kill Switch' })
    fireEvent.click(submitBtn)

    await waitFor(() => {
      expect(postKillSpy).toHaveBeenCalledWith(true, 'Circuit breaker volatility')
    })
  })

  it('closes kill switch modal when Escape key is pressed', async () => {
    vi.spyOn(api, 'getAdminServiceStatus').mockResolvedValue(mockStatus)

    renderStatusPage()

    await waitFor(() => {
      expect(screen.getByText('Engage Kill Switch')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByText('Engage Kill Switch'))
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    // Press Escape
    fireEvent.keyDown(document, { key: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })
})


