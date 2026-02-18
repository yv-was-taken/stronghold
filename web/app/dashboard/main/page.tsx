'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import {
  Shield,
  Wallet,
  Activity,
  LogOut,
  ChevronRight,
  AlertTriangle,
  FileText,
} from 'lucide-react';
import { useAuth } from '@/components/providers/AuthProvider';
import { Skeleton } from '@/components/ui/Skeleton';
import { CopyButton } from '@/components/ui/CopyButton';
import { formatUSDC, truncateAddress } from '@/lib/utils';
import { API_URL } from '@/lib/api';

const LOW_BALANCE_THRESHOLD_MICRO_USDC = BigInt('1000000');

export default function DashboardPage() {
  const { account, isAuthenticated, isLoading, logout } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.replace('/dashboard/login');
    }
  }, [isAuthenticated, isLoading, router]);

  const handleLogout = async () => {
    await logout();
    router.push('/dashboard/login');
  };

  if (isLoading || !account) {
    return (
      <div className="min-h-screen bg-[#0a0a0a]">
        {/* Skeleton Header */}
        <header className="border-b border-[#222]">
          <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Skeleton className="w-10 h-10 rounded-xl" />
              <Skeleton className="w-24 h-6" />
            </div>
            <div className="flex items-center gap-4">
              <div className="text-right hidden sm:block">
                <Skeleton className="w-16 h-4 mb-1" />
                <Skeleton className="w-32 h-5" />
              </div>
              <Skeleton className="w-9 h-9 rounded" />
            </div>
          </div>
        </header>

        {/* Skeleton Content */}
        <main className="max-w-6xl mx-auto px-4 py-8">
          {/* Balance Card Skeleton */}
          <div className="bg-gradient-to-br from-[#111] to-[#1a1a1a] border border-[#222] rounded-2xl p-6 mb-6">
            <div className="flex items-start justify-between mb-4">
              <div>
                <Skeleton className="w-16 h-4 mb-2" />
                <Skeleton className="w-40 h-10" />
              </div>
              <Skeleton className="w-12 h-12 rounded-xl" />
            </div>
            <div className="flex gap-3">
              <Skeleton className="flex-1 h-11 rounded-lg" />
              <Skeleton className="w-24 h-11 rounded-lg" />
            </div>
          </div>

          <div className="grid md:grid-cols-2 gap-6">
            {/* Wallet Skeleton */}
            <div className="bg-[#111] border border-[#222] rounded-2xl p-6">
              <Skeleton className="w-16 h-5 mb-4" />
              <Skeleton className="w-full h-16 rounded-lg" />
              <Skeleton className="w-48 h-4 mt-3" />
            </div>

            {/* Activity Skeleton */}
            <div className="bg-[#111] border border-[#222] rounded-2xl p-6">
              <Skeleton className="w-16 h-5 mb-4" />
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <Skeleton className="w-10 h-10 rounded-lg" />
                  <div className="flex-1">
                    <Skeleton className="w-24 h-4 mb-1" />
                    <Skeleton className="w-16 h-3" />
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <Skeleton className="w-10 h-10 rounded-lg" />
                  <div className="flex-1">
                    <Skeleton className="w-28 h-4 mb-1" />
                    <Skeleton className="w-12 h-3" />
                  </div>
                </div>
              </div>
              <Skeleton className="w-32 h-4 mt-4 mx-auto" />
            </div>
          </div>

          {/* Account Info Skeleton */}
          <div className="bg-[#111] border border-[#222] rounded-2xl p-6 mt-6">
            <Skeleton className="w-40 h-5 mb-4" />
            <div className="grid sm:grid-cols-2 gap-4">
              <div>
                <Skeleton className="w-24 h-4 mb-1" />
                <Skeleton className="w-36 h-5" />
              </div>
              <div>
                <Skeleton className="w-12 h-4 mb-1" />
                <Skeleton className="w-16 h-5" />
              </div>
              <div>
                <Skeleton className="w-16 h-4 mb-1" />
                <Skeleton className="w-24 h-5" />
              </div>
              <div>
                <Skeleton className="w-20 h-4 mb-1" />
                <Skeleton className="w-32 h-5" />
              </div>
            </div>
          </div>
        </main>
      </div>
    );
  }

  let balanceMicroUSDC = BigInt(0);
  try {
    balanceMicroUSDC = BigInt(account.balance_usdc || '0');
  } catch {
    balanceMicroUSDC = BigInt(0);
  }
  const isLowBalance = balanceMicroUSDC < LOW_BALANCE_THRESHOLD_MICRO_USDC;

  return (
    <div className="min-h-screen bg-[#0a0a0a]">
      {/* Header */}
      <header className="border-b border-[#222]">
        <div className="max-w-6xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-[#00D4AA] to-[#00a884] flex items-center justify-center">
              <Shield className="w-5 h-5 text-black" />
            </div>
            <span className="text-xl font-bold text-white">Stronghold</span>
          </div>

          <div className="flex items-center gap-4">
            <div className="text-right hidden sm:block">
              <div className="text-sm text-gray-400">Account</div>
              <div className="font-mono text-white">
                {account.account_number}
              </div>
            </div>
            {API_URL && (
              <a
                href={`${API_URL}/docs`}
                target="_blank"
                rel="noopener noreferrer"
                className="p-2 text-gray-400 hover:text-white transition-colors"
                title="API Docs"
              >
                <FileText className="w-5 h-5" />
              </a>
            )}
            <button
              onClick={handleLogout}
              className="p-2 text-gray-400 hover:text-white transition-colors"
              title="Logout"
            >
              <LogOut className="w-5 h-5" />
            </button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-6xl mx-auto px-4 py-8">
        {/* Balance Card */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-gradient-to-br from-[#111] to-[#1a1a1a] border border-[#222] rounded-2xl p-6 mb-6"
        >
          <div className="flex items-start justify-between mb-4">
            <div>
              <h2 className="text-gray-400 text-sm mb-1">Balance</h2>
              <div className="text-4xl font-bold text-white">
                {formatUSDC(account.balance_usdc)}{' '}
                <span className="text-lg text-gray-500">USDC</span>
              </div>
            </div>
            <div className="w-12 h-12 rounded-xl bg-[#00D4AA]/10 flex items-center justify-center">
              <Wallet className="w-6 h-6 text-[#00D4AA]" />
            </div>
          </div>

          {isLowBalance && (
            <div className="flex items-center gap-2 text-yellow-400 text-sm mb-4">
              <AlertTriangle className="w-4 h-4" />
              Low balance. Add funds to continue using Stronghold.
            </div>
          )}

          <div className="flex gap-3">
            <a
              href="/dashboard/main/deposit"
              className="flex-1 py-2.5 px-4 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors text-center"
            >
              Add Funds
            </a>
            <a
              href="/dashboard/main/settings"
              className="py-2.5 px-4 bg-[#222] hover:bg-[#333] text-white font-semibold rounded-lg transition-colors"
            >
              Settings
            </a>
          </div>
        </motion.div>

        <div className="grid md:grid-cols-2 gap-6">
          {/* Wallet Info */}
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.1 }}
            className="bg-[#111] border border-[#222] rounded-2xl p-6"
          >
            <h3 className="text-white font-semibold mb-4">Wallets</h3>
            {(account.evm_wallet_address || account.solana_wallet_address) ? (
              <div className="space-y-3">
                {/* EVM Wallet */}
                {account.evm_wallet_address && (
                  <div className="font-mono text-white bg-[#0a0a0a] rounded-lg p-3 flex items-center justify-between gap-2">
                    <span className="truncate text-sm">{truncateAddress(account.evm_wallet_address)}</span>
                    <div className="flex items-center gap-2 flex-shrink-0">
                      <CopyButton text={account.evm_wallet_address} />
                      <span className="text-xs text-[#00D4AA] bg-[#00D4AA]/10 px-2 py-1 rounded">
                        Base
                      </span>
                    </div>
                  </div>
                )}

                {/* Solana Wallet */}
                {account.solana_wallet_address && (
                  <div className="font-mono text-white bg-[#0a0a0a] rounded-lg p-3 flex items-center justify-between gap-2">
                    <span className="truncate text-sm">{truncateAddress(account.solana_wallet_address)}</span>
                    <div className="flex items-center gap-2 flex-shrink-0">
                      <CopyButton text={account.solana_wallet_address} />
                      <span className="text-xs text-purple-400 bg-purple-400/10 px-2 py-1 rounded">
                        Solana
                      </span>
                    </div>
                  </div>
                )}

                <p className="text-gray-500 text-sm mt-3">
                  <span className="text-[#00D4AA] font-medium">To fund your account:</span> Send USDC to your wallet on the corresponding network.
                </p>
              </div>
            ) : (
              <div className="text-center py-4">
                <p className="text-gray-400 mb-3">
                  No wallets linked to your account.
                </p>
                <p className="text-gray-500 text-sm">
                  Set up via CLI: <code className="text-gray-400">stronghold init</code>
                </p>
              </div>
            )}
          </motion.div>

          {/* Quick Stats */}
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.2 }}
            className="bg-[#111] border border-[#222] rounded-2xl p-6"
          >
            <h3 className="text-white font-semibold mb-4">Activity</h3>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-lg bg-[#00D4AA]/10 flex items-center justify-center">
                    <Activity className="w-5 h-5 text-[#00D4AA]" />
                  </div>
                  <div>
                    <div className="text-white">API Requests</div>
                    <div className="text-sm text-gray-500">Last 30 days</div>
                  </div>
                </div>
                <ChevronRight className="w-5 h-5 text-gray-500" />
              </div>

              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center">
                    <Shield className="w-5 h-5 text-blue-400" />
                  </div>
                  <div>
                    <div className="text-white">Threats Blocked</div>
                    <div className="text-sm text-gray-500">All time</div>
                  </div>
                </div>
                <ChevronRight className="w-5 h-5 text-gray-500" />
              </div>
            </div>

            <a
              href="/dashboard/main/usage"
              className="block mt-4 text-center text-[#00D4AA] hover:underline text-sm"
            >
              View detailed usage →
            </a>
          </motion.div>
        </div>

        {/* Account Info */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6 mt-6"
        >
          <h3 className="text-white font-semibold mb-4">Account Information</h3>
          <div className="grid sm:grid-cols-2 gap-4 text-sm">
            <div>
              <div className="text-gray-500 mb-1">Account Number</div>
              <div className="font-mono text-white">
                {account.account_number}
              </div>
            </div>
            <div>
              <div className="text-gray-500 mb-1">Status</div>
              <div className="flex items-center gap-2">
                <span
                  className={`w-2 h-2 rounded-full ${
                    account.status === 'active'
                      ? 'bg-green-400'
                      : 'bg-yellow-400'
                  }`}
                />
                <span className="text-white capitalize">
                  {account.status}
                </span>
              </div>
            </div>
            <div>
              <div className="text-gray-500 mb-1">Created</div>
              <div className="text-white">
                {new Date(account.created_at).toLocaleDateString()}
              </div>
            </div>
            <div>
              <div className="text-gray-500 mb-1">Last Login</div>
              <div className="text-white">
                {account.last_login_at
                  ? new Date(account.last_login_at).toLocaleString()
                  : 'N/A'}
              </div>
            </div>
          </div>
        </motion.div>
      </main>
    </div>
  );
}
