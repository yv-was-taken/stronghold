import Link from 'next/link'

export default function NotFound() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center px-4">
      <div className="text-center max-w-md">
        <h1 className="font-display font-bold text-6xl mb-4 gradient-text">404</h1>
        <h2 className="font-display font-semibold text-2xl mb-3">Page not found</h2>
        <p className="text-gray-500 mb-8">
          The page you&apos;re looking for doesn&apos;t exist or has been moved.
        </p>
        <Link
          href="/"
          className="inline-block px-6 py-3 bg-stronghold-cyan/10 border border-stronghold-cyan/30 text-stronghold-cyan rounded-lg font-medium hover:bg-stronghold-cyan/20 transition-colors"
        >
          Go Home
        </Link>
      </div>
    </div>
  )
}
