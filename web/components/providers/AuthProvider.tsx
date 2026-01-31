'use client';

import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

interface Account {
  id: string;
  account_number: string;
  wallet_address?: string;
  balance_usdc: number;
  status: string;
  created_at: string;
  last_login_at?: string;
}

interface AuthTokens {
  accessToken: string;
  refreshToken: string;
  expiresAt: string;
}

interface AuthContextType {
  account: Account | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (accountNumber: string) => Promise<void>;
  createAccount: (walletAddress?: string) => Promise<{ accountNumber: string; recoveryFile: string }>;
  logout: () => Promise<void>;
  refreshAccessToken: () => Promise<boolean>;
  getAccessToken: () => string | null;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [account, setAccount] = useState<Account | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [tokens, setTokens] = useState<AuthTokens | null>(null);

  // Load tokens from localStorage on mount
  useEffect(() => {
    const storedTokens = localStorage.getItem('stronghold_tokens');
    if (storedTokens) {
      try {
        const parsed = JSON.parse(storedTokens);
        setTokens(parsed);
      } catch {
        localStorage.removeItem('stronghold_tokens');
      }
    }
    setIsLoading(false);
  }, []);

  // Fetch account info when we have tokens
  useEffect(() => {
    if (tokens?.accessToken) {
      fetchAccount();
    }
  }, [tokens?.accessToken]);

  const fetchAccount = async () => {
    if (!tokens?.accessToken) return;

    try {
      const response = await fetch(`${API_URL}/v1/auth/me`, {
        headers: {
          'Authorization': `Bearer ${tokens.accessToken}`,
        },
      });

      if (response.status === 401) {
        // Token expired, try to refresh
        const refreshed = await refreshAccessToken();
        if (!refreshed) {
          logout();
          return;
        }
        // Retry with new token
        return fetchAccount();
      }

      if (!response.ok) {
        throw new Error('Failed to fetch account');
      }

      const data = await response.json();
      setAccount(data);
    } catch (error) {
      console.error('Error fetching account:', error);
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
        body: JSON.stringify({ account_number: accountNumber }),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'Login failed');
      }

      const data = await response.json();
      const newTokens: AuthTokens = {
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        expiresAt: data.expires_at,
      };

      setTokens(newTokens);
      localStorage.setItem('stronghold_tokens', JSON.stringify(newTokens));

      // Fetch account info
      await fetchAccount();
    } finally {
      setIsLoading(false);
    }
  };

  const createAccount = async (walletAddress?: string): Promise<{ accountNumber: string; recoveryFile: string }> => {
    setIsLoading(true);
    try {
      const body: { wallet_address?: string } = {};
      if (walletAddress) {
        body.wallet_address = walletAddress;
      }

      const response = await fetch(`${API_URL}/v1/auth/account`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'Account creation failed');
      }

      const data = await response.json();
      const newTokens: AuthTokens = {
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        expiresAt: data.expires_at,
      };

      setTokens(newTokens);
      localStorage.setItem('stronghold_tokens', JSON.stringify(newTokens));

      return {
        accountNumber: data.account_number,
        recoveryFile: data.recovery_file,
      };
    } finally {
      setIsLoading(false);
    }
  };

  const logout = async () => {
    if (tokens?.accessToken) {
      try {
        await fetch(`${API_URL}/v1/auth/logout`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${tokens.accessToken}`,
          },
        });
      } catch (error) {
        console.error('Error during logout:', error);
      }
    }

    setTokens(null);
    setAccount(null);
    localStorage.removeItem('stronghold_tokens');
  };

  const refreshAccessToken = useCallback(async (): Promise<boolean> => {
    if (!tokens?.refreshToken) return false;

    try {
      const response = await fetch(`${API_URL}/v1/auth/refresh`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ refresh_token: tokens.refreshToken }),
      });

      if (!response.ok) {
        return false;
      }

      const data = await response.json();
      const newTokens: AuthTokens = {
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        expiresAt: data.expires_at,
      };

      setTokens(newTokens);
      localStorage.setItem('stronghold_tokens', JSON.stringify(newTokens));
      return true;
    } catch (error) {
      console.error('Error refreshing token:', error);
      return false;
    }
  }, [tokens?.refreshToken]);

  const getAccessToken = () => tokens?.accessToken || null;

  return (
    <AuthContext.Provider
      value={{
        account,
        isLoading,
        isAuthenticated: !!account,
        login,
        createAccount,
        logout,
        refreshAccessToken,
        getAccessToken,
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
