'use client';

import { Suspense, useState, useEffect, useCallback } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  CreditCard,
  DollarSign,
  ExternalLink,
  AlertCircle,
  CheckCircle,
} from 'lucide-react';
import Link from 'next/link';
import { useAuth } from '@/components/providers/AuthProvider';
import { Skeleton } from '@/components/ui/Skeleton';
import { purchaseCredits, getBillingInfo, createBillingPortalSession } from '@/lib/api';
import { formatUSDC } from '@/lib/utils';

interface BillingInfoData {
  credit_balance_usdc: string;
  stripe_customer_id?: string;
  total_purchased_usdc?: string;
  total_used_usdc?: string;
}

function BillingPageContent() {
  const { account, isAuthenticated, isLoading: authLoading } = useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();

  const [billingInfo, setBillingInfo] = useState<BillingInfoData | null>(null);
  const [isLoadingBilling, setIsLoadingBilling] = useState(true);
  const [error, setError] = useState('');
  const [successMessage, setSuccessMessage] = useState('');

  // Purchase state
  const [amount, setAmount] = useState('');
  const [isPurchasing, setIsPurchasing] = useState(false);

  // Portal state
  const [isOpeningPortal, setIsOpeningPortal] = useState(false);

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.replace('/dashboard/login');
    }
  }, [isAuthenticated, authLoading, router]);

  // Show success message from Stripe redirect
  useEffect(() => {
    if (searchParams.get('status') === 'success') {
      setSuccessMessage('Payment completed successfully! Credits will appear in your balance shortly.');
      // Clean up URL
      const url = new URL(window.location.href);
      url.searchParams.delete('status');
      window.history.replaceState({}, '', url.toString());
    }
  }, [searchParams]);

  const loadBillingInfo = useCallback(async () => {
    setIsLoadingBilling(true);
    setError('');
    try {
      const data = await getBillingInfo();
      setBillingInfo(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load billing info');
    } finally {
      setIsLoadingBilling(false);
    }
  }, []);

  useEffect(() => {
    if (isAuthenticated) {
      loadBillingInfo();
    }
  }, [isAuthenticated, loadBillingInfo]);

  const handlePurchase = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    const amountNum = parseFloat(amount);
    if (isNaN(amountNum) || amountNum <= 0) {
      setError('Please enter a valid amount');
      return;
    }

    if (amountNum < 10) {
      setError('Minimum purchase is $10');
      return;
    }

    if (amountNum > 10000) {
      setError('Maximum purchase is $10,000');
      return;
    }

    setIsPurchasing(true);
    try {
      const result = await purchaseCredits(amountNum);
      if (result.checkout_url) {
        window.location.href = result.checkout_url;
      } else {
        setError('Purchase initiated but no checkout URL was returned. Please try again.');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to initiate purchase');
    } finally {
      setIsPurchasing(false);
    }
  };

  const handleOpenPortal = async () => {
    setError('');
    setIsOpeningPortal(true);
    try {
      const result = await createBillingPortalSession();
      if (result.portal_url) {
        window.location.href = result.portal_url;
      } else {
        setError('Could not open the billing portal. Please try again.');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to open billing portal');
    } finally {
      setIsOpeningPortal(false);
    }
  };

  if (authLoading || !account) {
    return <BillingSkeleton />;
  }

  return (
    <div className="min-h-screen bg-[#0a0a0a]">
      {/* Header */}
      <header className="border-b border-[#222]">
        <div className="max-w-2xl mx-auto px-4 py-4 flex items-center gap-4">
          <Link
            href="/dashboard/main"
            aria-label="Back to dashboard"
            className="p-2 text-gray-400 hover:text-white transition-colors"
          >
            <ArrowLeft className="w-5 h-5" />
          </Link>
          <h1 className="text-xl font-bold text-white">Billing</h1>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-2xl mx-auto px-4 py-8 space-y-6">
        {/* Success Message */}
        {successMessage && (
          <motion.div
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
            className="flex items-center gap-3 bg-[#00D4AA]/10 border border-[#00D4AA]/30 rounded-xl p-4"
          >
            <CheckCircle className="w-5 h-5 text-[#00D4AA] flex-shrink-0" />
            <p className="text-[#00D4AA] text-sm">{successMessage}</p>
            <button
              onClick={() => setSuccessMessage('')}
              className="ml-auto text-gray-400 hover:text-white text-sm"
            >
              Dismiss
            </button>
          </motion.div>
        )}

        {/* Credit Balance */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-gradient-to-br from-[#111] to-[#1a1a1a] border border-[#222] rounded-2xl p-6"
        >
          <div className="flex items-start justify-between">
            <div>
              <h2 className="text-gray-400 text-sm mb-1">Credit Balance</h2>
              {isLoadingBilling ? (
                <Skeleton className="w-40 h-10" />
              ) : (
                <div className="text-4xl font-bold text-white">
                  {billingInfo ? formatUSDC(billingInfo.credit_balance_usdc) : '$0.00'}{' '}
                  <span className="text-lg text-gray-500">USDC</span>
                </div>
              )}
            </div>
            <div className="w-12 h-12 rounded-xl bg-[#00D4AA]/10 flex items-center justify-center">
              <DollarSign className="w-6 h-6 text-[#00D4AA]" />
            </div>
          </div>

          {!isLoadingBilling && billingInfo?.total_purchased_usdc && (
            <div className="mt-4 pt-4 border-t border-[#222] grid grid-cols-2 gap-4 text-sm">
              <div>
                <div className="text-gray-500">Total Purchased</div>
                <div className="text-white font-medium">
                  {formatUSDC(billingInfo.total_purchased_usdc)}
                </div>
              </div>
              {billingInfo.total_used_usdc && (
                <div>
                  <div className="text-gray-500">Total Used</div>
                  <div className="text-white font-medium">
                    {formatUSDC(billingInfo.total_used_usdc)}
                  </div>
                </div>
              )}
            </div>
          )}
        </motion.div>

        {/* Buy Credits */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
            <CreditCard className="w-5 h-5 text-[#00D4AA]" />
            Buy Credits
          </h2>

          <form onSubmit={handlePurchase} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">
                Amount (USD)
              </label>
              <div className="relative">
                <span className="absolute left-4 top-1/2 -translate-y-1/2 text-gray-500">$</span>
                <input
                  type="number"
                  value={amount}
                  onChange={(e) => { setAmount(e.target.value); setError(''); }}
                  placeholder="100"
                  min="10"
                  max="10000"
                  step="1"
                  className="w-full pl-8 pr-16 py-3 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors"
                />
                <span className="absolute right-4 top-1/2 -translate-y-1/2 text-gray-500">
                  USDC
                </span>
              </div>
              <p className="text-gray-500 text-xs mt-2">
                Min $10, max $10,000. 1 credit = 1 USDC. Payment via Stripe.
              </p>
            </div>

            {/* Quick Amount Buttons */}
            <div className="flex gap-2">
              {[25, 100, 500, 1000].map((preset) => (
                <button
                  key={preset}
                  type="button"
                  onClick={() => { setAmount(String(preset)); setError(''); }}
                  className={`flex-1 py-2 px-3 rounded-lg border text-sm font-medium transition-colors ${
                    amount === String(preset)
                      ? 'border-[#00D4AA] bg-[#00D4AA]/10 text-white'
                      : 'border-[#333] text-gray-400 hover:border-[#444] hover:text-gray-300'
                  }`}
                >
                  ${preset}
                </button>
              ))}
            </div>

            {error && (
              <div className="flex items-center gap-2 text-red-400 text-sm">
                <AlertCircle className="w-4 h-4 flex-shrink-0" />
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={isPurchasing || !amount}
              className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
            >
              {isPurchasing ? 'Redirecting to Stripe...' : 'Purchase Credits'}
            </button>
          </form>
        </motion.div>

        {/* Manage Payment Methods */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <h2 className="text-lg font-semibold text-white mb-2">Payment Methods</h2>
          <p className="text-gray-400 text-sm mb-4">
            Manage your payment methods, view invoices, and update billing details through the Stripe portal.
          </p>
          <button
            onClick={handleOpenPortal}
            disabled={isOpeningPortal}
            className="w-full py-2.5 px-4 bg-[#222] hover:bg-[#333] disabled:opacity-50 text-white font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
          >
            {isOpeningPortal ? (
              'Opening...'
            ) : (
              <>
                Manage Payment Methods
                <ExternalLink className="w-4 h-4" />
              </>
            )}
          </button>
        </motion.div>
      </main>
    </div>
  );
}

function BillingSkeleton() {
  return (
    <div className="min-h-screen bg-[#0a0a0a]">
      <header className="border-b border-[#222]">
        <div className="max-w-2xl mx-auto px-4 py-4 flex items-center gap-4">
          <Skeleton className="w-9 h-9 rounded" />
          <Skeleton className="w-24 h-6" />
        </div>
      </header>
      <main className="max-w-2xl mx-auto px-4 py-8 space-y-6">
        <Skeleton className="w-full h-32 rounded-2xl" />
        <Skeleton className="w-full h-48 rounded-2xl" />
      </main>
    </div>
  );
}

export default function BillingPage() {
  return (
    <Suspense fallback={<BillingSkeleton />}>
      <BillingPageContent />
    </Suspense>
  );
}
