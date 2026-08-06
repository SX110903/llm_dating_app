import { z } from "zod"

export const matchSchema = z.object({
  id: z.string().uuid(),
  other_user_id: z.string().uuid(),
  display_name: z.string(),
  bio: z.string(),
  city: z.string(),
  photo_url: z.string(),
  matched_at: z.string(),
  last_active_at: z.string(),
})

export const matchPageSchema = z.object({
  matches: z.array(matchSchema),
  next_cursor: z.string().optional(),
})

export type Match = z.infer<typeof matchSchema>
