import { z } from "zod"

export const profileSchema = z.object({
  user_id: z.string(),
  bio: z.string(),
  interests: z.array(z.string()),
  city: z.string(),
  has_location: z.boolean(),
  questionnaire: z.record(z.string(), z.unknown()),
  onboarding_completed: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
})
export type Profile = z.infer<typeof profileSchema>

export const preferencesSchema = z.object({
  min_age: z.number(),
  max_age: z.number(),
  max_distance_km: z.number(),
  genders: z.array(z.string()),
})
export type Preferences = z.infer<typeof preferencesSchema>

export const photoSchema = z.object({
  id: z.string(),
  mime_type: z.string(),
  byte_size: z.number(),
  width: z.number(),
  height: z.number(),
  position: z.number(),
  is_primary: z.boolean(),
  created_at: z.string(),
})
export type Photo = z.infer<typeof photoSchema>
