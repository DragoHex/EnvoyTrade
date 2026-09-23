/**
 * Validates whether the given string is a valid IPv4 address in dotted-decimal format.
 * Rejects leading zeros (e.g. 01.02.03.04) and port numbers.
 */
export function isValidIPv4(ip: string): boolean {
  const trimmed = ip.trim()
  const parts = trimmed.split('.')
  if (parts.length !== 4) return false
  for (const part of parts) {
    if (!/^\d+$/.test(part)) return false
    if (part.length > 1 && part.startsWith('0')) return false
    const num = Number(part)
    if (num < 0 || num > 255) return false
  }
  return true
}

/**
 * Validates whether the given string is a valid IPv6 address in standard or compressed format.
 */
export function isValidIPv6(ip: string): boolean {
  const trimmed = ip.trim()
  if (!trimmed || trimmed.includes(':::')) return false

  const doubleColonMatches = trimmed.match(/::/g)
  const doubleColonCount = doubleColonMatches ? doubleColonMatches.length : 0
  if (doubleColonCount > 1) return false

  // Handle optional embedded IPv4 (e.g. ::ffff:192.168.1.1)
  let checkStr = trimmed
  const lastColon = checkStr.lastIndexOf(':')
  if (lastColon !== -1) {
    const possibleIpv4 = checkStr.slice(lastColon + 1)
    if (possibleIpv4.includes('.')) {
      if (!isValidIPv4(possibleIpv4)) return false
      // Replace the IPv4 segment with two 16-bit hex words for group counting
      checkStr = checkStr.slice(0, lastColon) + ':0:0'
    }
  }

  const hasDoubleColon = doubleColonCount === 1

  // If starts or ends with single colon, invalid
  if (checkStr.startsWith(':') && !checkStr.startsWith('::')) return false
  if (checkStr.endsWith(':') && !checkStr.endsWith('::')) return false

  const parts = checkStr.split(':')
  const filtered = parts.filter((p) => p.length > 0)

  if (hasDoubleColon) {
    if (filtered.length >= 8) return false
  } else {
    if (filtered.length !== 8) return false
  }

  for (const part of filtered) {
    if (!/^[0-9a-fA-F]{1,4}$/.test(part)) return false
  }

  return true
}

/**
 * Checks if the given string is either a valid IPv4 or valid IPv6 address.
 */
export function isValidIP(ip: string): boolean {
  const trimmed = ip.trim()
  if (!trimmed) return false
  return isValidIPv4(trimmed) || isValidIPv6(trimmed)
}
