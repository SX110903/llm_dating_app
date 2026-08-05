import { z } from "zod"

import { environment } from "@/shared/lib/env"

const healthSchema = z.object({
  status: z.enum(["healthy", "degraded"]),
  checks: z.record(z.string(), z.enum(["up", "down"])),
})

export type Health = z.infer<typeof healthSchema>

export async function getReadiness(): Promise<Health> {
  const response = await fetch(`${environment.VITE_API_BASE_URL}/health/ready`, {
    headers: { Accept: "application/json" },
  })
  if (!response.ok) {
    throw new Error("Readiness check failed")
  }
  return healthSchema.parse(await response.json())
}

