import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import Button from '@/components/ui/Button'

describe('Button', () => {
  it('renders with children text', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByText('Click me')).toBeInTheDocument()
  })

  it('renders as a button element by default', () => {
    render(<Button>Click</Button>)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('renders as an anchor element when href is provided', () => {
    render(<Button href="/test">Link</Button>)
    expect(screen.getByRole('link')).toHaveAttribute('href', '/test')
  })

  it('calls onClick handler when clicked', () => {
    const handleClick = vi.fn()
    render(<Button onClick={handleClick}>Click me</Button>)

    fireEvent.click(screen.getByRole('button'))
    expect(handleClick).toHaveBeenCalledTimes(1)
  })

  it('applies primary variant styles by default', () => {
    render(<Button>Primary</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('bg-stronghold-cyan')
  })

  it('applies secondary variant styles', () => {
    render(<Button variant="secondary">Secondary</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('bg-stronghold-stone')
  })

  it('applies outline variant styles', () => {
    render(<Button variant="outline">Outline</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('bg-transparent')
  })

  it('applies small size styles', () => {
    render(<Button size="sm">Small</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('px-4', 'py-2')
  })

  it('applies medium size styles by default', () => {
    render(<Button>Medium</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('px-6', 'py-3')
  })

  it('applies large size styles', () => {
    render(<Button size="lg">Large</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('px-8', 'py-4')
  })

  it('renders with an icon', () => {
    const TestIcon = () => <span data-testid="test-icon">★</span>
    render(<Button icon={<TestIcon />}>With Icon</Button>)

    expect(screen.getByTestId('test-icon')).toBeInTheDocument()
    expect(screen.getByText('With Icon')).toBeInTheDocument()
  })

  it('applies custom className', () => {
    render(<Button className="custom-class">Custom</Button>)
    const button = screen.getByRole('button')
    expect(button).toHaveClass('custom-class')
  })

  it('renders corner accents for styling', () => {
    const { container } = render(<Button>Styled</Button>)
    // Button should have corner accent spans
    const cornerAccents = container.querySelectorAll('span.absolute')
    expect(cornerAccents.length).toBeGreaterThan(0)
  })
})
