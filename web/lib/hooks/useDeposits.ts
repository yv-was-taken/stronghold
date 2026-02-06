'use client';

import { useState, useCallback } from 'react';
import { API_URL, fetchWithAuth } from '@/lib/api';

export interface Deposit {
  id: string;
  amount_usdc: number;
  fee_usdc: number;
  net_usdc: number;
  provider: 'stripe' | 'direct';
  status: 'pending' | 'completed' | 'failed';
  created_at: string;
  completed_at?: string;
}

interface DepositsState {
  data: Deposit[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
}

export function useDeposits() {
  const [state, setState] = useState<DepositsState>({
    data: [],
    loading: false,
    error: null,
    hasMore: true,
  });

  const fetchDeposits = useCallback(async (limit = 20, offset = 0, append = false) => {
    setState(prev => ({ ...prev, loading: true, error: null }));

    try {
      const response = await fetchWithAuth(
        `${API_URL}/v1/account/deposits?limit=${limit}&offset=${offset}`
      );

      if (!response.ok) {
        throw new Error('Failed to fetch deposits');
      }

      const result = await response.json();
      const deposits: Deposit[] = result.deposits || [];

      setState(prev => ({
        data: append ? [...prev.data, ...deposits] : deposits,
        loading: false,
        error: null,
        hasMore: deposits.length === limit,
      }));

      return deposits;
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Unknown error';
      setState(prev => ({ ...prev, loading: false, error: errorMessage }));
      return [];
    }
  }, []);

  const loadMore = useCallback(async () => {
    if (state.loading || !state.hasMore) return;
    await fetchDeposits(20, state.data.length, true);
  }, [fetchDeposits, state.loading, state.hasMore, state.data.length]);

  const refetch = useCallback(() => fetchDeposits(20, 0, false), [fetchDeposits]);

  return {
    ...state,
    fetchDeposits,
    loadMore,
    refetch,
  };
}
