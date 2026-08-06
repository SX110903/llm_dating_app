import { useQuery } from "@tanstack/react-query"
import { useEffect, useState } from "react"

import { apiFetchBlob } from "@/shared/lib/api-client"

export function useProtectedImage(path: string): string | null {
  const [objectURL, setObjectURL] = useState<string | null>(null)
  const imageQuery = useQuery({
    queryKey: ["protected-image", path],
    queryFn: () => apiFetchBlob(path),
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    let active = true
    let nextObjectURL: string | null = null

    queueMicrotask(() => {
      if (!active) return
      nextObjectURL = imageQuery.data ? URL.createObjectURL(imageQuery.data) : null
      setObjectURL(nextObjectURL)
    })

    return () => {
      active = false
      if (nextObjectURL) URL.revokeObjectURL(nextObjectURL)
    }
  }, [imageQuery.data])

  return objectURL
}
