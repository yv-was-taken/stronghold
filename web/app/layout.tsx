import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = {
  title: 'Stronghold — The Security Layer for AI Infrastructure',
  description: 'Protect your AI agents from prompt injection attacks and credential leaks. Stronghold scans every request through a transparent proxy, blocking threats before they reach your models.',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en">
      <body className="antialiased">{children}</body>
    </html>
  )
}
