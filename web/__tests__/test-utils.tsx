import { ReactElement } from 'react'
import { render, RenderOptions } from '@testing-library/react'
import { vi } from 'vitest'
import { AuthProvider } from '@/components/providers/AuthProvider'

// Custom render function that wraps components with providers
function customRender(
  ui: ReactElement,
  options?: Omit<RenderOptions, 'wrapper'>
) {
  return render(ui, {
    wrapper: ({ children }) => <AuthProvider>{children}</AuthProvider>,
    ...options,
  })
}

// Re-export everything from testing-library
export * from '@testing-library/react'
export { customRender as render }

// Helper to create mock fetch responses
export function mockFetch(responses: Record<string, { ok: boolean; json: () => any; status?: number }>) {
  return vi.fn((url: string) => {
    for (const [pattern, response] of Object.entries(responses)) {
      if (url.includes(pattern)) {
        return Promise.resolve({
          ok: response.ok,
          status: response.status || (response.ok ? 200 : 400),
          json: () => Promise.resolve(response.json()),
        })
      }
    }
    return Promise.resolve({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: 'Not found' }),
    })
  })
}

// Mock account data for testing
export const mockAccount = {
  id: 'test-uuid-1234',
  account_number: '1234-5678-9012-3456',
  wallet_address: '0x1234567890123456789012345678901234567890',
  balance_usdc: '100500000',
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
  last_login_at: '2024-01-15T12:00:00Z',
}
