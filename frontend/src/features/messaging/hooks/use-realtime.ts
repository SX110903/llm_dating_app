import { useQueryClient } from "@tanstack/react-query"
import { useCallback, useEffect, useRef, useState } from "react"

import * as messagingApi from "@/features/messaging/api/messaging-api"
import { conversationsQueryKey } from "@/features/messaging/hooks/use-conversations"
import { insertMessageInCache, messagesQueryKey } from "@/features/messaging/hooks/use-messages"
import { RealtimeSocket, type RealtimeStatus } from "@/features/messaging/lib/realtime-socket"
import { payloadToMessage, type ServerEvent } from "@/features/messaging/types"

/** How long a typing hint stays visible without a refresh from the sender. */
const TYPING_TTL_MS = 4_000

export interface RealtimeMessaging {
  status: RealtimeStatus
  typingMatchIds: string[]
  sendTyping: (matchId: string) => void
}

/**
 * Mounts the single session socket and routes its events into the query cache.
 * The socket only carries live delivery: everything it pushes is already
 * durable, so a dropped connection costs latency, never data.
 */
export function useRealtimeMessaging(enabled: boolean): RealtimeMessaging {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<RealtimeStatus>("idle")
  const [typingMatchIds, setTypingMatchIds] = useState<string[]>([])
  const socketRef = useRef<RealtimeSocket | null>(null)
  const typingTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>())

  const markTyping = useCallback((matchId: string) => {
    setTypingMatchIds((current) => (current.includes(matchId) ? current : [...current, matchId]))
    const timers = typingTimers.current
    const existing = timers.get(matchId)
    if (existing) clearTimeout(existing)
    timers.set(
      matchId,
      setTimeout(() => {
        timers.delete(matchId)
        setTypingMatchIds((current) => current.filter((id) => id !== matchId))
      }, TYPING_TTL_MS),
    )
  }, [])

  const handleEvent = useCallback(
    (event: ServerEvent) => {
      switch (event.type) {
        case "message":
          if (!event.message) return
          insertMessageInCache(queryClient, payloadToMessage(event.message))
          void queryClient.invalidateQueries({ queryKey: conversationsQueryKey })
          if (event.message.match_id) {
            setTypingMatchIds((current) => current.filter((id) => id !== event.message?.match_id))
          }
          return
        case "typing":
          if (event.match_id) markTyping(event.match_id)
          return
        case "conversation_closed":
          // Unmatch or block: drop the local history so a stale transcript is
          // never shown for a conversation the user no longer has access to.
          if (event.match_id) queryClient.removeQueries({ queryKey: messagesQueryKey(event.match_id) })
          void queryClient.invalidateQueries({ queryKey: conversationsQueryKey })
          return
        case "ready":
          return
      }
    },
    [markTyping, queryClient],
  )

  const handleEventRef = useRef(handleEvent)
  useEffect(() => {
    handleEventRef.current = handleEvent
  }, [handleEvent])

  useEffect(() => {
    if (!enabled) return
    const timers = typingTimers.current
    const socket = new RealtimeSocket({
      issueTicket: messagingApi.issueTicket,
      onEvent: (event) => handleEventRef.current(event),
      onStatusChange: setStatus,
    })
    socketRef.current = socket
    socket.start()

    return () => {
      socket.stop()
      socketRef.current = null
      timers.forEach((timer) => clearTimeout(timer))
      timers.clear()
    }
  }, [enabled])

  const sendTyping = useCallback((matchId: string) => {
    socketRef.current?.sendTyping(matchId)
  }, [])

  return { status, typingMatchIds, sendTyping }
}
