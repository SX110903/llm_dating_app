import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { App } from "@/app/App"

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
})
