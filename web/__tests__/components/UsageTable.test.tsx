import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { UsageTable } from '@/components/dashboard/UsageTable'
import type { UsageLog } from '@/lib/hooks/useUsage'

const mockLogs: UsageLog[] = [
  {
    id: '1',
    endpoint: '/v1/scan/content',
    cost_usdc: 0.001,
    status: 'success',
    threat_detected: false,
    latency_ms: 45,
    created_at: new Date().toISOString(),
  },
  {
    id: '2',
    endpoint: '/v1/scan/output',
    cost_usdc: 0.001,
    status: 'error',
    threat_detected: true,
    latency_ms: 52,
    created_at: new Date(Date.now() - 3600000).toISOString(), // 1 hour ago
  },
]

describe('UsageTable', () => {
  it('renders skeleton rows when loading with no data', () => {
    const { container } = render(
      <UsageTable logs={[]} loading={true} hasMore={true} onLoadMore={() => {}} />
    )

    // Should have skeleton rows
    const skeletonRows = container.querySelectorAll('.animate-pulse')
    expect(skeletonRows.length).toBeGreaterThan(0)

    // Should have table headers
    expect(screen.getByText('Time')).toBeInTheDocument()
    expect(screen.getByText('Endpoint')).toBeInTheDocument()
    expect(screen.getByText('Cost')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
    expect(screen.getByText('Threat')).toBeInTheDocument()
  })

  it('renders empty state when no logs and not loading', () => {
    render(
      <UsageTable logs={[]} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    expect(screen.getByText('No usage data yet')).toBeInTheDocument()
    expect(
      screen.getByText('Your API usage will appear here once you start making requests.')
    ).toBeInTheDocument()
  })

  it('renders usage logs correctly', () => {
    render(
      <UsageTable logs={mockLogs} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    // Endpoint labels
    expect(screen.getByText('Content Scan')).toBeInTheDocument()
    expect(screen.getByText('Output Scan')).toBeInTheDocument()

    // Status badges
    expect(screen.getByText('Success')).toBeInTheDocument()
    expect(screen.getByText('Error')).toBeInTheDocument()

    // Threat indicator
    expect(screen.getByText('Detected')).toBeInTheDocument()
  })

  it('shows Load More button when hasMore is true', () => {
    render(
      <UsageTable logs={mockLogs} loading={false} hasMore={true} onLoadMore={() => {}} />
    )

    expect(screen.getByRole('button', { name: /load more/i })).toBeInTheDocument()
  })

  it('hides Load More button when hasMore is false', () => {
    render(
      <UsageTable logs={mockLogs} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument()
  })

  it('calls onLoadMore when Load More is clicked', () => {
    const handleLoadMore = vi.fn()
    render(
      <UsageTable logs={mockLogs} loading={false} hasMore={true} onLoadMore={handleLoadMore} />
    )

    fireEvent.click(screen.getByRole('button', { name: /load more/i }))
    expect(handleLoadMore).toHaveBeenCalledTimes(1)
  })

  it('shows loading text on button when loading more', () => {
    render(
      <UsageTable logs={mockLogs} loading={true} hasMore={true} onLoadMore={() => {}} />
    )

    expect(screen.getByRole('button', { name: /loading/i })).toBeInTheDocument()
  })

  it('disables Load More button when loading', () => {
    render(
      <UsageTable logs={mockLogs} loading={true} hasMore={true} onLoadMore={() => {}} />
    )

    expect(screen.getByRole('button', { name: /loading/i })).toBeDisabled()
  })

  it('formats cost with dollar sign', () => {
    render(
      <UsageTable logs={mockLogs} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    // Cost should be formatted
    const costCells = screen.getAllByText(/\$0\.001/)
    expect(costCells.length).toBeGreaterThan(0)
  })

  it('shows dash for logs without threat detected', () => {
    render(
      <UsageTable logs={[mockLogs[0]]} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    // First log has no threat
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
