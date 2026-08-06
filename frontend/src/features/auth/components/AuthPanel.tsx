import { useState } from "react"

import { LoginForm } from "@/features/auth/components/LoginForm"
import { RegisterForm } from "@/features/auth/components/RegisterForm"

export function AuthPanel() {
  const [mode, setMode] = useState<"login" | "register">("login")

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
