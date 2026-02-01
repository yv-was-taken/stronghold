import '@testing-library/jest-dom'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
import React from 'react'

// Cleanup after each test
afterEach(() => {
  cleanup()
})

// Mock Next.js router
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
    back: vi.fn(),
  }),
  usePathname: () => '/',
  useSearchParams: () => new URLSearchParams(),
}))

// Mock framer-motion to avoid animation issues in tests
vi.mock('framer-motion', () => {
  const createMotionComponent = (tag: string) => {
    return React.forwardRef(({ children, ...props }: any, ref: any) => {
      return React.createElement(tag, { ...props, ref }, children)
    })
  }

  return {
    motion: {
      div: createMotionComponent('div'),
      span: createMotionComponent('span'),
      p: createMotionComponent('p'),
      h1: createMotionComponent('h1'),
      h2: createMotionComponent('h2'),
      h3: createMotionComponent('h3'),
      button: createMotionComponent('button'),
      a: createMotionComponent('a'),
      ul: createMotionComponent('ul'),
      li: createMotionComponent('li'),
      nav: createMotionComponent('nav'),
      section: createMotionComponent('section'),
      header: createMotionComponent('header'),
      footer: createMotionComponent('footer'),
    },
    AnimatePresence: ({ children }: any) => children,
    useAnimation: () => ({
      start: vi.fn(),
      set: vi.fn(),
    }),
    useInView: () => true,
  }
})
