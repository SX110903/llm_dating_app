import { z } from "zod"

export const authUserSchema = z.object({
  id: z.string(),
  email: z.string(),
  display_name: z.string(),
  birth_date: z.string(),
  gender: z.string(),
  status: z.enum(["active", "suspended", "deleted"]),
  email_verified_at: z.string().nullable(),
  created_at: z.string(),
})

export const loginResponseSchema = z.object({
  access_token: z.string(),
  access_token_expires_at: z.string(),
  user: authUserSchema,
})
export type LoginResponse = z.infer<typeof loginResponseSchema>
