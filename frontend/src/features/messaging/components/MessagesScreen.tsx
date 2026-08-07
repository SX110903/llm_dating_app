import { useMemo, useState } from "react"

import { ChatWindow } from "@/features/messaging/components/ChatWindow"
import { ConversationList } from "@/features/messaging/components/ConversationList"
import { useConversationsQuery } from "@/features/messaging/hooks/use-conversations"
import { useRealtimeMessaging } from "@/features/messaging/hooks/use-realtime"
import { useAuthStore } from "@/shared/state/auth-store"

export function MessagesScreen() {
  const viewerId = useAuthStore((state) => state.user?.id ?? "")
  const conversations = useConversationsQuery()
  const realtime = useRealtimeMessaging(viewerId !== "")
  const [selectedMatchId, setSelectedMatchId] = useState<string | null>(null)

  const items = useMemo(() => conversations.data?.conversations ?? [], [conversations.data])
  // Resolved from the live list rather than stored, so an unmatch or block
  // removes the open conversation instead of leaving a dangling selection.
  const selected = items.find((conversation) => conversation.match_id === selectedMatchId) ?? null

  if (conversations.isPending) {
    return <p className="py-20 text-center text-sm text-white/45">Cargando conversaciones...</p>
  }
  if (conversations.isError) {
    return (
      <div className="py-20 text-center">
        <p className="text-sm text-rose-300">No se pudieron cargar tus conversaciones.</p>
        <button
          type="button"
          onClick={() => void conversations.refetch()}
          className="mt-4 min-h-11 text-sm text-white/60 underline"
        >
          Reintentar
        </button>
      </div>
    )
  }

  return (
    <div className="mx-auto w-full max-w-4xl">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.22em] text-pink-300/70">Mensajes</p>
          <h1 className="mt-1 font-display text-3xl font-semibold">Tus conversaciones</h1>
        </div>
        <ConnectionBadge status={realtime.status} />
      </div>

      <div className="overflow-hidden rounded-3xl border border-white/10 bg-white/[0.025] sm:grid sm:grid-cols-[minmax(0,18rem)_1fr] sm:divide-x sm:divide-white/8">
        <div className={`p-2 ${selected ? "hidden sm:block" : "block"}`}>
          <ConversationList
            conversations={items}
            activeMatchId={selected?.match_id ?? null}
            typingMatchIds={realtime.typingMatchIds}
            onSelect={setSelectedMatchId}
          />
        </div>

        <div className={selected ? "block" : "hidden sm:block"}>
          {selected ? (
            <ChatWindow
              key={selected.match_id}
              conversation={selected}
              viewerId={viewerId}
              typing={realtime.typingMatchIds.includes(selected.match_id)}
              onTyping={realtime.sendTyping}
              onBack={() => setSelectedMatchId(null)}
            />
          ) : (
            <p className="flex h-[600px] items-center justify-center px-6 text-center text-sm text-white/40">
              Elige una conversacion para empezar.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

function ConnectionBadge({ status }: { status: string }) {
  if (status === "open") return null
  const label = status === "reconnecting" ? "Reconectando..." : "Conectando..."
  return (
    <p role="status" className="text-xs text-white/40">
      {label}
    </p>
  )
}
