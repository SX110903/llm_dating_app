import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PhotoGrid } from "@/features/profile/components/PhotoGrid"
import { useAuthStore } from "@/shared/state/auth-store"

vi.mock("@/features/profile/components/PhotoUploadDialog", () => ({
  default: ({ file, onCancel, onReady }: { file: File; onCancel: () => void; onReady: (file: File) => void }) => (
    <div role="dialog" aria-label="Editor de foto de prueba">
      <span>{file.name}</span>
      <button type="button" onClick={onCancel}>Cancelar</button>
      <button type="button" onClick={() => onReady(file)}>Preparar foto</button>
    </div>
  ),
}))

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

const secondPhoto = {
  ...samplePhoto,
  id: "photo-2",
  position: 1,
  is_primary: false,
}

class MockXHR {
  static failNext = false

  status = 201
  responseText = ""
  upload: { onprogress: ((event: ProgressEvent) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  withCredentials = false

  open() {
    // no-op: URL/method are irrelevant to this test double
  }

  setRequestHeader() {
    // no-op
  }

  send() {
    queueMicrotask(() => {
      this.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 2 } as ProgressEvent)
      if (MockXHR.failNext) {
        MockXHR.failNext = false
        this.status = 503
        this.responseText = JSON.stringify({ code: "UNAVAILABLE", message: "try later" })
      } else {
        this.responseText = JSON.stringify(samplePhoto)
      }
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
  MockXHR.failNext = false
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
  it("shows a touch-sized add tile when there are no photos yet", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, [])))

    renderGrid()

    const addButton = await screen.findByRole("button", { name: /añadir foto/i })
    expect(addButton).toHaveClass("min-h-24")
    expect(await screen.findByText(/0\/6 fotos/i)).toBeInTheDocument()
  })

  it("prepares a selected photo before uploading it and reports real progress", async () => {
    let photos: unknown[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith("/profile/photos")) return Promise.resolve(jsonResponse(200, photos))
        if (url.includes("/content")) return Promise.resolve(new Response(new Blob(["x"]), { status: 200 }))
        throw new Error(`unexpected fetch: ${url}`)
      }),
    )
    vi.stubGlobal("XMLHttpRequest", MockXHR as unknown as typeof XMLHttpRequest)

    renderGrid()
    await screen.findByRole("button", { name: /añadir foto/i })

    const file = new File(["x"], "photo.png", { type: "image/png" })
    const input = document.querySelector('input[type="file"]')
    expect(input).not.toBeNull()
    const user = userEvent.setup()
    await user.upload(input as HTMLInputElement, file)

    expect(await screen.findByRole("dialog", { name: /editor de foto/i })).toBeInTheDocument()
    photos = [samplePhoto]
    await user.click(screen.getByRole("button", { name: /preparar foto/i }))

    await waitFor(() => expect(screen.getByText(/1\/6 fotos/i)).toBeInTheDocument())
  })

  it("accepts a dropped photo and opens the fixed preparation step", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, [])))
    renderGrid()

    const file = new File(["x"], "dropped.webp", { type: "image/webp" })
    const zone = await screen.findByRole("region", { name: /zona de fotos/i })
    fireEvent.drop(zone, {
      dataTransfer: { files: [file], types: ["Files"] },
    })

    expect(await screen.findByRole("dialog", { name: /editor de foto/i })).toHaveTextContent("dropped.webp")
  })

  it("keeps a failed upload available for an explicit retry", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, [])))
    vi.stubGlobal("XMLHttpRequest", MockXHR as unknown as typeof XMLHttpRequest)
    MockXHR.failNext = true
    renderGrid()

    const user = userEvent.setup()
    const input = document.querySelector('input[type="file"]')
    await screen.findByRole("button", { name: /añadir foto/i })
    await user.upload(input as HTMLInputElement, new File(["x"], "retry.jpg", { type: "image/jpeg" }))
    await user.click(await screen.findByRole("button", { name: /preparar foto/i }))

    const retry = await screen.findByRole("button", { name: /reintentar subida/i })
    await user.click(retry)
    await waitFor(() => expect(screen.queryByRole("button", { name: /reintentar subida/i })).not.toBeInTheDocument())
  })

  it("offers keyboard-accessible reorder controls as an alternative to dragging", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith("/profile/photos") && !init?.method) return Promise.resolve(jsonResponse(200, [samplePhoto, secondPhoto]))
      if (url.endsWith("/profile/photos/order") && init?.method === "PUT") return Promise.resolve(new Response(null, { status: 204 }))
      if (url.includes("/content")) return Promise.resolve(new Response(new Blob(["x"]), { status: 200 }))
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal("fetch", fetchMock)
    renderGrid()

    const user = userEvent.setup()
    const actions = await screen.findByRole("button", { name: /acciones de la foto 2/i })
    expect(actions).toHaveClass("h-11", "w-11")
    actions.focus()
    await user.keyboard("{Enter}")
    const moveLeft = screen.getByRole("button", { name: /mover a la izquierda/i })
    moveLeft.focus()
    await user.keyboard("{Enter}")

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/profile\/photos\/order$/),
        expect.objectContaining({ method: "PUT", body: JSON.stringify({ photo_ids: ["photo-2", "photo-1"] }) }),
      )
    })
  })

  it("reorders photo cards with drag and drop", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith("/profile/photos") && !init?.method) return Promise.resolve(jsonResponse(200, [samplePhoto, secondPhoto]))
      if (url.endsWith("/profile/photos/order") && init?.method === "PUT") return Promise.resolve(new Response(null, { status: 204 }))
      if (url.includes("/content")) return Promise.resolve(new Response(new Blob(["x"]), { status: 200 }))
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal("fetch", fetchMock)
    renderGrid()

    const firstCard = (await screen.findByRole("button", { name: /acciones de la foto 1/i })).parentElement
    const secondCard = screen.getByRole("button", { name: /acciones de la foto 2/i }).parentElement
    expect(firstCard).not.toBeNull()
    expect(secondCard).not.toBeNull()
    const dataTransfer = { effectAllowed: "none", types: [], setData: vi.fn() }
    fireEvent.dragStart(firstCard as HTMLElement, { dataTransfer })
    fireEvent.dragOver(secondCard as HTMLElement, { dataTransfer })
    fireEvent.drop(secondCard as HTMLElement, { dataTransfer })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringMatching(/\/profile\/photos\/order$/),
        expect.objectContaining({ method: "PUT", body: JSON.stringify({ photo_ids: ["photo-2", "photo-1"] }) }),
      )
    })
  })
})
