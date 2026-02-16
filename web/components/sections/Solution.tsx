'use client'

import { motion } from 'framer-motion'
import { Download, Power, CheckCircle, ArrowRight } from 'lucide-react'

const steps = [
  {
    number: '01',
    icon: Download,
    title: 'Install',
    description: 'One command setup with automatic OS keyring configuration. Your wallet is created locally—private keys never leave your device.',
    command: 'curl -fsSL https://getstronghold.xyz/install.sh | sh',
  },
  {
    number: '02',
    icon: Power,
    title: 'Enable',
    description: 'Transparent proxy intercepts all HTTP/HTTPS traffic at the network level. Works system-wide, no code changes needed.',
    command: 'sudo stronghold enable',
  },
  {
    number: '03',
    icon: CheckCircle,
    title: 'Verify',
    description: 'Confirm your setup is running properly. Once enabled, Stronghold automatically scans every request and response\u2014you\u2019re protected.',
    command: 'stronghold status',
  },
]

export default function Solution() {
  return (
    <section id="solution" className="py-24 relative">
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
            How It Works
          </span>
          <h2 className="font-display text-3xl sm:text-4xl lg:text-5xl font-bold mt-4 mb-6">
            Transparent <span className="gradient-text">Protection</span>
          </h2>
          <p className="text-gray-400 text-lg max-w-2xl mx-auto">
            Install once. Protect everything. Stronghold operates at the network level,
            so it works with any AI agent without code changes.
          </p>
        </motion.div>

        {/* Steps */}
        <div className="relative">
          {/* Connection line */}
          <div className="hidden lg:block absolute top-1/2 left-0 right-0 h-px bg-gradient-to-r from-transparent via-stronghold-cyan/30 to-transparent" />

          <div className="grid lg:grid-cols-3 gap-8">
            {steps.map((step, index) => (
              <motion.div
                key={step.number}
                initial={{ opacity: 0, y: 30 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.6, delay: index * 0.15 }}
                className="relative"
              >
                <div className="fortress-card rounded-xl p-8 h-full flex flex-col">
                  {/* Step number */}
                  <span className="font-mono text-5xl font-bold text-stronghold-stone-light/30 absolute top-4 right-4">
                    {step.number}
                  </span>

                  {/* Icon */}
                  <div className="w-14 h-14 rounded-lg bg-stronghold-cyan/10 flex items-center justify-center mb-6 relative z-10">
                    <step.icon className="text-stronghold-cyan" size={28} />
                  </div>

                  {/* Content */}
                  <h3 className="font-display text-2xl font-semibold mb-4 relative z-10">
                    {step.title}
                  </h3>
                  <p className="text-gray-400 mb-6 flex-grow relative z-10">
                    {step.description}
                  </p>

                  {/* Command */}
                  <div className="bg-stronghold-darker rounded-lg p-4 font-mono text-xs text-gray-400 border border-stronghold-stone-light/30 overflow-x-auto">
                    <span className="text-stronghold-cyan">$</span> {step.command}
                  </div>

                  {/* Arrow (except last) */}
                  {index < steps.length - 1 && (
                    <div className="hidden lg:flex absolute -right-4 top-1/2 -translate-y-1/2 z-20">
                      <ArrowRight className="text-stronghold-cyan" size={24} />
                    </div>
                  )}
                </div>
              </motion.div>
            ))}
          </div>
        </div>

        {/* Architecture Note */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6, delay: 0.5 }}
          className="mt-16 text-center"
        >
          <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-stronghold-stone/30 border border-stronghold-stone-light/30">
            <span className="w-2 h-2 rounded-full bg-stronghold-cyan animate-pulse" />
            <span className="text-sm text-gray-400">
              Uses iptables/nftables on Linux, pf on macOS — cannot be bypassed
            </span>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
