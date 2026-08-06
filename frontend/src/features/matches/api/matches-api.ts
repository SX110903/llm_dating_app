import { matchPageSchema } from "@/features/matches/types"
import { apiFetch } from "@/shared/lib/api-client"

export async function listMatches() {
  return matchPageSchema.parse(await apiFetch<unknown>("/matches?limit=50"))
}

export async function unmatch(matchId: string): Promise<void> {
  await apiFetch<void>(`/matches/${matchId}`, { method: "DELETE" })
}
