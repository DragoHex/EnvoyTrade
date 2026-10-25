import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  POLL_INTERVAL_MS,
  ACTIVE_POLL_INTERVAL_MS,
  OFF_HOURS_POLL_INTERVAL_MS,
  registerSyncSubscriber,
  triggerSyncNow,
  resetSyncBusForTest,
  isOffMarketHoursIST,
  getEffectivePollInterval,
  setTestPollIntervalOverride,
} from './syncBus'

describe('syncBus utility', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetSyncBusForTest()
  })

  afterEach(() => {
    resetSyncBusForTest()
    vi.useRealTimers()
  })

  it('exports POLL_INTERVAL_MS as 1500', () => {
    expect(POLL_INTERVAL_MS).toBe(1500)
  })

  it('notifies all registered subscribers concurrently on tick', async () => {
    const fn1 = vi.fn()
    const fn2 = vi.fn()

    const unreg1 = registerSyncSubscriber(fn1)
    const unreg2 = registerSyncSubscriber(fn2)

    expect(fn1).not.toHaveBeenCalled()
    expect(fn2).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)

    expect(fn1).toHaveBeenCalledTimes(1)
    expect(fn2).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)

    expect(fn1).toHaveBeenCalledTimes(2)
    expect(fn2).toHaveBeenCalledTimes(2)

    unreg1()
    unreg2()
  })

  it('stops calling unsubscribed callbacks', async () => {
    const fn1 = vi.fn()
    const fn2 = vi.fn()

    const unreg1 = registerSyncSubscriber(fn1)
    const unreg2 = registerSyncSubscriber(fn2)

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(fn1).toHaveBeenCalledTimes(1)
    expect(fn2).toHaveBeenCalledTimes(1)

    unreg1()

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(fn1).toHaveBeenCalledTimes(1)
    expect(fn2).toHaveBeenCalledTimes(2)

    unreg2()

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(fn1).toHaveBeenCalledTimes(1)
    expect(fn2).toHaveBeenCalledTimes(2)
  })

  it('triggerSyncNow fires subscribers immediately and resets interval', async () => {
    const fn = vi.fn()
    const unreg = registerSyncSubscriber(fn)

    // Advance 500ms (less than 1500ms)
    await vi.advanceTimersByTimeAsync(500)
    expect(fn).not.toHaveBeenCalled()

    // Trigger sync immediately
    triggerSyncNow()
    expect(fn).toHaveBeenCalledTimes(1)

    // Advancing 500ms shouldn't trigger another call (timer was reset)
    await vi.advanceTimersByTimeAsync(500)
    expect(fn).toHaveBeenCalledTimes(1)

    // Advancing remaining 1000ms triggers the next tick
    await vi.advanceTimersByTimeAsync(1000)
    expect(fn).toHaveBeenCalledTimes(2)

    unreg()
  })

  it('handles subscriber exceptions gracefully without affecting others', async () => {
    const badFn = vi.fn(() => {
      throw new Error('Subscriber failure')
    })
    const goodFn = vi.fn()

    const unreg1 = registerSyncSubscriber(badFn)
    const unreg2 = registerSyncSubscriber(goodFn)

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)

    expect(badFn).toHaveBeenCalledTimes(1)
    expect(goodFn).toHaveBeenCalledTimes(1)

    unreg1()
    unreg2()
  })

  it('pauses polling when document becomes hidden and resumes on visible', async () => {
    const fn = vi.fn()
    const unreg = registerSyncSubscriber(fn)

    // Advance one interval while visible
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(fn).toHaveBeenCalledTimes(1)

    // Simulate switching away: document becomes hidden
    Object.defineProperty(document, 'visibilityState', {
      value: 'hidden',
      writable: true,
      configurable: true,
    })
    document.dispatchEvent(new Event('visibilitychange'))

    // Advance 10 seconds while hidden — NO subscriber calls should occur
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fn).toHaveBeenCalledTimes(1)

    // Simulate returning: document becomes visible
    Object.defineProperty(document, 'visibilityState', {
      value: 'visible',
      writable: true,
      configurable: true,
    })
    document.dispatchEvent(new Event('visibilitychange'))

    // Returning to visible triggers an immediate sync tick
    expect(fn).toHaveBeenCalledTimes(2)

    // Advancing one interval continues polling
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(fn).toHaveBeenCalledTimes(3)

    unreg()
  })

  it('does not start timer when subscriber is registered while document is hidden', async () => {
    Object.defineProperty(document, 'visibilityState', {
      value: 'hidden',
      writable: true,
      configurable: true,
    })

    const fn = vi.fn()
    const unreg = registerSyncSubscriber(fn)

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3)
    expect(fn).toHaveBeenCalledTimes(0)

    // Become visible
    Object.defineProperty(document, 'visibilityState', {
      value: 'visible',
      writable: true,
      configurable: true,
    })
    document.dispatchEvent(new Event('visibilitychange'))

    expect(fn).toHaveBeenCalledTimes(1)

    unreg()
  })

  describe('adaptive off-market hours schedule', () => {
    // Helper to create UTC Date that corresponds to a specific IST time
    // IST is UTC + 5:30. So UTC = IST - 5:30.
    function createISTDate(year: number, month: number, day: number, hours: number, minutes: number): Date {
      // Month is 0-indexed in JS Date (0 = Jan, 9 = Oct)
      // Construct in UTC with offset
      const d = new Date(Date.UTC(year, month, day, hours, minutes))
      // Subtract 5 hours 30 minutes to make the IST time equal to (hours, minutes)
      return new Date(d.getTime() - (5 * 60 + 30) * 60000)
    }

    it('exports ACTIVE_POLL_INTERVAL_MS as 1500 and OFF_HOURS_POLL_INTERVAL_MS as 30000', () => {
      expect(ACTIVE_POLL_INTERVAL_MS).toBe(1500)
      expect(OFF_HOURS_POLL_INTERVAL_MS).toBe(30000)
      expect(POLL_INTERVAL_MS).toBe(1500)
    })

    describe('isOffMarketHoursIST positive, negative, and edge cases', () => {
      // 2026-10-21 is a Wednesday
      it('returns false at 12:14 AM IST on Wednesday (active market boundary - 1 min)', () => {
        const d = createISTDate(2026, 9, 21, 0, 14)
        expect(isOffMarketHoursIST(d)).toBe(false)
      })

      it('returns true at exactly 12:15 AM IST on Wednesday (off-market start boundary)', () => {
        const d = createISTDate(2026, 9, 21, 0, 15)
        expect(isOffMarketHoursIST(d)).toBe(true)
      })

      it('returns true in the middle of the night (3:30 AM IST on Wednesday)', () => {
        const d = createISTDate(2026, 9, 21, 3, 30)
        expect(isOffMarketHoursIST(d)).toBe(true)
      })

      it('returns true at 7:59 AM IST on Wednesday (off-market end boundary - 1 min)', () => {
        const d = createISTDate(2026, 9, 21, 7, 59)
        expect(isOffMarketHoursIST(d)).toBe(true)
      })

      it('returns false at exactly 8:00 AM IST on Wednesday (market open boundary)', () => {
        const d = createISTDate(2026, 9, 21, 8, 0)
        expect(isOffMarketHoursIST(d)).toBe(false)
      })

      it('returns false during prime trading hours (9:15 AM IST and 2:30 PM IST on Wednesday)', () => {
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 21, 9, 15))).toBe(false)
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 21, 14, 30))).toBe(false)
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 21, 23, 30))).toBe(false)
      })

      // 2026-10-24 is Saturday, 2026-10-25 is Sunday
      it('returns true all day on Saturday regardless of time', () => {
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 24, 0, 5))).toBe(true)
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 24, 11, 0))).toBe(true)
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 24, 15, 30))).toBe(true)
      })

      it('returns true all day on Sunday regardless of time', () => {
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 25, 10, 0))).toBe(true)
        expect(isOffMarketHoursIST(createISTDate(2026, 9, 25, 20, 0))).toBe(true)
      })
    })

    describe('getEffectivePollInterval and timer execution', () => {
      it('returns OFF_HOURS_POLL_INTERVAL_MS during off-market hours', () => {
        const offHoursDate = createISTDate(2026, 9, 21, 2, 0)
        expect(getEffectivePollInterval(offHoursDate)).toBe(OFF_HOURS_POLL_INTERVAL_MS)
      })

      it('returns ACTIVE_POLL_INTERVAL_MS during market hours', () => {
        const activeDate = createISTDate(2026, 9, 21, 10, 0)
        expect(getEffectivePollInterval(activeDate)).toBe(ACTIVE_POLL_INTERVAL_MS)
      })

      it('respects setTestPollIntervalOverride', () => {
        setTestPollIntervalOverride(5000)
        expect(getEffectivePollInterval()).toBe(5000)
        setTestPollIntervalOverride(null)
      })

      it('polls at off-hours interval when configured', async () => {
        setTestPollIntervalOverride(OFF_HOURS_POLL_INTERVAL_MS)
        const fn = vi.fn()
        const unreg = registerSyncSubscriber(fn)

        // Advancing 1500ms should NOT trigger tick in off-hours
        await vi.advanceTimersByTimeAsync(1500)
        expect(fn).toHaveBeenCalledTimes(0)

        // Advancing remaining time to 30,000ms triggers tick
        await vi.advanceTimersByTimeAsync(OFF_HOURS_POLL_INTERVAL_MS - 1500)
        expect(fn).toHaveBeenCalledTimes(1)

        unreg()
        setTestPollIntervalOverride(null)
      })

      it('triggerSyncNow fires immediately even during off-hours and reschedules correctly', async () => {
        setTestPollIntervalOverride(OFF_HOURS_POLL_INTERVAL_MS)
        const fn = vi.fn()
        const unreg = registerSyncSubscriber(fn)

        await vi.advanceTimersByTimeAsync(5000)
        expect(fn).not.toHaveBeenCalled()

        // Immediate user trigger
        triggerSyncNow()
        expect(fn).toHaveBeenCalledTimes(1)

        // Advancing 10000ms should not fire again yet
        await vi.advanceTimersByTimeAsync(10000)
        expect(fn).toHaveBeenCalledTimes(1)

        // Advancing remaining 20000ms triggers the next tick
        await vi.advanceTimersByTimeAsync(20000)
        expect(fn).toHaveBeenCalledTimes(2)

        unreg()
        setTestPollIntervalOverride(null)
      })
    })
  })
})


