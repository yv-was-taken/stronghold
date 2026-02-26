'use client'

import { motion } from 'framer-motion'
import Logo from '../Logo'
import { Github, Send, Twitter } from 'lucide-react'

const footerLinks = {
  Product: [
    { label: 'Features', href: '#features' },
    { label: 'Pricing', href: '#pricing' },
    { label: 'Documentation', href: 'https://docs.getstronghold.xyz', external: true },
    { label: 'Changelog', href: 'https://github.com/yv-was-taken/stronghold/commits/master', external: true },
  ],
  Resources: [
    { label: 'GitHub', href: 'https://github.com/yv-was-taken/stronghold', external: true },
    { label: 'Telegram', href: 'https://t.me/getstronghold', external: true },
    { label: 'Twitter', href: 'https://x.com/strongholdxyz', external: true },
  ],
}

export default function Footer() {
  return (
    <footer className="border-t border-stronghold-stone-light/20 pt-16 pb-8">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-8 mb-12">
          {/* Brand */}
          <div className="col-span-2">
            <motion.a
              href="#"
              className="flex items-center gap-3 mb-4"
              whileHover={{ scale: 1.02 }}
            >
              <Logo size={32} />
              <span className="font-display font-bold text-xl">Stronghold</span>
            </motion.a>
            <p className="text-gray-500 text-sm mb-6 max-w-xs">
              The security layer for AI infrastructure. Protect your agents from
              prompt injection and credential leaks.
            </p>
            <div className="flex gap-4">
              <a
                href="https://github.com/yv-was-taken/stronghold"
                target="_blank"
                rel="noopener noreferrer"
                className="text-gray-500 hover:text-stronghold-cyan transition-colors"
              >
                <Github size={20} />
              </a>
              <a
                href="https://t.me/getstronghold"
                target="_blank"
                rel="noopener noreferrer"
                className="text-gray-500 hover:text-stronghold-cyan transition-colors"
              >
                <Send size={20} />
              </a>
              <a
                href="https://x.com/strongholdxyz"
                target="_blank"
                rel="noopener noreferrer"
                className="text-gray-500 hover:text-stronghold-cyan transition-colors"
              >
                <Twitter size={20} />
              </a>
            </div>
          </div>

          {/* Links */}
          {Object.entries(footerLinks).map(([category, links]) => (
            <div key={category}>
              <h4 className="font-display font-semibold mb-4">{category}</h4>
              <ul className="space-y-2">
                {links.map((link) => (
                  <li key={link.label}>
                    <a
                      href={link.href}
                      target={'external' in link && link.external ? '_blank' : undefined}
                      rel={'external' in link && link.external ? 'noopener noreferrer' : undefined}
                      className="text-gray-500 hover:text-stronghold-cyan transition-colors text-sm"
                    >
                      {link.label}
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {/* Bottom */}
        <div className="pt-8 border-t border-stronghold-stone-light/20 flex flex-col md:flex-row justify-between items-center gap-4">
          <p className="text-gray-600 text-sm">
            &copy; {new Date().getFullYear()} Stronghold. Open source under MIT License.
          </p>
          <div className="flex items-center gap-2 text-gray-600 text-sm">
            <span className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
            All systems operational
          </div>
        </div>
      </div>
    </footer>
  )
}
