import { describe, it, expect } from 'vitest'
import { formatISTDateTime, formatISTTime, formatHeroTimestamp } from './time'

describe('time utils', () => {
  describe('formatISTDateTime', () => {
    it('returns "—" for empty, null, or undefined values', () => {
      expect(formatISTDateTime(null)).toBe('—')
      expect(formatISTDateTime(undefined)).toBe('—')
      expect(formatISTDateTime('')).toBe('—')
      expect(formatISTDateTime('   ')).toBe('—')
    })

    it('preserves already formatted "YYYY-MM-DD HH:mm:ss" strings', () => {
      expect(formatISTDateTime('2026-09-11 09:15:00')).toBe('2026-09-11 09:15:00')
      expect(formatISTDateTime('2026-10-07 14:30:45')).toBe('2026-10-07 14:30:45')
    })

    it('converts UTC ISO strings to IST (UTC+05:30)', () => {
      // 03:45:00 UTC = 09:15:00 IST
      expect(formatISTDateTime('2026-10-07T03:45:00.000Z')).toBe('2026-10-07 09:15:00')
      // 22:00:00 UTC previous day = 03:30:00 IST next day
      expect(formatISTDateTime('2026-10-06T22:00:00.000Z')).toBe('2026-10-07 03:30:00')
    })

    it('converts Date instances to IST', () => {
      const date = new Date(Date.UTC(2026, 9, 7, 3, 45, 0)) // 2026-10-07 03:45:00 UTC
      expect(formatISTDateTime(date)).toBe('2026-10-07 09:15:00')
    })

    it('returns raw string for unparseable input', () => {
      expect(formatISTDateTime('not-a-date')).toBe('not-a-date')
    })
  })

  describe('formatISTTime', () => {
    it('returns "—" for empty or null values', () => {
      expect(formatISTTime(null)).toBe('—')
      expect(formatISTTime(undefined)).toBe('—')
      expect(formatISTTime('')).toBe('—')
    })

    it('preserves "HH:mm:ss" strings', () => {
      expect(formatISTTime('09:15:00')).toBe('09:15:00')
      expect(formatISTTime('14:30:45')).toBe('14:30:45')
    })

    it('extracts time from "YYYY-MM-DD HH:mm:ss" strings', () => {
      expect(formatISTTime('2026-09-11 09:15:00')).toBe('09:15:00')
      expect(formatISTTime('2026-10-07 14:30:45')).toBe('14:30:45')
    })

    it('converts UTC ISO strings to IST time', () => {
      expect(formatISTTime('2026-10-07T03:45:00.000Z')).toBe('09:15:00')
    })

    it('converts Date instances to IST time', () => {
      const date = new Date(Date.UTC(2026, 9, 7, 3, 45, 12))
      expect(formatISTTime(date)).toBe('09:15:12')
    })
  })

  describe('formatHeroTimestamp', () => {
    it('formats a Date object to "Taken @ D MMM YYYY, h:mm A" in IST', () => {
      const d = new Date('2026-10-25T11:50:00+05:30')
      expect(formatHeroTimestamp(d)).toBe('Taken @ 25 Oct 2026, 11:50 AM')
    })

    it('formats an ISO timestamp string accurately in IST', () => {
      // 06:20:00 UTC = 11:50:00 IST
      expect(formatHeroTimestamp('2026-10-25T06:20:00.000Z')).toBe('Taken @ 25 Oct 2026, 11:50 AM')
    })

    it('formats evening PM hours and single digit dates accurately', () => {
      const evening = new Date('2026-10-05T20:15:00+05:30')
      expect(formatHeroTimestamp(evening)).toBe('Taken @ 5 Oct 2026, 8:15 PM')
    })

    it('defaults gracefully to current time if null or undefined', () => {
      const res = formatHeroTimestamp(null)
      expect(res).toMatch(/^Taken @ \d{1,2} [A-Z][a-z]{2} \d{4}, \d{1,2}:\d{2} (AM|PM)$/)
    })
  })
})
