'use client'

import { motion } from 'framer-motion'
import Button from '../ui/Button'
import { Shield, ArrowRight, Github } from 'lucide-react'

export default function Hero() {
  return (
    <section className="relative min-h-screen flex items-center justify-center overflow-hidden pt-20">
      {/* Background Effects */}
      <div className="absolute inset-0 grid-bg opacity-50" />
      <div className="absolute inset-0 bg-fortress-glow" />

      {/* Animated gradient orbs */}
      <motion.div
        className="absolute top-1/4 left-1/4 w-96 h-96 bg-stronghold-cyan/10 rounded-full blur-3xl"
        animate={{
          scale: [1, 1.2, 1],
          opacity: [0.3, 0.5, 0.3],
        }}
        transition={{
          duration: 8,
          repeat: Infinity,
          ease: 'easeInOut',
        }}
      />
      <motion.div
        className="absolute bottom-1/4 right-1/4 w-80 h-80 bg-stronghold-cyan/5 rounded-full blur-3xl"
        animate={{
          scale: [1.2, 1, 1.2],
          opacity: [0.2, 0.4, 0.2],
        }}
        transition={{
          duration: 10,
          repeat: Infinity,
          ease: 'easeInOut',
        }}
      />

      {/* Content */}
      <div className="relative z-10 max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 text-center">
        {/* Headline */}
        <motion.h1
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, delay: 0.1 }}
          className="font-display text-4xl sm:text-5xl lg:text-7xl font-bold tracking-tight mb-6"
        >
          <span className="gradient-text">The Security Layer</span>
          <br />
          <span className="text-white">for AI Infrastructure</span>
        </motion.h1>

        {/* Subheadline */}
        <motion.p
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, delay: 0.2 }}
          className="text-lg sm:text-xl text-gray-400 max-w-2xl mx-auto mb-10 leading-relaxed"
        >
          Protect your AI agents from prompt injection attacks and credential leaks.
          Stronghold scans every request through a transparent proxy, blocking threats
          before they reach your models.
        </motion.p>

        {/* CTAs */}
        <motion.div
          initial={{ opacity: 0, y: 30 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.6, delay: 0.3 }}
          className="flex flex-col sm:flex-row items-center justify-center gap-4"
        >
          <Button variant="primary" size="lg" icon={<ArrowRight size={18} />}>
            Get Started
          </Button>
          <Button
            variant="outline"
            size="lg"
            href="https://github.com/yv-was-taken/stronghold"
            icon={<Github size={18} />}
          >
            View on GitHub
          </Button>
        </motion.div>

        {/* Architecture Diagram */}
        <motion.div
          initial={{ opacity: 0, y: 40 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.8, delay: 0.5 }}
          className="mt-16 relative"
        >
          <div className="fortress-card rounded-xl p-8 max-w-3xl mx-auto">
            {/* Flow Diagram */}
            <div className="flex items-center justify-between gap-4 text-sm">
              {/* User */}
              <div className="flex flex-col items-center gap-2">
                <div className="w-16 h-16 rounded-lg bg-stronghold-stone-light/50 flex items-center justify-center border border-stronghold-stone-light">
                  <span className="font-mono text-gray-400">User</span>
                </div>
              </div>

              {/* Arrow */}
              <div className="flex-1 h-px bg-gradient-to-r from-stronghold-stone-light via-stronghold-cyan/50 to-stronghold-stone-light relative">
                <motion.div
                  className="absolute top-1/2 left-0 w-2 h-2 bg-stronghold-cyan rounded-full -translate-y-1/2"
                  animate={{ left: ['0%', '100%', '0%'] }}
                  transition={{ duration: 3, repeat: Infinity, ease: 'linear' }}
                />
              </div>

              {/* Proxy */}
              <div className="flex flex-col items-center gap-2">
                <div className="w-20 h-20 rounded-lg bg-stronghold-cyan/10 flex items-center justify-center border border-stronghold-cyan/50 relative overflow-hidden">
                  <div className="absolute inset-0 bg-stronghold-cyan/5 animate-pulse" />
                  <Shield size={28} className="text-stronghold-cyan relative z-10" />
                </div>
                <span className="font-mono text-stronghold-cyan text-xs">Stronghold</span>
              </div>

              {/* Arrow */}
              <div className="flex-1 h-px bg-gradient-to-r from-stronghold-stone-light via-stronghold-cyan/50 to-stronghold-stone-light relative">
                <motion.div
                  className="absolute top-1/2 left-0 w-2 h-2 bg-stronghold-cyan rounded-full -translate-y-1/2"
                  animate={{ left: ['0%', '100%', '0%'] }}
                  transition={{ duration: 3, repeat: Infinity, ease: 'linear', delay: 1.5 }}
                />
              </div>

              {/* AI */}
              <div className="flex flex-col items-center gap-2">
                <div className="w-16 h-16 rounded-lg bg-stronghold-stone-light/50 flex items-center justify-center border border-stronghold-stone-light">
                  <span className="font-mono text-gray-400">AI</span>
                </div>
              </div>
            </div>

            {/* Status indicators */}
            <div className="flex justify-center gap-8 mt-6">
              <div className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
                <span className="text-xs text-gray-500 font-mono">ALLOW</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-yellow-500" />
                <span className="text-xs text-gray-500 font-mono">WARN</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-red-500" />
                <span className="text-xs text-gray-500 font-mono">BLOCK</span>
              </div>
            </div>
          </div>
        </motion.div>
      </div>

      {/* Scroll indicator */}
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 1.5 }}
        className="absolute bottom-8 left-1/2 -translate-x-1/2"
      >
        <motion.div
          animate={{ y: [0, 8, 0] }}
          transition={{ duration: 2, repeat: Infinity }}
          className="w-6 h-10 rounded-full border-2 border-stronghold-stone-light/50 flex justify-center pt-2"
        >
          <motion.div className="w-1 h-2 rounded-full bg-stronghold-cyan" />
        </motion.div>
      </motion.div>
    </section>
  )
}
