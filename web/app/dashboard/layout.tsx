import type { Metadata } from 'next'
import { AuthProvider } from '@/components/providers/AuthProvider'
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
    <AuthProvider>
      {children}
    </AuthProvider>
  )
}
