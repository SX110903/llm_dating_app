import { useQuery } from "@tanstack/react-query"
import { useEffect, useMemo } from "react"

import { apiFetchBlob } from "@/shared/lib/api-client"

export function useProtectedImage(path: string): string | null {
  const imageQuery = useQuery({
    queryKey: ["protected-image", path],
    queryFn: () => apiFetchBlob(path),
    staleTime: 5 * 60 * 1000,
  })
  const objectURL = useMemo(() => (imageQuery.data ? URL.createObjectURL(imageQuery.data) : null), [imageQuery.data])

  useEffect(
    () => () => {
      if (objectURL) URL.revokeObjectURL(objectURL)
    },
    [objectURL],
  )

  return objectURL
}
