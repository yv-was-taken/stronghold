'use client';

import { useState, useCallback } from 'react';
import { API_URL, fetchWithAuth } from '@/lib/api';

export interface Deposit {
  id: string;
  amount_usdc: string;
  fee_usdc: string;
  net_amount_usdc: string;
  provider: 'stripe' | 'direct';
  status: 'pending' | 'completed' | 'failed';
  created_at: string;
  completed_at?: string;
}

interface DepositWire {
  id: string;
  amount_usdc: string;
  fee_usdc: string;
  net_amount_usdc?: string;
  net_usdc?: string; // legacy fallback
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
      const wireDeposits: DepositWire[] = result.deposits || [];
      const deposits: Deposit[] = wireDeposits.map((deposit) => ({
        id: deposit.id,
        amount_usdc: deposit.amount_usdc,
        fee_usdc: deposit.fee_usdc,
        net_amount_usdc: deposit.net_amount_usdc ?? deposit.net_usdc ?? '0',
        provider: deposit.provider,
        status: deposit.status,
        created_at: deposit.created_at,
        completed_at: deposit.completed_at,
      }));

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
