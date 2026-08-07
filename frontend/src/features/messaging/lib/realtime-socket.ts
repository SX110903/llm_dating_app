import { serverEventSchema, type ServerEvent, type Ticket } from "@/features/messaging/types"
import { environment } from "@/shared/lib/env"

export type RealtimeStatus = "idle" | "connecting" | "open" | "reconnecting" | "closed"

export interface RealtimeSocketOptions {
  issueTicket: () => Promise<Ticket>
  onEvent: (event: ServerEvent) => void
  onStatusChange?: (status: RealtimeStatus) => void
  /** Injected in tests so the reconnect policy runs without real timers. */
  createSocket?: (url: string, protocols: string[]) => WebSocket
  now?: () => number
}

const BASE_RECONNECT_DELAY_MS = 500
const MAX_RECONNECT_DELAY_MS = 30_000

/**
 * Owns one socket for the whole session. The hub is per user, not per match,
 * so a single connection serves every open conversation.
 *
 * Reconnects use exponential backoff with jitter and always fetch a new
 * ticket, because tickets are consumed by the handshake and cannot be reused.
 */
export class RealtimeSocket {
  private socket: WebSocket | null = null
  private reconnectAttempt = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private stopped = false
  private status: RealtimeStatus = "idle"
  /** Guards against a late handshake resolving after stop() or a newer connect. */
  private generation = 0

  constructor(private readonly options: RealtimeSocketOptions) {}

  start(): void {
    this.stopped = false
    void this.connect()
  }

  stop(): void {
    this.stopped = true
    this.generation += 1
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    const socket = this.socket
    this.socket = null
    socket?.close(1000, "client closed")
    this.setStatus("closed")
  }

  /**
   * Typing is advisory: it is dropped when the socket is not open rather than
   * queued, since a typing hint that arrives late is noise.
   */
  sendTyping(matchId: string): void {
    if (this.socket?.readyState !== WebSocket.OPEN) return
    this.socket.send(JSON.stringify({ type: "typing", match_id: matchId }))
  }

  private async connect(): Promise<void> {
    if (this.stopped) return
    const generation = ++this.generation
    this.setStatus(this.reconnectAttempt === 0 ? "connecting" : "reconnecting")

    let ticket: Ticket
    try {
      ticket = await this.options.issueTicket()
    } catch {
      this.scheduleReconnect()
      return
    }
    if (this.stopped || generation !== this.generation) return

    const create = this.options.createSocket ?? ((url, protocols) => new WebSocket(url, protocols))
    // The ticket travels as a subprotocol entry, never in the URL, so it stays
    // out of proxy logs, referrers and browser history.
    const socket = create(realtimeURL(), [ticket.subprotocol, ticket.ticket])
    this.socket = socket

    socket.onopen = () => {
      if (generation !== this.generation) return
      this.reconnectAttempt = 0
      this.setStatus("open")
    }
    socket.onmessage = (event) => {
      if (generation !== this.generation) return
      const parsed = parseEvent(event.data)
      if (parsed) this.options.onEvent(parsed)
    }
    socket.onclose = () => {
      if (generation !== this.generation) return
      this.socket = null
      this.scheduleReconnect()
    }
    socket.onerror = () => {
      // onclose always follows, and that is where reconnection is decided.
    }
  }

  private scheduleReconnect(): void {
    if (this.stopped) return
    this.setStatus("reconnecting")
    const delay = reconnectDelay(this.reconnectAttempt)
    this.reconnectAttempt += 1
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      void this.connect()
    }, delay)
  }

  private setStatus(status: RealtimeStatus): void {
    if (this.status === status) return
    this.status = status
    this.options.onStatusChange?.(status)
  }
}

/** Full jitter, so a server restart does not bring every client back at once. */
export function reconnectDelay(attempt: number, random: () => number = Math.random): number {
  const ceiling = Math.min(MAX_RECONNECT_DELAY_MS, BASE_RECONNECT_DELAY_MS * 2 ** attempt)
  return Math.round(ceiling * (0.5 + random() * 0.5))
}

export function realtimeURL(): string {
  const base = new URL(environment.VITE_API_BASE_URL.replace(/\/$/, ""), window.location.origin)
  base.protocol = base.protocol === "https:" ? "wss:" : "ws:"
  base.pathname = `${base.pathname}/ws`
  return base.toString()
}

function parseEvent(data: unknown): ServerEvent | null {
  if (typeof data !== "string") return null
  try {
    const result = serverEventSchema.safeParse(JSON.parse(data))
    return result.success ? result.data : null
  } catch {
    return null
  }
}
