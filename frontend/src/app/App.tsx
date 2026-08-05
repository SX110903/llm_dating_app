import { motion, useReducedMotion } from "framer-motion"

import { AuthPanel } from "@/features/auth/components/AuthPanel"

function LandingPage() {
  const prefersReducedMotion = useReducedMotion()

  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[var(--surface)] px-6 py-16 text-[var(--foreground)]">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_50%_-10%,rgba(167,139,250,.14),transparent_45%)]"
      />
      <motion.div
        initial={prefersReducedMotion ? false : { opacity: 0, y: 14 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        className="relative flex w-full max-w-sm flex-col items-center text-center"
      >
        <img src="/logo.svg" alt="LLMatch" className="mb-8 h-12 w-12" />
        <h1 className="font-display text-4xl font-semibold tracking-tight sm:text-5xl">
          Encuentra a alguien <span className="gradient-text">real.</span>
        </h1>
        <p className="mt-4 text-base text-white/55">Crea tu cuenta o inicia sesión para empezar.</p>
        <div className="mt-10 w-full">
          <AuthPanel />
        </div>
      </motion.div>
    </main>
  )
}

export function App() {
  return <LandingPage />
}
