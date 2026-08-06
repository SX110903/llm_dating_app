import { useEffect, useState } from "react"

import { environment } from "@/shared/lib/env"
import { useAuthStore } from "@/shared/state/auth-store"

// The content endpoint requires a Bearer token, which a plain <img src>
// cannot send, so the blob is fetched manually and exposed as an object URL.
// This does not go through the shared refresh queue; acceptable since photos
// are viewed right after login/upload, well within the access token's TTL.
export function usePhotoImageUrl(photoId: string): string | null {
  const [url, setUrl] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    let objectUrl: string | null = null

    async function load() {
      const token = useAuthStore.getState().accessToken
      const response = await fetch(`${environment.VITE_API_BASE_URL}/profile/photos/${photoId}/content`, {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        credentials: "include",
      })
      if (!response.ok || cancelled) return
      const blob = await response.blob()
      if (cancelled) return
      objectUrl = URL.createObjectURL(blob)
      setUrl(objectUrl)
    }

    void load()

    return () => {
      cancelled = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [photoId])

  return url
}
