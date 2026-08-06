import { discoveryPageSchema, swipeResultSchema, type ReportReason, type SwipeAction, type SwipeResult } from "@/features/swipe/types"
import { apiFetch } from "@/shared/lib/api-client"

export async function getDiscovery(cursor?: string) {
  const query = new URLSearchParams({ limit: "10" })
  if (cursor) query.set("cursor", cursor)
  return discoveryPageSchema.parse(await apiFetch<unknown>(`/discovery?${query.toString()}`))
}

export async function createSwipe(input: { targetId: string; action: SwipeAction }): Promise<SwipeResult> {
  return swipeResultSchema.parse(
    await apiFetch<unknown>("/swipes", {
      method: "POST",
      body: { target_id: input.targetId, action: input.action },
    }),
  )
}

export async function blockUser(userId: string): Promise<void> {
  await apiFetch<void>("/blocks", { method: "POST", body: { user_id: userId } })
}

export async function reportUser(input: { userId: string; reason: ReportReason; description: string }): Promise<void> {
  await apiFetch<unknown>("/reports", {
    method: "POST",
    body: { user_id: input.userId, reason: input.reason, description: input.description },
  })
}
