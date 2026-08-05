import { z } from "zod"

export const loginSchema = z.object({
  email: z.string().min(1, "Introduce tu email").email("Email no válido"),
  password: z.string().min(1, "Introduce tu contraseña"),
})
export type LoginFormValues = z.infer<typeof loginSchema>

export const registerSchema = z.object({
  email: z.string().min(1, "Introduce tu email").email("Email no válido"),
  password: z
    .string()
    .min(12, "La contraseña debe tener al menos 12 caracteres")
    .max(128, "La contraseña es demasiado larga"),
  displayName: z.string().min(1, "Introduce un nombre").max(100, "Máximo 100 caracteres"),
  birthDate: z.string().min(1, "Introduce tu fecha de nacimiento"),
  gender: z.string().min(1, "Selecciona una opción").max(50, "Máximo 50 caracteres"),
})
export type RegisterFormValues = z.infer<typeof registerSchema>
