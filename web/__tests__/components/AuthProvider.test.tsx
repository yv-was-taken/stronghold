import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider, useAuth } from '@/components/providers/AuthProvider'
import { mockAccount } from '../test-utils'

// Test component that uses the auth context
function TestConsumer() {
  const { account, isLoading, isAuthenticated, login, logout, createAccount } = useAuth()

  return (
    <div>
      <div data-testid="loading">{isLoading ? 'loading' : 'ready'}</div>
      <div data-testid="authenticated">{isAuthenticated ? 'yes' : 'no'}</div>
      <div data-testid="account">{account?.account_number || 'none'}</div>
      <button onClick={() => login('1234-5678-9012-3456')}>Login</button>
      <button onClick={() => logout()}>Logout</button>
      <button onClick={() => createAccount()}>Create</button>
    </div>
  )
}

describe('AuthProvider', () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it('provides isAuthenticated as false initially when not logged in', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'Unauthorized' }),
    })

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    )

    // Wait for initial auth check to complete
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('ready')
    })

    expect(screen.getByTestId('authenticated')).toHaveTextContent('no')
    expect(screen.getByTestId('account')).toHaveTextContent('none')
  })

  it('provides account data when authenticated', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockAccount),
    })

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    )

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('ready')
    })

    expect(screen.getByTestId('authenticated')).toHaveTextContent('yes')
    expect(screen.getByTestId('account')).toHaveTextContent('1234-5678-9012-3456')
  })

  it('login updates authentication state', async () => {
    const user = userEvent.setup()

    // Initial auth check fails
    let callCount = 0
    global.fetch = vi.fn().mockImplementation((url: string) => {
      callCount++
      if (url.includes('/login')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ message: 'Login successful' }),
        })
      }
      if (url.includes('/auth/me')) {
        // First call returns 401, subsequent calls return account
        if (callCount <= 2) {
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
        status: 404,
        json: () => Promise.resolve({ error: 'Not found' }),
      })
    })

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    )

    // Wait for initial load
    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('ready')
    })

    expect(screen.getByTestId('authenticated')).toHaveTextContent('no')

    // Click login button
    await act(async () => {
      await user.click(screen.getByText('Login'))
    })

    // Should now be authenticated
    await waitFor(() => {
      expect(screen.getByTestId('authenticated')).toHaveTextContent('yes')
    })
  })

  it('logout clears account data', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.includes('/logout')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({}),
        })
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockAccount),
      })
    })

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    )

    // Wait for authenticated state
    await waitFor(() => {
      expect(screen.getByTestId('authenticated')).toHaveTextContent('yes')
    })

    // Click logout
    await act(async () => {
      await user.click(screen.getByText('Logout'))
    })

    expect(screen.getByTestId('authenticated')).toHaveTextContent('no')
    expect(screen.getByTestId('account')).toHaveTextContent('none')
  })

  it('createAccount returns account number and recovery file', async () => {
    const user = userEvent.setup()

    global.fetch = vi.fn().mockImplementation((url: string) => {
      if (url.includes('/auth/account') && !url.includes('/me')) {
        return Promise.resolve({
          ok: true,
          status: 201,
          json: () =>
            Promise.resolve({
              account_number: '9999-8888-7777-6666',
              recovery_file: 'recovery-content-here',
            }),
        })
      }
      if (url.includes('/auth/me')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              ...mockAccount,
              account_number: '9999-8888-7777-6666',
            }),
        })
      }
      return Promise.resolve({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ error: 'Not found' }),
      })
    })

    let createResult: { accountNumber: string; recoveryFile: string } | null = null

    function TestCreateAccount() {
      const { createAccount, account } = useAuth()

      const handleCreate = async () => {
        createResult = await createAccount()
      }

      return (
        <div>
          <button onClick={handleCreate}>Create</button>
          <div data-testid="account-number">{account?.account_number || 'none'}</div>
        </div>
      )
    }

    render(
      <AuthProvider>
        <TestCreateAccount />
      </AuthProvider>
    )

    await act(async () => {
      await user.click(screen.getByText('Create'))
    })

    expect(createResult).toEqual({
      accountNumber: '9999-8888-7777-6666',
      recoveryFile: 'recovery-content-here',
    })
  })

  it('throws error when useAuth is used outside AuthProvider', () => {
    // Suppress console.error for this test
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})

    expect(() => {
      render(<TestConsumer />)
    }).toThrow('useAuth must be used within an AuthProvider')

    consoleSpy.mockRestore()
  })
})
