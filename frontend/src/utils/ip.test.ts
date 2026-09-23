import { describe, expect, it } from 'vitest'
import { isValidIP, isValidIPv4, isValidIPv6 } from './ip'

describe('ip validation utils', () => {
  describe('isValidIPv4', () => {
    it('accepts valid standard IPv4 addresses', () => {
      expect(isValidIPv4('127.0.0.1')).toBe(true)
      expect(isValidIPv4('192.168.1.1')).toBe(true)
      expect(isValidIPv4('10.0.0.0')).toBe(true)
      expect(isValidIPv4('255.255.255.255')).toBe(true)
      expect(isValidIPv4('0.0.0.0')).toBe(true)
      expect(isValidIPv4(' 192.168.1.100 ')).toBe(true)
    })

    it('rejects invalid IPv4 addresses', () => {
      expect(isValidIPv4('256.0.0.1')).toBe(false)
      expect(isValidIPv4('192.168.1')).toBe(false)
      expect(isValidIPv4('192.168.1.1.1')).toBe(false)
      expect(isValidIPv4('192.168.01.1')).toBe(false)
      expect(isValidIPv4('192.168.1.1:8080')).toBe(false)
      expect(isValidIPv4('192.168.1.1/24')).toBe(false)
      expect(isValidIPv4('abc.def.ghi.jkl')).toBe(false)
      expect(isValidIPv4('')).toBe(false)
    })
  })

  describe('isValidIPv6', () => {
    it('accepts valid standard and compressed IPv6 addresses', () => {
      expect(isValidIPv6('::1')).toBe(true)
      expect(isValidIPv6('::')).toBe(true)
      expect(isValidIPv6('2001:0db8:85a3:0000:0000:8a2e:0370:7334')).toBe(true)
      expect(isValidIPv6('2001:db8:85a3::8a2e:370:7334')).toBe(true)
      expect(isValidIPv6('fe80::1')).toBe(true)
      expect(isValidIPv6('fe80::')).toBe(true)
      expect(isValidIPv6('::ffff:192.168.1.1')).toBe(true)
      expect(isValidIPv6(' 2001:db8::1 ')).toBe(true)
    })

    it('rejects invalid IPv6 addresses', () => {
      expect(isValidIPv6('2001:db8:::1')).toBe(false)
      expect(isValidIPv6('2001:db8::1::2')).toBe(false)
      expect(isValidIPv6('2001:0db8:85a3:0000:0000:8a2e:0370:7334:extra')).toBe(false)
      expect(isValidIPv6(':2001:db8::1')).toBe(false)
      expect(isValidIPv6('2001:db8::1:')).toBe(false)
      expect(isValidIPv6('2001:xyz::1')).toBe(false)
      expect(isValidIPv6('127.0.0.1')).toBe(false)
      expect(isValidIPv6('')).toBe(false)
    })
  })

  describe('isValidIP', () => {
    it('returns true for either IPv4 or IPv6', () => {
      expect(isValidIP('192.168.1.1')).toBe(true)
      expect(isValidIP('::1')).toBe(true)
      expect(isValidIP('2001:db8::1')).toBe(true)
    })

    it('returns false for invalid addresses', () => {
      expect(isValidIP('')).toBe(false)
      expect(isValidIP('not an ip')).toBe(false)
      expect(isValidIP('999.999.999.999')).toBe(false)
      expect(isValidIP('192.168.1.1:8080')).toBe(false)
    })
  })
})
