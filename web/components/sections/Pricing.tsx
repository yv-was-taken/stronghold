'use client'

import { motion } from 'framer-motion'
import { Check, Zap } from 'lucide-react'
import Button from '../ui/Button'

const features = [
  'Managed infrastructure',
  'Automatic scaling',
  'Usage dashboard',
  'x402 crypto payments',
  'Sub-50ms latency SLA',
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
            Pay only for what you use. No subscriptions, no hidden fees.
            Powered by the x402 payment protocol.
          </p>
        </motion.div>

        {/* Pricing Content - Two Column Layout */}
        <motion.div
          initial={{ opacity: 0, y: 30 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5 }}
          className="max-w-2xl mx-auto"
        >
          <div className="rounded-xl bg-stronghold-stone/50 border border-stronghold-stone-light/30 p-8 md:p-12">
            <div className="grid md:grid-cols-2 gap-8 md:gap-12 items-center">
              {/* Price Column */}
              <div className="text-center">
                <div className="flex items-baseline gap-3 justify-center mb-6">
                  <span className="font-display text-6xl sm:text-7xl font-bold">
                    $1
                  </span>
                  <span className="text-gray-400 text-lg">
                    per 1,000 scans
                  </span>
                </div>
                <Button variant="primary" size="lg" href="/dashboard/create">
                  Start Scanning
                </Button>
              </div>

              {/* Features Column */}
              <div>
                <ul className="space-y-4">
                  {features.map((feature) => (
                    <li key={feature} className="flex items-center gap-3">
                      <Check
                        className="text-stronghold-cyan flex-shrink-0"
                        size={20}
                      />
                      <span className="text-gray-300">{feature}</span>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        </motion.div>

        {/* x402 Note */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6, delay: 0.2 }}
          className="mt-12 text-center"
        >
          <div className="inline-flex items-center gap-3 px-6 py-3 rounded-full bg-stronghold-stone/30 border border-stronghold-stone-light/30">
            <Zap className="text-stronghold-cyan" size={18} />
            <span className="text-sm text-gray-400">
              Payments powered by{' '}
              <span className="text-stronghold-cyan font-mono">x402</span> —
              the open standard for internet payments
            </span>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
