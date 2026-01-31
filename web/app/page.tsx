import Navigation from '@/components/sections/Navigation'
import Hero from '@/components/sections/Hero'
import Problem from '@/components/sections/Problem'
import Solution from '@/components/sections/Solution'
import Features from '@/components/sections/Features'
import Terminal from '@/components/sections/Terminal'
import Pricing from '@/components/sections/Pricing'
import Trust from '@/components/sections/Trust'
import FAQ from '@/components/sections/FAQ'
import Footer from '@/components/sections/Footer'

export default function Home() {
  return (
    <main className="min-h-screen bg-stronghold-dark">
      <Navigation />
      <Hero />
      <Problem />
      <Solution />
      <Features />
      <Terminal />
      <Pricing />
      <Trust />
      <FAQ />
      <Footer />
    </main>
  )
}
