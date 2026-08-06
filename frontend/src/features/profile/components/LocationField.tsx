import { useGeolocation } from "@/features/profile/hooks/use-geolocation"

/**
 * The three intents a save can carry for the location. "unchanged" is the
 * default so editing anything else can never erase the stored coordinates.
 */
export type LocationIntent =
  | { kind: "unchanged" }
  | { kind: "captured"; latitude: number; longitude: number }
  | { kind: "cleared" }

export function LocationField({
  hasStoredLocation,
  intent,
  onChange,
}: {
  hasStoredLocation: boolean
  intent: LocationIntent
  onChange: (intent: LocationIntent) => void
}) {
  const geolocation = useGeolocation()

  const isActive = intent.kind === "captured" || (intent.kind === "unchanged" && hasStoredLocation)

  const statusText = () => {
    if (intent.kind === "captured") return "Ubicación lista para guardar"
    if (intent.kind === "cleared") return "Se dejará de compartir al guardar"
    return hasStoredLocation ? "Compartiendo tu ubicación" : "Sin ubicación"
  }

  return (
    <div className="mt-5 rounded-2xl border border-white/10 bg-white/[.03] p-4">
      <div className="flex items-center gap-2">
        <span
          aria-hidden
          className={`h-2 w-2 shrink-0 rounded-full ${isActive ? "bg-emerald-400" : "bg-white/25"}`}
        />
        <p className="text-sm text-white/70">Ubicación</p>
      </div>
      <p className="mt-1 text-xs text-white/45">
        Necesaria para descubrir gente cerca. Sin ella tu perfil no aparecerá en las sugerencias.
      </p>
      <p className="mt-2 text-sm text-white/60">{statusText()}</p>

      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          onClick={() => geolocation.request((coordinates) => onChange({ kind: "captured", ...coordinates }))}
          disabled={geolocation.isRequesting}
          className="rounded-full bg-white px-4 py-2 text-sm font-medium text-zinc-950 disabled:opacity-50"
        >
          {geolocation.isRequesting
            ? "Obteniendo…"
            : isActive
              ? "Actualizar ubicación"
              : "Usar mi ubicación"}
        </button>

        {isActive && (
          <button
            type="button"
            onClick={() => onChange({ kind: "cleared" })}
            className="rounded-full border border-white/15 bg-white/5 px-4 py-2 text-sm text-white/70 hover:bg-white/10"
          >
            Dejar de compartir
          </button>
        )}

        {intent.kind === "cleared" && (
          <button
            type="button"
            onClick={() => onChange({ kind: "unchanged" })}
            className="rounded-full border border-white/15 bg-white/5 px-4 py-2 text-sm text-white/70 hover:bg-white/10"
          >
            Deshacer
          </button>
        )}
      </div>

      {geolocation.errorMessage && <p className="mt-2 text-xs text-rose-300">{geolocation.errorMessage}</p>}
    </div>
  )
}
