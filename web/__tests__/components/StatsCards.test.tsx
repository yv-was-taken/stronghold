import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StatsCards } from '@/components/dashboard/StatsCards'
import type { UsageStats } from '@/lib/hooks/useUsage'

const mockStats: UsageStats = {
  total_requests: 1500,
  total_cost_usdc: '3500000',
  threats_detected: 42,
  avg_latency_ms: 48.5,
  period_days: 30,
}

describe('StatsCards', () => {
  it('renders skeleton cards when loading', () => {
    const { container } = render(<StatsCards stats={null} loading={true} />)

    // Should render 4 skeleton cards
    const skeletonCards = container.querySelectorAll('.animate-pulse')
    expect(skeletonCards.length).toBeGreaterThan(0)

    // Should not show actual stats
    expect(screen.queryByText('1,500')).not.toBeInTheDocument()
  })

  it('renders placeholder cards when stats is null and not loading', () => {
    render(<StatsCards stats={null} loading={false} />)

    // Should show placeholder values
    expect(screen.getAllByText('--')).toHaveLength(4)

    // Should show labels
    expect(screen.getByText('Total Requests')).toBeInTheDocument()
    expect(screen.getByText('Total Cost')).toBeInTheDocument()
    expect(screen.getByText('Threats Detected')).toBeInTheDocument()
    expect(screen.getByText('Avg Latency')).toBeInTheDocument()
  })

  it('renders stats data correctly', () => {
    render(<StatsCards stats={mockStats} loading={false} />)

    // Total requests
    expect(screen.getByText('1,500')).toBeInTheDocument()
    expect(screen.getByText('Total Requests')).toBeInTheDocument()

    // Total cost
    expect(screen.getByText('$3.50')).toBeInTheDocument()
    expect(screen.getByText('Total Cost')).toBeInTheDocument()

    // Threats detected
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('Threats Detected')).toBeInTheDocument()

    // Avg latency (rounded)
    expect(screen.getByText('49ms')).toBeInTheDocument()
    expect(screen.getByText('Avg Latency')).toBeInTheDocument()
  })

  it('shows period days in sublabel', () => {
    render(<StatsCards stats={mockStats} loading={false} />)
    expect(screen.getByText('Last 30 days')).toBeInTheDocument()
  })

  it('renders in a 4-column grid on large screens', () => {
    const { container } = render(<StatsCards stats={mockStats} loading={false} />)
    const grid = container.firstChild as HTMLElement
    expect(grid).toHaveClass('grid', 'lg:grid-cols-4')
  })

  it('formats large numbers with commas', () => {
    const largeStats: UsageStats = {
      ...mockStats,
      total_requests: 1234567,
    }
    render(<StatsCards stats={largeStats} loading={false} />)
    expect(screen.getByText('1,234,567')).toBeInTheDocument()
  })

  it('formats cost with proper decimals', () => {
    const preciseStats: UsageStats = {
      ...mockStats,
      total_cost_usdc: '123456789',
    }
    render(<StatsCards stats={preciseStats} loading={false} />)
    expect(screen.getByText('$123.456789')).toBeInTheDocument()
  })
})
