import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { MatchesList } from "@/features/matches/components/MatchesList"

vi.mock("@/features/swipe/hooks/use-protected-image", () => ({ useProtectedImage: () => null }))

const match = {
  id: "c233f7c8-4e32-49df-aecd-a24ba3782c23",
  other_user_id: "1ad37df5-0a84-4c22-87ed-8b4d09a94af8",
  display_name: "Alex",
  bio: "Senderismo y musica",
  city: "Madrid",
  photo_url: "/api/v1/matching/photos/bf283889-f0da-4322-b20f-e5e23903ef27/content",
  matched_at: "2026-08-06T12:01:00Z",
  last_active_at: "2026-08-06T12:00:00Z",
}

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("MatchesList", () => {
  it("unmatches and refreshes the list", async () => {
    let removed = false
    let listCalls = 0
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes("/matches?") && !init?.method) {
        listCalls += 1
        return Promise.resolve(jsonResponse(200, { matches: removed ? [] : [match] }))
      }
      if (url.endsWith(`/matches/${match.id}`) && init?.method === "DELETE") {
        removed = true
        return Promise.resolve(new Response(null, { status: 204 }))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal("fetch", fetchMock)
    vi.stubGlobal("confirm", vi.fn(() => true))

    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MatchesList />
      </QueryClientProvider>,
    )

    expect(await screen.findByRole("heading", { name: "Alex" })).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole("button", { name: /deshacer/i }))

    expect(await screen.findByRole("heading", { name: /aun no tienes matches/i })).toBeInTheDocument()
    await waitFor(() => expect(listCalls).toBeGreaterThanOrEqual(2))
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining(`/matches/${match.id}`),
      expect.objectContaining({ method: "DELETE" }),
    )
  })
})
