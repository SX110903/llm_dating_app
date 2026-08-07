import {
  conversationListSchema,
  historyPageSchema,
  markReadSchema,
  messageSchema,
  ticketSchema,
} from "@/features/messaging/types"
import { apiFetch } from "@/shared/lib/api-client"

export async function listConversations() {
  return conversationListSchema.parse(await apiFetch<unknown>("/conversations?limit=50"))
}

export async function listMessages(matchId: string, cursor?: string) {
  const query = new URLSearchParams({ limit: "40" })
  if (cursor) query.set("cursor", cursor)
  return historyPageSchema.parse(await apiFetch<unknown>(`/matches/${matchId}/messages?${query.toString()}`))
}

/**
 * The nonce makes this safe to retry: the server returns the stored message
 * instead of creating a second one, so a resend after a timeout cannot
 * duplicate the conversation.
 */
export async function sendMessage(matchId: string, clientNonce: string, content: string) {
  return messageSchema.parse(
    await apiFetch<unknown>(`/matches/${matchId}/messages`, {
      method: "POST",
      body: { client_nonce: clientNonce, type: "text", content },
    }),
  )
}

export async function markRead(matchId: string) {
  return markReadSchema.parse(await apiFetch<unknown>(`/matches/${matchId}/messages/read`, { method: "POST" }))
}

/** Single-use and short-lived: every socket connection needs a fresh one. */
export async function issueTicket() {
  return ticketSchema.parse(await apiFetch<unknown>("/messaging/tickets", { method: "POST" }))
}
