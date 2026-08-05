import { useQuery } from "@tanstack/react-query"
import { motion, useReducedMotion } from "framer-motion"
import { HeartHandshake, LockKeyhole, ServerCog, Sparkles } from "lucide-react"

import { AuthPanel } from "@/features/auth/components/AuthPanel"
import { getReadiness } from "@/shared/lib/health"

function FoundationPage() {
  const prefersReducedMotion = useReducedMotion()
  const health = useQuery({
    queryKey: ["health", "ready"],
    queryFn: getReadiness,
    retry: 1,
    refetchInterval: 30_000,
  })

  const isReady = health.data?.status === "healthy"
  const statusLabel = health.isPending
    ? "Comprobando servicios"
    : isReady
      ? "Fundación operativa"
      : "Servicios no disponibles"

  return (
    <main className="relative min-h-screen overflow-hidden bg-[var(--surface)] text-[var(--foreground)]">
      <div aria-hidden className="absolute inset-0 bg-[radial-gradient(circle_at_20%_10%,rgba(167,139,250,.18),transparent_36%),radial-gradient(circle_at_80%_75%,rgba(236,72,153,.14),transparent_32%)]" />
      <div className="relative mx-auto flex min-h-screen max-w-6xl flex-col px-6 py-8 lg:px-10">
        <header className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <img src="/logo.svg" alt="" className="h-10 w-10" />
            <span className="font-display text-xl font-semibold tracking-tight">LLMatch</span>
          </div>
          <div className="flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 py-2 text-xs text-white/70 backdrop-blur">
            <span className={`h-2 w-2 rounded-full ${isReady ? "bg-emerald-400" : health.isPending ? "bg-amber-300" : "bg-rose-400"}`} />
            {statusLabel}
          </div>
        </header>

        <section className="grid flex-1 items-center gap-14 py-16 lg:grid-cols-[1.08fr_.92fr]">
          <motion.div
            initial={prefersReducedMotion ? false : { opacity: 0, y: 18 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.55, ease: "easeOut" }}
            className="max-w-2xl"
          >
            <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-violet-300/20 bg-violet-300/10 px-3 py-1.5 text-sm text-violet-200">
              <Sparkles className="h-4 w-4" aria-hidden />
              Fase 1 · Cuentas seguras
            </div>
            <h1 className="font-display text-5xl font-semibold leading-[1.03] tracking-[-.04em] sm:text-7xl">
              Conexiones reales, con una base <span className="gradient-text">segura.</span>
            </h1>
            <p className="mt-7 max-w-xl text-lg leading-8 text-white/62">
              La nueva LLMatch empieza por lo que no se ve: arquitectura limpia, secretos fuera del código y servicios aislados antes de construir el primer perfil.
            </p>
            <div className="mt-9 grid gap-4 sm:grid-cols-3">
              <FoundationCard icon={LockKeyhole} title="Arranque seguro" text="Sin secretos predeterminados y con claves RSA verificadas." />
              <FoundationCard icon={ServerCog} title="Servicios aislados" text="PostGIS y Redis viven únicamente en la red interna." />
              <FoundationCard icon={HeartHandshake} title="Clean architecture" text="HTTP e infraestructura dependen de casos de uso, nunca al revés." />
            </div>
          </motion.div>

          <motion.div
            initial={prefersReducedMotion ? false : { opacity: 0, scale: 0.97 }}
            animate={{ opacity: 1, scale: 1 }}
            transition={{ duration: 0.55, delay: prefersReducedMotion ? 0 : 0.12 }}
          >
            <AuthPanel />
          </motion.div>
        </section>
      </div>
    </main>
  )
}

function FoundationCard({ icon: Icon, title, text, className = "" }: { icon: typeof LockKeyhole; title: string; text: string; className?: string }) {
  return (
    <article className={`rounded-3xl border border-white/10 bg-white/[.045] p-6 shadow-2xl shadow-black/15 backdrop-blur-xl ${className}`}>
      <div className="mb-7 flex h-11 w-11 items-center justify-center rounded-2xl bg-white/8 text-violet-200">
        <Icon className="h-5 w-5" aria-hidden />
      </div>
      <h2 className="font-display text-xl font-semibold">{title}</h2>
      <p className="mt-2 text-sm leading-6 text-white/55">{text}</p>
    </article>
  )
}

export function App() {
  return <FoundationPage />
}
