import { Heart } from "lucide-react"
import { motion } from "framer-motion"
import { useEffect } from "react"

import { useProtectedImage } from "@/features/swipe/hooks/use-protected-image"
import type { Candidate } from "@/features/swipe/types"

interface MatchModalProps {
  candidate: Candidate
  onClose: () => void
  onViewMatches: () => void
}

export function MatchModal({ candidate, onClose, onViewMatches }: MatchModalProps) {
  const imageURL = useProtectedImage(candidate.photo_url)

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose()
    }
    window.addEventListener("keydown", closeOnEscape)
    return () => window.removeEventListener("keydown", closeOnEscape)
  }, [onClose])

  return (
    <motion.div
      role="dialog"
      aria-modal="true"
      aria-labelledby="match-title"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      className="fixed inset-0 z-50 flex items-center justify-center bg-[#0d0910]/85 p-6 backdrop-blur-xl"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) onClose()
      }}
    >
      <motion.div
        initial={{ opacity: 0, scale: 0.9, y: 16 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        exit={{ opacity: 0, scale: 0.94 }}
        transition={{ type: "spring", stiffness: 260, damping: 24 }}
        className="w-full max-w-sm rounded-[2rem] border border-pink-300/20 bg-[#1a1420] p-7 text-center shadow-2xl shadow-pink-950/30"
      >
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br from-violet-400 to-pink-400 text-white shadow-lg shadow-pink-500/20">
          <Heart aria-hidden className="h-8 w-8 fill-current" />
        </div>
        <h2 id="match-title" className="gradient-text mt-5 font-display text-4xl font-semibold">
          Hay match
        </h2>
        <p className="mt-2 text-sm text-white/60">A {candidate.display_name} tambi&eacute;n le gustas.</p>
        <div className="mx-auto mt-6 h-28 w-28 overflow-hidden rounded-full border-4 border-white/10 bg-white/5">
          {imageURL ? <img src={imageURL} alt="" className="h-full w-full object-cover" /> : null}
        </div>
        <button
          type="button"
          autoFocus
          onClick={onViewMatches}
          className="mt-7 w-full rounded-full bg-white px-5 py-3 text-sm font-semibold text-zinc-950 hover:bg-white/90"
        >
          Ver mis matches
        </button>
        <button type="button" onClick={onClose} className="mt-3 text-sm text-white/45 hover:text-white/75">
          Seguir descubriendo
        </button>
      </motion.div>
    </motion.div>
  )
}
