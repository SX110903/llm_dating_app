import { create } from "zustand"

export interface AuthUser {
  id: string
  email: string
  display_name: string
  birth_date: string
  gender: string
  status: "active" | "suspended" | "deleted"
  email_verified_at: string | null
  created_at: string
}

export interface AuthSession {
  accessToken: string
  accessTokenExpiresAt: string
  user: AuthUser
}

interface AuthState {
  accessToken: string | null
  accessTokenExpiresAt: string | null
  user: AuthUser | null
  setSession: (session: AuthSession) => void
  clear: () => void
}

// The access token only ever lives here, in memory. It is never written to
// localStorage/sessionStorage; reloading the page loses it and relies on a
// silent refresh against the HttpOnly refresh cookie instead.
export const useAuthStore = create<AuthState>((set) => ({
  accessToken: null,
  accessTokenExpiresAt: null,
  user: null,
  setSession: (session) =>
    set({
      accessToken: session.accessToken,
      accessTokenExpiresAt: session.accessTokenExpiresAt,
      user: session.user,
    }),
  clear: () => set({ accessToken: null, accessTokenExpiresAt: null, user: null }),
}))
