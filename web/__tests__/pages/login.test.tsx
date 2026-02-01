import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import LoginPage from '@/app/dashboard/login/page'
import { AuthProvider } from '@/components/providers/AuthProvider'
import { mockAccount } from '../test-utils'

// Mock next/navigation
const mockPush = vi.fn()
const mockReplace = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
    prefetch: vi.fn(),
  }),
  usePathname: () => '/dashboard/login',
}))

describe('LoginPage', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
    mockPush.mockClear()
    mockReplace.mockClear()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('renders login form', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByText('Welcome back')).toBeInTheDocument()
    })

    expect(screen.getByLabelText(/account number/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument()
    expect(screen.getByText(/create one/i)).toBeInTheDocument()
  })

  it('formats account number input with dashes', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByLabelText(/account number/i)).toBeInTheDocument()
    })

    const input = screen.getByLabelText(/account number/i)
    await user.type(input, '1234567890123456')

    expect(input).toHaveValue('1234-5678-9012-3456')
  })

  it('disables submit button for invalid account number', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument()
    })

    const submitButton = screen.getByRole('button', { name: /login/i })
    expect(submitButton).toBeDisabled()

    // Type partial account number
    const input = screen.getByLabelText(/account number/i)
    await user.type(input, '1234567890')

    expect(submitButton).toBeDisabled()
  })

  it('enables submit button for valid account number', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByLabelText(/account number/i)).toBeInTheDocument()
    })

    const input = screen.getByLabelText(/account number/i)
    await user.type(input, '1234567890123456')

    const submitButton = screen.getByRole('button', { name: /login/i })
    expect(submitButton).not.toBeDisabled()
  })

  it('shows error for invalid account number on submit', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByLabelText(/account number/i)).toBeInTheDocument()
    })

    const input = screen.getByLabelText(/account number/i)
    await user.type(input, '123456789012345') // 15 digits - invalid

    // Force enable button for test
    const form = input.closest('form')!
    await act(async () => {
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })

    await waitFor(() => {
      expect(screen.getByText(/valid 16-digit account number/i)).toBeInTheDocument()
    })
  })

  it('redirects to dashboard on successful login', async () => {
    const user = userEvent.setup()

    // Track calls to auth/me to return 401 initially, then success after login
    let loginCalled = false

    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.includes('/login')) {
        loginCalled = true
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ message: 'Login successful' }),
        })
      }
      if (url.includes('/auth/me')) {
        // Return 401 until login is called
        if (!loginCalled) {
          return Promise.resolve({
            ok: false,
            status: 401,
            json: () => Promise.resolve({ error: 'Unauthorized' }),
          })
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(mockAccount),
        })
      }
      return Promise.resolve({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      })
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByLabelText(/account number/i)).toBeInTheDocument()
    })

    const input = screen.getByLabelText(/account number/i)
    await user.type(input, '1234567890123456')

    const submitButton = screen.getByRole('button', { name: /login/i })
    await user.click(submitButton)

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/dashboard/main')
    })
  })

  it('shows error message on login failure', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.includes('/login')) {
        return Promise.resolve({
          ok: false,
          status: 401,
          json: () => Promise.resolve({ error: 'Invalid account number' }),
        })
      }
      return Promise.resolve({
        ok: false,
        status: 401,
        json: () => Promise.resolve({ error: 'Unauthorized' }),
      })
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByLabelText(/account number/i)).toBeInTheDocument()
    })

    const input = screen.getByLabelText(/account number/i)
    await user.type(input, '1234567890123456')

    const submitButton = screen.getByRole('button', { name: /login/i })
    await user.click(submitButton)

    await waitFor(() => {
      expect(screen.getByText(/invalid account number/i)).toBeInTheDocument()
    })
  })

  it('redirects authenticated users to dashboard', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockAccount),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith('/dashboard/main')
    })
  })

  it('shows loading state while checking auth', async () => {
    // Make fetch hang to show loading state
    global.fetch = vi.fn().mockImplementation(
      () => new Promise(() => {}) // Never resolves
    )

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    expect(screen.getByText('Loading...')).toBeInTheDocument()
  })

  it('has link to create account page', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByText(/create one/i)).toBeInTheDocument()
    })

    const createLink = screen.getByText(/create one/i)
    expect(createLink).toHaveAttribute('href', '/dashboard/create')
  })

  it('shows security note about account number', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByText(/your account number is your password/i)).toBeInTheDocument()
    })
  })
})
