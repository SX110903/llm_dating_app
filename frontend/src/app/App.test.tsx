import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { App } from "@/app/App"
import { useAuthStore } from "@/shared/state/auth-store"

beforeEach(() => {
  useAuthStore.getState().clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

describe("App", () => {
  it("renders the landing page with the login form", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    render(
      <QueryClientProvider client={client}>
        <App />
      </QueryClientProvider>,
    )

    expect(screen.getByRole("heading", { name: /encuentra a alguien real/i })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /iniciar sesión/i })).toBeInTheDocument()
  })

  it("renders the profile editor once a session is active", async () => {
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

    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith("/profile")) {
          return Promise.resolve(
            jsonResponse(200, {
              user_id: "1",
              bio: "",
              interests: [],
              city: "",
              has_location: false,
              questionnaire: {},
              onboarding_completed: false,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            }),
          )
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
        return Promise.resolve(jsonResponse(404, { code: "NOT_FOUND", message: "unhandled", request_id: "r1" }))
      }),
    )

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <App />
      </QueryClientProvider>,
    )

    expect(await screen.findByRole("heading", { name: /fotos/i })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /finalizar perfil/i })).toBeInTheDocument()
  })

  it("keeps every mobile header action named and at least 44px tall", async () => {
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
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith("/profile")) {
          return Promise.resolve(
            jsonResponse(200, {
              user_id: "1",
              bio: "Ready",
              interests: ["music"],
              city: "Madrid",
              has_location: true,
              questionnaire: {},
              onboarding_completed: true,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            }),
          )
        }
        if (url.includes("/discovery")) return Promise.resolve(jsonResponse(200, { candidates: [] }))
        return Promise.resolve(jsonResponse(404, { code: "NOT_FOUND", message: "none", request_id: "r1" }))
      }),
    )

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <App />
      </QueryClientProvider>,
    )

    const navigationLabels = ["Descubrir", "Matches", "Mensajes", "Perfil"]
    for (const label of navigationLabels) {
      const button = await screen.findByRole("button", { name: label })
      expect(button).toHaveClass("min-h-11", "min-w-11")
    }
    expect(screen.getByRole("button", { name: "Ir a Descubrir" })).toHaveClass("min-h-11")
    expect(screen.getByRole("button", { name: "Cerrar sesión" })).toHaveClass("h-11", "w-11")

    const profileButton = screen.getByRole("button", { name: "Perfil" })
    profileButton.focus()
    await userEvent.setup().keyboard("{Enter}")
    expect(profileButton).toHaveAttribute("aria-current", "page")
  })
})
