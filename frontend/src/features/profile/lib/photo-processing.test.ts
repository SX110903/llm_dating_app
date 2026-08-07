import { describe, expect, it } from "vitest"

import { calculateSquareCrop, isSupportedPhotoType } from "@/features/profile/lib/photo-processing"

describe("photo processing", () => {
  it("keeps a centered square crop inside a landscape image", () => {
    expect(calculateSquareCrop(1200, 800, { zoom: 1, offsetX: 0, offsetY: 0 })).toEqual({
      x: 200,
      y: 0,
      size: 800,
    })
  })

  it("clamps zoom and offsets so the crop never leaves the image", () => {
    const crop = calculateSquareCrop(1200, 800, { zoom: 10, offsetX: 4, offsetY: -4 })
    expect(crop.x).toBeCloseTo(933.33)
    expect(crop.y).toBe(0)
    expect(crop.size).toBeCloseTo(266.67)
  })

  it("only accepts the three server-supported MIME types", () => {
    expect(isSupportedPhotoType("image/jpeg")).toBe(true)
    expect(isSupportedPhotoType("image/png")).toBe(true)
    expect(isSupportedPhotoType("image/webp")).toBe(true)
    expect(isSupportedPhotoType("image/avif")).toBe(false)
  })
})
