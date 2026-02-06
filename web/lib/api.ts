// Centralized API configuration and authenticated fetch wrapper

export const API_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.getstronghold.xyz');

/**
 * Fetch wrapper that includes credentials and handles 401 with token refresh.
 * On 401, attempts to refresh the auth token and retries the original request once.
 * On refresh failure, redirects to the login page.
 */
export async function fetchWithAuth(
  input: string,
  init?: RequestInit
): Promise<Response> {
  const options: RequestInit = {
    ...init,
    credentials: 'include',
  };

  const response = await fetch(input, options);

  if (response.status === 401) {
    // Attempt token refresh
    const refreshResponse = await fetch(`${API_URL}/v1/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });

    if (refreshResponse.ok) {
      // Retry the original request
      return fetch(input, options);
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
