export const MAX_PHOTO_BYTES = 10 * 1024 * 1024
export const SUPPORTED_PHOTO_TYPES = ["image/jpeg", "image/png", "image/webp"] as const

const MAX_OUTPUT_EDGE = 1600
const MIN_OUTPUT_EDGE = 480

export interface CropSettings {
  zoom: number
  offsetX: number
  offsetY: number
}

export interface CropRect {
  x: number
  y: number
  size: number
}

export function calculateSquareCrop(width: number, height: number, settings: CropSettings): CropRect {
  const zoom = clamp(settings.zoom, 1, 3)
  const size = Math.min(width, height) / zoom
  const maxX = (width - size) / 2
  const maxY = (height - size) / 2

  return {
    x: maxX + clamp(settings.offsetX, -1, 1) * maxX,
    y: maxY + clamp(settings.offsetY, -1, 1) * maxY,
    size,
  }
}

export function isSupportedPhotoType(type: string): type is (typeof SUPPORTED_PHOTO_TYPES)[number] {
  return SUPPORTED_PHOTO_TYPES.includes(type as (typeof SUPPORTED_PHOTO_TYPES)[number])
}

export function drawCropPreview(
  canvas: HTMLCanvasElement,
  image: HTMLImageElement,
  settings: CropSettings,
  outputEdge = 640,
) {
  const context = canvas.getContext("2d")
  if (!context) throw new Error("Canvas is not available")
  const crop = calculateSquareCrop(image.naturalWidth, image.naturalHeight, settings)
  canvas.width = outputEdge
  canvas.height = outputEdge
  context.clearRect(0, 0, outputEdge, outputEdge)
  context.drawImage(image, crop.x, crop.y, crop.size, crop.size, 0, 0, outputEdge, outputEdge)
}

export async function preparePhotoForUpload(
  image: HTMLImageElement,
  sourceFile: File,
  settings: CropSettings,
): Promise<File> {
  if (!isSupportedPhotoType(sourceFile.type)) throw new Error("Unsupported photo type")

  const crop = calculateSquareCrop(image.naturalWidth, image.naturalHeight, settings)
  let outputEdge = Math.min(MAX_OUTPUT_EDGE, Math.max(MIN_OUTPUT_EDGE, Math.floor(crop.size)))
  let quality = sourceFile.type === "image/png" ? undefined : 0.88

  for (let attempt = 0; attempt < 8; attempt += 1) {
    const canvas = document.createElement("canvas")
    canvas.width = outputEdge
    canvas.height = outputEdge
    const context = canvas.getContext("2d")
    if (!context) throw new Error("Canvas is not available")
    context.drawImage(image, crop.x, crop.y, crop.size, crop.size, 0, 0, outputEdge, outputEdge)

    const blob = await canvasToBlob(canvas, sourceFile.type, quality)
    if (blob.size <= MAX_PHOTO_BYTES) {
      return new File([blob], normalizedFileName(sourceFile.name, sourceFile.type), {
        type: sourceFile.type,
        lastModified: Date.now(),
      })
    }

    if (quality !== undefined && quality > 0.58) quality = Math.max(0.58, quality - 0.1)
    else outputEdge = Math.max(MIN_OUTPUT_EDGE, Math.floor(outputEdge * 0.8))
  }

  throw new Error("The prepared photo is still larger than 10 MiB")
}

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (blob) resolve(blob)
        else reject(new Error("The browser could not encode the photo"))
      },
      type,
      quality,
    )
  })
}

function normalizedFileName(name: string, type: string): string {
  const base = name.replace(/\.[^.]+$/, "") || "photo"
  const extension = type === "image/jpeg" ? "jpg" : type === "image/png" ? "png" : "webp"
  return `${base}.${extension}`
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value))
}
