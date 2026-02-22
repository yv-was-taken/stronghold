import { describe, it, expect, vi, beforeEach, afterEach, type MockInstance } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ErrorBoundary, withErrorBoundary } from '@/components/ui/ErrorBoundary'

// Component that throws an error
function ThrowError({ shouldThrow = true }: { shouldThrow?: boolean }) {
  if (shouldThrow) {
    throw new Error('Test error message')
  }
  return <div>No error</div>
}

// Component that can be toggled to throw
function ToggleableError({ error }: { error: boolean }) {
  if (error) {
    throw new Error('Toggled error')
  }
  return <div>Content rendered</div>
}

describe('ErrorBoundary', () => {
  let consoleSpy: MockInstance

  beforeEach(() => {
    // Suppress console.error for cleaner test output
    consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleSpy.mockRestore()
  })

  it('renders children when no error occurs', () => {
    render(
      <ErrorBoundary>
        <div>Test content</div>
      </ErrorBoundary>
    )
    expect(screen.getByText('Test content')).toBeInTheDocument()
  })

  it('renders error UI when child throws', () => {
    render(
      <ErrorBoundary>
        <ThrowError />
      </ErrorBoundary>
    )
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
    expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument()
  })

  it('displays the error message', () => {
    render(
      <ErrorBoundary>
        <ThrowError />
      </ErrorBoundary>
    )
    expect(screen.getByText('Test error message')).toBeInTheDocument()
  })

  it('shows retry button', () => {
    render(
      <ErrorBoundary>
        <ThrowError />
      </ErrorBoundary>
    )
    expect(screen.getByRole('button', { name: /try again/i })).toBeInTheDocument()
  })

  it('resets error state when retry is clicked', () => {
    let shouldThrow = true

    function ConditionalThrow() {
      if (shouldThrow) {
        throw new Error('Error')
      }
      return <div>Recovered</div>
    }

    const { rerender } = render(
      <ErrorBoundary>
        <ConditionalThrow />
      </ErrorBoundary>
    )

    // Should show error UI
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()

    // Fix the error condition
    shouldThrow = false

    // Click retry
    fireEvent.click(screen.getByRole('button', { name: /try again/i }))

    // Force rerender to pick up the fixed component
    rerender(
      <ErrorBoundary>
        <ConditionalThrow />
      </ErrorBoundary>
    )

    // Should now show recovered content
    expect(screen.getByText('Recovered')).toBeInTheDocument()
  })

  it('renders custom fallback when provided', () => {
    render(
      <ErrorBoundary fallback={<div>Custom error UI</div>}>
        <ThrowError />
      </ErrorBoundary>
    )
    expect(screen.getByText('Custom error UI')).toBeInTheDocument()
    expect(screen.queryByText('Something went wrong')).not.toBeInTheDocument()
  })

  it('logs error to console', () => {
    render(
      <ErrorBoundary>
        <ThrowError />
      </ErrorBoundary>
    )
    expect(consoleSpy).toHaveBeenCalled()
  })
})

describe('withErrorBoundary HOC', () => {
  let consoleSpy: MockInstance

  beforeEach(() => {
    consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    consoleSpy.mockRestore()
  })

  it('wraps component with error boundary', () => {
    function MyComponent() {
      return <div>My content</div>
    }
    const WrappedComponent = withErrorBoundary(MyComponent)

    render(<WrappedComponent />)
    expect(screen.getByText('My content')).toBeInTheDocument()
  })

  it('catches errors from wrapped component', () => {
    const WrappedThrowError = withErrorBoundary(ThrowError)

    render(<WrappedThrowError />)
    expect(screen.getByText('Something went wrong')).toBeInTheDocument()
  })

  it('uses custom fallback when provided', () => {
    const WrappedThrowError = withErrorBoundary(
      ThrowError,
      <div>Custom fallback</div>
    )

    render(<WrappedThrowError />)
    expect(screen.getByText('Custom fallback')).toBeInTheDocument()
  })

  it('passes props through to wrapped component', () => {
    function PropsComponent({ message }: { message: string }) {
      return <div>{message}</div>
    }
    const WrappedComponent = withErrorBoundary(PropsComponent)

    render(<WrappedComponent message="Hello props" />)
    expect(screen.getByText('Hello props')).toBeInTheDocument()
  })
})
