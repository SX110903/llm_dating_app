import { useState } from "react"

import { LoginForm } from "@/features/auth/components/LoginForm"
import { RegisterForm } from "@/features/auth/components/RegisterForm"
import { useLogout } from "@/features/auth/hooks/use-auth"
import { useAuthStore } from "@/shared/state/auth-store"

export function AuthPanel() {
  const user = useAuthStore((state) => state.user)
  const [mode, setMode] = useState<"login" | "register">("login")
  const logout = useLogout()

  if (user) {
    return (
      <div className="rounded-3xl border border-white/10 bg-white/[.045] p-6 shadow-2xl shadow-black/15 backdrop-blur-xl">
        <p className="text-sm text-white/55">Sesión iniciada como</p>
        <p className="mt-1 text-lg font-semibold">{user.display_name}</p>
        <p className="text-sm text-white/55">{user.email}</p>
        <button
          type="button"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
          className="mt-5 rounded-xl border border-white/15 bg-white/5 px-4 py-2 text-sm text-white hover:bg-white/10 disabled:opacity-50"
        >
          {logout.isPending ? "Saliendo…" : "Cerrar sesión"}
        </button>
      </div>
    )
  }

  return (
    <div className="rounded-3xl border border-white/10 bg-white/[.045] p-6 shadow-2xl shadow-black/15 backdrop-blur-xl">
      <div className="mb-5 flex gap-2 text-sm">
        <button
          type="button"
          onClick={() => setMode("login")}
          className={`rounded-full px-3 py-1.5 transition-colors ${
            mode === "login" ? "bg-white text-zinc-950" : "text-white/60 hover:text-white"
          }`}
        >
          Entrar
        </button>
        <button
          type="button"
          onClick={() => setMode("register")}
          className={`rounded-full px-3 py-1.5 transition-colors ${
            mode === "register" ? "bg-white text-zinc-950" : "text-white/60 hover:text-white"
          }`}
        >
          Crear cuenta
        </button>
      </div>
      {mode === "login" ? <LoginForm /> : <RegisterForm onSuccess={() => setMode("login")} />}
    </div>
  )
}
