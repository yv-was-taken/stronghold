'use client'

import { motion } from 'framer-motion'
import { Check, Zap, CreditCard, Bot, Building2 } from 'lucide-react'
import Button from '../ui/Button'

const sharedFeatures = [
  'Managed infrastructure',
  'Automatic scaling',
  'Sub-50ms latency SLA',
  'Usage dashboard',
]

const b2cFeatures = [
  'x402 crypto payments (USDC)',
  'Autonomous agent billing',
  'No human in the loop',
  'CLI + transparent proxy',
]

const b2bFeatures = [
  'Card billing via Stripe',
  'Prepaid credits + metered overflow',
  'API keys for server-to-server',
  'SSO login (WorkOS)',
]

export default function Pricing() {
  return (
    <section id="pricing" className="py-24 relative">
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
            Pricing
          </span>
          <h2 className="font-display text-3xl sm:text-4xl lg:text-5xl font-bold mt-4 mb-6">
            Simple, Transparent{' '}
            <span className="gradient-text">Pricing</span>
          </h2>
          <p className="text-gray-400 text-lg max-w-2xl mx-auto">
            Same price, two payment paths. Choose crypto for autonomous agents
            or card billing for business integration.
          </p>
        </motion.div>

        {/* Unified Price */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5 }}
          className="text-center mb-12"
        >
          <div className="flex items-baseline gap-3 justify-center mb-2">
            <span className="font-display text-6xl sm:text-7xl font-bold">
              $1
            </span>
            <span className="text-gray-400 text-lg">
              per 1,000 scans
            </span>
          </div>
          <p className="text-gray-500 text-sm">
            $0.001 per request — both payment methods, same rate
          </p>
        </motion.div>

        {/* Two Paths */}
        <div className="grid md:grid-cols-2 gap-6 max-w-4xl mx-auto">
          {/* B2C Card */}
          <motion.div
            initial={{ opacity: 0, y: 30 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.5 }}
            className="rounded-xl bg-stronghold-stone/50 border border-stronghold-stone-light/30 p-8"
          >
            <div className="flex items-center gap-3 mb-2">
              <div className="w-10 h-10 rounded-lg bg-stronghold-cyan/10 flex items-center justify-center">
                <Bot className="text-stronghold-cyan" size={20} />
              </div>
              <h3 className="font-display text-xl font-bold">Individual</h3>
            </div>
            <p className="text-gray-500 text-sm mb-6">
              For developers running autonomous AI agents
            </p>

            <ul className="space-y-3 mb-6">
              {b2cFeatures.map((feature) => (
                <li key={feature} className="flex items-center gap-3">
                  <Check className="text-stronghold-cyan flex-shrink-0" size={18} />
                  <span className="text-gray-300 text-sm">{feature}</span>
                </li>
              ))}
            </ul>

            <div className="pt-4 border-t border-stronghold-stone-light/20">
              <p className="text-gray-500 text-xs mb-4">
                Agents pay per-request with USDC wallets — no human intervention needed for billing.
              </p>
              <Button variant="secondary" size="md" href="/dashboard/create" className="w-full">
                Get Started
              </Button>
            </div>
          </motion.div>

          {/* B2B Card */}
          <motion.div
            initial={{ opacity: 0, y: 30 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.5, delay: 0.1 }}
            className="rounded-xl bg-stronghold-stone/50 border border-stronghold-stone-light/30 p-8"
          >
            <div className="flex items-center gap-3 mb-2">
              <div className="w-10 h-10 rounded-lg bg-stronghold-cyan/10 flex items-center justify-center">
                <Building2 className="text-stronghold-cyan" size={20} />
              </div>
              <h3 className="font-display text-xl font-bold">Business</h3>
            </div>
            <p className="text-gray-500 text-sm mb-6">
              For companies integrating scanning into their stack
            </p>

            <ul className="space-y-3 mb-6">
              {b2bFeatures.map((feature) => (
                <li key={feature} className="flex items-center gap-3">
                  <Check className="text-stronghold-cyan flex-shrink-0" size={18} />
                  <span className="text-gray-300 text-sm">{feature}</span>
                </li>
              ))}
            </ul>

            <div className="pt-4 border-t border-stronghold-stone-light/20">
              <p className="text-gray-500 text-xs mb-4">
                Use API keys with standard card billing — no crypto wallet or blockchain knowledge required.
              </p>
              <Button variant="primary" size="md" href="/dashboard/login" className="w-full">
                Sign In with SSO
              </Button>
            </div>
          </motion.div>
        </div>

        {/* Shared Features */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6, delay: 0.2 }}
          className="mt-10 flex flex-wrap justify-center gap-x-8 gap-y-3"
        >
          {sharedFeatures.map((feature) => (
            <div key={feature} className="flex items-center gap-2 text-gray-500 text-sm">
              <Check size={14} className="text-stronghold-cyan" />
              {feature}
            </div>
          ))}
        </motion.div>
      </div>
    </section>
  )
}
