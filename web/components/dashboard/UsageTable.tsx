'use client';

import { motion } from 'framer-motion';
import { CheckCircle, XCircle, AlertTriangle } from 'lucide-react';
import { SkeletonTableRow } from '@/components/ui/Skeleton';
import { formatRelativeTime, formatUSDC } from '@/lib/utils';
import type { UsageLog } from '@/lib/hooks/useUsage';

interface UsageTableProps {
  logs: UsageLog[];
  loading: boolean;
  hasMore: boolean;
  onLoadMore: () => void;
}

const endpointLabels: Record<string, string> = {
  '/v1/scan/content': 'Content Scan',
  '/v1/scan/output': 'Output Scan',
};

export function UsageTable({ logs, loading, hasMore, onLoadMore }: UsageTableProps) {
  if (loading && logs.length === 0) {
    return (
      <div className="bg-[#111] border border-[#222] rounded-xl overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-[#222]">
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Time</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Endpoint</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Cost</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Status</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Threat</th>
            </tr>
          </thead>
          <tbody>
            <SkeletonTableRow columns={5} />
            <SkeletonTableRow columns={5} />
            <SkeletonTableRow columns={5} />
            <SkeletonTableRow columns={5} />
            <SkeletonTableRow columns={5} />
          </tbody>
        </table>
      </div>
    );
  }

  if (!loading && logs.length === 0) {
    return (
      <div className="bg-[#111] border border-[#222] rounded-xl p-12 text-center">
        <div className="w-16 h-16 rounded-full bg-[#1a1a1c] flex items-center justify-center mx-auto mb-4">
          <AlertTriangle className="w-8 h-8 text-gray-600" />
        </div>
        <h3 className="text-white font-semibold mb-2">No usage data yet</h3>
        <p className="text-gray-500 text-sm">
          Your API usage will appear here once you start making requests.
        </p>
      </div>
    );
  }

  return (
    <div className="bg-[#111] border border-[#222] rounded-xl overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b border-[#222]">
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Time</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Endpoint</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Cost</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Status</th>
              <th className="text-left text-gray-500 text-sm font-medium py-3 px-4">Threat</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((log, index) => (
              <motion.tr
                key={log.id}
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.02 }}
                className="border-b border-[#222] last:border-b-0 hover:bg-[#1a1a1c] transition-colors"
              >
                <td className="py-3 px-4 text-gray-400 text-sm">
                  {formatRelativeTime(log.created_at)}
                </td>
                <td className="py-3 px-4">
                  <span className="text-white text-sm font-mono">
                    {endpointLabels[log.endpoint] || log.endpoint}
                  </span>
                </td>
                <td className="py-3 px-4 text-gray-300 text-sm font-mono">
                  ${formatUSDC(log.cost_usdc)}
                </td>
                <td className="py-3 px-4">
                  {log.status === 'success' ? (
                    <span className="inline-flex items-center gap-1.5 text-green-400 text-sm">
                      <CheckCircle className="w-3.5 h-3.5" />
                      Success
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1.5 text-red-400 text-sm">
                      <XCircle className="w-3.5 h-3.5" />
                      Error
                    </span>
                  )}
                </td>
                <td className="py-3 px-4">
                  {log.threat_detected ? (
                    <span className="inline-flex items-center gap-1.5 px-2 py-0.5 bg-red-500/10 text-red-400 text-xs rounded-full">
                      <AlertTriangle className="w-3 h-3" />
                      Detected
                    </span>
                  ) : (
                    <span className="text-gray-600 text-sm">—</span>
                  )}
                </td>
              </motion.tr>
            ))}
          </tbody>
        </table>
      </div>

      {hasMore && (
        <div className="p-4 border-t border-[#222]">
          <button
            onClick={onLoadMore}
            disabled={loading}
            className="w-full py-2.5 text-sm text-gray-400 hover:text-white hover:bg-[#1a1a1c] rounded-lg transition-colors disabled:opacity-50"
          >
            {loading ? 'Loading...' : 'Load More'}
          </button>
        </div>
      )}
    </div>
  );
}
