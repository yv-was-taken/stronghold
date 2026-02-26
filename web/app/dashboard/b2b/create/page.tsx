'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { ArrowRight, AlertCircle } from 'lucide-react';
import Logo from '@/components/Logo';
import { useAuth } from '@/components/providers/AuthProvider';

export default function B2BOnboardPage() {
  const [companyName, setCompanyName] = useState('');
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const { isAuthenticated, isLoading, needsOnboarding, onboardB2B, account } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      // Not authenticated — redirect to login
      router.replace('/dashboard/login');
    } else if (!isLoading && isAuthenticated && !needsOnboarding) {
      // Already onboarded — go to dashboard
      router.replace('/dashboard/main');
    }
  }, [isAuthenticated, isLoading, needsOnboarding, router]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!companyName.trim()) {
      setError('Please enter your company name');
      return;
    }

    setIsSubmitting(true);
    try {
      await onboardB2B(companyName.trim());
      router.push('/dashboard/main');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Onboarding failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  if (isLoading || !isAuthenticated || !needsOnboarding) {
    return (
      <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center">
        <div className="animate-pulse text-[#00D4AA]">Loading...</div>
      </div>
    );
  }

  const isFormValid = companyName.trim().length > 0;

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
            <Logo size={48} />
            <span className="text-2xl font-bold text-white">Stronghold</span>
          </div>
        </div>

        {/* Onboarding Card */}
        <div className="bg-[#111111] border border-[#222] rounded-2xl p-8">
          <h1 className="text-2xl font-bold text-white mb-2">Welcome to Stronghold</h1>
          <p className="text-gray-400 mb-6">
            One last step — tell us your company name to complete setup.
            {account?.email && (
              <span className="block mt-1 text-gray-500 text-sm">
                Signed in as {account.email}
              </span>
            )}
          </p>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="companyName"
                className="block text-sm font-medium text-gray-300 mb-2"
              >
                Company Name
              </label>
              <input
                type="text"
                id="companyName"
                value={companyName}
                onChange={(e) => { setCompanyName(e.target.value); setError(''); }}
                placeholder="Acme Inc."
                autoFocus
                className="w-full px-4 py-3 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors"
              />
            </div>

            {error && (
              <motion.div
                role="alert"
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
              disabled={isSubmitting || !isFormValid}
              className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
            >
              {isSubmitting ? (
                'Setting up...'
              ) : (
                <>
                  Continue
                  <ArrowRight className="w-4 h-4" />
                </>
              )}
            </button>
          </form>
        </div>

        {/* Note */}
        <p className="text-center text-gray-500 text-sm mt-6">
          Business accounts include API key management and Stripe billing.
        </p>
      </motion.div>
    </div>
  );
}
