import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { apiFetch, apiFetchBlob } from "@/shared/lib/api-client"
import { ApiError } from "@/shared/lib/errors"
import { useAuthStore } from "@/shared/state/auth-store"

const user = {
  id: "1",
  email: "person@example.com",
  display_name: "Person",
  birth_date: "1995-01-01",
  gender: "woman",
  status: "active" as const,
  email_verified_at: null,
  created_at: "2026-01-01T00:00:00Z",
}

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

beforeEach(() => {
  useAuthStore.getState().clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("apiFetch", () => {
  it("queues a single refresh for concurrent 401s and retries each request with the refreshed token", async () => {
    useAuthStore.getState().setSession({
      accessToken: "stale-token",
      accessTokenExpiresAt: "2020-01-01T00:00:00Z",
      user,
    })

    let refreshCalls = 0
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const authHeader = new Headers(init?.headers).get("Authorization")

      if (url.endsWith("/auth/refresh")) {
        refreshCalls += 1
        return jsonResponse(200, {
          access_token: "fresh-token",
          access_token_expires_at: "2030-01-01T00:00:00Z",
          user,
        })
      }
      if (authHeader === "Bearer fresh-token") {
        return jsonResponse(200, { path: url })
      }
      return jsonResponse(401, { code: "UNAUTHORIZED", message: "expired" })
    })
    vi.stubGlobal("fetch", fetchMock)

    const [first, second] = await Promise.all([apiFetch<{ path: string }>("/first"), apiFetch<{ path: string }>("/second")])

    expect(first.path).toContain("/first")
    expect(second.path).toContain("/second")
    expect(refreshCalls).toBe(1)
    expect(useAuthStore.getState().accessToken).toBe("fresh-token")
  })

  it("clears the session and propagates the error when the refresh itself fails", async () => {
    useAuthStore.getState().setSession({
      accessToken: "stale-token",
      accessTokenExpiresAt: "2020-01-01T00:00:00Z",
      user,
    })

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.endsWith("/auth/refresh")) {
          return jsonResponse(401, { code: "UNAUTHORIZED", message: "no session" })
        }
        return jsonResponse(401, { code: "UNAUTHORIZED", message: "expired" })
      }),
    )

    await expect(apiFetch("/protected")).rejects.toBeInstanceOf(ApiError)
    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().accessToken).toBeNull()
  })

  it("does not attach a token or attempt a refresh for auth: false requests", async () => {
    const fetchMock = vi.fn(async () => jsonResponse(401, { code: "INVALID_CREDENTIALS", message: "bad" }))
    vi.stubGlobal("fetch", fetchMock)

    await expect(apiFetch("/auth/login", { method: "POST", auth: false, body: {} })).rejects.toMatchObject({
      code: "INVALID_CREDENTIALS",
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("downloads protected image content with the current bearer token", async () => {
    useAuthStore.getState().setSession({
      accessToken: "image-token",
      accessTokenExpiresAt: "2030-01-01T00:00:00Z",
      user,
    })
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer image-token")
      return new Response("image-bytes", { status: 200, headers: { "Content-Type": "image/png" } })
    })
    vi.stubGlobal("fetch", fetchMock)

    const blob = await apiFetchBlob("/api/v1/matching/photos/photo-id/content")

    expect(blob.type).toBe("image/png")
    expect(await blob.text()).toBe("image-bytes")
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/matching/photos/photo-id/content",
      expect.any(Object),
    )
  })
})
