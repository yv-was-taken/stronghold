'use client';

import { useState } from 'react';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  Wallet,
  AlertCircle,
  Check,
  Copy,
  LogOut,
  Shield,
} from 'lucide-react';
import { useAuth } from '@/components/providers/AuthProvider';
import { truncateAddress, copyToClipboard } from '@/lib/utils';

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export default function SettingsPage() {
  const { account, getAccessToken, logout } = useAuth();
  const [walletAddress, setWalletAddress] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleCopyAccountNumber = async () => {
    if (account?.account_number) {
      const success = await copyToClipboard(account.account_number);
      if (success) {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }
    }
  };

  const handleLinkWallet = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess('');

    if (!/^0x[a-fA-F0-9]{40}$/.test(walletAddress)) {
      setError('Invalid wallet address format');
      return;
    }

    setIsSubmitting(true);
    try {
      const token = getAccessToken();
      const response = await fetch(`${API_URL}/v1/account/wallet`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          wallet_address: walletAddress,
        }),
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to link wallet');
      }

      setSuccess('Wallet linked successfully');
      setWalletAddress('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to link wallet');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    window.location.href = '/dashboard/login';
  };

  return (
    <div className="min-h-screen bg-[#0a0a0a]">
      {/* Header */}
      <header className="border-b border-[#222]">
        <div className="max-w-2xl mx-auto px-4 py-4 flex items-center gap-4">
          <a
            href="/dashboard/main"
            className="p-2 text-gray-400 hover:text-white transition-colors"
          >
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-xl font-bold text-white">Settings</h1>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-2xl mx-auto px-4 py-8 space-y-6">
        {/* Account Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold text-white mb-4">
            Account Information
          </h2>

          <div className="space-y-4">
            <div>
              <label className="block text-sm text-gray-400 mb-1">
                Account Number
              </label>
              <div className="flex items-center gap-2">
                <code className="flex-1 font-mono text-white bg-[#0a0a0a] rounded-lg px-3 py-2">
                  {account?.account_number}
                </code>
                <button
                  onClick={handleCopyAccountNumber}
                  className="p-2 text-gray-400 hover:text-white transition-colors"
                  title="Copy account number"
                >
                  {copied ? (
                    <Check className="w-4 h-4 text-[#00D4AA]" />
                  ) : (
                    <Copy className="w-4 h-4" />
                  )}
                </button>
              </div>
              <p className="text-gray-500 text-xs mt-2">
                This is your login credential. Keep it secure.
              </p>
            </div>
          </div>
        </motion.div>

        {/* Wallet Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <Wallet className="w-5 h-5 text-[#00D4AA]" />
            Wallet
          </h2>

          {account?.wallet_address ? (
            <div className="mb-6">
              <label className="block text-sm text-gray-400 mb-1">
                Linked Address
              </label>
              <div className="flex items-center gap-2">
                <code className="flex-1 font-mono text-white bg-[#0a0a0a] rounded-lg px-3 py-2 text-sm">
                  {truncateAddress(account.wallet_address, 20, 10)}
                </code>
                <span className="text-xs text-[#00D4AA] bg-[#00D4AA]/10 px-2 py-1 rounded">
                  Base
                </span>
              </div>
            </div>
          ) : (
            <div className="mb-6">
              <p className="text-gray-400 mb-4">
                No wallet linked. Link a wallet to enable direct USDC deposits.
              </p>
            </div>
          )}

          <form onSubmit={handleLinkWallet} className="space-y-4">
            <div>
              <label className="block text-sm text-gray-400 mb-1">
                {account?.wallet_address
                  ? 'Change Wallet Address'
                  : 'Link Wallet Address'}
              </label>
              <input
                type="text"
                value={walletAddress}
                onChange={(e) => {
                  setWalletAddress(e.target.value);
                  setError('');
                  setSuccess('');
                }}
                placeholder="0x..."
                className="w-full px-4 py-2 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors font-mono text-sm"
              />
            </div>

            {error && (
              <div className="flex items-center gap-2 text-red-400 text-sm">
                <AlertCircle className="w-4 h-4" />
                {error}
              </div>
            )}

            {success && (
              <div className="flex items-center gap-2 text-[#00D4AA] text-sm">
                <Check className="w-4 h-4" />
                {success}
              </div>
            )}

            <button
              type="submit"
              disabled={isSubmitting || !walletAddress}
              className="w-full py-2.5 px-4 bg-[#222] hover:bg-[#333] disabled:bg-[#1a1a1a] disabled:cursor-not-allowed text-white font-semibold rounded-lg transition-colors"
            >
              {isSubmitting
                ? 'Linking...'
                : account?.wallet_address
                ? 'Update Wallet'
                : 'Link Wallet'}
            </button>
          </form>
        </motion.div>

        {/* Security Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <Shield className="w-5 h-5 text-[#00D4AA]" />
            Security
          </h2>

          <div className="space-y-4">
            <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-4">
              <h3 className="text-yellow-400 font-medium mb-2">
                Account Recovery
              </h3>
              <p className="text-yellow-300/80 text-sm mb-3">
                If you lose your account number, you cannot recover your account
                without your recovery file.
              </p>
              <a
                href="/dashboard/create"
                className="text-yellow-400 hover:underline text-sm"
              >
                Create new account →
              </a>
            </div>

            <button
              onClick={handleLogout}
              className="w-full py-2.5 px-4 bg-red-500/10 hover:bg-red-500/20 text-red-400 font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
            >
              <LogOut className="w-4 h-4" />
              Logout
            </button>
          </div>
        </motion.div>
      </main>
    </div>
  );
}
