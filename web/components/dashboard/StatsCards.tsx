'use client';

import { motion } from 'framer-motion';
import { Activity, DollarSign, Shield, Clock, TrendingUp, TrendingDown } from 'lucide-react';
import { SkeletonStatCard } from '@/components/ui/Skeleton';
import { formatUSDC } from '@/lib/utils';
import type { UsageStats } from '@/lib/hooks/useUsage';

interface StatsCardsProps {
  stats: UsageStats | null;
  loading: boolean;
}

interface StatCardProps {
  icon: React.ReactNode;
  iconBg: string;
  label: string;
  value: string;
  subLabel?: string;
  trend?: 'up' | 'down' | null;
  delay?: number;
}

function StatCard({ icon, iconBg, label, value, subLabel, trend, delay = 0 }: StatCardProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay }}
      className="bg-[#111] border border-[#222] rounded-xl p-5"
    >
      <div className="flex items-center justify-between mb-3">
        <div className={`w-10 h-10 rounded-lg ${iconBg} flex items-center justify-center`}>
          {icon}
        </div>
        {trend && (
          <div className={`flex items-center gap-1 text-xs ${
            trend === 'up' ? 'text-green-400' : 'text-red-400'
          }`}>
            {trend === 'up' ? (
              <TrendingUp className="w-3 h-3" />
            ) : (
              <TrendingDown className="w-3 h-3" />
            )}
          </div>
        )}
      </div>
      <div className="text-2xl font-bold text-white font-mono">{value}</div>
      <div className="text-gray-500 text-sm">{label}</div>
      {subLabel && (
        <div className="text-gray-600 text-xs mt-1">{subLabel}</div>
      )}
    </motion.div>
  );
}

export function StatsCards({ stats, loading }: StatsCardsProps) {
  if (loading) {
    return (
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <SkeletonStatCard />
        <SkeletonStatCard />
        <SkeletonStatCard />
        <SkeletonStatCard />
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <StatCard
          icon={<Activity className="w-5 h-5 text-[#00D4AA]" />}
          iconBg="bg-[#00D4AA]/10"
          label="Total Requests"
          value="--"
        />
        <StatCard
          icon={<DollarSign className="w-5 h-5 text-blue-400" />}
          iconBg="bg-blue-500/10"
          label="Total Cost"
          value="--"
          delay={0.05}
        />
        <StatCard
          icon={<Shield className="w-5 h-5 text-red-400" />}
          iconBg="bg-red-500/10"
          label="Threats Detected"
          value="--"
          delay={0.1}
        />
        <StatCard
          icon={<Clock className="w-5 h-5 text-yellow-400" />}
          iconBg="bg-yellow-500/10"
          label="Avg Latency"
          value="--"
          delay={0.15}
        />
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      <StatCard
        icon={<Activity className="w-5 h-5 text-[#00D4AA]" />}
        iconBg="bg-[#00D4AA]/10"
        label="Total Requests"
        value={(stats.total_requests ?? 0).toLocaleString()}
        subLabel={`Last ${stats.period_days ?? 30} days`}
      />
      <StatCard
        icon={<DollarSign className="w-5 h-5 text-blue-400" />}
        iconBg="bg-blue-500/10"
        label="Total Cost"
        value={formatUSDC(stats.total_cost_usdc ?? "0")}
        subLabel="USDC"
        delay={0.05}
      />
      <StatCard
        icon={<Shield className="w-5 h-5 text-red-400" />}
        iconBg="bg-red-500/10"
        label="Threats Detected"
        value={(stats.threats_detected ?? 0).toLocaleString()}
        subLabel="Blocked"
        delay={0.1}
      />
      <StatCard
        icon={<Clock className="w-5 h-5 text-yellow-400" />}
        iconBg="bg-yellow-500/10"
        label="Avg Latency"
        value={`${Math.round(stats.avg_latency_ms ?? 0)}ms`}
        subLabel="Response time"
        delay={0.15}
      />
    </div>
  );
}
