import { AnimatePresence, motion } from "framer-motion"
import { SlidersHorizontal } from "lucide-react"
import { useCallback, useState } from "react"

import { MatchModal } from "@/features/swipe/components/MatchModal"
import { SwipeCard } from "@/features/swipe/components/SwipeCard"
import { useDiscoveryQuery, useSwipeMutation } from "@/features/swipe/hooks/use-swipe"
import type { Candidate, SwipeAction } from "@/features/swipe/types"
import { ApiError } from "@/shared/lib/errors"

interface DiscoveryDeckProps {
  onEditProfile: () => void
  onViewMatches: () => void
}

export function DiscoveryDeck({ onEditProfile, onViewMatches }: DiscoveryDeckProps) {
  const discovery = useDiscoveryQuery()
  const swipe = useSwipeMutation()
  const [matchedCandidate, setMatchedCandidate] = useState<Candidate | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const candidates = discovery.data?.pages.flatMap((page) => page.candidates) ?? []
  const candidate = candidates[0]

  const act = async (action: SwipeAction) => {
    if (!candidate) return
    setActionError(null)
    try {
      const result = await swipe.mutateAsync({ targetId: candidate.user_id, action })
      if (result.match) setMatchedCandidate(candidate)
    } catch (error) {
      setActionError(actionErrorMessage(error))
      throw error
    }
  }

  const closeMatch = useCallback(() => setMatchedCandidate(null), [])
  const viewMatches = useCallback(() => {
    setMatchedCandidate(null)
    onViewMatches()
  }, [onViewMatches])

  if (discovery.isPending) {
    return <CardSkeleton />
  }

  if (discovery.isError) {
    const notReady = discovery.error instanceof ApiError && discovery.error.code === "DISCOVERY_NOT_READY"
    return (
      <EmptyState
        title={notReady ? "Tu perfil aun no aparece en discovery" : "No se pudo cargar discovery"}
        description={
          notReady
            ? "Revisa tus preferencias, consentimiento, ubicacion y foto principal."
            : "Comprueba tu conexion y vuelve a intentarlo."
        }
        actionLabel={notReady ? "Revisar perfil" : "Reintentar"}
        onAction={notReady ? onEditProfile : () => void discovery.refetch()}
      />
    )
  }

  if (!candidate) {
    return (
      <EmptyState
        title="Ya has visto todos los perfiles disponibles"
        description="Vuelve mas tarde o ajusta tus preferencias para descubrir nuevas personas."
        actionLabel={discovery.hasNextPage ? "Cargar mas" : "Editar preferencias"}
        onAction={discovery.hasNextPage ? () => void discovery.fetchNextPage() : onEditProfile}
      />
    )
  }

  return (
    <div className="mx-auto w-full max-w-md">
      <div className="mb-5 flex items-center justify-between px-1">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.22em] text-violet-300/70">Discovery</p>
          <h1 className="mt-1 font-display text-2xl font-semibold">Personas para ti</h1>
        </div>
        <button
          type="button"
          onClick={onEditProfile}
          className="flex h-10 w-10 items-center justify-center rounded-full border border-white/10 text-white/55 hover:bg-white/5 hover:text-white"
          aria-label="Editar preferencias"
        >
          <SlidersHorizontal aria-hidden className="h-4 w-4" />
        </button>
      </div>

      <div className="relative">
        <div aria-hidden className="absolute inset-x-5 top-2 h-full rounded-[2rem] border border-white/5 bg-white/[0.025]" />
        <AnimatePresence mode="popLayout">
          <SwipeCard key={candidate.user_id} candidate={candidate} disabled={swipe.isPending} onAction={act} />
        </AnimatePresence>
      </div>
      <p className="mt-4 text-center text-xs text-white/35">Arrastra la tarjeta o usa los botones</p>
      {actionError && <p className="mt-2 text-center text-sm text-rose-300">{actionError}</p>}

      <AnimatePresence>
        {matchedCandidate && (
          <MatchModal candidate={matchedCandidate} onClose={closeMatch} onViewMatches={viewMatches} />
        )}
      </AnimatePresence>
    </div>
  )
}

function CardSkeleton() {
  return (
    <div className="mx-auto w-full max-w-md animate-pulse">
      <div className="mb-5 h-12 w-44 rounded-xl bg-white/5" />
      <div className="aspect-[4/6] rounded-[2rem] border border-white/5 bg-white/5" />
    </div>
  )
}

function EmptyState({
  title,
  description,
  actionLabel,
  onAction,
}: {
  title: string
  description: string
  actionLabel: string
  onAction: () => void
}) {
  return (
    <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} className="mx-auto max-w-md py-20 text-center">
      <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/5">
        <SlidersHorizontal aria-hidden className="h-5 w-5 text-violet-300" />
      </div>
      <h1 className="mt-5 font-display text-2xl font-semibold">{title}</h1>
      <p className="mx-auto mt-2 max-w-sm text-sm leading-6 text-white/50">{description}</p>
      <button type="button" onClick={onAction} className="mt-6 rounded-full bg-white px-5 py-2.5 text-sm font-medium text-zinc-950">
        {actionLabel}
      </button>
    </motion.div>
  )
}

function actionErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "DAILY_SWIPE_LIMIT") return "Has alcanzado el limite diario. Vuelve manana."
    if (error.code === "DEPENDENCY_UNAVAILABLE") return "Discovery no esta disponible temporalmente."
    return error.message
  }
  return "No se pudo guardar tu decision."
}
