import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { MessagesScreen } from "@/features/messaging/components/MessagesScreen"
import { useAuthStore } from "@/shared/state/auth-store"

vi.mock("@/features/swipe/hooks/use-protected-image", () => ({ useProtectedImage: () => null }))
vi.mock("@/features/messaging/lib/realtime-socket", () => ({
  RealtimeSocket: class {
    start() {}
    stop() {}
    sendTyping() {}
  },
}))

const VIEWER_ID = "3c4d5e6f-7a8b-4c9d-8e1f-2a3b4c5d6e7f"
const OTHER_ID = "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e"
const MATCH_ID = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"

const conversation = {
  match_id: MATCH_ID,
  other_user_id: OTHER_ID,
  display_name: "Alex",
  unread_count: 2,
  matched_at: "2026-08-07T10:00:00Z",
  last_message: {
    id: "0f1b1f2e-4a6b-4f7c-9b1a-2f3d4e5a6b7c",
    match_id: MATCH_ID,
    sender_id: OTHER_ID,
    type: "text",
    content: "Que tal?",
    read_at: null,
    created_at: "2026-08-07T11:00:00Z",
  },
}

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MessagesScreen />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  useAuthStore.setState({
    accessToken: "token",
    accessTokenExpiresAt: "2026-08-07T13:00:00Z",
    user: {
      id: VIEWER_ID,
      email: "demo@llmatch.test",
      display_name: "Demo",
      birth_date: "1996-01-01",
      gender: "female",
      status: "active",
      email_verified_at: null,
      created_at: "2026-08-01T10:00:00Z",
    },
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  useAuthStore.getState().clear()
})

describe("MessagesScreen", () => {
  it("opens a conversation, marks it read and sends a message with an idempotency nonce", async () => {
    const sendBodies: Array<Record<string, unknown>> = []
    let markReadCalls = 0
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes("/conversations")) {
        return Promise.resolve(jsonResponse(200, { conversations: [conversation] }))
      }
      if (url.includes(`/matches/${MATCH_ID}/messages/read`)) {
        markReadCalls += 1
        return Promise.resolve(jsonResponse(200, { updated: 2 }))
      }
      if (url.includes(`/matches/${MATCH_ID}/messages`) && init?.method === "POST") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>
        sendBodies.push(body)
        return Promise.resolve(
          jsonResponse(201, {
            id: "4d5e6f7a-8b9c-4d0e-9f1a-2b3c4d5e6f7a",
            match_id: MATCH_ID,
            sender_id: VIEWER_ID,
            client_nonce: body.client_nonce,
            type: "text",
            content: body.content,
            read_at: null,
            created_at: "2026-08-07T11:05:00Z",
          }),
        )
      }
      if (url.includes(`/matches/${MATCH_ID}/messages`)) {
        return Promise.resolve(jsonResponse(200, { messages: [conversation.last_message] }))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal("fetch", fetchMock)

    renderScreen()

    const row = await screen.findByRole("button", { name: /Alex/ })
    expect(within(row).getByText("2 mensajes sin leer")).toBeInTheDocument()
    await userEvent.click(row)

    // Scoped to the chat panel: the preview in the list shows the same text.
    const chat = await screen.findByRole("region", { name: "Conversacion con Alex" })
    expect(await within(chat).findByText("Que tal?")).toBeInTheDocument()
    await waitFor(() => expect(markReadCalls).toBe(1))

    await userEvent.type(screen.getByLabelText("Escribe un mensaje"), "Hola!")
    await userEvent.click(screen.getByRole("button", { name: "Enviar mensaje" }))

    await waitFor(() => expect(sendBodies).toHaveLength(1))
    expect(sendBodies[0]).toMatchObject({ type: "text", content: "Hola!" })
    expect(sendBodies[0].client_nonce).toEqual(expect.stringMatching(/^[0-9a-f-]{36}$/))
    // Object keys are generated server-side; the client must never propose one.
    expect(sendBodies[0]).not.toHaveProperty("storage_key")
    expect(await within(chat).findByText("Hola!")).toBeInTheDocument()
  })

  it("keeps the same nonce when retrying a failed send so the server cannot store it twice", async () => {
    const sendBodies: Array<Record<string, unknown>> = []
    let failNext = true
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes("/conversations")) {
        return Promise.resolve(jsonResponse(200, { conversations: [{ ...conversation, unread_count: 0 }] }))
      }
      if (url.includes(`/matches/${MATCH_ID}/messages/read`)) {
        return Promise.resolve(jsonResponse(200, { updated: 0 }))
      }
      if (url.includes(`/matches/${MATCH_ID}/messages`) && init?.method === "POST") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>
        sendBodies.push(body)
        if (failNext) {
          failNext = false
          return Promise.resolve(jsonResponse(503, { code: "MESSAGING_DEPENDENCY_UNAVAILABLE", message: "down" }))
        }
        return Promise.resolve(
          jsonResponse(200, {
            id: "4d5e6f7a-8b9c-4d0e-9f1a-2b3c4d5e6f7a",
            match_id: MATCH_ID,
            sender_id: VIEWER_ID,
            client_nonce: body.client_nonce,
            type: "text",
            content: body.content,
            read_at: null,
            created_at: "2026-08-07T11:05:00Z",
          }),
        )
      }
      if (url.includes(`/matches/${MATCH_ID}/messages`)) {
        return Promise.resolve(jsonResponse(200, { messages: [] }))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal("fetch", fetchMock)

    renderScreen()

    await userEvent.click(await screen.findByRole("button", { name: /Alex/ }))
    await userEvent.type(await screen.findByLabelText("Escribe un mensaje"), "Reintento")
    await userEvent.click(screen.getByRole("button", { name: "Enviar mensaje" }))

    const retry = await screen.findByRole("button", { name: "Reintentar" })
    await userEvent.click(retry)

    await waitFor(() => expect(sendBodies).toHaveLength(2))
    expect(sendBodies[1].client_nonce).toBe(sendBodies[0].client_nonce)
    await waitFor(() => expect(screen.queryByRole("button", { name: "Reintentar" })).not.toBeInTheDocument())
  })

  it("shows the empty state when there are no conversations", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(jsonResponse(200, { conversations: [] }))),
    )

    renderScreen()

    expect(await screen.findByText("Sin conversaciones")).toBeInTheDocument()
  })
})
