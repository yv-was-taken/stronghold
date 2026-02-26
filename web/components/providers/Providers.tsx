'use client';

import { AuthKitProvider } from '@workos-inc/authkit-react';
import { AuthProvider } from './AuthProvider';

const workosClientId = process.env.NEXT_PUBLIC_WORKOS_CLIENT_ID ?? '';

export function Providers({ children }: { children: React.ReactNode }) {
  const redirectUri = typeof window !== 'undefined'
    ? `${window.location.origin}/dashboard/login`
    : undefined;

  return (
    <AuthKitProvider clientId={workosClientId} redirectUri={redirectUri}>
      <AuthProvider>
        {children}
      </AuthProvider>
    </AuthKitProvider>
  );
}
