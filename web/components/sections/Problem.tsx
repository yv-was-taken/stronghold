'use client'

import { motion } from 'framer-motion'
import { AlertTriangle, Lock, EyeOff, ShieldAlert } from 'lucide-react'

const threats = [
  {
    icon: AlertTriangle,
    title: 'Prompt Injection',
    description: 'Attackers embed malicious instructions that override your system prompts.',
  },
  {
    icon: EyeOff,
    title: 'Credential Leaks',
    description: 'AI models accidentally expose API keys, passwords, or sensitive data.',
  },
  {
    icon: ShieldAlert,
    title: 'Jailbreak Attacks',
    description: 'Sophisticated techniques bypass safety guardrails entirely.',
  },
  {
    icon: Lock,
    title: 'Data Exfiltration',
    description: 'Malicious prompts trick AI into sending private data to attackers.',
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
                <AlertTriangle className="text-red-500" size={20} />
              </div>
              <h3 className="font-display text-xl font-semibold text-red-400">
                Without Stronghold
              </h3>
            </div>

            <ul className="space-y-4">
              {[
                'Direct access to AI models',
                'No input validation',
                'Sensitive data exposed in outputs',
                'No audit trail of requests',
                'Attacks go undetected',
              ].map((item, i) => (
                <li key={i} className="flex items-start gap-3 text-gray-400">
                  <span className="text-red-500 mt-1">×</span>
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
                'Transparent proxy intercepts all traffic',
                '4-layer scanning detects attacks',
                'Credential leaks blocked automatically',
                'Complete audit trail via headers',
                'Real-time threat detection',
              ].map((item, i) => (
                <li key={i} className="flex items-start gap-3 text-gray-300">
                  <span className="text-stronghold-cyan mt-1">✓</span>
                  {item}
                </li>
              ))}
            </ul>
          </motion.div>
        </div>

        {/* Threat Grid */}
        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
          {threats.map((threat, index) => (
            <motion.div
              key={threat.title}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: index * 0.1 }}
              className="fortress-card rounded-xl p-6 group hover:border-red-500/30 transition-colors"
            >
              <threat.icon
                className="text-red-500 mb-4 group-hover:scale-110 transition-transform"
                size={28}
              />
              <h4 className="font-display font-semibold text-lg mb-2">{threat.title}</h4>
              <p className="text-sm text-gray-400">{threat.description}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
