'use client';

import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { API_URL } from '@/lib/api';

interface Account {
  id: string;
  account_number: string;
  wallet_address?: string;
  balance_usdc: number;
  status: string;
  created_at: string;
  last_login_at?: string;
}

interface AuthContextType {
  account: Account | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (accountNumber: string) => Promise<void>;
  createAccount: () => Promise<{ accountNumber: string; recoveryFile: string; walletAddress: string }>;
  logout: () => Promise<void>;
  refreshAuth: () => Promise<boolean>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [account, setAccount] = useState<Account | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Check auth status on mount by fetching account info
  useEffect(() => {
    checkAuth();
  }, []);

  const checkAuth = async () => {
    try {
      const response = await fetch(`${API_URL}/v1/auth/me`, {
        credentials: 'include', // Send httpOnly cookies
      });

      if (response.ok) {
        const data = await response.json();
        setAccount(data);
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
          }
        }
      }
    } catch (error) {
      console.error('Error checking auth:', error);
    } finally {
      setIsLoading(false);
    }
  };

  const login = async (accountNumber: string) => {
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

      // Fetch account info after successful login
      await checkAuth();
    } finally {
      setIsLoading(false);
    }
  };

  const createAccount = async (): Promise<{ accountNumber: string; recoveryFile: string; walletAddress: string }> => {
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

      return {
        accountNumber: data.account_number,
        recoveryFile: data.recovery_file,
        walletAddress: data.wallet_address,
      };
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
  };

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

  return (
    <AuthContext.Provider
      value={{
        account,
        isLoading,
        isAuthenticated: !!account,
        login,
        createAccount,
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
