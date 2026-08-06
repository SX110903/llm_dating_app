import { AnimatePresence, motion } from "framer-motion"
import { Ban, Flag, HeartOff, MapPin, Users } from "lucide-react"
import { useState } from "react"

import { ReportDialog } from "@/features/matches/components/ReportDialog"
import { useMatchesQuery, useUnmatchMutation } from "@/features/matches/hooks/use-matches"
import type { Match } from "@/features/matches/types"
import { useBlockMutation } from "@/features/swipe/hooks/use-swipe"
import { useProtectedImage } from "@/features/swipe/hooks/use-protected-image"

export function MatchesList() {
  const matches = useMatchesQuery()
  const unmatch = useUnmatchMutation()
  const block = useBlockMutation()
  const [reportTarget, setReportTarget] = useState<Match | null>(null)

  if (matches.isPending) {
    return <p className="py-20 text-center text-sm text-white/45">Cargando matches...</p>
  }
  if (matches.isError) {
    return (
      <div className="py-20 text-center">
        <p className="text-sm text-rose-300">No se pudieron cargar tus matches.</p>
        <button type="button" onClick={() => void matches.refetch()} className="mt-4 text-sm text-white/60 underline">
          Reintentar
        </button>
      </div>
    )
  }

  const items = matches.data?.matches ?? []
  return (
    <div className="mx-auto w-full max-w-3xl">
      <div className="mb-8">
        <p className="text-xs font-medium uppercase tracking-[0.22em] text-pink-300/70">Conexiones</p>
        <h1 className="mt-1 font-display text-3xl font-semibold">Tus matches</h1>
        <p className="mt-2 text-sm text-white/45">Personas con las que el interes es mutuo.</p>
      </div>

      {items.length === 0 ? (
        <div className="rounded-3xl border border-dashed border-white/10 py-16 text-center">
          <Users aria-hidden className="mx-auto h-8 w-8 text-white/25" />
          <h2 className="mt-4 font-display text-xl font-semibold">Aun no tienes matches</h2>
          <p className="mt-2 text-sm text-white/45">Cuando el interes sea mutuo aparecera aqui.</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          <AnimatePresence initial={false}>
            {items.map((match) => (
              <MatchCard
                key={match.id}
                match={match}
                busy={unmatch.isPending || block.isPending}
                onUnmatch={() => {
                  if (window.confirm(`Deshacer el match con ${match.display_name}?`)) unmatch.mutate(match.id)
                }}
                onBlock={() => {
                  if (window.confirm(`Bloquear a ${match.display_name}? El match se eliminara.`)) block.mutate(match.other_user_id)
                }}
                onReport={() => setReportTarget(match)}
              />
            ))}
          </AnimatePresence>
        </div>
      )}

      {reportTarget && (
        <ReportDialog
          userId={reportTarget.other_user_id}
          displayName={reportTarget.display_name}
          onClose={() => setReportTarget(null)}
        />
      )}
    </div>
  )
}

function MatchCard({
  match,
  busy,
  onUnmatch,
  onBlock,
  onReport,
}: {
  match: Match
  busy: boolean
  onUnmatch: () => void
  onBlock: () => void
  onReport: () => void
}) {
  const imageURL = useProtectedImage(match.photo_url)
  return (
    <motion.article
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, scale: 0.96 }}
      className="overflow-hidden rounded-3xl border border-white/10 bg-white/[0.035]"
    >
      <div className="flex gap-4 p-4">
        <div className="h-24 w-24 shrink-0 overflow-hidden rounded-2xl bg-white/5">
          {imageURL ? <img src={imageURL} alt={`Foto de ${match.display_name}`} className="h-full w-full object-cover" /> : null}
        </div>
        <div className="min-w-0 flex-1 py-1">
          <h2 className="truncate font-display text-xl font-semibold">{match.display_name}</h2>
          <p className="mt-1 flex items-center gap-1 text-xs text-white/45">
            <MapPin aria-hidden className="h-3.5 w-3.5" /> {match.city || "Cerca de ti"}
          </p>
          <p className="mt-3 line-clamp-2 text-sm leading-5 text-white/58">{match.bio || "Nuevo match"}</p>
        </div>
      </div>
      <div className="grid grid-cols-3 border-t border-white/8">
        <SafetyButton label="Deshacer" disabled={busy} onClick={onUnmatch}>
          <HeartOff aria-hidden className="h-4 w-4" />
        </SafetyButton>
        <SafetyButton label="Reportar" disabled={busy} onClick={onReport}>
          <Flag aria-hidden className="h-4 w-4" />
        </SafetyButton>
        <SafetyButton label="Bloquear" disabled={busy} onClick={onBlock} danger>
          <Ban aria-hidden className="h-4 w-4" />
        </SafetyButton>
      </div>
    </motion.article>
  )
}

function SafetyButton({
  label,
  disabled,
  onClick,
  danger = false,
  children,
}: {
  label: string
  disabled: boolean
  onClick: () => void
  danger?: boolean
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`flex items-center justify-center gap-1.5 px-2 py-3 text-xs transition hover:bg-white/5 disabled:opacity-35 ${danger ? "text-rose-300/70" : "text-white/45"}`}
    >
      {children}
      {label}
    </button>
  )
}
