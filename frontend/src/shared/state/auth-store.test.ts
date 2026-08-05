import { describe, expect, it } from "vitest"

import { useAuthStore } from "@/shared/state/auth-store"

const user = {
  id: "1",
  email: "a@b.com",
  display_name: "A",
  birth_date: "1990-01-01",
  gender: "woman",
  status: "active" as const,
  email_verified_at: null,
  created_at: "2026-01-01T00:00:00Z",
}

describe("useAuthStore", () => {
  it("starts with no session", () => {
    useAuthStore.getState().clear()
    expect(useAuthStore.getState().accessToken).toBeNull()
    expect(useAuthStore.getState().user).toBeNull()
  })

  it("stores and clears a session", () => {
    useAuthStore.getState().setSession({
      accessToken: "token",
      accessTokenExpiresAt: "2030-01-01T00:00:00Z",
      user,
    })

    expect(useAuthStore.getState().accessToken).toBe("token")
    expect(useAuthStore.getState().user?.email).toBe("a@b.com")

    useAuthStore.getState().clear()
    expect(useAuthStore.getState().accessToken).toBeNull()
    expect(useAuthStore.getState().user).toBeNull()
  })
})
