'use client';

import { useState, useCallback } from 'react';
import { API_URL, fetchWithAuth } from '@/lib/api';

export interface UsageLog {
  id: string;
  endpoint: string;
  cost_usdc: number;
  status: 'success' | 'error';
  threat_detected: boolean;
  latency_ms: number;
  created_at: string;
}

export interface UsageStats {
  total_requests: number;
  total_cost_usdc: number;
  threats_detected: number;
  avg_latency_ms: number;
  period_days: number;
}

interface UsageLogsState {
  data: UsageLog[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
}

interface UsageStatsState {
  data: UsageStats | null;
  loading: boolean;
  error: string | null;
}

export function useUsageLogs() {
  const [state, setState] = useState<UsageLogsState>({
    data: [],
    loading: false,
    error: null,
    hasMore: true,
  });

  const fetchLogs = useCallback(async (limit = 20, offset = 0, append = false) => {
    setState(prev => ({ ...prev, loading: true, error: null }));

    try {
      const response = await fetchWithAuth(
        `${API_URL}/v1/account/usage?limit=${limit}&offset=${offset}`
      );

      if (!response.ok) {
        throw new Error('Failed to fetch usage logs');
      }

      const result = await response.json();
      const logs: UsageLog[] = result.usage || [];

      setState(prev => ({
        data: append ? [...prev.data, ...logs] : logs,
        loading: false,
        error: null,
        hasMore: logs.length === limit,
      }));

      return logs;
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Unknown error';
      setState(prev => ({ ...prev, loading: false, error: errorMessage }));
      return [];
    }
  }, []);

  const loadMore = useCallback(async () => {
    if (state.loading || !state.hasMore) return;
    await fetchLogs(20, state.data.length, true);
  }, [fetchLogs, state.loading, state.hasMore, state.data.length]);

  const refetch = useCallback(() => fetchLogs(20, 0, false), [fetchLogs]);

  return {
    ...state,
    fetchLogs,
    loadMore,
    refetch,
  };
}

export function useUsageStats() {
  const [state, setState] = useState<UsageStatsState>({
    data: null,
    loading: false,
    error: null,
  });

  const fetchStats = useCallback(async (days = 30) => {
    setState(prev => ({ ...prev, loading: true, error: null }));

    try {
      const response = await fetchWithAuth(
        `${API_URL}/v1/account/usage/stats?days=${days}`
      );

      if (!response.ok) {
        throw new Error('Failed to fetch usage stats');
      }

      const stats: UsageStats = await response.json();

      setState({
        data: stats,
        loading: false,
        error: null,
      });

      return stats;
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Unknown error';
      setState(prev => ({ ...prev, loading: false, error: errorMessage }));
      return null;
    }
  }, []);

  const refetch = useCallback(() => fetchStats(30), [fetchStats]);

  return {
    ...state,
    fetchStats,
    refetch,
  };
}
