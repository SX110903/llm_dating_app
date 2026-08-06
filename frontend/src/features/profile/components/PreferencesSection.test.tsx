import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PreferencesSection } from "@/features/profile/components/PreferencesSection"
import { useAuthStore } from "@/shared/state/auth-store"

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <PreferencesSection />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  useAuthStore.getState().setSession({
    accessToken: "token",
    accessTokenExpiresAt: "2030-01-01T00:00:00Z",
    user: {
      id: "1",
      email: "person@example.com",
      display_name: "Person",
      birth_date: "1995-01-01",
      gender: "woman",
      status: "active",
      email_verified_at: null,
      created_at: "2026-01-01T00:00:00Z",
    },
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  useAuthStore.getState().clear()
})

describe("PreferencesSection", () => {
  it("shows the consent gate when there is no active consent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith("/profile/preferences")) {
          return Promise.resolve(jsonResponse(404, { code: "PREFERENCES_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        return Promise.resolve(jsonResponse(404, { code: "CONSENT_NOT_FOUND", message: "none", request_id: "r1" }))
      }),
    )

    renderSection()

    expect(await screen.findByRole("button", { name: /dar consentimiento/i })).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /^mujer$/i })).not.toBeInTheDocument()
  })

  it("reveals gender options after granting consent and saves preferences with them", async () => {
    const putCalls: unknown[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.endsWith("/profile/preferences") && init?.method === "PUT") {
          putCalls.push(init.body)
          return Promise.resolve(
            jsonResponse(200, { min_age: 18, max_age: 45, max_distance_km: 50, genders: ["woman"] }),
          )
        }
        if (url.endsWith("/profile/preferences")) {
          return Promise.resolve(jsonResponse(404, { code: "PREFERENCES_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        if (url.endsWith("/account/consents") && init?.method === "POST") {
          return Promise.resolve(
            jsonResponse(201, { purpose: "matching_gender_preferences", policy_version: "v1", granted_at: "2026-01-01T00:00:00Z" }),
          )
        }
        if (url.includes("/account/consents/")) {
          return Promise.resolve(jsonResponse(404, { code: "CONSENT_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        throw new Error(`unexpected request: ${url}`)
      }),
    )

    const user = userEvent.setup()
    renderSection()

    await user.click(await screen.findByRole("button", { name: /dar consentimiento/i }))

    const womanOption = await screen.findByRole("button", { name: /^mujer$/i })
    await user.click(womanOption)
    await user.click(screen.getByRole("button", { name: /guardar preferencias/i }))

    await waitFor(() => expect(putCalls).toHaveLength(1))
    const payload = JSON.parse(putCalls[0] as string)
    expect(payload.genders).toEqual(["woman"])
  })
})
