import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { AuthPanel } from "@/features/auth/components/AuthPanel"
import { useAuthStore } from "@/shared/state/auth-store"

function renderAuthPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <AuthPanel />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  useAuthStore.getState().clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("AuthPanel", () => {
  it("defaults to the login tab and switches to registration", async () => {
    const user = userEvent.setup()
    renderAuthPanel()

    expect(screen.getByRole("button", { name: /iniciar sesión/i })).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: /^crear cuenta$/i }))

    expect(screen.getByRole("button", { name: /crear mi cuenta/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/nombre visible/i)).toBeInTheDocument()
  })

  it("shows the authenticated view and logs out when a session is active", async () => {
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
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })))

    const user = userEvent.setup()
    renderAuthPanel()

    expect(screen.getByText("Person")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: /cerrar sesión/i }))

    await screen.findByRole("button", { name: /iniciar sesión/i })
    expect(useAuthStore.getState().user).toBeNull()
  })
})
