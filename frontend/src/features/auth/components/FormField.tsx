import type { ReactNode } from "react"

export const authInputClass =
  "mt-1 w-full rounded-xl border border-white/15 bg-white/5 px-3 py-2 text-sm text-white outline-none focus:border-violet-300"

export function FormField({
  label,
  htmlFor,
  error,
  children,
}: {
  label: string
  htmlFor: string
  error?: string
  children: ReactNode
}) {
  return (
    <div>
      <label htmlFor={htmlFor} className="text-sm text-white/70">
        {label}
      </label>
      {children}
      {error && <p className="mt-1 text-xs text-rose-300">{error}</p>}
    </div>
  )
}
