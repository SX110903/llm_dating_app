import { z } from "zod"

const environmentSchema = z.object({
  VITE_API_BASE_URL: z.string().default("/api/v1"),
})

export const environment = environmentSchema.parse(import.meta.env)

