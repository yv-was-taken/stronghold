import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { DepositsTable } from '@/components/dashboard/DepositsTable'
import type { Deposit } from '@/lib/hooks/useDeposits'

const mockDeposits: Deposit[] = [
  {
    id: '1',
    amount_usdc: '100000000',
    fee_usdc: '3000000',
    net_amount_usdc: '97000000',
    provider: 'stripe',
    status: 'completed',
    created_at: '2024-01-15T12:00:00Z',
    completed_at: '2024-01-15T12:01:00Z',
  },
  {
    id: '2',
    amount_usdc: '50000000',
    fee_usdc: '0',
    net_amount_usdc: '50000000',
    provider: 'direct',
    status: 'pending',
    created_at: '2024-01-14T10:00:00Z',
  },
  {
    id: '3',
    amount_usdc: '25000000',
    fee_usdc: '1000000',
    net_amount_usdc: '24000000',
    provider: 'stripe',
    status: 'failed',
    created_at: '2024-01-13T08:00:00Z',
  },
]

describe('DepositsTable', () => {
  it('renders skeleton rows when loading with no data', () => {
    const { container } = render(
      <DepositsTable deposits={[]} loading={true} hasMore={true} onLoadMore={() => {}} />
    )

    const skeletonRows = container.querySelectorAll('.animate-pulse')
    expect(skeletonRows.length).toBeGreaterThan(0)

    // Should have table headers
    expect(screen.getByText('Date')).toBeInTheDocument()
    expect(screen.getByText('Amount')).toBeInTheDocument()
    expect(screen.getByText('Fee')).toBeInTheDocument()
    expect(screen.getByText('Net')).toBeInTheDocument()
    expect(screen.getByText('Provider')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
  })

  it('renders empty state when no deposits and not loading', () => {
    render(
      <DepositsTable deposits={[]} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    expect(screen.getByText('No deposits yet')).toBeInTheDocument()
    expect(
      screen.getByText('Add funds to your account to start using Stronghold.')
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /add funds/i })).toHaveAttribute(
      'href',
      '/dashboard/main/deposit'
    )
  })

  it('renders deposits correctly', () => {
    render(
      <DepositsTable deposits={mockDeposits} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    // Amounts
    expect(screen.getByText('$100.00')).toBeInTheDocument()
    expect(screen.getByText('$50.00')).toBeInTheDocument()

    // Net amounts
    expect(screen.getByText('+$97.00')).toBeInTheDocument()
    expect(screen.getByText('+$50.00')).toBeInTheDocument()

    // Provider labels
    expect(screen.getAllByText('Card')).toHaveLength(2)
    expect(screen.getByText('Crypto')).toBeInTheDocument()

    // Status badges
    expect(screen.getByText('Completed')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
  })

  it('shows dash for zero fees', () => {
    render(
      <DepositsTable
        deposits={[mockDeposits[1]]}
        loading={false}
        hasMore={false}
        onLoadMore={() => {}}
      />
    )

    // The direct deposit has 0 fee
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('shows negative sign for fees', () => {
    render(
      <DepositsTable
        deposits={[mockDeposits[0]]}
        loading={false}
        hasMore={false}
        onLoadMore={() => {}}
      />
    )

    expect(screen.getByText('-$3.00')).toBeInTheDocument()
  })

  it('shows Load More button when hasMore is true', () => {
    render(
      <DepositsTable deposits={mockDeposits} loading={false} hasMore={true} onLoadMore={() => {}} />
    )

    expect(screen.getByRole('button', { name: /load more/i })).toBeInTheDocument()
  })

  it('hides Load More button when hasMore is false', () => {
    render(
      <DepositsTable deposits={mockDeposits} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    expect(screen.queryByRole('button', { name: /load more/i })).not.toBeInTheDocument()
  })

  it('calls onLoadMore when Load More is clicked', () => {
    const handleLoadMore = vi.fn()
    render(
      <DepositsTable
        deposits={mockDeposits}
        loading={false}
        hasMore={true}
        onLoadMore={handleLoadMore}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /load more/i }))
    expect(handleLoadMore).toHaveBeenCalledTimes(1)
  })

  it('disables Load More button when loading', () => {
    render(
      <DepositsTable deposits={mockDeposits} loading={true} hasMore={true} onLoadMore={() => {}} />
    )

    expect(screen.getByRole('button', { name: /loading/i })).toBeDisabled()
  })

  it('applies correct status colors', () => {
    render(
      <DepositsTable deposits={mockDeposits} loading={false} hasMore={false} onLoadMore={() => {}} />
    )

    // Completed = green
    const completedBadge = screen.getByText('Completed').closest('span')
    expect(completedBadge).toHaveClass('text-green-400')

    // Pending = yellow
    const pendingBadge = screen.getByText('Pending').closest('span')
    expect(pendingBadge).toHaveClass('text-yellow-400')

    // Failed = red
    const failedBadge = screen.getByText('Failed').closest('span')
    expect(failedBadge).toHaveClass('text-red-400')
  })
})
