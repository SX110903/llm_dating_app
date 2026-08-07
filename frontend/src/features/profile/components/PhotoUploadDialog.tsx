import { useEffect, useRef, useState } from "react"

import {
  drawCropPreview,
  isSupportedPhotoType,
  preparePhotoForUpload,
  type CropSettings,
} from "@/features/profile/lib/photo-processing"
import "@/features/profile/components/PhotoUploadDialog.css"

const INITIAL_CROP: CropSettings = { zoom: 1, offsetX: 0, offsetY: 0 }

interface PhotoUploadDialogProps {
  file: File
  onCancel: () => void
  onReady: (file: File) => void
}

export default function PhotoUploadDialog({ file, onCancel, onReady }: PhotoUploadDialogProps) {
  const [image, setImage] = useState<HTMLImageElement | null>(null)
  const [crop, setCrop] = useState<CropSettings>(INITIAL_CROP)
  const [error, setError] = useState<string | null>(null)
  const [processing, setProcessing] = useState(false)
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    if (!isSupportedPhotoType(file.type)) {
      queueMicrotask(() => setError("El archivo debe ser JPEG, PNG o WebP."))
      return
    }

    let active = true
    const sourceURL = URL.createObjectURL(file)
    const nextImage = new Image()
    nextImage.onload = () => {
      if (active) setImage(nextImage)
    }
    nextImage.onerror = () => {
      if (active) setError("No se pudo leer la imagen seleccionada.")
    }
    nextImage.src = sourceURL

    return () => {
      active = false
      URL.revokeObjectURL(sourceURL)
    }
  }, [file])

  useEffect(() => {
    if (!image || !canvasRef.current) return
    try {
      drawCropPreview(canvasRef.current, image, crop)
    } catch {
      queueMicrotask(() => setError("No se pudo generar la previsualización."))
    }
  }, [crop, image])

  const prepare = async () => {
    if (!image || processing) return
    setProcessing(true)
    setError(null)
    try {
      onReady(await preparePhotoForUpload(image, file, crop))
    } catch {
      setError("No se pudo comprimir la foto por debajo de 10 MB.")
      setProcessing(false)
    }
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="photo-crop-title"
      onKeyDown={(event) => {
        if (event.key === "Escape") onCancel()
      }}
      className="photo-crop"
    >
      <div className="photo-crop__panel">
        <h2 id="photo-crop-title" className="photo-crop__title">
          Recorta tu foto
        </h2>
        <p className="photo-crop__description">La foto se guardará con formato cuadrado antes de subirla.</p>

        <div className="photo-crop__preview">
          <canvas ref={canvasRef} aria-label="Previsualización del recorte cuadrado" />
        </div>

        <div className="photo-crop__controls">
          <CropControl
            label="Zoom"
            value={crop.zoom}
            min={1}
            max={3}
            step={0.05}
            onChange={(zoom) => setCrop((current) => ({ ...current, zoom }))}
          />
          <CropControl
            label="Encuadre horizontal"
            value={crop.offsetX}
            min={-1}
            max={1}
            step={0.05}
            onChange={(offsetX) => setCrop((current) => ({ ...current, offsetX }))}
          />
          <CropControl
            label="Encuadre vertical"
            value={crop.offsetY}
            min={-1}
            max={1}
            step={0.05}
            onChange={(offsetY) => setCrop((current) => ({ ...current, offsetY }))}
          />
        </div>

        {error && <p className="photo-crop__error">{error}</p>}
        {!image && !error && <p className="photo-crop__loading">Preparando previsualización…</p>}

        <div className="photo-crop__actions">
          <button type="button" onClick={onCancel} className="photo-crop__cancel">
            Cancelar
          </button>
          <button
            type="button"
            onClick={() => void prepare()}
            disabled={!image || processing}
            className="photo-crop__prepare"
          >
            {processing ? "Comprimiendo…" : "Preparar foto"}
          </button>
        </div>
      </div>
    </div>
  )
}

function CropControl({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string
  value: number
  min: number
  max: number
  step: number
  onChange: (value: number) => void
}) {
  return (
    <label className="photo-crop__control">
      {label}
      <output>{value.toFixed(2)}</output>
      <input
        type="range"
        aria-label={label}
        value={value}
        min={min}
        max={max}
        step={step}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  )
}
