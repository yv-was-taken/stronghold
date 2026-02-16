'use client'

import { motion } from 'framer-motion'
import { Shield, Lock, Plug, Wallet } from 'lucide-react'

const cards = [
  {
    icon: Shield,
    title: 'Prompt Injection Defense',
    highlight: 'Stronghold Protects',
    description: 'Malicious instructions embedded in user inputs get caught by 4-layer scanning before they can hijack your agent\u2019s behavior.',
  },
  {
    icon: Lock,
    title: 'Credential Leak Prevention',
    highlight: 'Stronghold Protects',
    description: 'API keys, passwords, and secrets in LLM outputs are detected and blocked before they ever leave your system.',
  },
  {
    icon: Plug,
    title: 'Plug & Play for Any Agent',
    highlight: 'Easy Integration',
    description: 'Drop Stronghold into your existing agent setup with zero code changes. Network-level interception works with any framework or model provider.',
  },
  {
    icon: Wallet,
    title: 'Autonomous Agent Payments',
    highlight: 'Powered by x402',
    description: 'Your agent pays per scan automatically via its local wallet. No subscriptions, no API keys to manage, no human in the loop for billing.',
  },
]

export default function Problem() {
  return (
    <section id="problem" className="py-24 relative">
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
            The Risk
          </span>
          <h2 className="font-display text-3xl sm:text-4xl lg:text-5xl font-bold mt-4 mb-6">
            AI Agents Are <span className="text-red-500">Vulnerable</span>
          </h2>
          <p className="text-gray-400 text-lg max-w-2xl mx-auto">
            As AI agents gain access to sensitive data and systems, they become prime targets.
            Traditional security tools weren&apos;t built for this threat model.
          </p>
        </motion.div>

        {/* Comparison */}
        <div className="grid lg:grid-cols-2 gap-8 mb-16">
          {/* Without Stronghold */}
          <motion.div
            initial={{ opacity: 0, x: -30 }}
            whileInView={{ opacity: 1, x: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.6 }}
            className="fortress-card rounded-xl p-8 border-red-500/20"
          >
            <div className="flex items-center gap-3 mb-6">
              <div className="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center">
                <Shield className="text-red-500" size={20} />
              </div>
              <h3 className="font-display text-xl font-semibold text-red-400">
                Without Stronghold
              </h3>
            </div>

            <ul className="space-y-4">
              {[
                'Prompts hijacked by injection attacks',
                'API keys and secrets leak through model outputs',
                'No visibility into agent network traffic',
                'Threats bypass your application layer undetected',
              ].map((item, i) => (
                <li key={i} className="flex items-start gap-3 text-gray-400">
                  <span className="text-red-500 mt-1">&times;</span>
                  {item}
                </li>
              ))}
            </ul>
          </motion.div>

          {/* With Stronghold */}
          <motion.div
            initial={{ opacity: 0, x: 30 }}
            whileInView={{ opacity: 1, x: 0 }}
            viewport={{ once: true }}
            transition={{ duration: 0.6, delay: 0.1 }}
            className="fortress-card rounded-xl p-8"
          >
            <div className="flex items-center gap-3 mb-6">
              <div className="w-10 h-10 rounded-lg bg-stronghold-cyan/10 flex items-center justify-center">
                <Lock className="text-stronghold-cyan" size={20} />
              </div>
              <h3 className="font-display text-xl font-semibold text-stronghold-cyan">
                With Stronghold
              </h3>
            </div>

            <ul className="space-y-4">
              {[
                'Every request scanned before reaching your models',
                '4-layer defense blocks injection attacks',
                'Credentials and secrets caught in real time',
                'Full visibility into all agent traffic',
              ].map((item, i) => (
                <li key={i} className="flex items-start gap-3 text-gray-300">
                  <span className="text-stronghold-cyan mt-1">&#10003;</span>
                  {item}
                </li>
              ))}
            </ul>
          </motion.div>
        </div>

        {/* Value Cards */}
        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {cards.map((card, index) => (
            <motion.div
              key={card.title}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: index * 0.1 }}
              className="fortress-card rounded-xl p-6 group hover:border-stronghold-cyan/30 transition-colors"
            >
              <card.icon
                className="text-stronghold-cyan mb-4 group-hover:scale-110 transition-transform"
                size={28}
              />
              <span className="inline-block px-2 py-1 rounded text-xs font-mono bg-stronghold-cyan/10 text-stronghold-cyan mb-3">
                {card.highlight}
              </span>
              <h4 className="font-display font-semibold text-lg mb-2">{card.title}</h4>
              <p className="text-sm text-gray-400">{card.description}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
