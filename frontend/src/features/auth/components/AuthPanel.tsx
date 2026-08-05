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
      <div className="rounded-[28px] border border-white/10 bg-white/[.04] p-8 text-center shadow-2xl shadow-black/20 backdrop-blur-2xl">
        <p className="text-sm text-white/50">Sesión iniciada como</p>
        <p className="mt-1 text-lg font-semibold">{user.display_name}</p>
        <p className="text-sm text-white/50">{user.email}</p>
        <button
          type="button"
          onClick={() => logout.mutate()}
          disabled={logout.isPending}
          className="mt-6 w-full rounded-full border border-white/15 bg-white/5 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-white/10 disabled:opacity-50"
        >
          {logout.isPending ? "Saliendo…" : "Cerrar sesión"}
        </button>
      </div>
    )
  }

  return (
    <div className="rounded-[28px] border border-white/10 bg-white/[.04] p-8 shadow-2xl shadow-black/20 backdrop-blur-2xl">
      <div className="mb-7 flex rounded-full bg-white/5 p-1 text-sm">
        <button
          type="button"
          onClick={() => setMode("login")}
          className={`flex-1 rounded-full py-2 font-medium transition-colors ${
            mode === "login" ? "bg-white text-zinc-950" : "text-white/60 hover:text-white"
          }`}
        >
          Entrar
        </button>
        <button
          type="button"
          onClick={() => setMode("register")}
          className={`flex-1 rounded-full py-2 font-medium transition-colors ${
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
