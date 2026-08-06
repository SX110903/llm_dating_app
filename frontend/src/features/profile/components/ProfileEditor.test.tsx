import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ProfileEditor } from "@/features/profile/components/ProfileEditor"
import { useAuthStore } from "@/shared/state/auth-store"

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

const emptyProfile = {
  user_id: "1",
  bio: "",
  interests: [],
  city: "",
  has_location: false,
  questionnaire: {},
  onboarding_completed: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
}

function renderEditor() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ProfileEditor />
    </QueryClientProvider>,
  )
}

function stubReadEndpoints() {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith("/profile") && (!init || init.method === undefined)) {
        return Promise.resolve(jsonResponse(200, emptyProfile))
      }
      if (url.endsWith("/profile/photos")) {
        return Promise.resolve(jsonResponse(200, []))
      }
      if (url.endsWith("/profile/preferences")) {
        return Promise.resolve(jsonResponse(404, { code: "PREFERENCES_NOT_FOUND", message: "none", request_id: "r1" }))
      }
      if (url.includes("/account/consents/")) {
        return Promise.resolve(jsonResponse(404, { code: "CONSENT_NOT_FOUND", message: "none", request_id: "r1" }))
      }
      if (url.endsWith("/profile") && init?.method === "PUT") {
        return Promise.resolve(jsonResponse(422, { code: "ONBOARDING_INCOMPLETE", message: "add a bio and a photo", request_id: "r1" }))
      }
      throw new Error(`unexpected request: ${url}`)
    }),
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

describe("ProfileEditor", () => {
  it("shows a friendly message when onboarding is incomplete", async () => {
    stubReadEndpoints()
    const user = userEvent.setup()
    renderEditor()

    await user.click(await screen.findByRole("button", { name: /finalizar perfil/i }))

    expect(await screen.findByText(/añade una bio y al menos una foto/i)).toBeInTheDocument()
  })

  it("saves the profile with the edited bio", async () => {
    const putBodies: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.endsWith("/profile") && init?.method === "PUT") {
          putBodies.push(init.body as string)
          return Promise.resolve(jsonResponse(200, { ...emptyProfile, bio: JSON.parse(init.body as string).bio }))
        }
        if (url.endsWith("/profile")) {
          return Promise.resolve(jsonResponse(200, emptyProfile))
        }
        if (url.endsWith("/profile/photos")) {
          return Promise.resolve(jsonResponse(200, []))
        }
        if (url.endsWith("/profile/preferences")) {
          return Promise.resolve(jsonResponse(404, { code: "PREFERENCES_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        if (url.includes("/account/consents/")) {
          return Promise.resolve(jsonResponse(404, { code: "CONSENT_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        throw new Error(`unexpected request: ${url}`)
      }),
    )

    const user = userEvent.setup()
    renderEditor()

    const bioField = await screen.findByLabelText(/^bio$/i)
    await user.type(bioField, "Me encanta viajar")
    await user.click(screen.getByRole("button", { name: /guardar y continuar más tarde/i }))

    await waitFor(() => expect(putBodies).toHaveLength(1))
    expect(JSON.parse(putBodies[0]).bio).toBe("Me encanta viajar")
    expect(JSON.parse(putBodies[0]).onboarding_completed).toBe(false)
  })

  // Regression test for the bug that made every profile undiscoverable: the
  // editor used to send no coordinates, and the backend read that as "erase
  // the stored location". The payload must now omit location entirely.
  it("omits every location field when the user does not touch the location", async () => {
    const putBodies: string[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.endsWith("/profile") && init?.method === "PUT") {
          putBodies.push(init.body as string)
          return Promise.resolve(jsonResponse(200, { ...emptyProfile, has_location: true }))
        }
        if (url.endsWith("/profile")) {
          return Promise.resolve(jsonResponse(200, { ...emptyProfile, has_location: true }))
        }
        if (url.endsWith("/profile/photos")) {
          return Promise.resolve(jsonResponse(200, []))
        }
        if (url.endsWith("/profile/preferences")) {
          return Promise.resolve(jsonResponse(404, { code: "PREFERENCES_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        if (url.includes("/account/consents/")) {
          return Promise.resolve(jsonResponse(404, { code: "CONSENT_NOT_FOUND", message: "none", request_id: "r1" }))
        }
        throw new Error(`unexpected request: ${url}`)
      }),
    )

    const user = userEvent.setup()
    renderEditor()

    await user.type(await screen.findByLabelText(/^bio$/i), "solo cambio la bio")
    await user.click(screen.getByRole("button", { name: /guardar y continuar más tarde/i }))

    await waitFor(() => expect(putBodies).toHaveLength(1))
    const payload = JSON.parse(putBodies[0])
    expect(payload).not.toHaveProperty("latitude")
    expect(payload).not.toHaveProperty("longitude")
    expect(payload).not.toHaveProperty("clear_location")
  })

  it("sends the captured coordinates after the user shares their location", async () => {
    vi.stubGlobal("navigator", {
      ...globalThis.navigator,
      geolocation: {
        getCurrentPosition: (success: PositionCallback) => {
          success({ coords: { latitude: 40.4168, longitude: -3.7038 } } as GeolocationPosition)
        },
      },
    })

    const putBodies: string[] = []
    const originalFetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith("/profile") && init?.method === "PUT") {
        putBodies.push(init.body as string)
        return Promise.resolve(jsonResponse(200, { ...emptyProfile, has_location: true }))
      }
      if (url.endsWith("/profile")) return Promise.resolve(jsonResponse(200, emptyProfile))
      if (url.endsWith("/profile/photos")) return Promise.resolve(jsonResponse(200, []))
      if (url.endsWith("/profile/preferences")) {
        return Promise.resolve(jsonResponse(404, { code: "PREFERENCES_NOT_FOUND", message: "none", request_id: "r1" }))
      }
      if (url.includes("/account/consents/")) {
        return Promise.resolve(jsonResponse(404, { code: "CONSENT_NOT_FOUND", message: "none", request_id: "r1" }))
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal("fetch", originalFetch)

    const user = userEvent.setup()
    renderEditor()

    await user.click(await screen.findByRole("button", { name: /usar mi ubicación/i }))
    await user.click(screen.getByRole("button", { name: /guardar y continuar más tarde/i }))

    await waitFor(() => expect(putBodies).toHaveLength(1))
    const payload = JSON.parse(putBodies[0])
    expect(payload.latitude).toBe(40.4168)
    expect(payload.longitude).toBe(-3.7038)
  })
})
