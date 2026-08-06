import { z } from "zod"

import { photoSchema, preferencesSchema, profileSchema, type Photo, type Preferences, type Profile } from "@/features/profile/types"
import { environment } from "@/shared/lib/env"
import { apiFetch } from "@/shared/lib/api-client"
import { ApiError } from "@/shared/lib/errors"
import { useAuthStore } from "@/shared/state/auth-store"

export async function getProfile(): Promise<Profile | null> {
  try {
    return profileSchema.parse(await apiFetch<unknown>("/profile"))
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null
    throw error
  }
}

export interface UpdateProfileInput {
  bio: string
  interests: string[]
  city: string
  latitude?: number | null
  longitude?: number | null
  questionnaire?: Record<string, unknown>
  onboarding_completed: boolean
}

export async function updateProfile(input: UpdateProfileInput): Promise<Profile> {
  return profileSchema.parse(await apiFetch<unknown>("/profile", { method: "PUT", body: input }))
}

export async function getPreferences(): Promise<Preferences | null> {
  try {
    return preferencesSchema.parse(await apiFetch<unknown>("/profile/preferences"))
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null
    throw error
  }
}

export interface UpdatePreferencesInput {
  min_age: number
  max_age: number
  max_distance_km: number
  genders?: string[]
}

export async function updatePreferences(input: UpdatePreferencesInput): Promise<Preferences> {
  return preferencesSchema.parse(await apiFetch<unknown>("/profile/preferences", { method: "PUT", body: input }))
}

export async function listPhotos(): Promise<Photo[]> {
  return z.array(photoSchema).parse(await apiFetch<unknown>("/profile/photos"))
}

// Uses XMLHttpRequest instead of apiFetch because fetch has no upload
// progress event; a single 401 here is not retried via the refresh queue,
// which is an accepted simplification for this upload path.
export function uploadPhoto(file: File, onProgress?: (percent: number) => void): Promise<Photo> {
  return new Promise((resolve, reject) => {
    const formData = new FormData()
    formData.append("photo", file)

    const xhr = new XMLHttpRequest()
    xhr.open("POST", `${environment.VITE_API_BASE_URL}/profile/photos`)
    xhr.withCredentials = true
    const token = useAuthStore.getState().accessToken
    if (token) {
      xhr.setRequestHeader("Authorization", `Bearer ${token}`)
    }

    xhr.upload.onprogress = (event) => {
      if (onProgress && event.lengthComputable) {
        onProgress(Math.round((event.loaded / event.total) * 100))
      }
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(photoSchema.parse(JSON.parse(xhr.responseText)))
        } catch (error) {
          reject(error instanceof Error ? error : new Error("could not parse upload response"))
        }
        return
      }
      let code = "UNKNOWN_ERROR"
      let message = "Upload failed"
      try {
        const parsed = JSON.parse(xhr.responseText) as { code?: string; message?: string }
        code = parsed.code ?? code
        message = parsed.message ?? message
      } catch {
        // malformed or empty error body: keep the defaults above
      }
      reject(new ApiError(xhr.status, code, message))
    }
    xhr.onerror = () => reject(new ApiError(0, "NETWORK_ERROR", "Network error while uploading the photo"))
    xhr.send(formData)
  })
}

export async function reorderPhotos(photoIds: string[]): Promise<void> {
  await apiFetch<void>("/profile/photos/order", { method: "PUT", body: { photo_ids: photoIds } })
}

export async function setPrimaryPhoto(photoId: string): Promise<void> {
  await apiFetch<void>(`/profile/photos/${photoId}/primary`, { method: "PUT" })
}

export async function deletePhoto(photoId: string): Promise<void> {
  await apiFetch<void>(`/profile/photos/${photoId}`, { method: "DELETE" })
}
