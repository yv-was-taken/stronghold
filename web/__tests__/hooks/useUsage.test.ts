import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useUsageLogs, useUsageStats } from '@/lib/hooks/useUsage'

const mockUsageLogs = [
  {
    id: '1',
    endpoint: '/v1/scan/content',
    cost_usdc: 0.001,
    status: 'success' as const,
    threat_detected: false,
    latency_ms: 45,
    created_at: '2024-01-15T12:00:00Z',
  },
  {
    id: '2',
    endpoint: '/v1/scan/output',
    cost_usdc: 0.001,
    status: 'success' as const,
    threat_detected: true,
    latency_ms: 52,
    created_at: '2024-01-15T11:00:00Z',
  },
]

const mockUsageStats = {
  total_requests: 1500,
  total_cost_usdc: 3.5,
  threats_detected: 42,
  avg_latency_ms: 48.5,
  period_days: 30,
}

describe('useUsageLogs', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('starts with empty data and not loading', () => {
    global.fetch = vi.fn()

    const { result } = renderHook(() => useUsageLogs())

    expect(result.current.data).toEqual([])
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeNull()
  })

  it('fetches usage logs successfully', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ usage: mockUsageLogs }),
    })

    const { result } = renderHook(() => useUsageLogs())

    await act(async () => {
      await result.current.fetchLogs(20, 0)
    })

    expect(result.current.data).toEqual(mockUsageLogs)
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeNull()
  })

  it('handles fetch error', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    })

    const { result } = renderHook(() => useUsageLogs())

    await act(async () => {
      await result.current.fetchLogs(20, 0)
    })

    expect(result.current.data).toEqual([])
    expect(result.current.error).toBe('Failed to fetch usage logs')
  })

  it('appends data when loading more', async () => {
    const firstPage = [mockUsageLogs[0]]
    const secondPage = [mockUsageLogs[1]]

    let callCount = 0
    global.fetch = vi.fn().mockImplementation(() => {
      callCount++
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({
          usage: callCount === 1 ? firstPage : secondPage,
        }),
      })
    })

    const { result } = renderHook(() => useUsageLogs())

    // First fetch
    await act(async () => {
      await result.current.fetchLogs(20, 0)
    })

    expect(result.current.data).toHaveLength(1)

    // Load more (append)
    await act(async () => {
      await result.current.fetchLogs(20, 1, true)
    })

    expect(result.current.data).toHaveLength(2)
  })

  it('sets hasMore to false when fewer results than limit', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ usage: [mockUsageLogs[0]] }),
    })

    const { result } = renderHook(() => useUsageLogs())

    await act(async () => {
      await result.current.fetchLogs(20, 0)
    })

    expect(result.current.hasMore).toBe(false)
  })

  it('calls correct API endpoint with params', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ usage: [] }),
    })

    const { result } = renderHook(() => useUsageLogs())

    await act(async () => {
      await result.current.fetchLogs(10, 5)
    })

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/v1/account/usage?limit=10&offset=5'),
      expect.any(Object)
    )
  })
})

describe('useUsageStats', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('starts with null data', () => {
    global.fetch = vi.fn()

    const { result } = renderHook(() => useUsageStats())

    expect(result.current.data).toBeNull()
    expect(result.current.loading).toBe(false)
  })

  it('fetches stats successfully', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockUsageStats),
    })

    const { result } = renderHook(() => useUsageStats())

    await act(async () => {
      await result.current.fetchStats(30)
    })

    expect(result.current.data).toEqual(mockUsageStats)
    expect(result.current.loading).toBe(false)
  })

  it('handles fetch error', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    })

    const { result } = renderHook(() => useUsageStats())

    await act(async () => {
      await result.current.fetchStats(30)
    })

    expect(result.current.data).toBeNull()
    expect(result.current.error).toBe('Failed to fetch usage stats')
  })

  it('calls correct API endpoint with days param', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockUsageStats),
    })

    const { result } = renderHook(() => useUsageStats())

    await act(async () => {
      await result.current.fetchStats(7)
    })

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/v1/account/usage/stats?days=7'),
      expect.any(Object)
    )
  })
})
