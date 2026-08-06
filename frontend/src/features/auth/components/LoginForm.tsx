import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"

import { FormField } from "@/features/auth/components/FormField"
import { textInputClass } from "@/shared/components/ui/styles"
import { useLogin } from "@/features/auth/hooks/use-auth"
import { loginSchema, type LoginFormValues } from "@/features/auth/schemas/auth-schemas"
import { Button } from "@/shared/components/ui/button"
import { ApiError } from "@/shared/lib/errors"

export function LoginForm() {
  const login = useLogin()
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormValues>({ resolver: zodResolver(loginSchema) })

  const onSubmit = handleSubmit((values) => {
    login.mutate(values)
  })

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      <FormField label="Email" htmlFor="login-email" error={errors.email?.message}>
        <input id="login-email" type="email" autoComplete="email" className={textInputClass} {...register("email")} />
      </FormField>
      <FormField label="Contraseña" htmlFor="login-password" error={errors.password?.message}>
        <input
          id="login-password"
          type="password"
          autoComplete="current-password"
          className={textInputClass}
          {...register("password")}
        />
      </FormField>
      {login.isError && <p className="text-sm text-rose-300">{loginErrorMessage(login.error)}</p>}
      <Button type="submit" disabled={login.isPending}>
        {login.isPending ? "Entrando…" : "Iniciar sesión"}
      </Button>
    </form>
  )
}

function loginErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "RATE_LIMITED") return "Demasiados intentos. Inténtalo más tarde."
    if (error.code === "INVALID_CREDENTIALS") return "Email o contraseña incorrectos."
    return error.message
  }
  return "No se pudo iniciar sesión."
}
