import { AnimatePresence, motion, useReducedMotion } from "framer-motion"
import { lazy, Suspense, useRef, useState, type DragEvent } from "react"

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

const PhotoUploadDialog = lazy(() => import("@/features/profile/components/PhotoUploadDialog"))

const MAX_PHOTOS = 6
const SUPPORTED_TYPES = new Set(["image/jpeg", "image/png", "image/webp"])

interface UploadAttempt {
  file: File
  progress: number
  status: "uploading" | "failed"
}

function PhotoThumbnail({ photo, index }: { photo: Photo; index: number }) {
  const url = usePhotoImageUrl(photo.id)
  return url ? (
    <img src={url} alt={`Foto de perfil ${index + 1}`} className="h-full w-full object-cover" />
  ) : (
    <div role="img" aria-label={`Cargando foto de perfil ${index + 1}`} className="h-full w-full animate-pulse bg-white/10" />
  )
}

export function PhotoGrid() {
  const photosQuery = usePhotosQuery()
  const upload = useUploadPhotoMutation()
  const reorder = useReorderPhotosMutation()
  const setPrimary = useSetPrimaryPhotoMutation()
  const remove = useDeletePhotoMutation()
  const reducedMotion = useReducedMotion()
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uploadAttempt, setUploadAttempt] = useState<UploadAttempt | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [draggingFile, setDraggingFile] = useState(false)
  const [draggedPhotoId, setDraggedPhotoId] = useState<string | null>(null)
  const [actionPhotoId, setActionPhotoId] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const photos = photosQuery.data ?? []
  const actionPhoto = photos.find((photo) => photo.id === actionPhotoId)
  const actionPhotoIndex = actionPhoto ? photos.findIndex((photo) => photo.id === actionPhoto.id) : -1

  const selectFile = (file: File | undefined) => {
    if (!file) return
    setError(null)
    if (!SUPPORTED_TYPES.has(file.type)) {
      setError("El archivo debe ser una imagen JPEG, PNG o WebP válida.")
      return
    }
    setSelectedFile(file)
  }

  const startUpload = (file: File) => {
    setError(null)
    setUploadAttempt({ file, progress: 0, status: "uploading" })
    upload.mutate(
      {
        file,
        onProgress: (progress) => {
          setUploadAttempt((current) => (current ? { ...current, progress } : current))
        },
      },
      {
        onSuccess: () => setUploadAttempt(null),
        onError: (mutationError) => {
          setError(uploadErrorMessage(mutationError))
          setUploadAttempt((current) => (current ? { ...current, status: "failed" } : current))
        },
      },
    )
  }

  const reorderPhoto = (sourceId: string, targetId: string) => {
    const sourceIndex = photos.findIndex((photo) => photo.id === sourceId)
    const targetIndex = photos.findIndex((photo) => photo.id === targetId)
    if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return
    const nextPhotos = [...photos]
    const [moved] = nextPhotos.splice(sourceIndex, 1)
    nextPhotos.splice(targetIndex, 0, moved)
    reorder.mutate(nextPhotos.map((photo) => photo.id))
  }

  const move = (photoId: string, direction: -1 | 1) => {
    const index = photos.findIndex((photo) => photo.id === photoId)
    const target = photos[index + direction]
    if (index < 0 || !target) return
    reorderPhoto(photoId, target.id)
  }

  const handleFileDrag = (event: DragEvent<HTMLDivElement>) => {
    if (!event.dataTransfer.types.includes("Files")) return
    event.preventDefault()
    event.dataTransfer.dropEffect = "copy"
    setDraggingFile(true)
  }

  const handleFileDrop = (event: DragEvent<HTMLDivElement>) => {
    if (!event.dataTransfer.types.includes("Files")) return
    event.preventDefault()
    setDraggingFile(false)
    selectFile(event.dataTransfer.files[0])
  }

  const animation = reducedMotion
    ? { initial: false as const, animate: undefined, exit: undefined, transition: { duration: 0 } }
    : {
        initial: { opacity: 0, scale: 0.94 },
        animate: { opacity: 1, scale: 1 },
        exit: { opacity: 0, scale: 0.94 },
        transition: { duration: 0.18 },
      }

  return (
    <div>
      <div
        role="region"
        aria-label="Zona de fotos de perfil"
        onDragEnter={handleFileDrag}
        onDragOver={handleFileDrag}
        onDragLeave={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDraggingFile(false)
        }}
        onDrop={handleFileDrop}
        className={`rounded-3xl border p-2 transition-colors ${
          draggingFile ? "border-violet-300 bg-violet-300/10" : "border-transparent"
        }`}
      >
        <div className="grid grid-cols-2 gap-3 min-[375px]:grid-cols-3 sm:grid-cols-6">
          <AnimatePresence initial={false}>
            {photos.map((photo, index) => (
              <motion.div
                layout={!reducedMotion}
                key={photo.id}
                {...animation}
                draggable
                onDragStartCapture={(event) => {
                  setDraggedPhotoId(photo.id)
                  event.dataTransfer.effectAllowed = "move"
                  event.dataTransfer.setData("text/plain", photo.id)
                }}
                onDragEndCapture={() => setDraggedPhotoId(null)}
                onDragOver={(event) => {
                  if (draggedPhotoId) event.preventDefault()
                }}
                onDrop={(event) => {
                  if (!draggedPhotoId) return
                  event.preventDefault()
                  event.stopPropagation()
                  reorderPhoto(draggedPhotoId, photo.id)
                  setDraggedPhotoId(null)
                }}
                className="group relative aspect-square overflow-hidden rounded-2xl border border-white/10 bg-white/5"
              >
                <PhotoThumbnail photo={photo} index={index} />
                {photo.is_primary && (
                  <span className="absolute left-2 top-2 rounded-full bg-white px-2 py-1 text-[10px] font-medium text-zinc-950">
                    Principal
                  </span>
                )}
                <span aria-hidden="true" className="absolute bottom-2 left-2 rounded-full bg-black/65 px-2 py-1 text-xs text-white/70">
                  ⠿
                </span>
                <button
                  type="button"
                  aria-label={`Acciones de la foto ${index + 1}`}
                  onClick={() => setActionPhotoId(photo.id)}
                  className="absolute bottom-2 right-2 flex h-11 w-11 items-center justify-center rounded-full bg-black/75 text-xl font-bold text-white shadow-lg hover:bg-black focus-visible:bg-black"
                >
                  <span aria-hidden="true">⋯</span>
                </button>
              </motion.div>
            ))}
          </AnimatePresence>

          {photos.length < MAX_PHOTOS && (
            <motion.button
              layout={!reducedMotion}
              type="button"
              onClick={() => fileInputRef.current?.click()}
              disabled={upload.isPending || selectedFile !== null}
              className="flex aspect-square min-h-24 flex-col items-center justify-center gap-1 rounded-2xl border border-dashed border-white/25 text-white/55 transition-colors hover:border-white/50 hover:text-white disabled:opacity-50"
            >
              <span aria-hidden="true" className="text-2xl leading-none">
                +
              </span>
              <span className="text-xs">Añadir foto</span>
            </motion.button>
          )}
        </div>
        <p className="mt-3 px-1 text-xs text-white/40">También puedes arrastrar aquí una foto desde tu dispositivo.</p>
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={(event) => {
          selectFile(event.target.files?.[0])
          event.target.value = ""
        }}
      />

      <p className="mt-3 text-xs text-white/45">{photos.length}/{MAX_PHOTOS} fotos · JPEG, PNG o WebP, máx. 10&nbsp;MB tras comprimir</p>
      {uploadAttempt && (
        <div className="mt-3 rounded-2xl border border-white/10 bg-white/5 p-3" role="status" aria-live="polite">
          <div className="flex items-center justify-between gap-3 text-xs">
            <span className="min-w-0 truncate text-white/70">{uploadAttempt.file.name}</span>
            <span className="shrink-0 tabular-nums text-white/55">
              {uploadAttempt.status === "failed" ? "Error" : `${uploadAttempt.progress}%`}
            </span>
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/10">
            <div className="h-full rounded-full bg-violet-300 transition-[width]" style={{ width: `${uploadAttempt.progress}%` }} />
          </div>
          {uploadAttempt.status === "failed" && (
            <button
              type="button"
              onClick={() => startUpload(uploadAttempt.file)}
              className="mt-2 rounded-full bg-white px-4 text-xs font-semibold text-zinc-950"
            >
              Reintentar subida
            </button>
          )}
        </div>
      )}
      {error && <p className="mt-2 text-sm text-rose-300">{error}</p>}

      {selectedFile && (
        <Suspense fallback={<p role="status" className="mt-3 text-sm text-white/55">Preparando editor…</p>}>
          <PhotoUploadDialog
            file={selectedFile}
            onCancel={() => setSelectedFile(null)}
            onReady={(preparedFile) => {
              setSelectedFile(null)
              startUpload(preparedFile)
            }}
          />
        </Suspense>
      )}

      {actionPhoto && (
        <PhotoActionsDialog
          photo={actionPhoto}
          index={actionPhotoIndex}
          photoCount={photos.length}
          busy={reorder.isPending || setPrimary.isPending || remove.isPending}
          onClose={() => setActionPhotoId(null)}
          onMove={(direction) => move(actionPhoto.id, direction)}
          onSetPrimary={() => {
            setPrimary.mutate(actionPhoto.id)
            setActionPhotoId(null)
          }}
          onDelete={() => {
            remove.mutate(actionPhoto.id)
            setActionPhotoId(null)
          }}
        />
      )}
    </div>
  )
}

