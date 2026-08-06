import { z } from "zod"

export const scoreSchema = z.object({
  interests: z.number(),
  questionnaire: z.number(),
  distance: z.number(),
  activity: z.number(),
  total: z.number(),
})

export const candidateSchema = z.object({
  user_id: z.string().uuid(),
  display_name: z.string(),
  age: z.number().int(),
  gender: z.string(),
  bio: z.string(),
  interests: z.array(z.string()),
  city: z.string(),
  distance_km: z.number(),
  last_active_at: z.string(),
  photo_url: z.string(),
  score: scoreSchema,
})

export const discoveryPageSchema = z.object({
  candidates: z.array(candidateSchema),
  next_cursor: z.string().optional(),
})

export const createdMatchSchema = z.object({
  id: z.string().uuid(),
  matched_at: z.string(),
})

export const swipeResultSchema = z.object({
  id: z.string().uuid(),
  target_id: z.string().uuid(),
  action: z.enum(["like", "dislike", "superlike"]),
  created_at: z.string(),
  match: createdMatchSchema.optional(),
})

export type Candidate = z.infer<typeof candidateSchema>
export type DiscoveryPage = z.infer<typeof discoveryPageSchema>
export type SwipeAction = z.infer<typeof swipeResultSchema>["action"]
export type SwipeResult = z.infer<typeof swipeResultSchema>

export const reportReasons = [
  "harassment",
  "spam",
  "inappropriate_content",
  "impersonation",
  "underage",
  "other",
] as const

export type ReportReason = (typeof reportReasons)[number]
