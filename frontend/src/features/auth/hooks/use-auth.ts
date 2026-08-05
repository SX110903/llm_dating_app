import { useMutation } from "@tanstack/react-query"

import * as authApi from "@/features/auth/api/auth-api"
import type { LoginFormValues, RegisterFormValues } from "@/features/auth/schemas/auth-schemas"

export function useLogin() {
  return useMutation({
    mutationFn: (values: LoginFormValues) => authApi.login(values),
  })
}

export function useRegister() {
  return useMutation({
    mutationFn: (values: RegisterFormValues) => authApi.register(values),
  })
}

export function useLogout() {
  return useMutation({
    mutationFn: () => authApi.logout(),
  })
}
