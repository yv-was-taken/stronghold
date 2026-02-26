// Centralized API configuration and authenticated fetch wrapper

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.getstronghold.xyz');

/** Default request timeout in milliseconds */
const REQUEST_TIMEOUT_MS = 30_000;

/**
 * Creates a fetch call with an AbortController timeout.
 * If the caller already provides a signal, it is not overridden;
 * otherwise a 30-second timeout signal is attached.
 */
function fetchWithTimeout(
  input: string,
  init?: RequestInit
): Promise<Response> {
  // If the caller already set a signal, respect it
  if (init?.signal) {
    return fetch(input, init);
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  return fetch(input, { ...init, signal: controller.signal }).finally(() => {
    clearTimeout(timeoutId);
  });
}

/**
 * Fetch wrapper that includes credentials and handles 401 with token refresh.
 * On 401, attempts to refresh the auth token and retries the original request once.
 * On refresh failure, redirects to the login page.
 * All requests have a 30-second timeout by default.
 */
export async function fetchWithAuth(
  input: string,
  init?: RequestInit
): Promise<Response> {
  const options: RequestInit = { ...init };

  if (_getB2BAccessToken) {
    // B2B: use WorkOS Bearer token (SDK handles refresh internally)
    try {
      const token = await _getB2BAccessToken();
      const headers = new Headers(options.headers);
      headers.set('Authorization', `Bearer ${token}`);
      options.headers = headers;
    } catch {
      // Token refresh failed — redirect to login
      if (typeof window !== 'undefined') {
        window.location.href = '/dashboard/login';
      }
      throw new Error('Session expired');
    }
  } else {
    // B2C: use httpOnly cookies
    options.credentials = 'include';
  }

  let response: Response;
  try {
    response = await fetchWithTimeout(input, options);
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new Error(`Request timed out after ${REQUEST_TIMEOUT_MS}ms`);
    }
    throw err;
  }

  // B2C-only: attempt cookie-based token refresh on 401
  if (!_getB2BAccessToken && response.status === 401) {
    const refreshResponse = await fetchWithTimeout(`${API_URL}/v1/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });

    if (refreshResponse.ok) {
      // Retry the original request
      return fetchWithTimeout(input, options);
    }

    // Refresh failed - redirect to login and throw so callers
    // don't try to parse the stale 401 response as valid data.
    if (typeof window !== 'undefined') {
      window.location.href = '/dashboard/login';
    }
    throw new Error('Session expired');
  }

  return response;
}

// --- Typed API helpers ---

/** Balance information for a single wallet chain */
export interface WalletBalanceInfo {
  address: string;
  balance_usdc: string;
  network: string;
  error?: string;
}

/** Response from GET /v1/account/balances */
export interface BalancesResponse {
  evm?: WalletBalanceInfo;
  solana?: WalletBalanceInfo;
  total_usdc: string;
}

/**
 * Fetch on-chain USDC balances for the authenticated account's wallets.
 */
export async function fetchBalances(): Promise<BalancesResponse> {
  const response = await fetchWithAuth(`${API_URL}/v1/account/balances`);
  if (!response.ok) {
    throw new Error('Failed to fetch balances');
  }
  return response.json();
}

// --- B2B token provider ---

// Module-level function that returns a fresh B2B access token (set by AuthProvider
// when a WorkOS user is detected). fetchWithAuth calls this before every request
// so tokens are always fresh (the WorkOS SDK handles refresh internally).
let _getB2BAccessToken: (() => Promise<string>) | null = null;

export function setB2BTokenProvider(fn: (() => Promise<string>) | null) {
  _getB2BAccessToken = fn;
}

// --- B2B API helpers ---

export async function onboardB2B(companyName: string): Promise<void> {
  const response = await fetchWithAuth(`${API_URL}/v1/auth/b2b/onboard`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ company_name: companyName }),
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Onboarding failed');
  }
}

export interface APIKeyItem {
  id: string;
  key_prefix: string;
  name: string;
  created_at: string;
  last_used_at?: string;
}

export async function listAPIKeys(): Promise<{ api_keys: APIKeyItem[] }> {
  const response = await fetchWithAuth(`${API_URL}/v1/api-keys`);
  if (!response.ok) throw new Error('Failed to list API keys');
  return response.json();
}

export async function createAPIKey(name: string): Promise<{ id: string; key: string; key_prefix: string; name: string }> {
  const response = await fetchWithAuth(`${API_URL}/v1/api-keys`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to create API key');
  }
  return response.json();
}

export async function revokeAPIKey(id: string): Promise<void> {
  const response = await fetchWithAuth(`${API_URL}/v1/api-keys/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to revoke API key');
  }
}

export async function purchaseCredits(amountUSDC: number): Promise<{ checkout_url: string }> {
  const response = await fetchWithAuth(`${API_URL}/v1/billing/credits`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ amount_usdc: amountUSDC }),
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to initiate purchase');
  }
  return response.json();
}

export async function getBillingInfo() {
  const response = await fetchWithAuth(`${API_URL}/v1/billing/info`);
  if (!response.ok) throw new Error('Failed to get billing info');
  return response.json();
}

export async function createBillingPortalSession(): Promise<{ portal_url: string }> {
  const response = await fetchWithAuth(`${API_URL}/v1/billing/portal`, {
    method: 'POST',
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || 'Failed to create billing portal session');
  }
  return response.json();
}

// --- Account settings types and helpers ---

/** Account-level feature settings */
export interface AccountSettings {
  jailbreak_detection_enabled: boolean;
  has_api_keys: boolean;
}

/**
 * Get account settings.
 */
export async function getAccountSettings(): Promise<AccountSettings> {
  const response = await fetchWithAuth(`${API_URL}/v1/account/settings`);
  if (!response.ok) {
    throw new Error('Failed to fetch account settings');
  }
  return response.json();
}

/**
 * Update account settings.
 */
export async function updateAccountSettings(
  settings: Partial<Pick<AccountSettings, 'jailbreak_detection_enabled'>>
): Promise<AccountSettings> {
  const response = await fetchWithAuth(`${API_URL}/v1/account/settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  });
  if (!response.ok) {
    const err = await response.json().catch(() => ({ error: 'Failed to update settings' }));
    throw new Error(err.error || 'Failed to update settings');
  }
  return response.json();
}
