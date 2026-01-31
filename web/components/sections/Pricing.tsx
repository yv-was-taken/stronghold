'use client'

import { motion } from 'framer-motion'
import { Check, Zap, Building2 } from 'lucide-react'
import Button from '../ui/Button'

const plans = [
  {
    name: 'Self-Hosted',
    description: 'Run your own instance',
    price: 'Free',
    priceNote: 'Open source',
    features: [
      'Full source code access',
      'Self-hosted proxy',
      'Bring your own API keys',
      'Community support',
      'Manual x402 payments',
    ],
    cta: 'Get Started',
    variant: 'secondary' as const,
    popular: false,
  },
  {
    name: 'Pay Per Scan',
    description: 'Usage-based pricing',
    price: '$0.001–0.005',
    priceNote: 'per scan',
    features: [
      'Managed infrastructure',
      'Automatic scaling',
      'Usage dashboard',
      'Email support',
      'x402 crypto payments',
      'Sub-50ms latency SLA',
    ],
    cta: 'Start Scanning',
    variant: 'primary' as const,
    popular: true,
  },
  {
    name: 'Enterprise',
    description: 'For large organizations',
    price: 'Custom',
    priceNote: 'Contact us',
    features: [
      'Dedicated infrastructure',
      'Custom ML models',
      'SSO & audit logs',
      '24/7 phone support',
      'Custom integrations',
      'SLA guarantees',
    ],
    cta: 'Contact Sales',
    variant: 'outline' as const,
    popular: false,
  },
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

        {/* Pricing Cards */}
        <div className="grid md:grid-cols-3 gap-8">
          {plans.map((plan, index) => (
            <motion.div
              key={plan.name}
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: index * 0.1 }}
              className={`relative ${plan.popular ? 'md:-mt-4 md:mb-4' : ''}`}
            >
              {plan.popular && (
                <div className="absolute -top-4 left-1/2 -translate-x-1/2">
                  <span className="px-4 py-1 rounded-full bg-stronghold-cyan text-stronghold-darker text-xs font-mono font-semibold">
                    Most Popular
                  </span>
                </div>
              )}

              <div
                className={`fortress-card rounded-xl p-8 h-full flex flex-col ${
                  plan.popular ? 'border-stronghold-cyan/50' : ''
                }`}
              >
                {/* Plan Header */}
                <div className="mb-6">
                  <h3 className="font-display text-2xl font-semibold mb-2">
                    {plan.name}
                  </h3>
                  <p className="text-gray-400 text-sm">{plan.description}</p>
                </div>

                {/* Price */}
                <div className="mb-6">
                  <span className="font-display text-4xl font-bold">
                    {plan.price}
                  </span>
                  <span className="text-gray-500 text-sm ml-2">
                    {plan.priceNote}
                  </span>
                </div>

                {/* Features */}
                <ul className="space-y-3 mb-8 flex-grow">
                  {plan.features.map((feature) => (
                    <li key={feature} className="flex items-start gap-3">
                      <Check
                        className="text-stronghold-cyan flex-shrink-0 mt-0.5"
                        size={18}
                      />
                      <span className="text-gray-300 text-sm">{feature}</span>
                    </li>
                  ))}
                </ul>

                {/* CTA */}
                <Button
                  variant={plan.variant}
                  size="md"
                  className="w-full"
                >
                  {plan.cta}
                </Button>
              </div>
            </motion.div>
          ))}
        </div>

        {/* x402 Note */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6, delay: 0.4 }}
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
