'use client'

import { motion, AnimatePresence } from 'framer-motion'
import { useState } from 'react'
import { ChevronDown } from 'lucide-react'

const faqs = [
  {
    question: 'What is Stronghold?',
    answer: 'Stronghold is a security layer for AI infrastructure that protects agents from prompt injection attacks and credential leaks. It operates as a transparent proxy, scanning all HTTP/HTTPS traffic before it reaches your AI models.',
  },
  {
    question: 'How does the transparent proxy work?',
    answer: 'Stronghold uses iptables (Linux) or pf (macOS) to intercept all network traffic at the system level. This means it works with any AI agent without requiring code changes, environment variables, or proxy configuration. Traffic flows through Stronghold\'s scanning engine, which analyzes content for threats in real-time.',
  },
  {
    question: 'What is x402 and why USDC?',
    answer: 'x402 is an open protocol for internet payments that uses EIP-712 signed authorizations. We chose USDC on Base because it offers fast, low-cost transactions with finality in seconds. This enables true pay-per-scan pricing without subscriptions or upfront commitments.',
  },
  {
    question: 'Is my wallet secure?',
    answer: 'Absolutely. Your private keys are stored in your operating system\'s native keyring (macOS Keychain, Linux Secret Service/KWallet/pass, or Windows Credential). Keys never leave your device and are never transmitted to our servers. Only your wallet address is shared for account linking.',
  },
  {
    question: 'Can I self-host Stronghold?',
    answer: 'Yes! Stronghold is fully open source under the MIT license. You can run your own instance on your infrastructure, use your own API keys, and have complete control over your data. The self-hosted version supports manual x402 payments.',
  },
  {
    question: 'What threats does Stronghold detect?',
    answer: 'Stronghold uses a 4-layer scanning architecture: (1) Heuristic pattern matching for known attack signatures, (2) ML classification for prompt injection detection, (3) Semantic similarity analysis for context-aware threats, and (4) LLM classification for sophisticated attacks.',
  },
]

function FAQItem({ question, answer, isOpen, onClick }: {
  question: string
  answer: string
  isOpen: boolean
  onClick: () => void
}) {
  return (
    <div className="border-b border-stronghold-stone-light/30 last:border-0">
      <button
        onClick={onClick}
        className="w-full py-6 flex items-center justify-between text-left group"
      >
        <span className="font-display text-lg font-medium group-hover:text-stronghold-cyan transition-colors pr-8">
          {question}
        </span>
        <motion.div
          animate={{ rotate: isOpen ? 180 : 0 }}
          transition={{ duration: 0.2 }}
          className="flex-shrink-0"
        >
          <ChevronDown className="text-gray-500" size={20} />
        </motion.div>
      </button>
      <AnimatePresence>
        {isOpen && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.3 }}
            className="overflow-hidden"
          >
            <p className="pb-6 text-gray-400 leading-relaxed">
              {answer}
            </p>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

export default function FAQ() {
  const [openIndex, setOpenIndex] = useState<number | null>(0)

  return (
    <section className="py-24 relative">
      <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Section Header */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="text-center mb-16"
        >
          <span className="text-stronghold-cyan font-mono text-sm tracking-wider uppercase">
            FAQ
          </span>
          <h2 className="font-display text-3xl sm:text-4xl font-bold mt-4">
            Common <span className="gradient-text">Questions</span>
          </h2>
        </motion.div>

        {/* FAQ List */}
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="fortress-card rounded-xl px-6"
        >
          {faqs.map((faq, index) => (
            <FAQItem
              key={index}
              question={faq.question}
              answer={faq.answer}
              isOpen={openIndex === index}
              onClick={() => setOpenIndex(openIndex === index ? null : index)}
            />
          ))}
        </motion.div>
      </div>
    </section>
  )
}
