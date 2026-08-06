import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PhotoGrid } from "@/features/profile/components/PhotoGrid"
import { useAuthStore } from "@/shared/state/auth-store"

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

const samplePhoto = {
  id: "photo-1",
  mime_type: "image/png",
  byte_size: 4,
  width: 10,
  height: 10,
  position: 0,
  is_primary: true,
  created_at: "2026-01-01T00:00:00Z",
}

class MockXHR {
  status = 201
  responseText = ""
  upload: { onprogress: ((event: ProgressEvent) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null

  open() {
    // no-op: URL/method are irrelevant to this test double
  }

  setRequestHeader() {
    // no-op
  }

  send() {
    queueMicrotask(() => {
      this.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 1 } as ProgressEvent)
      this.responseText = JSON.stringify(samplePhoto)
      this.onload?.()
    })
  }
}

function renderGrid() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <PhotoGrid />
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

describe("PhotoGrid", () => {
  it("shows the add tile when there are no photos yet", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, [])))

    renderGrid()

    expect(await screen.findByText(/añadir/i)).toBeInTheDocument()
    expect(await screen.findByText(/0\/6 fotos/i)).toBeInTheDocument()
  })

  it("uploads a photo and shows it in the grid afterwards", async () => {
    let photos: unknown[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith("/profile/photos")) {
          return Promise.resolve(jsonResponse(200, photos))
        }
        if (url.includes("/content")) {
          return Promise.resolve(new Response(new Blob(["x"]), { status: 200 }))
        }
        throw new Error(`unexpected fetch: ${url}`)
      }),
    )
    vi.stubGlobal("XMLHttpRequest", MockXHR as unknown as typeof XMLHttpRequest)

    renderGrid()
    await screen.findByText(/añadir/i)

    photos = [samplePhoto]
    const file = new File(["x"], "photo.png", { type: "image/png" })
    const input = document.querySelector('input[type="file"]')
    expect(input).not.toBeNull()

    const user = userEvent.setup()
    await user.upload(input as HTMLInputElement, file)

    await waitFor(() => expect(screen.getByText(/1\/6 fotos/i)).toBeInTheDocument())
  })
})
