import type { LoginFormValues, RegisterFormValues } from "@/features/auth/schemas/auth-schemas"
import { loginResponseSchema, type LoginResponse } from "@/features/auth/types"
import { apiFetch } from "@/shared/lib/api-client"
import { useAuthStore } from "@/shared/state/auth-store"

export async function register(input: RegisterFormValues): Promise<void> {
  await apiFetch("/auth/register", {
    method: "POST",
    auth: false,
    body: {
      email: input.email,
      password: input.password,
      display_name: input.displayName,
      birth_date: input.birthDate,
      gender: input.gender,
    },
  })
}

export async function login(input: LoginFormValues): Promise<LoginResponse> {
  const raw = await apiFetch<unknown>("/auth/login", {
    method: "POST",
    auth: false,
    body: input,
  })
  const result = loginResponseSchema.parse(raw)
  useAuthStore.getState().setSession({
    accessToken: result.access_token,
    accessTokenExpiresAt: result.access_token_expires_at,
    user: result.user,
  })
  return result
}

export async function logout(): Promise<void> {
  try {
    await apiFetch<void>("/auth/logout", { method: "POST" })
  } finally {
    useAuthStore.getState().clear()
  }
}
