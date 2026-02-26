'use client';

import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { useAuth as useWorkOSAuth } from '@workos-inc/authkit-react';
import { API_URL, setB2BTokenProvider, onboardB2B as onboardB2BAPI } from '@/lib/api';

interface Account {
  id: string;
  account_number: string;
  evm_wallet_address?: string;   // EVM (Base) wallet
  solana_wallet_address?: string; // Solana wallet
  balance_usdc: string;
  status: string;
  created_at: string;
  last_login_at?: string;
  account_type?: 'b2c' | 'b2b';
  email?: string;
  company_name?: string;
  stripe_customer_id?: string;
}

interface AuthContextType {
  account: Account | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  totpRequired: boolean;
  needsOnboarding: boolean;
  login: (accountNumber: string) => Promise<{ totpRequired: boolean }>;
  createAccount: () => Promise<{ accountNumber: string; recoveryFile: string; walletAddress?: string }>;
  verifyTotp: (code: string, isRecovery: boolean, ttlDays: number) => Promise<void>;
  resetTotp: () => void;
  logout: () => Promise<void>;
  refreshAuth: () => Promise<boolean>;
  b2bSignIn: () => Promise<void>;
  b2bSignOut: () => void;
  onboardB2B: (companyName: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [account, setAccount] = useState<Account | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [totpRequired, setTotpRequired] = useState(false);
  const [needsOnboarding, setNeedsOnboarding] = useState(false);

  const {
    user: workosUser,
    isLoading: workosLoading,
    getAccessToken,
    signIn: workosSignIn,
    signOut: workosSignOut,
  } = useWorkOSAuth();

  // Wire up the B2B token provider so fetchWithAuth uses Bearer tokens
  useEffect(() => {
    if (workosUser) {
      setB2BTokenProvider(getAccessToken);
    } else {
      setB2BTokenProvider(null);
    }
    return () => setB2BTokenProvider(null);
  }, [workosUser, getAccessToken]);

  const refreshAuth = useCallback(async (): Promise<boolean> => {
    try {
      const response = await fetch(`${API_URL}/v1/auth/refresh`, {
        method: 'POST',
        credentials: 'include', // Send refresh token cookie, receive new cookies
      });

      return response.ok;
    } catch (error) {
      console.error('Error refreshing auth:', error);
      return false;
    }
  }, []);

  const checkB2BAuth = useCallback(async () => {
    try {
      const token = await getAccessToken();
      const response = await fetch(`${API_URL}/v1/auth/me`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      if (response.ok) {
        const data = await response.json();
        setAccount(data);
        setTotpRequired(false);
        setNeedsOnboarding(data.account_type === 'b2b' && !data.company_name);
      }
    } catch (error) {
      console.error('Error checking B2B auth:', error);
    } finally {
      setIsLoading(false);
    }
  }, [getAccessToken]);

  const checkB2CAuth = useCallback(async () => {
    try {
      const response = await fetch(`${API_URL}/v1/auth/me`, {
        credentials: 'include', // Send httpOnly cookies
      });

      if (response.ok) {
        const data = await response.json();
        setAccount(data);
        setTotpRequired(false);
        setNeedsOnboarding(false);
      } else if (response.status === 401) {
        // Try to refresh the token
        const refreshed = await refreshAuth();
        if (refreshed) {
          // Retry fetching account after refresh
          const retryResponse = await fetch(`${API_URL}/v1/auth/me`, {
            credentials: 'include',
          });
          if (retryResponse.ok) {
            const data = await retryResponse.json();
            setAccount(data);
            setTotpRequired(false);
          }
        }
      } else if (response.status === 403) {
        const data = await response.json().catch(() => null);
        if (data?.totp_required) {
          setTotpRequired(true);
        }
      }
    } catch (error) {
      console.error('Error checking auth:', error);
    } finally {
      setIsLoading(false);
    }
  }, [refreshAuth]);

  // Check auth status on mount (wait for WorkOS to finish loading first)
  useEffect(() => {
    if (workosLoading) return;

    if (workosUser) {
      checkB2BAuth();
    } else {
      checkB2CAuth();
    }
  }, [workosLoading, workosUser, checkB2BAuth, checkB2CAuth]);

  const login = async (accountNumber: string): Promise<{ totpRequired: boolean }> => {
    setIsLoading(true);
    try {
      const response = await fetch(`${API_URL}/v1/auth/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include', // Receive and store httpOnly cookies
        body: JSON.stringify({ account_number: accountNumber }),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'Login failed');
      }

      const data = await response.json();
      if (data?.totp_required) {
        setTotpRequired(true);
        return { totpRequired: true };
      }

      // Fetch account info after successful login
      await checkB2CAuth();
      return { totpRequired: false };
    } finally {
      setIsLoading(false);
    }
  };

  const verifyTotp = async (code: string, isRecovery: boolean, ttlDays: number) => {
    setIsLoading(true);
    try {
      const label =
        typeof navigator !== 'undefined'
          ? `${detectBrowser()} on ${detectPlatform()}`
          : 'web';

      const body = isRecovery
        ? { recovery_code: code, device_label: label, device_ttl_days: ttlDays }
        : { code, device_label: label, device_ttl_days: ttlDays };

      const response = await fetch(`${API_URL}/v1/auth/totp/verify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'TOTP verification failed');
      }

      setTotpRequired(false);
      await checkB2CAuth();
    } finally {
      setIsLoading(false);
    }
  };

  const createAccount = async (): Promise<{ accountNumber: string; recoveryFile: string; walletAddress?: string }> => {
    setIsLoading(true);
    try {
      const response = await fetch(`${API_URL}/v1/auth/account`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include', // Receive and store httpOnly cookies
        body: JSON.stringify({}),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'Account creation failed');
      }

      const data = await response.json();

      // Fetch account info after successful creation
      await checkB2CAuth();

      const result: { accountNumber: string; recoveryFile: string; walletAddress?: string } = {
        accountNumber: data.account_number,
        recoveryFile: data.recovery_file,
      };
      if (data.wallet_address) {
        result.walletAddress = data.wallet_address;
      }
      return result;
    } finally {
      setIsLoading(false);
    }
  };

