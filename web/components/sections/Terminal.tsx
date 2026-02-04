'use client'

import { motion } from 'framer-motion'
import { useState, useEffect } from 'react'

const commands = [
  { prompt: '$ stronghold doctor', output: '✓ OS: Linux (Ubuntu 22.04)\n✓ Keyring: Secret Service available\n✓ Firewall: iptables installed\n✓ All checks passed!' },
  { prompt: '$ stronghold init', output: 'Initializing Stronghold...\n✓ Wallet created and stored in keyring\n✓ Account created: 4829-1056-7734-2891\n✓ Proxy configured on port 8080\n✓ Initialization complete!' },
  { prompt: '$ stronghold account balance', output: '💳 Account: 4829-1056-7734-2891\n\nWallet: 0x742d35Cc...7595f8f3a\nBalance: 12.45 USDC' },
  { prompt: '$ stronghold enable', output: 'Starting Stronghold proxy...\n✓ Proxy listening on 127.0.0.1:8080\n✓ iptables rules applied\n✓ Protection enabled' },
]

export default function Terminal() {
  const [currentCommand, setCurrentCommand] = useState(0)
  const [typedText, setTypedText] = useState('')
  const [showOutput, setShowOutput] = useState(false)

  useEffect(() => {
    const command = commands[currentCommand]
    let charIndex = 0
    setTypedText('')
    setShowOutput(false)

    const typeInterval = setInterval(() => {
      if (charIndex <= command.prompt.length) {
        setTypedText(command.prompt.slice(0, charIndex))
        charIndex++
      } else {
        clearInterval(typeInterval)
        setTimeout(() => setShowOutput(true), 300)
        setTimeout(() => {
          setCurrentCommand((prev) => (prev + 1) % commands.length)
        }, 4000)
      }
    }, 50)

    return () => clearInterval(typeInterval)
  }, [currentCommand])

  return (
    <section className="py-24 relative">
      <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="text-center mb-12"
        >
          <span className="text-stronghold-cyan font-mono text-sm tracking-wider uppercase">
            CLI Experience
          </span>
          <h2 className="font-display text-3xl sm:text-4xl font-bold mt-4">
            Simple, Powerful, <span className="gradient-text">Fast</span>
          </h2>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, scale: 0.95 }}
          whileInView={{ opacity: 1, scale: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="fortress-card rounded-xl overflow-hidden"
        >
          {/* Terminal Header */}
          <div className="bg-stronghold-stone px-4 py-3 flex items-center gap-2 border-b border-stronghold-stone-light">
            <div className="flex gap-2">
              <span className="w-3 h-3 rounded-full bg-red-500/80" />
              <span className="w-3 h-3 rounded-full bg-yellow-500/80" />
              <span className="w-3 h-3 rounded-full bg-green-500/80" />
            </div>
            <span className="ml-4 text-xs text-gray-500 font-mono">stronghold — zsh</span>
          </div>

          {/* Terminal Body */}
          <div className="bg-stronghold-darker p-6 font-mono text-sm min-h-[280px]">
            {/* Previous commands */}
            {currentCommand > 0 && (
              <div className="mb-4 opacity-50">
                {commands.slice(0, currentCommand).map((cmd, i) => (
                  <div key={i} className="mb-2">
                    <span className="text-stronghold-cyan">$</span>{' '}
                    <span className="text-gray-300">{cmd.prompt.replace('$ ', '')}</span>
                  </div>
                ))}
              </div>
            )}

            {/* Current command */}
            <div className="mb-4">
              <span className="text-stronghold-cyan">$</span>{' '}
              <span className="text-gray-100">{typedText.replace('$ ', '')}</span>
              <motion.span
                animate={{ opacity: [1, 0] }}
                transition={{ duration: 0.5, repeat: Infinity }}
                className="inline-block w-2 h-4 bg-stronghold-cyan ml-1 align-middle"
              />
            </div>

            {/* Output */}
            {showOutput && (
              <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ duration: 0.3 }}
                className="text-gray-400 whitespace-pre-line"
              >
                {commands[currentCommand].output}
              </motion.div>
            )}
          </div>
        </motion.div>
      </div>
    </section>
  )
}
