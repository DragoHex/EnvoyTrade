import { describe, it, expect } from 'vitest'
import { formatISTDateTime, formatISTTime } from './time'

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
})
