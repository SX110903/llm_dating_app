import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { RealtimeSocket, realtimeURL, reconnectDelay } from "@/features/messaging/lib/realtime-socket"
import type { ServerEvent } from "@/features/messaging/types"

class FakeSocket {
  static instances: FakeSocket[] = []
  readyState = 0
  sent: string[] = []
  closedWith: number | null = null
  onopen: (() => void) | null = null
  onmessage: ((event: { data: unknown }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(
    readonly url: string,
    readonly protocols: string[],
  ) {
    FakeSocket.instances.push(this)
  }

  open() {
    this.readyState = 1
    this.onopen?.()
  }

  emit(data: unknown) {
    this.onmessage?.({ data })
  }

  drop() {
    this.readyState = 3
    this.onclose?.()
  }

  send(data: string) {
    this.sent.push(data)
  }

  close(code: number) {
    this.closedWith = code
  }
}

const ticket = { ticket: "opaque-ticket", expires_at: "2026-08-07T12:00:30Z", subprotocol: "llmatch.v1" }

/** Drains the microtask queue so the awaited ticket handshake settles. */
async function flush() {
  for (let tick = 0; tick < 5; tick += 1) await Promise.resolve()
}

function messageEvent(): ServerEvent {
  return {
    type: "message",
    sent_at: "2026-08-07T12:00:00Z",
    message: {
      id: "0f1b1f2e-4a6b-4f7c-9b1a-2f3d4e5a6b7c",
      match_id: "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
      sender_id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e",
      type: "text",
      content: "hola",
      created_at: "2026-08-07T12:00:00Z",
    },
  }
}

beforeEach(() => {
  FakeSocket.instances = []
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe("realtimeURL", () => {
  it("derives a ws endpoint from the api base without leaking credentials in the query", () => {
    const url = new URL(realtimeURL())
    expect(url.protocol).toBe("ws:")
    expect(url.pathname).toBe("/api/v1/ws")
    expect(url.search).toBe("")
  })
})

describe("reconnectDelay", () => {
  it("stays within the jitter window and never exceeds the cap", () => {
    expect(reconnectDelay(0, () => 0)).toBe(250)
    expect(reconnectDelay(0, () => 1)).toBe(500)
    expect(reconnectDelay(3, () => 1)).toBe(4000)
    expect(reconnectDelay(40, () => 1)).toBe(30_000)
  })
})

describe("RealtimeSocket", () => {
  it("sends the ticket as a subprotocol, never in the url", async () => {
    const issueTicket = vi.fn().mockResolvedValue(ticket)
    const socket = new RealtimeSocket({
      issueTicket,
      onEvent: vi.fn(),
      createSocket: (url, protocols) => new FakeSocket(url, protocols) as unknown as WebSocket,
    })

    socket.start()
    await flush()

    const created = FakeSocket.instances[0]
    expect(created.protocols).toEqual(["llmatch.v1", "opaque-ticket"])
    expect(created.url).not.toContain("opaque-ticket")
    socket.stop()
  })

  it("issues a fresh ticket on every reconnect because tickets are single use", async () => {
    const issueTicket = vi.fn().mockResolvedValue(ticket)
    const socket = new RealtimeSocket({
      issueTicket,
      onEvent: vi.fn(),
      createSocket: (url, protocols) => new FakeSocket(url, protocols) as unknown as WebSocket,
    })

    socket.start()
    await flush()
    FakeSocket.instances[0].open()
    FakeSocket.instances[0].drop()
    await vi.advanceTimersByTimeAsync(30_000)
    await flush()

    expect(issueTicket).toHaveBeenCalledTimes(2)
    expect(FakeSocket.instances).toHaveLength(2)
    socket.stop()
  })

  it("stops reconnecting once closed by the caller", async () => {
    const issueTicket = vi.fn().mockResolvedValue(ticket)
    const socket = new RealtimeSocket({
      issueTicket,
      onEvent: vi.fn(),
      createSocket: (url, protocols) => new FakeSocket(url, protocols) as unknown as WebSocket,
    })

    socket.start()
    await flush()
    socket.stop()
    FakeSocket.instances[0].drop()
    await vi.advanceTimersByTimeAsync(60_000)
    await flush()

    expect(issueTicket).toHaveBeenCalledTimes(1)
    expect(FakeSocket.instances).toHaveLength(1)
  })

  it("retries when the ticket cannot be issued instead of giving up", async () => {
    const issueTicket = vi.fn().mockRejectedValueOnce(new Error("503")).mockResolvedValue(ticket)
    const socket = new RealtimeSocket({
      issueTicket,
      onEvent: vi.fn(),
      createSocket: (url, protocols) => new FakeSocket(url, protocols) as unknown as WebSocket,
    })

    socket.start()
    await flush()
    expect(FakeSocket.instances).toHaveLength(0)

    await vi.advanceTimersByTimeAsync(30_000)
    await flush()
    expect(FakeSocket.instances).toHaveLength(1)
    socket.stop()
  })

  it("delivers valid events and drops frames that do not match the contract", async () => {
    const onEvent = vi.fn()
    const socket = new RealtimeSocket({
      issueTicket: vi.fn().mockResolvedValue(ticket),
      onEvent,
      createSocket: (url, protocols) => new FakeSocket(url, protocols) as unknown as WebSocket,
    })

    socket.start()
    await flush()
    const created = FakeSocket.instances[0]
    created.open()

    created.emit("not json")
    created.emit(JSON.stringify({ type: "unknown", sent_at: "2026-08-07T12:00:00Z" }))
    created.emit(new ArrayBuffer(4))
    expect(onEvent).not.toHaveBeenCalled()

    created.emit(JSON.stringify(messageEvent()))
    expect(onEvent).toHaveBeenCalledTimes(1)
    expect(onEvent.mock.calls[0][0]).toMatchObject({ type: "message" })
    socket.stop()
  })

  it("drops typing hints while the socket is not open rather than queueing them", async () => {
    const socket = new RealtimeSocket({
      issueTicket: vi.fn().mockResolvedValue(ticket),
      onEvent: vi.fn(),
      createSocket: (url, protocols) => new FakeSocket(url, protocols) as unknown as WebSocket,
    })

    socket.start()
    await flush()
    const created = FakeSocket.instances[0]

    socket.sendTyping("1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d")
    expect(created.sent).toHaveLength(0)

    created.open()
    socket.sendTyping("1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d")
    expect(JSON.parse(created.sent[0])).toEqual({ type: "typing", match_id: "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d" })
    socket.stop()
  })
})
