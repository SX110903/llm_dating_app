import { useState } from "react"

import { useReportMutation } from "@/features/swipe/hooks/use-swipe"
import { reportReasons, type ReportReason } from "@/features/swipe/types"

const reasonLabels: Record<ReportReason, string> = {
  harassment: "Acoso",
  spam: "Spam",
  inappropriate_content: "Contenido inapropiado",
  impersonation: "Suplantacion de identidad",
  underage: "Posible menor de edad",
  other: "Otro",
}

export function ReportDialog({ userId, displayName, onClose }: { userId: string; displayName: string; onClose: () => void }) {
  const [reason, setReason] = useState<ReportReason>("harassment")
  const [description, setDescription] = useState("")
  const report = useReportMutation()

  return (
    <div role="dialog" aria-modal="true" aria-labelledby="report-title" className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-5 backdrop-blur-md">
      <form
        className="w-full max-w-md rounded-3xl border border-white/10 bg-[#1a151f] p-6 shadow-2xl"
        onSubmit={(event) => {
          event.preventDefault()
          report.mutate(
            { userId, reason, description },
            {
              onSuccess: onClose,
            },
          )
        }}
      >
        <h2 id="report-title" className="font-display text-xl font-semibold">
          Reportar a {displayName}
        </h2>
        <p className="mt-2 text-sm leading-6 text-white/50">El reporte se revisara de forma confidencial.</p>
        <label className="mt-5 block text-sm text-white/70">
          Motivo
          <select
            value={reason}
            onChange={(event) => setReason(event.target.value as ReportReason)}
            className="mt-2 w-full rounded-xl border border-white/10 bg-white/5 px-3 py-2.5 text-white outline-none focus:border-violet-300/50"
          >
            {reportReasons.map((value) => (
              <option key={value} value={value} className="bg-[#1a151f]">
                {reasonLabels[value]}
              </option>
            ))}
          </select>
        </label>
        <label className="mt-4 block text-sm text-white/70">
          Detalles opcionales
          <textarea
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            maxLength={1000}
            rows={4}
            className="mt-2 w-full resize-none rounded-xl border border-white/10 bg-white/5 px-3 py-2.5 text-white outline-none focus:border-violet-300/50"
          />
        </label>
        {report.isError && <p className="mt-3 text-sm text-rose-300">No se pudo enviar el reporte.</p>}
        <div className="mt-6 flex justify-end gap-3">
          <button type="button" onClick={onClose} className="rounded-full px-4 py-2 text-sm text-white/55 hover:text-white">
            Cancelar
          </button>
          <button type="submit" disabled={report.isPending} className="rounded-full bg-white px-5 py-2 text-sm font-medium text-zinc-950 disabled:opacity-50">
            {report.isPending ? "Enviando..." : "Enviar reporte"}
          </button>
        </div>
      </form>
    </div>
  )
}
