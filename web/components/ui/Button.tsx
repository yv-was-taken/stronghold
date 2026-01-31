'use client'

import { motion } from 'framer-motion'
import { ReactNode } from 'react'

interface ButtonProps {
  children: ReactNode
  variant?: 'primary' | 'secondary' | 'outline'
  size?: 'sm' | 'md' | 'lg'
  href?: string
  onClick?: () => void
  className?: string
  icon?: ReactNode
}

export default function Button({
  children,
  variant = 'primary',
  size = 'md',
  href,
  onClick,
  className = '',
  icon,
}: ButtonProps) {
  const baseStyles = 'inline-flex items-center justify-center gap-2 font-mono font-medium transition-all duration-200 relative overflow-hidden group'

  const variants = {
    primary: 'bg-stronghold-cyan text-stronghold-darker hover:bg-opacity-90',
    secondary: 'bg-stronghold-stone text-white border border-stronghold-stone-light hover:border-stronghold-cyan',
    outline: 'bg-transparent text-white border border-stronghold-stone-light hover:border-stronghold-cyan hover:text-stronghold-cyan',
  }

  const sizes = {
    sm: 'px-4 py-2 text-sm rounded-md',
    md: 'px-6 py-3 text-sm rounded-lg',
    lg: 'px-8 py-4 text-base rounded-lg',
  }

  const Component = href ? motion.a : motion.button

  return (
    <Component
      href={href}
      onClick={onClick}
      className={`${baseStyles} ${variants[variant]} ${sizes[size]} ${className}`}
      whileHover={{ scale: 1.02 }}
      whileTap={{ scale: 0.98 }}
    >
      {/* Glow effect for primary */}
      {variant === 'primary' && (
        <span className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent translate-x-[-100%] group-hover:translate-x-[100%] transition-transform duration-700" />
      )}

      {/* Corner accents */}
      <span className="absolute top-0 left-0 w-2 h-2 border-t border-l border-current opacity-0 group-hover:opacity-100 transition-opacity" />
      <span className="absolute top-0 right-0 w-2 h-2 border-t border-r border-current opacity-0 group-hover:opacity-100 transition-opacity" />
      <span className="absolute bottom-0 left-0 w-2 h-2 border-b border-l border-current opacity-0 group-hover:opacity-100 transition-opacity" />
      <span className="absolute bottom-0 right-0 w-2 h-2 border-b border-r border-current opacity-0 group-hover:opacity-100 transition-opacity" />

      {icon && <span className="relative z-10">{icon}</span>}
      <span className="relative z-10">{children}</span>
    </Component>
  )
}
