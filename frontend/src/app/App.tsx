import { motion, useReducedMotion } from "framer-motion"

import { AuthPanel } from "@/features/auth/components/AuthPanel"
import { ProfileEditor } from "@/features/profile/components/ProfileEditor"
import { useAuthStore } from "@/shared/state/auth-store"

function AuthScreen() {
  const prefersReducedMotion = useReducedMotion()

  return (
    <div className="relative flex w-full max-w-sm flex-col items-center text-center">
      <img src="/logo.svg" alt="LLMatch" className="mb-8 h-12 w-12" />
      <h1 className="font-display text-4xl font-semibold tracking-tight sm:text-5xl">
        Encuentra a alguien <span className="gradient-text">real.</span>
      </h1>
      <p className="mt-4 text-base text-white/55">Crea tu cuenta o inicia sesión para empezar.</p>
      <motion.div
        initial={prefersReducedMotion ? false : { opacity: 0, y: 14 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        className="mt-10 w-full"
      >
        <AuthPanel />
      </motion.div>
    </div>
  )
}

export function App() {
  const isAuthenticated = useAuthStore((state) => state.user !== null)

  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-[var(--surface)] px-6 py-16 text-[var(--foreground)]">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_50%_-10%,rgba(167,139,250,.14),transparent_45%)]"
      />
      <div className="relative flex w-full justify-center">{isAuthenticated ? <ProfileEditor /> : <AuthScreen />}</div>
    </main>
  )
}
