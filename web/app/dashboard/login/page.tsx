'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { ArrowRight, AlertCircle } from 'lucide-react';
import Logo from '@/components/Logo';
import { useAuth } from '@/components/providers/AuthProvider';
import { formatAccountNumber, isValidAccountNumber } from '@/lib/utils';

type LoginTab = 'personal' | 'business';

export default function LoginPage() {
  const [tab, setTab] = useState<LoginTab>('personal');
  const [accountNumber, setAccountNumber] = useState('');
  const [error, setError] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const [useRecovery, setUseRecovery] = useState(false);
  const [ttlDays, setTtlDays] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const { login, verifyTotp, resetTotp, b2bSignIn, totpRequired, isAuthenticated, isLoading, needsOnboarding } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (isAuthenticated && !isLoading) {
      if (needsOnboarding) {
        router.replace('/dashboard/b2b/create');
      } else {
        router.replace('/dashboard/main');
      }
    }
  }, [isAuthenticated, isLoading, needsOnboarding, router]);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const formatted = formatAccountNumber(e.target.value);
    setAccountNumber(formatted);
    setError('');
  };

  const handleTabChange = (newTab: LoginTab) => {
    setTab(newTab);
    setError('');
  };

  const handleB2BSignIn = async () => {
    setError('');
    setIsSubmitting(true);
    try {
      await b2bSignIn();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign in failed');
      setIsSubmitting(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (totpRequired) {
      if (!totpCode.trim()) {
        setError('Please enter your TOTP or recovery code');
        return;
      }
      setIsSubmitting(true);
      try {
        await verifyTotp(totpCode.trim(), useRecovery, ttlDays);
        router.push('/dashboard/main');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'TOTP verification failed');
      } finally {
        setIsSubmitting(false);
      }
      return;
    }

    if (!isValidAccountNumber(accountNumber)) {
      setError('Please enter a valid 16-digit account number');
      return;
    }

    setIsSubmitting(true);
    try {
      const result = await login(accountNumber);
      if (!result.totpRequired) {
        router.push('/dashboard/main');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleGoBack = () => {
    setTotpCode('');
    setUseRecovery(false);
    setError('');
    resetTotp();
  };

  if (isLoading || isAuthenticated) {
    return (
      <div className="min-h-screen bg-[#0a0a0a] flex items-center justify-center">
        <div className="animate-pulse text-[#00D4AA]">Loading...</div>
      </div>
    );
  }

  const isB2CFormValid = accountNumber.length >= 19;

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

        {/* Login Card */}
        <div className="bg-[#111111] border border-[#222] rounded-2xl p-8">
          <h1 className="text-2xl font-bold text-white mb-2">Welcome back</h1>

          {/* Tab Toggle - only show when TOTP is not required */}
          {!totpRequired && (
            <div role="tablist" className="flex bg-[#0a0a0a] rounded-lg p-1 mb-6">
              <button
                type="button"
                role="tab"
                aria-selected={tab === 'personal'}
                onClick={() => handleTabChange('personal')}
                className={`flex-1 py-2 px-4 rounded-md text-sm font-medium transition-colors ${
                  tab === 'personal'
                    ? 'bg-[#222] text-white'
                    : 'text-gray-400 hover:text-gray-300'
                }`}
              >
                Personal
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={tab === 'business'}
                onClick={() => handleTabChange('business')}
                className={`flex-1 py-2 px-4 rounded-md text-sm font-medium transition-colors ${
                  tab === 'business'
                    ? 'bg-[#222] text-white'
                    : 'text-gray-400 hover:text-gray-300'
                }`}
              >
                Business
              </button>
            </div>
          )}

          <p className="text-gray-400 mb-6">
            {totpRequired
              ? 'Enter your TOTP or recovery code to trust this device'
              : tab === 'business'
                ? 'Sign in with your business account via SSO'
                : 'Enter your account number to access your dashboard'}
          </p>

          {tab === 'business' && !totpRequired ? (
            // B2B: Single SSO button
            <div className="space-y-4">
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
                type="button"
                onClick={handleB2BSignIn}
                disabled={isSubmitting}
                className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isSubmitting ? (
                  'Redirecting...'
                ) : (
                  <>
                    Sign in with SSO
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </div>
          ) : (
            // B2C: Account number + TOTP form
            <form onSubmit={handleSubmit} className="space-y-4">
              {totpRequired ? (
                <>
                  <div>
                    <label
                      htmlFor="totpCode"
                      className="block text-sm font-medium text-gray-300 mb-2"
                    >
                      {useRecovery ? 'Recovery Code' : 'TOTP Code'}
                    </label>
                    <input
                      type="text"
                      id="totpCode"
                      value={totpCode}
                      onChange={(e) => setTotpCode(e.target.value)}
                      placeholder={useRecovery ? 'XXXX-XXXX-XXXX-XXXX' : '123456'}
                      className="w-full px-4 py-3 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors font-mono text-lg tracking-wider"
                    />
                  </div>
                  <div className="flex items-center gap-2 text-sm text-gray-400">
                    <input
                      id="useRecovery"
                      type="checkbox"
                      checked={useRecovery}
                      onChange={(e) => setUseRecovery(e.target.checked)}
                      className="h-4 w-4 rounded border-[#333] bg-[#0a0a0a] text-[#00D4AA] focus:ring-[#00D4AA]"
                    />
                    <label htmlFor="useRecovery">Use recovery code</label>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-300 mb-2">
                      Trust this device for
                    </label>
                    <select
                      value={ttlDays}
                      onChange={(e) => setTtlDays(Number(e.target.value))}
                      className="w-full px-3 py-2 bg-[#0a0a0a] border border-[#333] rounded-lg text-white"
                    >
                      <option value={0}>Indefinitely (default)</option>
                      <option value={30}>30 days</option>
                      <option value={90}>90 days</option>
                    </select>
                  </div>
                  <button
                    type="button"
                    onClick={handleGoBack}
                    className="w-full py-2 px-4 bg-[#0a0a0a] border border-[#333] text-gray-300 rounded-lg hover:border-[#00D4AA] hover:text-white transition-colors"
                  >
                    Go Back
                  </button>
                </>
              ) : (
                <div>
                  <label
                    htmlFor="accountNumber"
                    className="block text-sm font-medium text-gray-300 mb-2"
                  >
                    Account Number
                  </label>
                  <input
                    type="text"
                    id="accountNumber"
                    value={accountNumber}
                    onChange={handleInputChange}
                    placeholder="XXXX-XXXX-XXXX-XXXX"
                    maxLength={19}
                    className="w-full px-4 py-3 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors font-mono text-lg tracking-wider"
                  />
                </div>
              )}

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
                disabled={
                  isSubmitting ||
                  (totpRequired && totpCode.trim().length === 0) ||
                  (!totpRequired && !isB2CFormValid)
                }
                className="w-full py-3 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors flex items-center justify-center gap-2"
              >
                {isSubmitting ? (
                  totpRequired ? 'Verifying...' : 'Logging in...'
                ) : (
                  <>
                    {totpRequired ? 'Verify' : 'Login'}
                    <ArrowRight className="w-4 h-4" />
                  </>
                )}
              </button>
            </form>
          )}

          {!totpRequired && (
            <div className="mt-6 pt-6 border-t border-[#222] text-center">
              {tab === 'business' ? (
                <p className="text-gray-400 text-sm">
                  New to Stronghold?{' '}
                  <button
                    onClick={handleB2BSignIn}
                    className="text-[#00D4AA] hover:underline"
                  >
                    Create a business account
                  </button>
                </p>
              ) : (
                <p className="text-gray-400 text-sm">
                  Don&apos;t have an account?{' '}
                  <a
                    href="/dashboard/create"
                    className="text-[#00D4AA] hover:underline"
                  >
                    Create one
                  </a>
                </p>
              )}
            </div>
          )}
        </div>

        {/* Security Note */}
        <p className="text-center text-gray-500 text-sm mt-6">
          {totpRequired
            ? 'This device is not trusted yet. Enter your TOTP to continue.'
            : tab === 'business'
              ? 'Business accounts use single sign-on via WorkOS.'
              : 'Your account number is your password. Store it securely.'}
        </p>
      </motion.div>
    </div>
  );
}
