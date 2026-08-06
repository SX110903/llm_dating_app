import { AnimatePresence, motion } from "framer-motion"
import { useRef, useState } from "react"

import {
  useDeletePhotoMutation,
  usePhotosQuery,
  useReorderPhotosMutation,
  useSetPrimaryPhotoMutation,
  useUploadPhotoMutation,
} from "@/features/profile/hooks/use-profile"
import { usePhotoImageUrl } from "@/features/profile/hooks/use-photo-image"
import type { Photo } from "@/features/profile/types"
import { ApiError } from "@/shared/lib/errors"

const MAX_PHOTOS = 6

function PhotoThumbnail({ photo }: { photo: Photo }) {
  const url = usePhotoImageUrl(photo.id)
  return url ? (
    <img src={url} alt="" className="h-full w-full object-cover" />
  ) : (
    <div className="h-full w-full animate-pulse bg-white/10" />
  )
}

export function PhotoGrid() {
  const photosQuery = usePhotosQuery()
  const upload = useUploadPhotoMutation()
  const reorder = useReorderPhotosMutation()
  const setPrimary = useSetPrimaryPhotoMutation()
  const remove = useDeletePhotoMutation()
  const [progress, setProgress] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const photos = photosQuery.data ?? []

  const handleFile = (file: File | undefined) => {
    if (!file) return
    setError(null)
    setProgress(0)
    upload.mutate(
      { file, onProgress: setProgress },
      {
        onSettled: () => setProgress(null),
        onError: (mutationError) => {
          setError(uploadErrorMessage(mutationError))
        },
      },
    )
  }

  const move = (photoId: string, direction: -1 | 1) => {
    const index = photos.findIndex((p) => p.id === photoId)
    const targetIndex = index + direction
    if (index < 0 || targetIndex < 0 || targetIndex >= photos.length) return
    const reordered = [...photos]
    const [moved] = reordered.splice(index, 1)
    reordered.splice(targetIndex, 0, moved)
    reorder.mutate(reordered.map((p) => p.id))
  }

  return (
    <div>
      <div className="grid grid-cols-3 gap-3 sm:grid-cols-6">
        <AnimatePresence initial={false}>
          {photos.map((photo, index) => (
            <motion.div
              layout
              key={photo.id}
              initial={{ opacity: 0, scale: 0.9 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.9 }}
              transition={{ duration: 0.2 }}
              className="group relative aspect-square overflow-hidden rounded-2xl border border-white/10 bg-white/5"
            >
              <PhotoThumbnail photo={photo} />
              {photo.is_primary && (
                <span className="absolute left-1.5 top-1.5 rounded-full bg-white px-2 py-0.5 text-[10px] font-medium text-zinc-950">
                  Principal
                </span>
              )}
              <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/60 opacity-0 transition-opacity group-hover:opacity-100">
                {!photo.is_primary && (
                  <button
                    type="button"
                    onClick={() => setPrimary.mutate(photo.id)}
                    className="rounded-full bg-white/90 px-2 py-1 text-[11px] font-medium text-zinc-950"
                  >
                    Hacer principal
                  </button>
                )}
                <div className="flex gap-1">
                  <button
                    type="button"
                    onClick={() => move(photo.id, -1)}
                    disabled={index === 0}
                    className="rounded-full bg-white/15 px-2 py-1 text-[11px] text-white disabled:opacity-30"
                  >
                    ←
                  </button>
                  <button
                    type="button"
                    onClick={() => move(photo.id, 1)}
                    disabled={index === photos.length - 1}
                    className="rounded-full bg-white/15 px-2 py-1 text-[11px] text-white disabled:opacity-30"
                  >
                    →
                  </button>
                  <button
                    type="button"
                    onClick={() => remove.mutate(photo.id)}
                    className="rounded-full bg-rose-500/90 px-2 py-1 text-[11px] text-white"
                  >
                    Eliminar
                  </button>
                </div>
              </div>
            </motion.div>
          ))}
        </AnimatePresence>

        {photos.length < MAX_PHOTOS && (
          <motion.button
            layout
            type="button"
            onClick={() => fileInputRef.current?.click()}
            disabled={upload.isPending}
            className="flex aspect-square flex-col items-center justify-center gap-1 rounded-2xl border border-dashed border-white/20 text-white/50 transition-colors hover:border-white/40 hover:text-white/80 disabled:opacity-50"
          >
            {upload.isPending ? (
              <span className="text-xs">{progress ?? 0}%</span>
            ) : (
              <>
                <span className="text-2xl leading-none">+</span>
                <span className="text-[11px]">Añadir</span>
              </>
            )}
          </motion.button>
        )}
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(event) => {
          handleFile(event.target.files?.[0])
          event.target.value = ""
        }}
      />

      <p className="mt-3 text-xs text-white/45">{photos.length}/{MAX_PHOTOS} fotos · JPEG, PNG o WebP, máx. 10&nbsp;MB</p>
      {error && <p className="mt-1 text-xs text-rose-300">{error}</p>}
    </div>
  )
}

function uploadErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "UNSUPPORTED_MIME_TYPE":
        return "El archivo debe ser una imagen JPEG, PNG o WebP válida."
      case "PHOTO_TOO_LARGE":
        return "La foto supera el tamaño máximo de 10 MB."
      case "PHOTO_LIMIT_REACHED":
        return "Ya tienes el máximo de 6 fotos."
      default:
        return error.message
    }
  }
  return "No se pudo subir la foto."
}
