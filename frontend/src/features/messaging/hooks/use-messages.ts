import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query"
import { useCallback, useMemo, useState } from "react"

import * as messagingApi from "@/features/messaging/api/messaging-api"
import { conversationsQueryKey } from "@/features/messaging/hooks/use-conversations"
import type { HistoryPage, Message, PendingMessage } from "@/features/messaging/types"

export const messagesQueryKey = (matchId: string) => ["messages", matchId] as const

type MessagesCache = InfiniteData<HistoryPage, string | undefined>

/**
 * Inserts a message into the newest page, keyed by server id so the same
 * message arriving twice — once as the send response and once as the socket
 * echo — cannot appear twice.
 *
 * A cache miss is deliberately a no-op: with no page loaded there is no
 * ordering to insert into, and the conversation will fetch it from the server
 * when opened.
 */
export function insertMessageInCache(queryClient: QueryClient, message: Message): void {
  queryClient.setQueryData<MessagesCache>(messagesQueryKey(message.match_id), (cache) => {
    if (!cache || cache.pages.length === 0) return cache
    const [newest, ...rest] = cache.pages
    if (newest.messages.some((existing) => existing.id === message.id)) return cache
    return { ...cache, pages: [{ ...newest, messages: [message, ...newest.messages] }, ...rest] }
  })
}

export function useMessageHistory(matchId: string) {
  return useInfiniteQuery({
    queryKey: messagesQueryKey(matchId),
    queryFn: ({ pageParam }) => messagingApi.listMessages(matchId, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
  })
}

export function useMarkRead(matchId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => messagingApi.markRead(matchId),
    onSuccess: async ({ updated }) => {
      if (updated > 0) await queryClient.invalidateQueries({ queryKey: conversationsQueryKey })
    },
  })
}

/**
 * Send with an optimistic echo. The nonce is generated once per attempt and
 * reused by retries, so the server collapses duplicates instead of the UI
 * having to guess whether a timed-out request landed.
 */
export function useSendMessage(matchId: string) {
  const queryClient = useQueryClient()
  const [pending, setPending] = useState<PendingMessage[]>([])

  const patch = useCallback((clientNonce: string, changes: Partial<PendingMessage>) => {
    setPending((current) => current.map((item) => (item.clientNonce === clientNonce ? { ...item, ...changes } : item)))
  }, [])

  const deliver = useCallback(
    async (clientNonce: string, content: string) => {
      try {
        const message = await messagingApi.sendMessage(matchId, clientNonce, content)
        insertMessageInCache(queryClient, message)
        setPending((current) => current.filter((item) => item.clientNonce !== clientNonce))
        await queryClient.invalidateQueries({ queryKey: conversationsQueryKey })
        return true
      } catch {
        patch(clientNonce, { failed: true })
        return false
      }
    },
    [matchId, patch, queryClient],
  )

  const send = useCallback(
    (content: string) => {
      const trimmed = content.trim()
      if (trimmed === "") return
      const clientNonce = crypto.randomUUID()
      setPending((current) => [
        ...current,
        { clientNonce, content: trimmed, createdAt: new Date().toISOString(), failed: false },
      ])
      void deliver(clientNonce, trimmed)
    },
    [deliver],
  )

  const retry = useCallback(
    (clientNonce: string) => {
      const target = pending.find((item) => item.clientNonce === clientNonce)
      if (!target) return
      patch(clientNonce, { failed: false })
      void deliver(clientNonce, target.content)
    },
    [deliver, patch, pending],
  )

  const discard = useCallback((clientNonce: string) => {
    setPending((current) => current.filter((item) => item.clientNonce !== clientNonce))
  }, [])

  return { pending, send, retry, discard }
}

/** Flattens the paginated history into render order: oldest first. */
export function useOrderedMessages(pages: HistoryPage[] | undefined): Message[] {
  return useMemo(() => {
    if (!pages) return []
    // Every page is newest-first and pages walk backwards in time, so the
    // whole list reversed is chronological.
    return pages.flatMap((page) => page.messages).reverse()
  }, [pages])
}
