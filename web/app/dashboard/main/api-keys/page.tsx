'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  Plus,
  Key,
  Trash2,
  AlertCircle,
  Copy,
  Check,
  AlertTriangle,
} from 'lucide-react';
import { useAuth } from '@/components/providers/AuthProvider';
import { Skeleton } from '@/components/ui/Skeleton';
import { listAPIKeys, createAPIKey, revokeAPIKey, type APIKeyItem } from '@/lib/api';
import { copyToClipboard, formatRelativeTime } from '@/lib/utils';

export default function APIKeysPage() {
  const { account, isAuthenticated, isLoading: authLoading } = useAuth();
  const router = useRouter();

  const [keys, setKeys] = useState<APIKeyItem[]>([]);
  const [isLoadingKeys, setIsLoadingKeys] = useState(true);
  const [error, setError] = useState('');

  // Create key state
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [createdKeyCopied, setCreatedKeyCopied] = useState(false);

  // Revoke state
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [confirmRevokeId, setConfirmRevokeId] = useState<string | null>(null);

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.replace('/dashboard/login');
    }
  }, [isAuthenticated, authLoading, router]);

  const loadKeys = useCallback(async () => {
    setIsLoadingKeys(true);
    try {
      const data = await listAPIKeys();
      setKeys(data.api_keys || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load API keys');
    } finally {
      setIsLoadingKeys(false);
    }
  }, []);

  useEffect(() => {
    if (isAuthenticated) {
      loadKeys();
    }
  }, [isAuthenticated, loadKeys]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!newKeyName.trim()) {
      setError('Please enter a name for the API key');
      return;
    }

    setIsCreating(true);
    try {
      const result = await createAPIKey(newKeyName.trim());
      setCreatedKey(result.key);
      setNewKeyName('');
      setShowCreateForm(false);
      await loadKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create API key');
    } finally {
      setIsCreating(false);
    }
  };

  const handleRevoke = async (id: string) => {
    setError('');
    setRevokingId(id);
    try {
      await revokeAPIKey(id);
      setConfirmRevokeId(null);
      await loadKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke API key');
    } finally {
      setRevokingId(null);
    }
  };

  const handleCopyCreatedKey = async () => {
    if (createdKey) {
      const success = await copyToClipboard(createdKey);
      if (success) {
        setCreatedKeyCopied(true);
        setTimeout(() => setCreatedKeyCopied(false), 2000);
      }
    }
  };

  if (authLoading || !account) {
    return (
      <div className="min-h-screen bg-[#0a0a0a]">
        <header className="border-b border-[#222]">
          <div className="max-w-2xl mx-auto px-4 py-4 flex items-center gap-4">
            <Skeleton className="w-9 h-9 rounded" />
            <Skeleton className="w-32 h-6" />
          </div>
        </header>
        <main className="max-w-2xl mx-auto px-4 py-8 space-y-4">
          <Skeleton className="w-full h-20 rounded-2xl" />
          <Skeleton className="w-full h-20 rounded-2xl" />
          <Skeleton className="w-full h-20 rounded-2xl" />
        </main>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-[#0a0a0a]">
      {/* Header */}
      <header className="border-b border-[#222]">
        <div className="max-w-2xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <a
              href="/dashboard/main"
              className="p-2 text-gray-400 hover:text-white transition-colors"
            >
              <ArrowLeft className="w-5 h-5" />
            </a>
            <h1 className="text-xl font-bold text-white">API Keys</h1>
          </div>
          {!showCreateForm && !createdKey && (
            <button
              onClick={() => { setShowCreateForm(true); setError(''); }}
              className="flex items-center gap-2 py-2 px-4 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors text-sm"
            >
              <Plus className="w-4 h-4" />
              Create Key
            </button>
          )}
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-2xl mx-auto px-4 py-8 space-y-6">
        {/* Newly Created Key Banner */}
        {createdKey && (
          <motion.div
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
            className="bg-[#00D4AA]/10 border border-[#00D4AA]/30 rounded-2xl p-6"
          >
            <div className="flex items-start gap-3 mb-3">
              <AlertTriangle className="w-5 h-5 text-[#00D4AA] flex-shrink-0 mt-0.5" />
              <div>
                <h3 className="text-white font-semibold mb-1">Save your API key</h3>
                <p className="text-gray-400 text-sm">
                  This is the only time you will see this key. Copy it now and store it securely.
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 bg-[#0a0a0a] rounded-lg p-3">
              <code className="flex-1 font-mono text-white text-sm break-all">
                {createdKey}
              </code>
              <button
                onClick={handleCopyCreatedKey}
                className="p-2 text-gray-400 hover:text-white transition-colors flex-shrink-0"
                title={createdKeyCopied ? 'Copied' : 'Copy'}
              >
                {createdKeyCopied ? (
                  <Check className="w-4 h-4 text-[#00D4AA]" />
                ) : (
                  <Copy className="w-4 h-4" />
                )}
              </button>
            </div>
            <button
              onClick={() => setCreatedKey(null)}
              className="mt-3 text-gray-400 hover:text-white text-sm transition-colors"
            >
              Dismiss
            </button>
          </motion.div>
        )}

        {/* Create Key Form */}
        {showCreateForm && (
          <motion.div
            initial={{ opacity: 0, y: -10 }}
            animate={{ opacity: 1, y: 0 }}
            className="bg-[#111] border border-[#222] rounded-2xl p-6"
          >
            <h2 className="text-lg font-semibold text-white mb-4">Create New API Key</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label
                  htmlFor="keyName"
                  className="block text-sm font-medium text-gray-300 mb-2"
                >
                  Key Name
                </label>
                <input
                  type="text"
                  id="keyName"
                  value={newKeyName}
                  onChange={(e) => setNewKeyName(e.target.value)}
                  placeholder="e.g. Production, Staging, CI/CD"
                  className="w-full px-4 py-3 bg-[#0a0a0a] border border-[#333] rounded-lg text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] focus:ring-1 focus:ring-[#00D4AA] transition-colors"
                  autoFocus
                />
              </div>
              <div className="flex gap-3">
                <button
                  type="submit"
                  disabled={isCreating || !newKeyName.trim()}
                  className="flex-1 py-2.5 px-4 bg-[#00D4AA] hover:bg-[#00b894] disabled:bg-[#004d3d] disabled:cursor-not-allowed text-black font-semibold rounded-lg transition-colors"
                >
                  {isCreating ? 'Creating...' : 'Create'}
                </button>
                <button
                  type="button"
                  onClick={() => { setShowCreateForm(false); setNewKeyName(''); setError(''); }}
                  className="py-2.5 px-4 bg-[#222] hover:bg-[#333] text-white font-semibold rounded-lg transition-colors"
                >
                  Cancel
                </button>
              </div>
            </form>
          </motion.div>
        )}

        {/* Error */}
        {error && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            className="flex items-center gap-2 text-red-400 text-sm bg-red-500/10 border border-red-500/20 rounded-lg p-3"
          >
            <AlertCircle className="w-4 h-4 flex-shrink-0" />
            {error}
          </motion.div>
        )}

        {/* Key List */}
        {isLoadingKeys ? (
          <div className="space-y-4">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="w-full h-20 rounded-2xl" />
            ))}
          </div>
        ) : keys.length === 0 ? (
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            className="bg-[#111] border border-[#222] rounded-2xl p-8 text-center"
          >
            <div className="w-16 h-16 rounded-full bg-[#222] flex items-center justify-center mx-auto mb-4">
              <Key className="w-8 h-8 text-gray-500" />
            </div>
            <h3 className="text-white font-semibold mb-2">No API keys yet</h3>
            <p className="text-gray-400 text-sm mb-4">
              Create an API key to start making authenticated requests.
            </p>
            <button
              onClick={() => setShowCreateForm(true)}
              className="inline-flex items-center gap-2 py-2.5 px-4 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors text-sm"
            >
              <Plus className="w-4 h-4" />
              Create Your First Key
            </button>
          </motion.div>
        ) : (
          <div className="space-y-3">
            {keys.map((apiKey, index) => (
              <motion.div
                key={apiKey.id}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: index * 0.05 }}
                className="bg-[#111] border border-[#222] rounded-2xl p-5"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex items-start gap-3 min-w-0">
                    <div className="w-10 h-10 rounded-lg bg-[#00D4AA]/10 flex items-center justify-center flex-shrink-0">
                      <Key className="w-5 h-5 text-[#00D4AA]" />
                    </div>
                    <div className="min-w-0">
                      <div className="text-white font-medium">{apiKey.name}</div>
                      <div className="font-mono text-gray-400 text-sm mt-0.5">
                        {apiKey.key_prefix}...
                      </div>
                      <div className="text-gray-500 text-xs mt-1">
                        Created {formatRelativeTime(apiKey.created_at)}
                        {apiKey.last_used_at && (
                          <> &middot; Last used {formatRelativeTime(apiKey.last_used_at)}</>
                        )}
                      </div>
                    </div>
                  </div>

                  {confirmRevokeId === apiKey.id ? (
                    <div className="flex items-center gap-2 flex-shrink-0">
                      <button
                        onClick={() => handleRevoke(apiKey.id)}
                        disabled={revokingId === apiKey.id}
                        className="py-1.5 px-3 bg-red-500/20 hover:bg-red-500/30 text-red-400 text-sm rounded-lg transition-colors disabled:opacity-50"
                      >
                        {revokingId === apiKey.id ? 'Revoking...' : 'Confirm'}
                      </button>
                      <button
                        onClick={() => setConfirmRevokeId(null)}
                        className="py-1.5 px-3 bg-[#222] hover:bg-[#333] text-gray-300 text-sm rounded-lg transition-colors"
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setConfirmRevokeId(apiKey.id)}
                      className="p-2 text-gray-500 hover:text-red-400 transition-colors flex-shrink-0"
                      title="Revoke key"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  )}
                </div>
              </motion.div>
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
