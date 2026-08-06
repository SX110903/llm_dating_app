import { useState } from "react"

export interface Coordinates {
  latitude: number
  longitude: number
}

type Status = "idle" | "requesting" | "denied" | "unavailable" | "failed"

const MESSAGES: Record<Exclude<Status, "idle" | "requesting">, string> = {
  denied: "No diste permiso de ubicación. Puedes activarlo desde los ajustes del navegador.",
  unavailable: "Este navegador no puede darnos tu ubicación.",
  failed: "No pudimos obtener tu ubicación. Inténtalo de nuevo.",
}

/**
 * Wraps navigator.geolocation. Denial is a normal outcome, not an error to
 * surface as a crash: the caller keeps working without coordinates and the
 * profile simply stays out of discovery.
 */
export function useGeolocation() {
  const [status, setStatus] = useState<Status>("idle")

  const request = (onSuccess: (coordinates: Coordinates) => void) => {
    if (typeof navigator === "undefined" || !navigator.geolocation) {
      setStatus("unavailable")
      return
    }

    setStatus("requesting")
    navigator.geolocation.getCurrentPosition(
      (position) => {
        setStatus("idle")
        onSuccess({ latitude: position.coords.latitude, longitude: position.coords.longitude })
      },
      (error) => {
        setStatus(error.code === error.PERMISSION_DENIED ? "denied" : "failed")
      },
      { enableHighAccuracy: false, timeout: 10_000, maximumAge: 300_000 },
    )
  }

  const errorMessage = status === "idle" || status === "requesting" ? null : MESSAGES[status]

  return { request, isRequesting: status === "requesting", errorMessage }
}
