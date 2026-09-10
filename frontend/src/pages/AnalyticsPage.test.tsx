import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { AnalyticsPage } from './AnalyticsPage'

describe('AnalyticsPage', () => {
  it('renders analytics heading, metric placeholder cards, and coming soon message', () => {
    render(() => <AnalyticsPage />)

    expect(screen.getByRole('heading', { level: 1, name: 'Analytics' })).toBeInTheDocument()
    expect(screen.getByText('Coming Soon')).toBeInTheDocument()
    expect(screen.getByText('Analytics & Performance Tracking')).toBeInTheDocument()
    expect(screen.getByText('Total Volume')).toBeInTheDocument()
    expect(screen.getByText('Copy Success Rate')).toBeInTheDocument()
    expect(screen.getByText('Avg Slippage')).toBeInTheDocument()
  })
})
