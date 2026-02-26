import type { Metadata } from 'next'
import { Providers } from '@/components/providers/Providers'
import '../globals.css'

export const metadata: Metadata = {
  title: 'Stronghold Dashboard',
  description: 'Manage your Stronghold account and view usage statistics',
}

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <Providers>
      {children}
    </Providers>
  )
}
