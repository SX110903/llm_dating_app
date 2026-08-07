import { AlertCircle, ArrowLeft, Send } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { useMarkRead, useMessageHistory, useOrderedMessages, useSendMessage } from "@/features/messaging/hooks/use-messages"
import type { Conversation, Message, PendingMessage } from "@/features/messaging/types"

/** Mirrors domainmessaging.MaxContentLength; the server rejects anything longer. */
const MAX_CONTENT_LENGTH = 2000

interface ChatWindowProps {
  conversation: Conversation
  viewerId: string
  typing: boolean
  onTyping: (matchId: string) => void
  onBack: () => void
}

export function ChatWindow({ conversation, viewerId, typing, onTyping, onBack }: ChatWindowProps) {
  const matchId = conversation.match_id
  const history = useMessageHistory(matchId)
  const messages = useOrderedMessages(history.data?.pages)
  const { pending, send, retry, discard } = useSendMessage(matchId)
  const markRead = useMarkRead(matchId)
  const bottomRef = useRef<HTMLDivElement | null>(null)

  const newestIncomingUnreadId = useMemo(() => {
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      const message = messages[index]
      if (message.sender_id !== viewerId) return message.read_at === null ? message.id : null
    }
    return null
  }, [messages, viewerId])

  const markReadMutate = markRead.mutate
  useEffect(() => {
    if (newestIncomingUnreadId) markReadMutate()
  }, [markReadMutate, newestIncomingUnreadId])

  useEffect(() => {
    // Optional call: jsdom has no scrollIntoView, and a missing scroll must not
    // break rendering.
    bottomRef.current?.scrollIntoView?.({ block: "end" })
  }, [messages.length, pending.length])

  return (
    <section aria-label={`Conversacion con ${conversation.display_name}`} className="flex h-[70vh] flex-col sm:h-[600px]">
      <header className="flex items-center gap-2 border-b border-white/8 px-2 py-2">
        <button
          type="button"
          onClick={onBack}
          aria-label="Volver a conversaciones"
          className="flex h-11 w-11 items-center justify-center rounded-full text-white/50 hover:bg-white/5 hover:text-white sm:hidden"
        >
          <ArrowLeft aria-hidden className="h-5 w-5" />
        </button>
        <div className="min-w-0 flex-1">
          <h2 className="truncate font-display text-lg font-semibold">{conversation.display_name}</h2>
          <p className="text-xs text-white/40">{typing ? "Escribiendo..." : "En tus matches"}</p>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto px-3 py-4">
        {history.hasNextPage && (
          <div className="mb-4 text-center">
            <button
              type="button"
              onClick={() => void history.fetchNextPage()}
              disabled={history.isFetchingNextPage}
              className="min-h-11 rounded-full border border-white/10 px-4 text-xs text-white/55 hover:bg-white/5 disabled:opacity-40"
            >
              {history.isFetchingNextPage ? "Cargando..." : "Ver mensajes anteriores"}
            </button>
          </div>
        )}

        {history.isPending && <p className="py-10 text-center text-sm text-white/45">Cargando mensajes...</p>}
        {history.isError && (
          <div className="py-10 text-center">
            <p className="text-sm text-rose-300">No se pudo cargar la conversacion.</p>
            <button
              type="button"
              onClick={() => void history.refetch()}
              className="mt-3 min-h-11 text-sm text-white/60 underline"
            >
              Reintentar
            </button>
          </div>
        )}
        {history.isSuccess && messages.length === 0 && pending.length === 0 && (
          <p className="py-10 text-center text-sm text-white/45">Todavia no hay mensajes. Rompe el hielo.</p>
        )}

        <ol className="flex flex-col gap-2">
          {messages.map((message) => (
            <MessageBubble key={message.id} message={message} own={message.sender_id === viewerId} />
          ))}
          {pending.map((item) => (
            <PendingBubble key={item.clientNonce} pending={item} onRetry={() => retry(item.clientNonce)} onDiscard={() => discard(item.clientNonce)} />
          ))}
        </ol>
        <div ref={bottomRef} />
      </div>

      <Composer matchId={matchId} onSend={send} onTyping={onTyping} />
    </section>
  )
}

function MessageBubble({ message, own }: { message: Message; own: boolean }) {
  return (
    <li className={`flex ${own ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[85%] rounded-2xl px-3 py-2 text-sm leading-5 sm:max-w-[70%] ${
          own ? "bg-pink-500/85 text-white" : "bg-white/8 text-white/85"
        }`}
      >
        <p className="whitespace-pre-wrap break-words">{message.content}</p>
        <p className={`mt-1 text-[10px] ${own ? "text-white/70" : "text-white/35"}`}>
          <time dateTime={message.created_at}>{formatTime(message.created_at)}</time>
          {own && message.read_at !== null && <span className="ml-1">Leido</span>}
        </p>
      </div>
    </li>
  )
}

function PendingBubble({
  pending,
  onRetry,
  onDiscard,
}: {
  pending: PendingMessage
  onRetry: () => void
  onDiscard: () => void
}) {
  return (
    <li className="flex justify-end">
      <div className={`max-w-[85%] rounded-2xl px-3 py-2 text-sm leading-5 sm:max-w-[70%] ${pending.failed ? "bg-rose-500/20" : "bg-pink-500/45"}`}>
        <p className="whitespace-pre-wrap break-words text-white/90">{pending.content}</p>
        {pending.failed ? (
          <p className="mt-1 flex items-center gap-2 text-[11px] text-rose-200">
            <AlertCircle aria-hidden className="h-3.5 w-3.5" />
            No se envio.
            <button type="button" onClick={onRetry} className="min-h-11 underline">
              Reintentar
            </button>
            <button type="button" onClick={onDiscard} className="min-h-11 underline">
              Descartar
            </button>
          </p>
        ) : (
          <p className="mt-1 text-[10px] text-white/70">Enviando...</p>
        )}
      </div>
    </li>
  )
}

function Composer({
  matchId,
  onSend,
  onTyping,
}: {
  matchId: string
  onSend: (content: string) => void
  onTyping: (matchId: string) => void
}) {
  const [draft, setDraft] = useState("")
  const lastTypingAt = useRef(0)

  const submit = () => {
    if (draft.trim() === "") return
    onSend(draft)
    setDraft("")
  }

  return (
    <form
      className="flex items-end gap-2 border-t border-white/8 p-3"
      onSubmit={(event) => {
        event.preventDefault()
        submit()
      }}
    >
      <label className="sr-only" htmlFor={`composer-${matchId}`}>
        Escribe un mensaje
      </label>
      <textarea
        id={`composer-${matchId}`}
        value={draft}
        rows={1}
        maxLength={MAX_CONTENT_LENGTH}
        placeholder="Escribe un mensaje"
        onChange={(event) => {
          setDraft(event.target.value)
          // Throttled so a fast typist does not emit a frame per keystroke.
          const now = Date.now()
          if (now - lastTypingAt.current > 2000) {
            lastTypingAt.current = now
            onTyping(matchId)
          }
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault()
            submit()
          }
        }}
        className="min-h-11 flex-1 resize-none rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3 text-sm outline-none placeholder:text-white/30 focus:border-white/25"
      />
      <button
        type="submit"
        disabled={draft.trim() === ""}
        aria-label="Enviar mensaje"
        className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-pink-500 text-white transition hover:bg-pink-400 disabled:opacity-35"
      >
        <Send aria-hidden className="h-4 w-4" />
      </button>
    </form>
  )
}

function formatTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}
