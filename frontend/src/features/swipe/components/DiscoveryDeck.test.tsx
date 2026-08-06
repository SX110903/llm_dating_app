import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { DiscoveryDeck } from "@/features/swipe/components/DiscoveryDeck"

vi.mock("@/features/swipe/hooks/use-protected-image", () => ({ useProtectedImage: () => null }))
vi.mock("framer-motion", async (importOriginal) => {
  const actual = await importOriginal<typeof import("framer-motion")>()
  return { ...actual, useReducedMotion: () => true }
})

const candidate = {
  user_id: "1ad37df5-0a84-4c22-87ed-8b4d09a94af8",
  display_name: "Alex",
  age: 31,
  gender: "man",
  bio: "Senderismo y musica",
  interests: ["hiking", "music"],
  city: "Madrid",
  distance_km: 2.4,
  last_active_at: "2026-08-06T12:00:00Z",
  photo_url: "/api/v1/matching/photos/bf283889-f0da-4322-b20f-e5e23903ef27/content",
  score: { interests: 1, questionnaire: 0.8, distance: 0.9, activity: 1, total: 0.91 },
}

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("DiscoveryDeck", () => {
  it("submits a like, invalidates discovery and opens the match modal", async () => {
    let swiped = false
    let discoveryCalls = 0
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes("/discovery")) {
        discoveryCalls += 1
        return Promise.resolve(jsonResponse(200, { candidates: swiped ? [] : [candidate] }))
      }
      if (url.endsWith("/swipes") && init?.method === "POST") {
        swiped = true
        return Promise.resolve(
          jsonResponse(201, {
            id: "6f7b81e0-46d7-41bf-bec2-111d5e85d8cf",
            target_id: candidate.user_id,
            action: "like",
            created_at: "2026-08-06T12:01:00Z",
            match: { id: "c233f7c8-4e32-49df-aecd-a24ba3782c23", matched_at: "2026-08-06T12:01:00Z" },
          }),
        )
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal("fetch", fetchMock)

    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <DiscoveryDeck onEditProfile={vi.fn()} onViewMatches={vi.fn()} />
      </QueryClientProvider>,
    )

    expect(await screen.findByRole("heading", { name: /alex, 31/i })).toBeInTheDocument()
    await userEvent.setup().click(screen.getByRole("button", { name: /me gusta/i }))

    expect(await screen.findByRole("dialog", { name: /hay match/i })).toBeInTheDocument()
    await waitFor(() => expect(discoveryCalls).toBeGreaterThanOrEqual(2))
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/swipes"),
      expect.objectContaining({ method: "POST", body: JSON.stringify({ target_id: candidate.user_id, action: "like" }) }),
    )
  })
})
