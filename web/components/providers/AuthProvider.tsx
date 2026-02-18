'use client';

import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { API_URL } from '@/lib/api';

interface Account {
  id: string;
  account_number: string;
  evm_wallet_address?: string;   // EVM (Base) wallet
  solana_wallet_address?: string; // Solana wallet
  balance_usdc: string;
  status: string;
  created_at: string;
  last_login_at?: string;
}

interface AuthContextType {
  account: Account | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  totpRequired: boolean;
  login: (accountNumber: string) => Promise<{ totpRequired: boolean }>;
  createAccount: () => Promise<{ accountNumber: string; recoveryFile: string; walletAddress?: string }>;
  verifyTotp: (code: string, isRecovery: boolean, ttlDays: number) => Promise<void>;
  resetTotp: () => void;
  logout: () => Promise<void>;
  refreshAuth: () => Promise<boolean>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [account, setAccount] = useState<Account | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [totpRequired, setTotpRequired] = useState(false);

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

  const checkAuth = useCallback(async () => {
    try {
      const response = await fetch(`${API_URL}/v1/auth/me`, {
        credentials: 'include', // Send httpOnly cookies
      });

      if (response.ok) {
        const data = await response.json();
        setAccount(data);
        setTotpRequired(false);
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

  // Check auth status on mount by fetching account info
  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

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
      await checkAuth();
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
      await checkAuth();
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
      await checkAuth();

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

  const logout = async () => {
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
        login,
        createAccount,
        verifyTotp,
        resetTotp,
        logout,
        refreshAuth,
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
