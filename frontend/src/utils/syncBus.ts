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
 * 2. By binding all active viewers to a shared clock interval (ACTIVE_POLL_INTERVAL_MS = 1500ms),
 *    every subscriber refetches on the EXACT same tick.
 * 3. Additionally, window focus, visibility change, and user actions trigger an immediate
 *    synchronized refetch via `triggerSyncNow()`.
 * 4. Adaptive timing: During off-market hours (12:15 AM to 8:00 AM IST on weekdays, and all day
 *    on weekends), polling frequency slows to OFF_HOURS_POLL_INTERVAL_MS (30,000ms), saving
 *    95% of background server load when Indian exchanges (NSE/BSE/MCX) are completely closed.
 */

export const ACTIVE_POLL_INTERVAL_MS = 1500
export const OFF_HOURS_POLL_INTERVAL_MS = 30000
export const POLL_INTERVAL_MS = ACTIVE_POLL_INTERVAL_MS

let testPollIntervalOverride: number | null = null

export function setTestPollIntervalOverride(override: number | null) {
  testPollIntervalOverride = override
}

/**
 * Returns true if the given date is during off-market hours in Indian Standard Time (IST):
 * - Weekends (Saturday and Sunday): all day
 * - Weekdays (Monday through Friday): between 12:15 AM (00:15) and 8:00 AM (08:00) IST
 */
export function isOffMarketHoursIST(date: Date = new Date()): boolean {
  const utc = date.getTime() + (date.getTimezoneOffset() * 60000)
  const istOffset = 5.5 * 3600000
  const istDate = new Date(utc + istOffset)
  const day = istDate.getDay() // 0 = Sunday, 6 = Saturday

  if (day === 0 || day === 6) return true

  const timeInMinutes = istDate.getHours() * 60 + istDate.getMinutes()
  // 12:15 AM = 15 minutes, 8:00 AM = 480 minutes
  return timeInMinutes >= 15 && timeInMinutes < 480
}

/**
 * Resolves the effective poll interval based on IST market schedule or test override.
 */
export function getEffectivePollInterval(date?: Date): number {
  if (testPollIntervalOverride !== null) {
    return testPollIntervalOverride
  }
  if (date !== undefined) {
    return isOffMarketHoursIST(date) ? OFF_HOURS_POLL_INTERVAL_MS : ACTIVE_POLL_INTERVAL_MS
  }
  const isTestEnv =
    (typeof (globalThis as any).process !== 'undefined' && (globalThis as any).process?.env?.NODE_ENV === 'test') ||
    import.meta.env?.MODE === 'test'
  if (isTestEnv) {
    return ACTIVE_POLL_INTERVAL_MS
  }
  return isOffMarketHoursIST() ? OFF_HOURS_POLL_INTERVAL_MS : ACTIVE_POLL_INTERVAL_MS
}

type SyncSubscriber = () => void | Promise<void>

const subscribers = new Set<SyncSubscriber>()
let timerId: ReturnType<typeof setTimeout> | null = null
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

function isDocumentVisible(): boolean {
  return typeof document === 'undefined' || document.visibilityState === 'visible'
}

function stopTimer() {
  if (timerId !== null) {
    clearTimeout(timerId)
    timerId = null
  }
}

function scheduleNextTick() {
  stopTimer()
  if (subscribers.size > 0 && isDocumentVisible()) {
    const delay = getEffectivePollInterval()
    timerId = setTimeout(() => {
      timerId = null
      tick()
      scheduleNextTick()
    }, delay)
  }
}

function startTimer() {
  if (timerId === null && subscribers.size > 0 && isDocumentVisible()) {
    scheduleNextTick()
  }
}

function initWindowListeners() {
  if (isInitialized) return
  isInitialized = true

  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') {
        if (subscribers.size > 0) {
          startTimer()
          triggerSyncNow()
        }
      } else {
        stopTimer()
      }
    })
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('focus', () => {
      if (isDocumentVisible() && subscribers.size > 0) {
        startTimer()
        triggerSyncNow()
      }
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
  if (subscribers.size > 0 && isDocumentVisible()) {
    scheduleNextTick()
  }
}

/**
 * For testing purposes: clear all subscribers and stop the interval timer.
 */
export function resetSyncBusForTest() {
  subscribers.clear()
  stopTimer()
  testPollIntervalOverride = null
}
