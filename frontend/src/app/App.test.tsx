import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { App } from "@/app/App"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("App", () => {
  it("shows the foundation as operational when dependencies are ready", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ status: "healthy", checks: { postgres: "up", redis: "up" } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    )
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    expect(screen.getByRole("heading", { name: /conexiones reales/i })).toBeInTheDocument()
    expect(await screen.findByText("Fundación operativa")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /próximamente/i })).toBeDisabled()
  })
})

