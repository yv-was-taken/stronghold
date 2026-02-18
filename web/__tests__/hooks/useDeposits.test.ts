import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDeposits } from '@/lib/hooks/useDeposits'

const mockDeposits = [
  {
    id: '1',
    amount_usdc: '100000000',
    fee_usdc: '3000000',
    net_amount_usdc: '97000000',
    provider: 'stripe' as const,
    status: 'completed' as const,
    created_at: '2024-01-15T12:00:00Z',
    completed_at: '2024-01-15T12:01:00Z',
  },
  {
    id: '2',
    amount_usdc: '50000000',
    fee_usdc: '0',
    net_amount_usdc: '50000000',
    provider: 'direct' as const,
    status: 'pending' as const,
    created_at: '2024-01-14T10:00:00Z',
  },
]

describe('useDeposits', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('starts with empty data and not loading', () => {
    global.fetch = vi.fn()

    const { result } = renderHook(() => useDeposits())

    expect(result.current.data).toEqual([])
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeNull()
    expect(result.current.hasMore).toBe(true)
  })

  it('fetches deposits successfully', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ deposits: mockDeposits }),
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.data).toEqual(mockDeposits)
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeNull()
  })

  it('maps legacy net_usdc to net_amount_usdc', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        deposits: [{
          id: 'legacy-1',
          amount_usdc: '1000000',
          fee_usdc: '0',
          net_usdc: '1000000',
          provider: 'direct',
          status: 'completed',
          created_at: '2024-01-15T12:00:00Z',
        }],
      }),
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.data[0].net_amount_usdc).toBe('1000000')
  })

  it('handles fetch error', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.data).toEqual([])
    expect(result.current.error).toBe('Failed to fetch deposits')
  })

  it('handles network error', async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.error).toBe('Network error')
  })

  it('appends data when loading more', async () => {
    const firstPage = [mockDeposits[0]]
    const secondPage = [mockDeposits[1]]

    let callCount = 0
    global.fetch = vi.fn().mockImplementation(() => {
      callCount++
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
          deposits: callCount === 1 ? firstPage : secondPage,
        }),
      })
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.data).toHaveLength(1)

    await act(async () => {
      await result.current.fetchDeposits(20, 1, true)
    })

    expect(result.current.data).toHaveLength(2)
  })

  it('sets hasMore to false when fewer results than limit', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ deposits: [mockDeposits[0]] }),
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.hasMore).toBe(false)
  })

  it('loadMore does not fetch when already loading', async () => {
    global.fetch = vi.fn().mockImplementation(() =>
      new Promise((resolve) => setTimeout(() => resolve({
        ok: true,
        json: () => Promise.resolve({ deposits: mockDeposits }),
      }), 100))
    )

    const { result } = renderHook(() => useDeposits())

    // Start initial fetch
    act(() => {
      result.current.fetchDeposits(20, 0)
    })

    // Try to load more while still loading
    await act(async () => {
      await result.current.loadMore()
    })

    // Should only have been called once
    expect(global.fetch).toHaveBeenCalledTimes(1)
  })

  it('loadMore does not fetch when hasMore is false', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ deposits: [] }),
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    // hasMore should be false since we got 0 results
    expect(result.current.hasMore).toBe(false)

    // Clear the mock
    vi.clearAllMocks()

    // Try to load more
    await act(async () => {
      await result.current.loadMore()
    })

    // Should not have made another request
    expect(global.fetch).not.toHaveBeenCalled()
  })

  it('calls correct API endpoint with params', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ deposits: [] }),
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(15, 10)
    })

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/v1/account/deposits?limit=15&offset=10'),
      expect.objectContaining({ credentials: 'include' })
    )
  })

  it('refetch resets data', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ deposits: mockDeposits }),
    })

    const { result } = renderHook(() => useDeposits())

    await act(async () => {
      await result.current.fetchDeposits(20, 0)
    })

    expect(result.current.data).toHaveLength(2)

    // Refetch should reset
    await act(async () => {
      await result.current.refetch()
    })

    // Data should still be the same (fresh fetch)
    expect(result.current.data).toHaveLength(2)
    expect(global.fetch).toHaveBeenCalledTimes(2)
  })
})
