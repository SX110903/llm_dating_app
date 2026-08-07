import { motion, useReducedMotion } from "framer-motion"
import { Heart, LogOut, MessagesSquare, User, Users } from "lucide-react"
import { useCallback, useState } from "react"

import { AuthPanel } from "@/features/auth/components/AuthPanel"
import { useLogout } from "@/features/auth/hooks/use-auth"
import { MatchesList } from "@/features/matches/components/MatchesList"
import { MessagesScreen } from "@/features/messaging/components/MessagesScreen"
import { ProfileEditor } from "@/features/profile/components/ProfileEditor"
import { useProfileQuery } from "@/features/profile/hooks/use-profile"
import { DiscoveryDeck } from "@/features/swipe/components/DiscoveryDeck"
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
    <main className="relative flex min-h-screen justify-center overflow-hidden bg-[var(--surface)] px-5 py-10 text-[var(--foreground)] sm:px-8 sm:py-14">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_50%_-10%,rgba(167,139,250,.14),transparent_45%)]"
      />
      <div className="relative flex w-full items-center justify-center">{isAuthenticated ? <AuthenticatedApp /> : <AuthScreen />}</div>
    </main>
  )
}

function AuthenticatedApp() {
  const profile = useProfileQuery()

  if (profile.isPending) {
    return <p className="text-sm text-white/45">Cargando tu espacio...</p>
  }
  if (!profile.data?.onboarding_completed) {
    return <ProfileEditor />
  }
  return <AppShell />
}

type AppTab = "discovery" | "matches" | "messages" | "profile"

function AppShell() {
  const [tab, setTab] = useState<AppTab>("discovery")
  const logout = useLogout()
  const user = useAuthStore((state) => state.user)
  const showMatches = useCallback(() => setTab("matches"), [])
  const showProfile = useCallback(() => setTab("profile"), [])

  return (
    <div className="w-full max-w-5xl">
      <header className="mb-10 flex items-center justify-between gap-4">
        <button type="button" onClick={() => setTab("discovery")} className="flex items-center gap-3 text-left">
          <img src="/logo.svg" alt="" className="h-9 w-9" />
          <div>
            <p className="font-display text-lg font-semibold leading-none">LLMatch</p>
            <p className="mt-1 text-[11px] text-white/35">Hola, {user?.display_name}</p>
          </div>
        </button>

        <nav aria-label="Navegacion principal" className="flex items-center rounded-full border border-white/10 bg-white/[0.035] p-1">
          <NavButton active={tab === "discovery"} label="Descubrir" onClick={() => setTab("discovery")}>
            <Heart aria-hidden className="h-4 w-4" />
          </NavButton>
          <NavButton active={tab === "matches"} label="Matches" onClick={showMatches}>
            <Users aria-hidden className="h-4 w-4" />
          </NavButton>
          <NavButton active={tab === "messages"} label="Mensajes" onClick={() => setTab("messages")}>
            <MessagesSquare aria-hidden className="h-4 w-4" />
          </NavButton>
          <NavButton active={tab === "profile"} label="Perfil" onClick={showProfile}>
            <User aria-hidden className="h-4 w-4" />
          </NavButton>
        </nav>

        <button
          type="button"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
          aria-label="Cerrar sesion"
          className="flex h-10 w-10 items-center justify-center rounded-full border border-white/10 text-white/40 hover:bg-white/5 hover:text-white/70 disabled:opacity-40"
        >
          <LogOut aria-hidden className="h-4 w-4" />
        </button>
      </header>

      <AnimateTab tab={tab}>
        {tab === "discovery" && <DiscoveryDeck onEditProfile={showProfile} onViewMatches={showMatches} />}
        {tab === "matches" && <MatchesList />}
        {tab === "messages" && <MessagesScreen />}
        {tab === "profile" && <ProfileEditor />}
      </AnimateTab>
    </div>
  )
}

function NavButton({
  active,
  label,
  onClick,
  children,
}: {
  active: boolean
  label: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={`flex items-center gap-2 rounded-full px-3 py-2 text-xs font-medium transition sm:px-4 ${
        active ? "bg-white text-zinc-950" : "text-white/45 hover:text-white/80"
      }`}
    >
      {children}
      <span className="hidden sm:inline">{label}</span>
    </button>
  )
}

function AnimateTab({ tab, children }: { tab: AppTab; children: React.ReactNode }) {
  return (
    <motion.div key={tab} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
      {children}
    </motion.div>
  )
}
