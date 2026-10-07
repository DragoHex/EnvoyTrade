import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  POLL_INTERVAL_MS,
  registerSyncSubscriber,
  triggerSyncNow,
  resetSyncBusForTest,
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
})
