'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { Shield, ArrowRight, AlertCircle, Check, Download } from 'lucide-react';
import { useAuth } from '@/components/providers/AuthProvider';
import { downloadTextFile } from '@/lib/utils';

export default function CreateAccountPage() {
  const [step, setStep] = useState<'form' | 'success'>('form');
  const [accountNumber, setAccountNumber] = useState('');
  const [walletAddress, setWalletAddress] = useState('');
  const [recoveryFile, setRecoveryFile] = useState('');
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isDownloading, setIsDownloading] = useState(false);
  const { createAccount, isAuthenticated, isLoading } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (isAuthenticated && !isLoading) {
      router.replace('/dashboard/main');
    }
  }, [isAuthenticated, isLoading, router]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    setIsSubmitting(true);
    try {
      const result = await createAccount();
      setAccountNumber(result.accountNumber);
      setWalletAddress(result.walletAddress);
      setRecoveryFile(result.recoveryFile);
      setStep('success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Account creation failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDownload = () => {
    setIsDownloading(true);
    downloadTextFile(
      `stronghold-recovery-${accountNumber.replace(/-/g, '')}.txt`,
      recoveryFile
    );
    setTimeout(() => setIsDownloading(false), 1000);
  };

  const handleContinue = () => {
    router.push('/dashboard/main');
  };

  if (isLoading || isAuthenticated) {
    return (
      <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center">
        <div className="animate-pulse text-[#00D4AA]">Loading...</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center p-4">
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5 }}
        className="w-full max-w-md"
      >
        {/* Logo */}
        <div className="flex justify-center mb-8">
          <div className="flex items-center gap-3">
            <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-[#00D4AA] to-[#00a884] flex items-center justify-center">
              <Shield className="w-7 h-7 text-black" />
            </div>
            <span className="text-2xl font-bold text-white">Stronghold</span>
          </div>
        </div>

        {step === 'form' ? (
          /* Create Account Form */
          <div className="bg-[#111111] border border-[#222] rounded-2xl p-8">
            <h1 className="text-2xl font-bold text-white mb-2">
              Create Account
            </h1>
            <p className="text-gray-400 mb-6">
              No email required. We&apos;ll generate a secure account number for
              you.
            </p>

            <form onSubmit={handleSubmit} className="space-y-4">
              {error && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  className="flex items-center gap-2 text-red-400 text-sm"
                >
                  <AlertCircle className="w-4 h-4" />
                  {error}
                </motion.div>
              )}

              <button
                type="submit"
                disabled={isSubmitting}
                className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isSubmitting ? (
                  'Creating...'
                ) : (
                  <>
                    Create Account
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </form>

            <div className="mt-6 pt-6 border-t border-[#222] text-center">
              <p className="text-gray-400 text-sm">
                Already have an account?{' '}
                <a
                  href="/dashboard/login"
                  className="text-[#00D4AA] hover:underline"
                >
                  Login
                </a>
              </p>
            </div>
          </div>
        ) : (
          /* Success Screen */
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            className="bg-[#111111] border border-[#222] rounded-2xl p-8"
          >
            <div className="flex justify-center mb-6">
              <div className="w-16 h-16 rounded-full bg-[#00D4AA]/20 flex items-center justify-center">
                <Check className="w-8 h-8 text-[#00D4AA]" />
              </div>
            </div>

            <h1 className="text-2xl font-bold text-white mb-2 text-center">
              Account Created!
            </h1>
            <p className="text-gray-400 mb-6 text-center">
              Save your account number. You&apos;ll need it to log in.
            </p>

            <div className="space-y-4 mb-6">
              <div className="bg-[#0a0a0a] border border-[#333] rounded-lg p-4">
                <label className="block text-xs text-gray-500 mb-1">
                  Your Account Number
                </label>
                <div className="font-mono text-xl text-white tracking-wider">
                  {accountNumber}
                </div>
              </div>

              <div className="bg-[#0a0a0a] border border-[#333] rounded-lg p-4">
                <label className="block text-xs text-gray-500 mb-1">
                  Your Wallet Address (Base USDC)
                </label>
                <div className="font-mono text-sm text-[#00D4AA] break-all">
                  {walletAddress}
                </div>
              </div>
            </div>

            <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-4 mb-6">
              <p className="text-yellow-400 text-sm">
                <strong>Important:</strong> Download your recovery file and
                store it securely. This is the only way to recover your account
                if you lose your account number.
              </p>
            </div>

            <div className="space-y-3">
              <button
                onClick={handleDownload}
                className="w-full py-3 px-4 bg-[#222] hover:bg-[#333] text-white font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                <Download className="w-4 h-4" />
                {isDownloading
                  ? 'Downloaded!'
                  : 'Download Recovery File'}
              </button>

              <button
                onClick={handleContinue}
                className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                Continue to Dashboard
                <ArrowRight className="w-4 h-4" />
              </button>
            </div>
          </motion.div>
        )}

        {/* Privacy Note */}
        <p className="text-center text-gray-500 text-sm mt-6">
          No personal information required. Your privacy is our priority.
        </p>
      </motion.div>
    </div>
  );
}
