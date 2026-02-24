'use client';

import { useState, useEffect, useCallback } from 'react';
import { motion } from 'framer-motion';
import {
  ArrowLeft,
  Plus,
  Copy,
  Check,
  Key,
  AlertTriangle,
  Trash2,
} from 'lucide-react';
import { useAuth } from '@/components/providers/AuthProvider';
import { copyToClipboard } from '@/lib/utils';
import {
  createAPIKey,
  listAPIKeys,
  revokeAPIKey,
  type APIKey,
  type CreateAPIKeyResponse,
} from '@/lib/api';

export default function APIKeysPage() {
  const { account } = useAuth();
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Create form state
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [newLabel, setNewLabel] = useState('');
  const [creating, setCreating] = useState(false);

  // Newly created key (shown once)
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResponse | null>(null);
  const [keyCopied, setKeyCopied] = useState(false);

  // Revoke confirmation
  const [revokingId, setRevokingId] = useState<string | null>(null);

  const loadKeys = useCallback(async () => {
    try {
      const data = await listAPIKeys();
      setKeys(data);
      setError(null);
    } catch (err) {
      console.error('Failed to load API keys:', err);
      setError('Failed to load API keys');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadKeys();
  }, [loadKeys]);

  const handleCreate = async () => {
    if (!newLabel.trim()) return;
    setCreating(true);
    try {
      const result = await createAPIKey(newLabel.trim());
      setCreatedKey(result);
      setNewLabel('');
      setShowCreateForm(false);
      await loadKeys();
    } catch (err) {
      console.error('Failed to create API key:', err);
      setError(err instanceof Error ? err.message : 'Failed to create API key');
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (id: string) => {
    setRevokingId(id);
    try {
      await revokeAPIKey(id);
      await loadKeys();
    } catch (err) {
      console.error('Failed to revoke API key:', err);
      setError(err instanceof Error ? err.message : 'Failed to revoke API key');
    } finally {
      setRevokingId(null);
    }
  };

  const handleCopyKey = async (key: string) => {
    const success = await copyToClipboard(key);
    if (success) {
      setKeyCopied(true);
      setTimeout(() => setKeyCopied(false), 2000);
    }
  };

  const formatDate = (dateStr: string | null) => {
    if (!dateStr) return 'Never';
    return new Date(dateStr).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
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
          <h1 className="text-xl font-bold text-white">API Keys</h1>
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
                <h3 className="text-[#00D4AA] font-semibold">
                  API Key Created
                </h3>
                <p className="text-[#00D4AA]/80 text-sm mt-1">
                  Copy this key now. It will not be shown again.
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 mt-3">
              <code className="flex-1 font-mono text-sm text-white bg-[#0a0a0a] rounded-lg px-3 py-2 break-all">
                {createdKey.key}
              </code>
              <button
                onClick={() => handleCopyKey(createdKey.key)}
                className="p-2 text-gray-400 hover:text-white transition-colors flex-shrink-0"
                title="Copy API key"
              >
                {keyCopied ? (
                  <Check className="w-4 h-4 text-[#00D4AA]" />
                ) : (
                  <Copy className="w-4 h-4" />
                )}
              </button>
            </div>
            <button
              onClick={() => setCreatedKey(null)}
              className="mt-3 text-sm text-gray-400 hover:text-white transition-colors"
            >
              Dismiss
            </button>
          </motion.div>
        )}

        {/* Create Key Section */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <Key className="w-5 h-5 text-[#00D4AA]" />
              Manage API Keys
            </h2>
            {!showCreateForm && (
              <button
                onClick={() => setShowCreateForm(true)}
                className="flex items-center gap-2 px-3 py-1.5 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors text-sm"
              >
                <Plus className="w-4 h-4" />
                Create API Key
              </button>
            )}
          </div>

          {/* Create Form */}
          {showCreateForm && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              className="mb-4"
            >
              <div className="bg-[#0a0a0a] rounded-lg p-4 border border-[#222]">
                <label className="block text-sm text-gray-400 mb-2">
                  Key Label
                </label>
                <input
                  type="text"
                  value={newLabel}
                  onChange={(e) => setNewLabel(e.target.value)}
                  placeholder="e.g., Production Server, CI/CD Pipeline"
                  className="w-full bg-[#111] border border-[#333] rounded-lg px-3 py-2 text-white placeholder-gray-600 focus:outline-none focus:border-[#00D4AA] transition-colors"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleCreate();
                  }}
                  autoFocus
                />
                <div className="flex items-center gap-2 mt-3">
                  <button
                    onClick={handleCreate}
                    disabled={!newLabel.trim() || creating}
                    className="px-4 py-1.5 bg-[#00D4AA] hover:bg-[#00b894] text-black font-semibold rounded-lg transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {creating ? 'Creating...' : 'Create'}
                  </button>
                  <button
                    onClick={() => {
                      setShowCreateForm(false);
                      setNewLabel('');
                    }}
                    className="px-4 py-1.5 bg-[#222] hover:bg-[#333] text-white rounded-lg transition-colors text-sm"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </motion.div>
          )}

          {/* Error */}
          {error && (
            <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm">
              {error}
            </div>
          )}

          {/* Keys List */}
          {loading ? (
            <div className="space-y-3">
              {[1, 2].map((i) => (
                <div
                  key={i}
                  className="bg-[#0a0a0a] rounded-lg p-4 animate-pulse"
                >
                  <div className="h-4 bg-[#222] rounded w-1/3 mb-2" />
                  <div className="h-3 bg-[#222] rounded w-1/4" />
                </div>
              ))}
            </div>
          ) : keys.length === 0 ? (
            <div className="text-center py-8">
              <Key className="w-10 h-10 text-gray-600 mx-auto mb-3" />
              <p className="text-gray-400 mb-1">No API keys yet</p>
              <p className="text-gray-600 text-sm">
                Create an API key to authenticate B2B API requests.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {keys.map((apiKey) => {
                const isRevoked = !!apiKey.revoked_at;
                return (
                  <motion.div
                    key={apiKey.id}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    className={`bg-[#0a0a0a] rounded-lg p-4 border border-[#222] ${
                      isRevoked ? 'opacity-60' : ''
                    }`}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2 mb-1">
                          <span className="text-white font-medium truncate">
                            {apiKey.label}
                          </span>
                          <span
                            className={`text-xs px-2 py-0.5 rounded ${
                              isRevoked
                                ? 'bg-red-500/10 text-red-400'
                                : 'bg-[#00D4AA]/10 text-[#00D4AA]'
                            }`}
                          >
                            {isRevoked ? 'Revoked' : 'Active'}
                          </span>
                        </div>
                        <code className="text-sm text-gray-500 font-mono">
                          {apiKey.key_prefix}...
                        </code>
                        <div className="flex items-center gap-4 mt-2 text-xs text-gray-600">
                          <span>Created {formatDate(apiKey.created_at)}</span>
                          <span>
                            Last used{' '}
                            {apiKey.last_used_at
                              ? formatDate(apiKey.last_used_at)
                              : 'never'}
                          </span>
                        </div>
                      </div>
                      {!isRevoked && (
                        <button
                          onClick={() => handleRevoke(apiKey.id)}
                          disabled={revokingId === apiKey.id}
                          className="p-2 text-gray-500 hover:text-red-400 transition-colors flex-shrink-0 disabled:opacity-50"
                          title="Revoke API key"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      )}
                    </div>
                  </motion.div>
                );
              })}
            </div>
          )}
        </motion.div>

        {/* Info */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.1 }}
          className="bg-[#111] border border-[#222] rounded-2xl p-6"
        >
          <h3 className="text-sm font-semibold text-gray-400 mb-3">
            About API Keys
          </h3>
          <ul className="space-y-2 text-sm text-gray-500">
            <li>
              API keys authenticate requests to the B2B scanning endpoints.
            </li>
            <li>
              Include your key in the{' '}
              <code className="text-gray-400 bg-[#0a0a0a] px-1.5 py-0.5 rounded">
                X-API-Key
              </code>{' '}
              header as{' '}
              <code className="text-gray-400 bg-[#0a0a0a] px-1.5 py-0.5 rounded">
                X-API-Key: &lt;key&gt;
              </code>
            </li>
            <li>
              Revoked keys stop working immediately and cannot be restored.
            </li>
          </ul>
        </motion.div>
      </main>
    </div>
  );
}
