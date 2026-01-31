'use client'

import { motion } from 'framer-motion'

interface LogoProps {
  className?: string
  size?: number
}

export default function Logo({ className = '', size = 40 }: LogoProps) {
  return (
    <motion.svg
      width={size}
      height={size}
      viewBox="0 0 48 48"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      initial={{ opacity: 0, scale: 0.9 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ duration: 0.5 }}
    >
      {/* Outer fortress wall - octagonal */}
      <motion.path
        d="M24 2L42 12V36L24 46L6 36V12L24 2Z"
        stroke="#00D4AA"
        strokeWidth="2"
        fill="none"
        initial={{ pathLength: 0 }}
        animate={{ pathLength: 1 }}
        transition={{ duration: 1, delay: 0.2 }}
      />

      {/* Inner keep - smaller octagon */}
      <motion.path
        d="M24 14L32 18V30L24 34L16 30V18L24 14Z"
        stroke="#00D4AA"
        strokeWidth="1.5"
        fill="rgba(0, 212, 170, 0.1)"
        initial={{ pathLength: 0 }}
        animate={{ pathLength: 1 }}
        transition={{ duration: 0.8, delay: 0.5 }}
      />

      {/* Center shield */}
      <motion.path
        d="M24 18C24 18 28 20 28 24C28 28 24 32 24 32C24 32 20 28 20 24C20 20 24 18 24 18Z"
        fill="#00D4AA"
        initial={{ scale: 0, opacity: 0 }}
        animate={{ scale: 1, opacity: 1 }}
        transition={{ duration: 0.4, delay: 0.8 }}
      />

      {/* Corner battlements */}
      <motion.g
        stroke="#00D4AA"
        strokeWidth="1.5"
        strokeLinecap="round"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.5, delay: 1 }}
      >
        <line x1="6" y1="12" x2="6" y2="8" />
        <line x1="42" y1="12" x2="42" y2="8" />
        <line x1="6" y1="36" x2="6" y2="40" />
        <line x1="42" y1="36" x2="42" y2="40" />
      </motion.g>

      {/* Glow effect */}
      <motion.circle
        cx="24"
        cy="24"
        r="20"
        fill="url(#glow)"
        initial={{ opacity: 0 }}
        animate={{ opacity: 0.3 }}
        transition={{ duration: 1, delay: 1.2 }}
      />

      <defs>
        <radialGradient id="glow" cx="24" cy="24" r="20">
          <stop stopColor="#00D4AA" stopOpacity="0.4" />
          <stop offset="1" stopColor="#00D4AA" stopOpacity="0" />
        </radialGradient>
      </defs>
    </motion.svg>
  )
}
