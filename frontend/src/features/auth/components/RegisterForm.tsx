import { zodResolver } from "@hookform/resolvers/zod"
import { useForm } from "react-hook-form"

import { FormField } from "@/features/auth/components/FormField"
import { textInputClass } from "@/shared/components/ui/styles"
import { useRegister } from "@/features/auth/hooks/use-auth"
import { registerSchema, type RegisterFormValues } from "@/features/auth/schemas/auth-schemas"
import { Button } from "@/shared/components/ui/button"
import { ApiError } from "@/shared/lib/errors"

const genderOptions = [
  { value: "woman", label: "Mujer" },
  { value: "man", label: "Hombre" },
  { value: "non-binary", label: "No binario" },
  { value: "other", label: "Otro" },
]

export function RegisterForm({ onSuccess }: { onSuccess: () => void }) {
  const registerAccount = useRegister()
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<RegisterFormValues>({ resolver: zodResolver(registerSchema) })

  const onSubmit = handleSubmit((values) => {
    registerAccount.mutate(values)
  })

  if (registerAccount.isSuccess) {
    return (
      <div className="flex flex-col gap-4">
        <p className="text-sm text-emerald-300">Cuenta creada correctamente. Ya puedes iniciar sesión.</p>
        <Button type="button" onClick={onSuccess}>
          Iniciar sesión
        </Button>
      </div>
    )
  }

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      <FormField label="Email" htmlFor="register-email" error={errors.email?.message}>
        <input id="register-email" type="email" autoComplete="email" className={textInputClass} {...register("email")} />
      </FormField>
      <FormField label="Nombre visible" htmlFor="register-display-name" error={errors.displayName?.message}>
        <input
          id="register-display-name"
          type="text"
          autoComplete="nickname"
          className={textInputClass}
          {...register("displayName")}
        />
      </FormField>
      <FormField label="Fecha de nacimiento" htmlFor="register-birth-date" error={errors.birthDate?.message}>
        <input id="register-birth-date" type="date" className={textInputClass} {...register("birthDate")} />
      </FormField>
      <FormField label="Género" htmlFor="register-gender" error={errors.gender?.message}>
        <select id="register-gender" defaultValue="" className={textInputClass} {...register("gender")}>
          <option value="" disabled>
            Selecciona una opción
          </option>
          {genderOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </FormField>
      <FormField label="Contraseña" htmlFor="register-password" error={errors.password?.message}>
        <input
          id="register-password"
          type="password"
          autoComplete="new-password"
          className={textInputClass}
          {...register("password")}
        />
      </FormField>
      {registerAccount.isError && <p className="text-sm text-rose-300">{registerErrorMessage(registerAccount.error)}</p>}
      <Button type="submit" disabled={registerAccount.isPending}>
        {registerAccount.isPending ? "Creando cuenta…" : "Crear mi cuenta"}
      </Button>
    </form>
  )
}

function registerErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "EMAIL_TAKEN":
        return "Ese email ya está registrado."
      case "UNDERAGE":
        return "Debes tener al menos 18 años."
      case "WEAK_PASSWORD":
        return "La contraseña no cumple la política mínima."
      case "RATE_LIMITED":
        return "Demasiados intentos. Inténtalo más tarde."
      default:
        return error.message
    }
  }
  return "No se pudo crear la cuenta."
}
