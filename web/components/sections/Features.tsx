'use client'

import { motion } from 'framer-motion'
import {
  Shield,
  Lock,
  Network,
  Wallet,
  Zap,
} from 'lucide-react'

const features = [
  {
    icon: Shield,
    title: 'Prompt Injection Detection',
    description: '4-layer scanning architecture: heuristics, ML classification, semantic similarity, and LLM classification catch even sophisticated attacks.',
    highlight: '4-Layer Defense',
  },
  {
    icon: Lock,
    title: 'Credential Leak Prevention',
    description: 'Scans LLM outputs for API keys, passwords, and sensitive data patterns. Blocks accidental exfiltration before it reaches users.',
    highlight: 'Output Protection',
  },
  {
    icon: Network,
    title: 'Transparent Proxy',
    description: 'System-wide protection at the network level. No code changes, no environment variables, no proxy configuration needed.',
    highlight: 'Zero Config',
  },
  {
    icon: Wallet,
    title: 'Pay As You Go',
    description: 'Pay-per-scan with no subscriptions or upfront costs. Only pay for what you use. Top up via dashboard with card or crypto.',
    highlight: '$0.001/scan',
  },
  {
    icon: Zap,
    title: 'Real-time Blocking',
    description: 'Sub-50ms latency with instant ALLOW/WARN/BLOCK decisions. Malicious requests stopped before they reach your AI models.',
    highlight: '<50ms Latency',
  },
]

export default function Features() {
  return (
    <section id="features" className="py-24 relative">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Section Header */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="text-center mb-16"
        >
          <span className="text-stronghold-cyan font-mono text-sm tracking-wider uppercase">
            Features
          </span>
          <h2 className="font-display text-3xl sm:text-4xl lg:text-5xl font-bold mt-4 mb-6">
            Everything You Need to{' '}
            <span className="gradient-text">Secure AI</span>
          </h2>
          <p className="text-gray-400 text-lg max-w-2xl mx-auto">
            A complete security layer designed specifically for AI infrastructure.
            Self-hosted, open source, and pay-as-you-go.
          </p>
        </motion.div>

        {/* Features Grid */}
        <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
          {features.map((feature, index) => (
            <motion.div
              key={feature.title}
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: index * 0.1 }}
              whileHover={{ y: -4 }}
              className="fortress-card rounded-xl p-8 group cursor-pointer"
            >
              {/* Highlight badge */}
              <span className="inline-block px-2 py-1 rounded text-xs font-mono bg-stronghold-cyan/10 text-stronghold-cyan mb-4">
                {feature.highlight}
              </span>

              {/* Icon */}
              <div className="w-12 h-12 rounded-lg bg-stronghold-stone-light/50 flex items-center justify-center mb-4 group-hover:bg-stronghold-cyan/10 transition-colors">
                <feature.icon
                  className="text-gray-400 group-hover:text-stronghold-cyan transition-colors"
                  size={24}
                />
              </div>

              {/* Content */}
              <h3 className="font-display text-xl font-semibold mb-3">
                {feature.title}
              </h3>
              <p className="text-gray-400 text-sm leading-relaxed">
                {feature.description}
              </p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
