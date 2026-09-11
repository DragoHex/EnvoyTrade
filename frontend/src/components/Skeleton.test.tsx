import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it, vi, afterEach } from 'vitest'
import { LoadingTimeout } from './Skeleton'

describe('LoadingTimeout', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders children until the timeout elapses', () => {
    vi.useFakeTimers()
    render(() => (
      <LoadingTimeout>
        <p>skeleton</p>
      </LoadingTimeout>
    ))
    expect(screen.getByText('skeleton')).toBeInTheDocument()

    vi.advanceTimersByTime(60_000)
    expect(screen.queryByText('skeleton')).not.toBeInTheDocument()
    expect(screen.getByText(/taking longer than expected/i)).toBeInTheDocument()
  })
})
