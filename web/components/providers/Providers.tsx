'use client';

import { AuthKitProvider } from '@workos-inc/authkit-react';
import { AuthProvider } from './AuthProvider';

const workosClientId = process.env.NEXT_PUBLIC_WORKOS_CLIENT_ID ?? '';

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <AuthKitProvider clientId={workosClientId}>
      <AuthProvider>
        {children}
      </AuthProvider>
    </AuthKitProvider>
  );
}
