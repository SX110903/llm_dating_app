import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { renderHook, waitFor } from "@testing-library/react"
import { StrictMode, type ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useProtectedImage } from "@/features/swipe/hooks/use-protected-image"
import { apiFetchBlob } from "@/shared/lib/api-client"

vi.mock("@/shared/lib/api-client", () => ({ apiFetchBlob: vi.fn() }))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useProtectedImage", () => {
  it("keeps a valid object URL when StrictMode replays effects", async () => {
    vi.mocked(apiFetchBlob).mockResolvedValue(new Blob(["image"], { type: "image/png" }))
    const createObjectURL = vi.fn().mockReturnValue("blob:image")
    const revokeObjectURL = vi.fn()
    const NativeURL = URL

    class ObjectURL extends NativeURL {
      static createObjectURL = createObjectURL
      static revokeObjectURL = revokeObjectURL
    }

    vi.stubGlobal("URL", ObjectURL)

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const wrapper = ({ children }: { children: ReactNode }) => (
      <StrictMode>
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      </StrictMode>
    )
    const { result, unmount } = renderHook(() => useProtectedImage("/api/v1/matching/photos/1/content"), { wrapper })

    await waitFor(() => expect(result.current).toBe("blob:image"))
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).not.toHaveBeenCalled()

    unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:image")
  })
})
