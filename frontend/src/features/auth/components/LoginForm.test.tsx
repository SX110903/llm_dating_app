import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { LoginForm } from "@/features/auth/components/LoginForm"
import { useAuthStore } from "@/shared/state/auth-store"

function renderLoginForm() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <LoginForm />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  useAuthStore.getState().clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("LoginForm", () => {
  it("shows validation errors when submitted empty", async () => {
    const user = userEvent.setup()
    renderLoginForm()

    await user.click(screen.getByRole("button", { name: /iniciar sesión/i }))

    expect(await screen.findByText(/introduce tu email/i)).toBeInTheDocument()
  })

  it("logs in and stores the session on success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            access_token: "access-token",
            access_token_expires_at: "2030-01-01T00:00:00Z",
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
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      ),
    )

    const user = userEvent.setup()
    renderLoginForm()

    await user.type(screen.getByLabelText(/email/i), "person@example.com")
    await user.type(screen.getByLabelText(/contraseña/i), "correct horse battery staple")
    await user.click(screen.getByRole("button", { name: /iniciar sesión/i }))

    await waitFor(() => expect(useAuthStore.getState().user?.email).toBe("person@example.com"))
    expect(useAuthStore.getState().accessToken).toBe("access-token")
  })

  it("shows an error message for invalid credentials", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: "INVALID_CREDENTIALS", message: "invalid", request_id: "r1" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    )

    const user = userEvent.setup()
    renderLoginForm()

    await user.type(screen.getByLabelText(/email/i), "person@example.com")
    await user.type(screen.getByLabelText(/contraseña/i), "wrong password")
    await user.click(screen.getByRole("button", { name: /iniciar sesión/i }))

    expect(await screen.findByText(/email o contraseña incorrectos/i)).toBeInTheDocument()
  })
})
