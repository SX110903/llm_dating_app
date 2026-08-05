import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { RegisterForm } from "@/features/auth/components/RegisterForm"

function renderRegisterForm(onSuccess: () => void = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <RegisterForm onSuccess={onSuccess} />
    </QueryClientProvider>,
  )
}

async function fillValidForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/^email$/i), "person@example.com")
  await user.type(screen.getByLabelText(/nombre visible/i), "Person")
  fireEvent.change(screen.getByLabelText(/fecha de nacimiento/i), { target: { value: "1995-01-01" } })
  await user.selectOptions(screen.getByLabelText(/género/i), "woman")
  await user.type(screen.getByLabelText(/contraseña/i), "correct horse battery staple")
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("RegisterForm", () => {
  it("shows a confirmation and lets the user switch to login after success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            id: "1",
            email: "person@example.com",
            display_name: "Person",
            birth_date: "1995-01-01",
            gender: "woman",
            status: "active",
            email_verified_at: null,
            created_at: "2026-01-01T00:00:00Z",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
      ),
    )

    const onSuccess = vi.fn()
    const user = userEvent.setup()
    renderRegisterForm(onSuccess)

    await fillValidForm(user)
    await user.click(screen.getByRole("button", { name: /crear mi cuenta/i }))

    expect(await screen.findByText(/cuenta creada correctamente/i)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: /iniciar sesión/i }))
    expect(onSuccess).toHaveBeenCalledOnce()
  })

  it("shows an error message when the email is already taken", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: "EMAIL_TAKEN", message: "taken", request_id: "r1" }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    )

    const user = userEvent.setup()
    renderRegisterForm()

    await fillValidForm(user)
    await user.click(screen.getByRole("button", { name: /crear mi cuenta/i }))

    expect(await screen.findByText(/ese email ya está registrado/i)).toBeInTheDocument()
  })

  it("rejects a password shorter than the minimum policy", async () => {
    const user = userEvent.setup()
    renderRegisterForm()

    await user.type(screen.getByLabelText(/^email$/i), "person@example.com")
    await user.type(screen.getByLabelText(/nombre visible/i), "Person")
    fireEvent.change(screen.getByLabelText(/fecha de nacimiento/i), { target: { value: "1995-01-01" } })
    await user.selectOptions(screen.getByLabelText(/género/i), "woman")
    await user.type(screen.getByLabelText(/contraseña/i), "short")
    await user.click(screen.getByRole("button", { name: /crear mi cuenta/i }))

    expect(await screen.findByText(/al menos 12 caracteres/i)).toBeInTheDocument()
  })
})
