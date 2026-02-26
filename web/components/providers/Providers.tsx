'use client';

import { AuthKitProvider } from '@workos-inc/authkit-react';
import { AuthProvider } from './AuthProvider';
import { API_URL } from '@/lib/api';

const workosClientId = process.env.NEXT_PUBLIC_WORKOS_CLIENT_ID ?? '';

// Derive AuthKit API connection from our API URL. We proxy WorkOS user management
// endpoints through our Go API to work around a WorkOS CORS bug where actual
// responses are missing Access-Control-Allow-Origin headers.
function getAuthKitApiConfig() {
  try {
    const url = new URL(API_URL);
    return {
      apiHostname: url.hostname,
      port: url.port ? Number(url.port) : undefined,
      https: url.protocol === 'https:',
    };
  } catch {
    return { apiHostname: 'localhost', port: 8080, https: false };
  }
}

export function Providers({ children }: { children: React.ReactNode }) {
  const redirectUri = typeof window !== 'undefined'
    ? `${window.location.origin}/dashboard/login`
    : undefined;

  const apiConfig = getAuthKitApiConfig();

  return (
    <AuthKitProvider
      clientId={workosClientId}
      redirectUri={redirectUri}
      apiHostname={apiConfig.apiHostname}
      port={apiConfig.port}
      https={apiConfig.https}
      devMode
    >
      <AuthProvider>
        {children}
      </AuthProvider>
    </AuthKitProvider>
  );
}
