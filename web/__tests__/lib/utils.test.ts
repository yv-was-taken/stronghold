import { describe, it, expect } from 'vitest'
import {
  formatAccountNumber,
  isValidAccountNumber,
  formatUSDC,
  truncateAddress,
} from '@/lib/utils'

describe('formatAccountNumber', () => {
  it('formats digits with dashes', () => {
    expect(formatAccountNumber('1234567890123456')).toBe('1234-5678-9012-3456')
  })

  it('handles partial input', () => {
    expect(formatAccountNumber('1234')).toBe('1234')
    expect(formatAccountNumber('12345678')).toBe('1234-5678')
    expect(formatAccountNumber('123456789012')).toBe('1234-5678-9012')
  })

  it('removes non-digit characters', () => {
    expect(formatAccountNumber('1234-5678-9012-3456')).toBe('1234-5678-9012-3456')
    expect(formatAccountNumber('1234 5678 9012 3456')).toBe('1234-5678-9012-3456')
    expect(formatAccountNumber('abcd1234efgh5678')).toBe('1234-5678')
  })

  it('limits to 16 digits', () => {
    expect(formatAccountNumber('12345678901234567890')).toBe('1234-5678-9012-3456')
  })

  it('handles empty input', () => {
    expect(formatAccountNumber('')).toBe('')
  })
})

describe('isValidAccountNumber', () => {
  it('returns true for 16-digit account number', () => {
    expect(isValidAccountNumber('1234-5678-9012-3456')).toBe(true)
    expect(isValidAccountNumber('1234567890123456')).toBe(true)
  })

  it('returns false for invalid account numbers', () => {
    expect(isValidAccountNumber('')).toBe(false)
    expect(isValidAccountNumber('1234')).toBe(false)
    expect(isValidAccountNumber('1234-5678-9012')).toBe(false)
    expect(isValidAccountNumber('1234-5678-9012-345')).toBe(false)
  })

  it('ignores non-digit characters when validating', () => {
    expect(isValidAccountNumber('1234-5678-9012-3456')).toBe(true)
    expect(isValidAccountNumber('1234 5678 9012 3456')).toBe(true)
  })
})

describe('formatUSDC', () => {
  it('formats with minimum 2 decimal places', () => {
    expect(formatUSDC(100)).toBe('100.00')
    expect(formatUSDC(0)).toBe('0.00')
  })

  it('handles decimal values', () => {
    expect(formatUSDC(100.5)).toBe('100.50')
    expect(formatUSDC(100.123456)).toBe('100.123456')
  })

  it('adds thousand separators', () => {
    expect(formatUSDC(1000)).toBe('1,000.00')
    expect(formatUSDC(1000000)).toBe('1,000,000.00')
  })

  it('limits to 6 decimal places', () => {
    expect(formatUSDC(0.12345678)).toBe('0.123457')
  })
})

describe('truncateAddress', () => {
  it('truncates long addresses', () => {
    const address = '0x1234567890abcdef1234567890abcdef12345678'
    expect(truncateAddress(address)).toBe('0x1234...5678')
  })

  it('uses custom start and end lengths', () => {
    const address = '0x1234567890abcdef1234567890abcdef12345678'
    expect(truncateAddress(address, 10, 6)).toBe('0x12345678...345678')
  })

  it('returns full address if short enough', () => {
    expect(truncateAddress('0x1234')).toBe('0x1234')
    expect(truncateAddress('0x12345678')).toBe('0x12345678')
  })
})
