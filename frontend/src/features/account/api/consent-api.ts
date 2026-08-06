import { z } from "zod"

import { apiFetch } from "@/shared/lib/api-client"
import { ApiError } from "@/shared/lib/errors"

const GENDER_PREFERENCE_PURPOSE = "matching_gender_preferences"

export const consentSchema = z.object({
  purpose: z.string(),
  policy_version: z.string(),
  granted_at: z.string(),
})
export type Consent = z.infer<typeof consentSchema>

export async function getGenderPreferenceConsent(): Promise<Consent | null> {
  try {
    return consentSchema.parse(await apiFetch<unknown>(`/account/consents/${GENDER_PREFERENCE_PURPOSE}`))
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null
    throw error
  }
}

export async function grantGenderPreferenceConsent(): Promise<Consent> {
  return consentSchema.parse(
    await apiFetch<unknown>("/account/consents", { method: "POST", body: { purpose: GENDER_PREFERENCE_PURPOSE } }),
  )
}

export async function withdrawGenderPreferenceConsent(): Promise<void> {
  await apiFetch<void>(`/account/consents/${GENDER_PREFERENCE_PURPOSE}`, { method: "DELETE" })
}