  const b2bSignIn = async () => {
    await workosSignIn();
  };

  const b2bSignOutHandler = () => {
    setAccount(null);
    setNeedsOnboarding(false);
    setB2BTokenProvider(null);
    workosSignOut();
  };

  const onboardB2BHandler = async (companyName: string) => {
    await onboardB2BAPI(companyName);
    setNeedsOnboarding(false);
    // Refresh account data to get updated company_name
    await checkB2BAuth();
  };

  const logout = async () => {
    if (workosUser) {
      b2bSignOutHandler();
      return;
    }

    try {
      await fetch(`${API_URL}/v1/auth/logout`, {
        method: 'POST',
        credentials: 'include', // Send cookies for auth, server will clear them
      });
    } catch (error) {
      console.error('Error during logout:', error);
    }

    setAccount(null);
    setTotpRequired(false);
  };

  const resetTotp = () => {
    setTotpRequired(false);
  };

  return (
    <AuthContext.Provider
      value={{
        account,
        isLoading,
        isAuthenticated: !!account,
        totpRequired,
        needsOnboarding,
        login,
        createAccount,
        verifyTotp,
        resetTotp,
        logout,
        refreshAuth,
        b2bSignIn,
        b2bSignOut: b2bSignOutHandler,
        onboardB2B: onboardB2BHandler,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}

function detectBrowser(): string {
  if (typeof navigator === 'undefined') {
    return 'Browser';
  }
  const ua = navigator.userAgent;
  if (ua.includes('Edg/')) return 'Edge';
  if (ua.includes('Chrome/')) return 'Chrome';
  if (ua.includes('Firefox/')) return 'Firefox';
  if (ua.includes('Safari/') && !ua.includes('Chrome/')) return 'Safari';
  return 'Browser';
}

function detectPlatform(): string {
  if (typeof navigator === 'undefined') {
    return 'unknown';
  }
  const navAny = navigator as Navigator & { userAgentData?: { platform?: string } };
  if (navAny.userAgentData?.platform) {
    return navAny.userAgentData.platform;
  }
  return navigator.platform || 'unknown';
}
