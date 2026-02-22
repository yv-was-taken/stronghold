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
  it('formats whole USDC amounts with minimum 2 decimal places', () => {
    expect(formatUSDC("1000000")).toBe('$1.00')      // 1 USDC
    expect(formatUSDC("100000000")).toBe('$100.00')   // 100 USDC
    expect(formatUSDC("1000000000000")).toBe('$1,000,000.00') // 1M USDC
    expect(formatUSDC("0")).toBe('$0.00')
  })

  it('formats fractional USDC amounts', () => {
    expect(formatUSDC("1250000")).toBe('$1.25')       // 1.25 USDC
    expect(formatUSDC("1500000")).toBe('$1.50')       // 1.50 USDC
    expect(formatUSDC("123456789")).toBe('$123.456789') // preserves all significant digits
  })

  it('formats sub-cent amounts correctly', () => {
    expect(formatUSDC("1000")).toBe('$0.001')         // 0.001 USDC
    expect(formatUSDC("100")).toBe('$0.0001')         // 0.0001 USDC
    expect(formatUSDC("1")).toBe('$0.000001')         // smallest unit
  })

  it('handles edge cases', () => {
    expect(formatUSDC("")).toBe('$0.00')
    expect(formatUSDC(undefined as unknown as string)).toBe('$0.00')
    expect(formatUSDC("not-a-number")).toBe('$0.00')
  })

  it('trims trailing zeros but keeps at least 2 decimal places', () => {
    expect(formatUSDC("3500000")).toBe('$3.50')       // keeps the trailing 0 for 2 places
    expect(formatUSDC("10000000")).toBe('$10.00')     // 10 USDC
  })

  it('formats negative amounts correctly', () => {
    expect(formatUSDC("-1250000")).toBe('-$1.25')
    expect(formatUSDC("-1")).toBe('-$0.000001')
  })

  it('preserves precision for values beyond Number.MAX_SAFE_INTEGER', () => {
    expect(formatUSDC("9007199254740993")).toBe('$9,007,199,254.740993')
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
