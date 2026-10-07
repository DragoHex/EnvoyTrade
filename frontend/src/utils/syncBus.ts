/**
 * Global Polling & Synchronization Bus (SyncBus)
 * 
 * Synchronizes background polling across Group Cards and Account Order Details.
 * 
 * Why this is needed:
 * 1. Previously, each component mounted its own `setInterval(..., 7000)`.
 *    Because components mounted at different moments, Master and Followers polled
 *    out-of-phase by up to 7 seconds. When a fast broker order completed, the Master
 *    row might update immediately, while Followers lagged behind by several seconds.
 * 2. By binding all active viewers to a shared clock interval (POLL_INTERVAL_MS = 1500ms),
 *    every subscriber refetches on the EXACT same tick.
 * 3. Additionally, window focus, visibility change, and user actions trigger an immediate
 *    synchronized refetch via `triggerSyncNow()`.
 */

export const POLL_INTERVAL_MS = 1500

type SyncSubscriber = () => void | Promise<void>

const subscribers = new Set<SyncSubscriber>()
let timerId: ReturnType<typeof setInterval> | null = null
let isInitialized = false

function tick() {
  for (const sub of Array.from(subscribers)) {
    try {
      sub()
    } catch {
      // Silently catch subscriber errors to prevent crashing other subscribers
    }
  }
}

function startTimer() {
  if (timerId === null && subscribers.size > 0) {
    timerId = setInterval(tick, POLL_INTERVAL_MS)
  }
}

function stopTimer() {
  if (timerId !== null) {
    clearInterval(timerId)
    timerId = null
  }
}

function initWindowListeners() {
  if (isInitialized) return
  isInitialized = true

  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') {
        triggerSyncNow()
      }
    })
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('focus', () => {
      triggerSyncNow()
    })
  }
}

/**
 * Register a callback to be invoked on every synchronized poll tick.
 * Automatically starts the timer if this is the first subscriber.
 * Returns an unsubscribe cleanup function.
 */
export function registerSyncSubscriber(subscriber: SyncSubscriber): () => void {
  initWindowListeners()
  subscribers.add(subscriber)
  startTimer()

  return () => {
    subscribers.delete(subscriber)
    if (subscribers.size === 0) {
      stopTimer()
    }
  }
}

/**
 * Trigger an immediate synchronized refetch across all subscribers
 * and reset the timer so the next interval starts from now.
 */
export function triggerSyncNow() {
  tick()
  if (timerId !== null) {
    clearInterval(timerId)
    timerId = setInterval(tick, POLL_INTERVAL_MS)
  }
}

/**
 * For testing purposes: clear all subscribers and stop the interval timer.
 */
export function resetSyncBusForTest() {
  subscribers.clear()
  stopTimer()
}
