import { Heart, MapPin, Star, X } from "lucide-react"
import { motion, useAnimationControls, useReducedMotion, type PanInfo } from "framer-motion"

import { useProtectedImage } from "@/features/swipe/hooks/use-protected-image"
import type { Candidate, SwipeAction } from "@/features/swipe/types"

const DRAG_THRESHOLD = 110

interface SwipeCardProps {
  candidate: Candidate
  disabled: boolean
  onAction: (action: SwipeAction) => Promise<void>
}

export function SwipeCard({ candidate, disabled, onAction }: SwipeCardProps) {
  const controls = useAnimationControls()
  const reducedMotion = useReducedMotion()
  const imageURL = useProtectedImage(candidate.photo_url)
  const compatibility = Math.round(candidate.score.total * 100)

  const commit = async (action: SwipeAction) => {
    if (disabled) return
    const direction = action === "dislike" ? -1 : 1
    if (!reducedMotion) {
      await controls.start({
        x: direction * 560,
        rotate: direction * 13,
        opacity: 0,
        transition: { type: "spring", stiffness: 230, damping: 24 },
      })
    }
    try {
      await onAction(action)
    } catch {
      controls.set({ x: 0, rotate: 0, opacity: 1 })
    }
  }

  const handleDragEnd = (_event: MouseEvent | TouchEvent | PointerEvent, info: PanInfo) => {
    if (info.offset.x > DRAG_THRESHOLD) {
      void commit("like")
    } else if (info.offset.x < -DRAG_THRESHOLD) {
      void commit("dislike")
    }
  }

  return (
    <motion.article
      aria-label={`Perfil de ${candidate.display_name}`}
      drag={disabled ? false : "x"}
      dragConstraints={{ left: 0, right: 0 }}
      dragElastic={0.72}
      dragSnapToOrigin
      onDragEnd={handleDragEnd}
      animate={controls}
      initial={reducedMotion ? false : { opacity: 0, scale: 0.97, y: 12 }}
      whileTap={disabled ? undefined : { cursor: "grabbing", scale: 1.01 }}
      className="relative w-full touch-pan-y select-none overflow-hidden rounded-[2rem] border border-white/10 bg-[#191520] shadow-2xl shadow-black/40"
    >
      <div className="relative aspect-[4/5] overflow-hidden bg-white/5">
        {imageURL ? (
          <img src={imageURL} alt={`Foto de ${candidate.display_name}`} draggable={false} className="h-full w-full object-cover" />
        ) : (
          <div className="h-full w-full animate-pulse bg-white/10" />
        )}
        <div className="absolute inset-0 bg-gradient-to-t from-[#151119] via-transparent to-black/10" />
        <span className="absolute right-4 top-4 rounded-full border border-white/15 bg-black/45 px-3 py-1 text-xs font-semibold backdrop-blur-md">
          {compatibility}% afinidad
        </span>
        <div className="absolute inset-x-0 bottom-0 p-6">
          <div className="flex items-end justify-between gap-4">
            <div>
              <h2 className="font-display text-3xl font-semibold tracking-tight">
                {candidate.display_name}, {candidate.age}
              </h2>
              <p className="mt-1 flex items-center gap-1.5 text-sm text-white/65">
                <MapPin aria-hidden className="h-4 w-4" />
                {candidate.city || "Cerca de ti"} &middot; {candidate.distance_km.toFixed(1)} km
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className="space-y-5 p-6 pt-5">
        {candidate.bio && <p className="text-sm leading-6 text-white/72">{candidate.bio}</p>}
        {candidate.interests.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {candidate.interests.slice(0, 6).map((interest) => (
              <span key={interest} className="rounded-full bg-white/[0.07] px-3 py-1.5 text-xs text-white/68">
                {interest}
              </span>
            ))}
          </div>
        )}

        <div className="flex items-center justify-center gap-4 border-t border-white/8 pt-5">
          <ActionButton label="Descartar" onClick={() => void commit("dislike")} disabled={disabled} tone="neutral">
            <X aria-hidden className="h-6 w-6" />
          </ActionButton>
          <ActionButton label="Superlike" onClick={() => void commit("superlike")} disabled={disabled} tone="accent">
            <Star aria-hidden className="h-5 w-5" />
          </ActionButton>
          <ActionButton label="Me gusta" onClick={() => void commit("like")} disabled={disabled} tone="like">
            <Heart aria-hidden className="h-6 w-6" />
          </ActionButton>
        </div>
      </div>
    </motion.article>
  )
}

function ActionButton({
  label,
  onClick,
  disabled,
  tone,
  children,
}: {
  label: string
  onClick: () => void
  disabled: boolean
  tone: "neutral" | "accent" | "like"
  children: React.ReactNode
}) {
  const tones = {
    neutral: "border-white/12 text-white/65 hover:border-white/25 hover:text-white",
    accent: "border-violet-400/25 text-violet-300 hover:bg-violet-400/10",
    like: "border-pink-400/25 text-pink-300 hover:bg-pink-400/10",
  }
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      disabled={disabled}
      className={`flex h-14 w-14 items-center justify-center rounded-full border bg-white/[0.03] transition disabled:opacity-35 ${tones[tone]}`}
    >
      {children}
    </button>
  )
}
