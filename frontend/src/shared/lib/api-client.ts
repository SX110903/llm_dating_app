import { environment } from "@/shared/lib/env"
import { ApiError } from "@/shared/lib/errors"
import { useAuthStore } from "@/shared/state/auth-store"

interface ErrorBody {
  code?: string
  message?: string
  request_id?: string
}

interface ApiFetchOptions extends Omit<RequestInit, "body"> {
  body?: unknown
  /** Attach the current access token and retry once via a silent refresh on 401. */
  auth?: boolean
}

// A single in-flight refresh is shared by every caller that hits a 401 at
// the same time, so concurrent requests never race each other into issuing
// multiple refresh calls.
let refreshPromise: Promise<boolean> | null = null

async function refreshSession(): Promise<boolean> {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

async function performRefresh(): Promise<boolean> {
  const response = await fetch(`${environment.VITE_API_BASE_URL}/auth/refresh`, {
    method: "POST",
    credentials: "include",
    headers: { Accept: "application/json" },
  })
  if (!response.ok) {
    useAuthStore.getState().clear()
    return false
  }
  const body = (await response.json()) as {
    access_token: string
    access_token_expires_at: string
    user: ReturnType<typeof useAuthStore.getState>["user"]
  }
  if (!body.user) {
    useAuthStore.getState().clear()
    return false
  }
  useAuthStore.getState().setSession({
    accessToken: body.access_token,
    accessTokenExpiresAt: body.access_token_expires_at,
    user: body.user,
  })
  return true
}

export async function apiFetch<T>(path: string, options: ApiFetchOptions = {}): Promise<T> {
  const { auth = true, body, headers, ...rest } = options

  const performFetch = () => {
    const requestHeaders = new Headers(headers)
    requestHeaders.set("Accept", "application/json")
    if (body !== undefined) {
      requestHeaders.set("Content-Type", "application/json")
    }
    const accessToken = useAuthStore.getState().accessToken
    if (auth && accessToken) {
      requestHeaders.set("Authorization", `Bearer ${accessToken}`)
    }
    return fetch(`${environment.VITE_API_BASE_URL}${path}`, {
      ...rest,
      credentials: "include",
      headers: requestHeaders,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  }

  let response = await performFetch()

  if (response.status === 401 && auth) {
    const refreshed = await refreshSession()
    if (refreshed) {
      response = await performFetch()
    }
  }

  if (!response.ok) {
    const errorBody: ErrorBody | null = await response.json().catch(() => null)
    throw new ApiError(
      response.status,
      errorBody?.code ?? "UNKNOWN_ERROR",
      errorBody?.message ?? "Request failed",
      errorBody?.request_id,
    )
  }

  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}
