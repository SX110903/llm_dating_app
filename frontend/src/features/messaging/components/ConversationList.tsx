import { MessagesSquare } from "lucide-react"

import type { Conversation } from "@/features/messaging/types"
import { useProtectedImage } from "@/features/swipe/hooks/use-protected-image"

interface ConversationListProps {
  conversations: Conversation[]
  activeMatchId: string | null
  typingMatchIds: string[]
  onSelect: (matchId: string) => void
}

export function ConversationList({ conversations, activeMatchId, typingMatchIds, onSelect }: ConversationListProps) {
  if (conversations.length === 0) {
    return (
      <div className="rounded-3xl border border-dashed border-white/10 py-16 text-center">
        <MessagesSquare aria-hidden className="mx-auto h-8 w-8 text-white/25" />
        <h2 className="mt-4 font-display text-xl font-semibold">Sin conversaciones</h2>
        <p className="mt-2 text-sm text-white/45">Cuando tengas un match podras escribirle aqui.</p>
      </div>
    )
  }

  return (
    <ul aria-label="Conversaciones" className="flex flex-col gap-1">
      {conversations.map((conversation) => (
        <li key={conversation.match_id}>
          <ConversationRow
            conversation={conversation}
            active={conversation.match_id === activeMatchId}
            typing={typingMatchIds.includes(conversation.match_id)}
            onSelect={() => onSelect(conversation.match_id)}
          />
        </li>
      ))}
    </ul>
  )
}

function ConversationRow({
  conversation,
  active,
  typing,
  onSelect,
}: {
  conversation: Conversation
  active: boolean
  typing: boolean
  onSelect: () => void
}) {
  const preview = typing ? "Escribiendo..." : (conversation.last_message?.content ?? "Di hola")

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={active ? "true" : undefined}
      className={`flex min-h-16 w-full items-center gap-3 rounded-2xl p-3 text-left transition ${
        active ? "bg-white/10" : "hover:bg-white/5"
      }`}
    >
      <Avatar photoId={conversation.primary_photo_id} displayName={conversation.display_name} />
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline justify-between gap-2">
          <span className="truncate font-medium">{conversation.display_name}</span>
          {conversation.unread_count > 0 && (
            <span className="shrink-0 rounded-full bg-pink-500 px-2 py-0.5 text-[11px] font-semibold text-white">
              <span aria-hidden>{conversation.unread_count}</span>
              <span className="sr-only">
                {conversation.unread_count} {conversation.unread_count === 1 ? "mensaje" : "mensajes"} sin leer
              </span>
            </span>
          )}
        </span>
        <span className={`mt-0.5 block truncate text-xs ${typing ? "text-pink-300/80" : "text-white/45"}`}>{preview}</span>
      </span>
    </button>
  )
}

function Avatar({ photoId, displayName }: { photoId: string | undefined; displayName: string }) {
  if (!photoId) {
    return (
      <span
        aria-hidden
        className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-white/8 text-sm font-semibold text-white/50"
      >
        {displayName.slice(0, 1).toUpperCase()}
      </span>
    )
  }
  return <AvatarImage photoId={photoId} displayName={displayName} />
}

function AvatarImage({ photoId, displayName }: { photoId: string; displayName: string }) {
  const imageURL = useProtectedImage(`/matching/photos/${photoId}/content`)
  return (
    <span className="h-12 w-12 shrink-0 overflow-hidden rounded-full bg-white/8">
      {imageURL ? <img src={imageURL} alt={`Foto de ${displayName}`} className="h-full w-full object-cover" /> : null}
    </span>
  )
}
