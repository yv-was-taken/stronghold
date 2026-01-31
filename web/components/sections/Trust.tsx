'use client'

import { motion } from 'framer-motion'
import { Github, Lock, Server, Code2 } from 'lucide-react'

const badges = [
  {
    icon: Code2,
    label: 'Open Source',
    description: 'MIT Licensed',
  },
  {
    icon: Server,
    label: 'Self-Hostable',
    description: 'Run on your infrastructure',
  },
  {
    icon: Lock,
    label: 'Non-Custodial',
    description: 'You control your keys',
  },
  {
    icon: Github,
    label: 'Community Driven',
    description: 'Built by AI engineers',
  },
]

export default function Trust() {
  return (
    <section className="py-24 relative border-y border-stronghold-stone-light/20">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="text-center mb-12"
        >
          <h3 className="font-display text-xl text-gray-400">
            Built for AI Engineers, by AI Engineers
          </h3>
        </motion.div>

        {/* Badges */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
          {badges.map((badge, index) => (
            <motion.div
              key={badge.label}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: index * 0.1 }}
              className="text-center"
            >
              <div className="w-14 h-14 mx-auto mb-4 rounded-xl bg-stronghold-stone/50 flex items-center justify-center border border-stronghold-stone-light/30">
                <badge.icon className="text-stronghold-cyan" size={24} />
              </div>
              <div className="font-display font-semibold mb-1">{badge.label}</div>
              <div className="text-sm text-gray-500">{badge.description}</div>
            </motion.div>
          ))}
        </div>

        {/* Stats */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6, delay: 0.4 }}
          className="mt-16 grid grid-cols-3 gap-8 text-center"
        >
          <div>
            <div className="font-display text-3xl font-bold text-stronghold-cyan">&lt;50ms</div>
            <div className="text-sm text-gray-500 mt-1">Scan Latency</div>
          </div>
          <div>
            <div className="font-display text-3xl font-bold text-stronghold-cyan">4-Layer</div>
            <div className="text-sm text-gray-500 mt-1">Defense Stack</div>
          </div>
          <div>
            <div className="font-display text-3xl font-bold text-stronghold-cyan">100%</div>
            <div className="text-sm text-gray-500 mt-1">Open Source</div>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
