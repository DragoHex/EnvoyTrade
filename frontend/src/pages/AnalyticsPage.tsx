export function AnalyticsPage() {
  return (
    <div class="page">
      <h1>Analytics</h1>

      <div class="analytics-metrics-grid">
        <div class="analytics-metric-card">
          <span class="analytics-metric-label">Total Volume</span>
          <span class="analytics-metric-value">—</span>
          <span class="analytics-metric-hint">Historical execution volume</span>
        </div>
        <div class="analytics-metric-card">
          <span class="analytics-metric-label">Copy Success Rate</span>
          <span class="analytics-metric-value">—</span>
          <span class="analytics-metric-hint">Fill accuracy vs master</span>
        </div>
        <div class="analytics-metric-card">
          <span class="analytics-metric-label">Avg Slippage</span>
          <span class="analytics-metric-value">—</span>
          <span class="analytics-metric-hint">Master vs follower fill delta</span>
        </div>
      </div>

      <section class="analytics-card" data-testid="analytics-card">
        <div class="analytics-badge">Coming Soon</div>
        <h2 class="analytics-title">Analytics & Performance Tracking</h2>
        <p class="analytics-description">
          Portfolio performance, trade execution analytics, slippage tracking, and copy-trade statistics will be available here.
        </p>
      </section>
    </div>
  )
}
