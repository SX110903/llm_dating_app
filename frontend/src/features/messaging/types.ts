import { z } from "zod"

/** Mirrors MessageResponse. `read_at` is always present, null when unread. */
export const messageSchema = z.object({
  id: z.string().uuid(),
  match_id: z.string().uuid(),
  sender_id: z.string().uuid(),
  client_nonce: z.string().uuid().optional(),
  type: z.string(),
  content: z.string().optional().default(""),
  storage_key: z.string().optional(),
  read_at: z.string().nullable().default(null),
  created_at: z.string(),
})

/**
 * Mirrors MessagePayload, the shape pushed over the socket. It carries no
 * `read_at`: read state is owned by the receiving side and only ever arrives
 * over HTTP, so the socket payload is normalised into a Message with the field
 * left unread rather than inventing a value for it.
 */
export const messagePayloadSchema = z.object({
  id: z.string().uuid(),
  match_id: z.string().uuid(),
  sender_id: z.string().uuid(),
  client_nonce: z.string().uuid().optional(),
  type: z.string(),
  content: z.string().optional().default(""),
  storage_key: z.string().optional(),
  created_at: z.string(),
})

export const historyPageSchema = z.object({
  messages: z.array(messageSchema),
  next_cursor: z.string().optional(),
})

export const conversationSchema = z.object({
  match_id: z.string().uuid(),
  other_user_id: z.string().uuid(),
  display_name: z.string(),
  primary_photo_id: z.string().uuid().optional(),
  last_message: messageSchema.optional(),
  unread_count: z.number().int().nonnegative(),
  matched_at: z.string(),
})

export const conversationListSchema = z.object({
  conversations: z.array(conversationSchema),
})

export const ticketSchema = z.object({
  ticket: z.string().min(1),
  expires_at: z.string(),
  subprotocol: z.string().min(1),
})

export const markReadSchema = z.object({ updated: z.number().int().nonnegative() })

export const serverEventSchema = z.object({
  type: z.enum(["message", "typing", "conversation_closed", "ready"]),
  message: messagePayloadSchema.optional(),
  match_id: z.string().uuid().optional(),
  user_id: z.string().uuid().optional(),
  sent_at: z.string(),
})

export type Message = z.infer<typeof messageSchema>
export type MessagePayload = z.infer<typeof messagePayloadSchema>
export type HistoryPage = z.infer<typeof historyPageSchema>
export type Conversation = z.infer<typeof conversationSchema>
export type Ticket = z.infer<typeof ticketSchema>
export type ServerEvent = z.infer<typeof serverEventSchema>

/** A message the user submitted that the server has not acknowledged yet. */
export interface PendingMessage {
  clientNonce: string
  content: string
  createdAt: string
  failed: boolean
}

export function payloadToMessage(payload: MessagePayload): Message {
  return { ...payload, read_at: null }
}