function PhotoActionsDialog({
  photo,
  index,
  photoCount,
  busy,
  onClose,
  onMove,
  onSetPrimary,
  onDelete,
}: {
  photo: Photo
  index: number
  photoCount: number
  busy: boolean
  onClose: () => void
  onMove: (direction: -1 | 1) => void
  onSetPrimary: () => void
  onDelete: () => void
}) {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="photo-actions-title"
      onKeyDown={(event) => {
        if (event.key === "Escape") onClose()
      }}
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/70 p-4 sm:items-center"
    >
      <div className="w-full max-w-sm rounded-3xl border border-white/10 bg-[#191520] p-5 shadow-2xl">
        <h2 id="photo-actions-title" className="font-display text-lg font-semibold">
          Acciones de la foto {index + 1}
        </h2>
        <div className="mt-4 grid gap-2">
          {!photo.is_primary && (
            <button type="button" disabled={busy} onClick={onSetPrimary} className="rounded-2xl bg-white/8 px-4 text-left text-sm hover:bg-white/12">
              Hacer principal
            </button>
          )}
          <button
            type="button"
            disabled={busy || index === 0}
            onClick={() => onMove(-1)}
            className="rounded-2xl bg-white/8 px-4 text-left text-sm hover:bg-white/12 disabled:opacity-35"
          >
            Mover a la izquierda
          </button>
          <button
            type="button"
            disabled={busy || index === photoCount - 1}
            onClick={() => onMove(1)}
            className="rounded-2xl bg-white/8 px-4 text-left text-sm hover:bg-white/12 disabled:opacity-35"
          >
            Mover a la derecha
          </button>
          <button type="button" disabled={busy} onClick={onDelete} className="rounded-2xl bg-rose-500/15 px-4 text-left text-sm text-rose-200 hover:bg-rose-500/25">
            Eliminar foto
          </button>
          <button type="button" autoFocus onClick={onClose} className="mt-2 rounded-2xl border border-white/15 px-4 text-sm text-white/65 hover:bg-white/5">
            Cerrar
          </button>
        </div>
      </div>
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
