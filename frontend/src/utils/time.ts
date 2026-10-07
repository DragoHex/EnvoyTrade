/**
 * Indian Standard Time (IST) utilities for EnvoyTrade.
 * Markets (NSE/BSE/MCX) operate strictly in IST (UTC+05:30).
 */
export const IST_TIMEZONE = 'Asia/Kolkata'

/**
 * Formats any date, ISO timestamp string, or Date object into standard IST format:
 * "YYYY-MM-DD HH:mm:ss"
 */
export function formatISTDateTime(input: string | Date | undefined | null): string {
  if (!input) return '—'
  if (typeof input === 'string') {
    const trimmed = input.trim()
    if (!trimmed) return '—'
    if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(trimmed)) {
      return trimmed
    }
  }

  const d = typeof input === 'string' ? new Date(input) : input
  if (isNaN(d.getTime())) {
    return typeof input === 'string' ? input : '—'
  }

  const formatter = new Intl.DateTimeFormat('en-IN', {
    timeZone: IST_TIMEZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })

  const parts = formatter.formatToParts(d)
  const p: Record<string, string> = {}
  for (const part of parts) {
    p[part.type] = part.value
  }

  // Handle midnight 24 edge-case if returned by some runtimes
  let hour = p.hour || '00'
  if (hour === '24') hour = '00'

  return `${p.year}-${p.month}-${p.day} ${hour}:${p.minute}:${p.second}`
}

/**
 * Formats the time component of any date or timestamp in IST:
 * "HH:mm:ss"
 */
export function formatISTTime(input: string | Date | undefined | null): string {
  if (!input) return '—'
  if (typeof input === 'string') {
    const trimmed = input.trim()
    if (!trimmed) return '—'
    if (/^\d{2}:\d{2}:\d{2}$/.test(trimmed)) {
      return trimmed
    }
    if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(trimmed)) {
      return trimmed.slice(11)
    }
  }

  const d = typeof input === 'string' ? new Date(input) : input
  if (isNaN(d.getTime())) {
    return typeof input === 'string' ? input : '—'
  }

  const formatter = new Intl.DateTimeFormat('en-IN', {
    timeZone: IST_TIMEZONE,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })

  const parts = formatter.formatToParts(d)
  const p: Record<string, string> = {}
  for (const part of parts) {
    p[part.type] = part.value
  }

  let hour = p.hour || '00'
  if (hour === '24') hour = '00'

  return `${hour}:${p.minute}:${p.second}`
}
