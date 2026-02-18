'use client';

import { useState, useMemo } from 'react';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  CreditCard,
  Wallet,
  Check,
  AlertCircle,
} from 'lucide-react';
import { useAuth } from '@/components/providers/AuthProvider';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { CopyButton } from '@/components/ui/CopyButton';
import { formatUSDC } from '@/lib/utils';
import { API_URL, fetchWithAuth } from '@/lib/api';

type Network = 'base' | 'solana';

export default function DepositPage() {
  const { account } = useAuth();
  const [amount, setAmount] = useState('');
  const [provider, setProvider] = useState<'stripe' | 'direct'>('stripe');
  const [network, setNetwork] = useState<Network>('base');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [checkoutUrl, setCheckoutUrl] = useState<string | null>(null);

  // Determine which wallets are linked
  const hasEVM = !!(account?.evm_wallet_address);
  const hasSolana = !!(account?.solana_wallet_address);
  const linkedNetworks = useMemo(() => {
    const nets: Network[] = [];
    if (hasEVM) nets.push('base');
    if (hasSolana) nets.push('solana');
    return nets;
  }, [hasEVM, hasSolana]);

  // Auto-select if only one wallet is linked
  const effectiveNetwork = useMemo(() => {
    if (linkedNetworks.length === 1) return linkedNetworks[0];
    return network;
  }, [linkedNetworks, network]);

  const showNetworkSelector = linkedNetworks.length > 1;

  // Get wallet address for the selected network
  const selectedWalletAddress = useMemo(() => {
    if (effectiveNetwork === 'base') return account?.evm_wallet_address;
    if (effectiveNetwork === 'solana') return account?.solana_wallet_address;
    return undefined;
  }, [effectiveNetwork, account]);

  const networkLabel = effectiveNetwork === 'base' ? 'Base (EVM)' : 'Solana';

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    const amountNum = parseFloat(amount);
    if (isNaN(amountNum) || amountNum <= 0) {
      setError('Please enter a valid amount');
      return;
    }

    if (amountNum < 10) {
      setError('Minimum deposit is 10 USDC');
      return;
    }

    if (!selectedWalletAddress && provider === 'stripe') {
      setError(`No ${networkLabel} wallet linked. Link one in Settings first.`);
      return;
    }

    setIsSubmitting(true);
    try {
      const response = await fetchWithAuth(`${API_URL}/v1/account/deposit`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          // Input remains human-readable USDC float; API responses use microUSDC strings.
          amount_usdc: amountNum,
          provider: provider,
          network: effectiveNetwork,
        }),
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to initiate deposit');
      }

      const data = await response.json();

      if (provider === 'stripe' && data.checkout_url) {
        setCheckoutUrl(data.checkout_url);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deposit failed');
    } finally {
      setIsSubmitting(false);
    }
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
          <h1 className="text-xl font-bold text-white">Add Funds</h1>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-2xl mx-auto px-4 py-8">
        {/* Current Balance */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6 mb-6"
        >
          <div className="text-gray-400 text-sm mb-1">Current Balance</div>
          <div className="text-3xl font-bold text-white">
            {account ? formatUSDC(account.balance_usdc) : '$0.00'}{' '}
            <span className="text-lg text-gray-500">USDC</span>
          </div>
        </motion.div>

        {checkoutUrl ? (
          /* Checkout Success */
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            className="bg-[#111] border border-[#222] rounded-2xl p-8 text-center"
          >
            <div className="w-16 h-16 rounded-full bg-[#00D4AA]/20 flex items-center justify-center mx-auto mb-4">
              <Check className="w-8 h-8 text-[#00D4AA]" />
            </div>
            <h2 className="text-2xl font-bold text-white mb-2">
              Deposit Initiated
            </h2>
            <p className="text-gray-400 mb-6">
              Complete your payment using Stripe. Funds will be credited to your
              account upon completion.
            </p>
            <a
              href={checkoutUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-block py-3 px-6 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors"
            >
              Complete Payment
            </a>
            <button
              onClick={() => setCheckoutUrl(null)}
              className="block mx-auto mt-4 text-gray-400 hover:text-white text-sm"
            >
              Start over
            </button>
          </motion.div>
        ) : (
          /* Deposit Form */
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            className="bg-[#111] border border-[#222] rounded-2xl p-6"
          >
            {/* Provider Selection */}
            <div className="grid grid-cols-2 gap-3 mb-6">
              <button
                onClick={() => setProvider('stripe')}
                className={`p-4 rounded-xl border transition-all flex flex-col items-center gap-2 ${
                  provider === 'stripe'
                    ? 'border-[#00D4AA] bg-[#00D4AA]/10'
                    : 'border-[#333] hover:border-[#444]'
                }`}
              >
                <CreditCard
                  className={`w-6 h-6 ${
                    provider === 'stripe' ? 'text-[#00D4AA]' : 'text-gray-400'
                  }`}
                />
                <span
                  className={`font-medium ${
                    provider === 'stripe' ? 'text-white' : 'text-gray-400'
                  }`}
                >
                  Card (Stripe)
                </span>
              </button>

              <button
                onClick={() => setProvider('direct')}
                className={`p-4 rounded-xl border transition-all flex flex-col items-center gap-2 ${
                  provider === 'direct'
                    ? 'border-[#00D4AA] bg-[#00D4AA]/10'
                    : 'border-[#333] hover:border-[#444]'
                }`}
              >
                <Wallet
                  className={`w-6 h-6 ${
                    provider === 'direct' ? 'text-[#00D4AA]' : 'text-gray-400'
                  }`}
                />
                <span
                  className={`font-medium ${
                    provider === 'direct' ? 'text-white' : 'text-gray-400'
                  }`}
                >
                  Crypto
                </span>
              </button>
            </div>

            {/* Network Selection - only shown if user has more than one wallet linked */}
            {showNetworkSelector && (
              <div className="mb-6">
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Target Network
                </label>
                <div className="grid grid-cols-2 gap-3">
                  <button
                    onClick={() => setNetwork('base')}
                    className={`p-3 rounded-xl border transition-all text-center ${
                      effectiveNetwork === 'base'
                        ? 'border-[#00D4AA] bg-[#00D4AA]/10'
                        : 'border-[#333] hover:border-[#444]'
                    }`}
                  >
                    <span
                      className={`font-medium text-sm ${
                        effectiveNetwork === 'base' ? 'text-white' : 'text-gray-400'
                      }`}
                    >
                      Base (EVM)
                    </span>
                  </button>
                  <button
                    onClick={() => setNetwork('solana')}
                    className={`p-3 rounded-xl border transition-all text-center ${
                      effectiveNetwork === 'solana'
                        ? 'border-[#00D4AA] bg-[#00D4AA]/10'
                        : 'border-[#333] hover:border-[#444]'
                    }`}
                  >
                    <span
                      className={`font-medium text-sm ${
                        effectiveNetwork === 'solana' ? 'text-white' : 'text-gray-400'
                      }`}
                    >
                      Solana
                    </span>
                  </button>
                </div>
              </div>
            )}

            {/* No wallets linked */}
            {linkedNetworks.length === 0 && (
              <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-4 mb-6">
                <div className="flex items-center gap-2 text-yellow-400 text-sm">
                  <AlertCircle className="w-4 h-4 flex-shrink-0" />
                  <span>
                    No wallets linked.{' '}
                    <a
                      href="/dashboard/main/settings"
                      className="underline hover:text-yellow-300"
                    >
                      Link a wallet in Settings
                    </a>{' '}
                    to deposit funds.
                  </span>
                </div>
              </div>
            )}

            {provider === 'stripe' ? (
              /* Stripe Form */
              <form onSubmit={handleSubmit} className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">
                    Amount (USDC)
                  </label>
                  <div className="relative">
                    <input
                      type="number"
                      value={amount}
                      onChange={(e) => setAmount(e.target.value)}
                      placeholder="100"
                      min="10"
                      step="1"
                      className="w-full px-4 py-3 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors"
                    />
                    <span className="absolute right-4 top-1/2 -translate-y-1/2 text-gray-500">
                      USDC
                    </span>
                  </div>
                  <p className="text-gray-500 text-xs mt-2">
                    Minimum deposit: 10 USDC. Fees apply. Targeting {networkLabel} wallet.
                  </p>
                </div>

                {error && (
                  <div className="flex items-center gap-2 text-red-400 text-sm">
                    <AlertCircle className="w-4 h-4" />
                    {error}
                  </div>
                )}

                <button
                  type="submit"
                  disabled={isSubmitting || !amount || !selectedWalletAddress}
                  className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
                >
                  {isSubmitting ? (
                    <>
                      <LoadingSpinner size="sm" className="border-black border-t-transparent" />
                      Processing...
                    </>
                  ) : (
                    'Continue to Payment'
                  )}
                </button>
              </form>
            ) : (
              /* Direct Deposit Info */
              <div className="space-y-4">
                <div className="bg-[#0a0a0a] border border-[#333] rounded-xl p-4">
                  <div className="text-sm text-gray-400 mb-2">
                    Send USDC on {networkLabel} to:
                  </div>
                  {selectedWalletAddress ? (
                    <div className="flex items-center gap-2">
                      <code className="flex-1 font-mono text-white text-sm break-all">
                        {selectedWalletAddress}
                      </code>
                      <CopyButton text={selectedWalletAddress} className="p-2" />
                    </div>
                  ) : (
                    <div className="text-center py-4">
                      <p className="text-gray-400 mb-2">
                        No {networkLabel} wallet linked to your account.
                      </p>
                      <a
                        href="/dashboard/main/settings"
                        className="text-[#00D4AA] hover:underline text-sm"
                      >
                        Link a wallet →
                      </a>
                    </div>
                  )}
                </div>

                <div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-4">
                  <h4 className="text-blue-400 font-medium mb-2">
                    Important
                  </h4>
                  <ul className="text-blue-300/80 text-sm space-y-1">
                    <li>• Only send USDC on {networkLabel} network</li>
                    <li>• Deposits are credited automatically</li>
                    <li>• No fees for direct deposits</li>
                    <li>• Processing time: 1-2 minutes</li>
                  </ul>
                </div>
              </div>
            )}
          </motion.div>
        )}
      </main>
    </div>
  );
}
