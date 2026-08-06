import { useState } from "react"

import {
  useGenderPreferenceConsentQuery,
  useGrantGenderPreferenceConsent,
  useWithdrawGenderPreferenceConsent,
} from "@/features/account/hooks/use-consent"
import { usePreferencesQuery, useUpdatePreferencesMutation } from "@/features/profile/hooks/use-profile"
import type { Preferences } from "@/features/profile/types"
import { textInputClass } from "@/shared/components/ui/styles"
import { ApiError } from "@/shared/lib/errors"

const GENDER_OPTIONS = [
  { value: "woman", label: "Mujer" },
  { value: "man", label: "Hombre" },
  { value: "non-binary", label: "No binario" },
  { value: "other", label: "Otro" },
]

export function PreferencesSection() {
  const preferencesQuery = usePreferencesQuery()
  const consentQuery = useGenderPreferenceConsentQuery()

  if (preferencesQuery.isPending || consentQuery.isPending) {
    return null
  }

  return <PreferencesForm preferences={preferencesQuery.data} hasConsent={Boolean(consentQuery.data)} />
}

// preferences seeds the form's initial state directly (no effect): this
// component only mounts once both queries settle.
function PreferencesForm({ preferences, hasConsent }: { preferences: Preferences | null | undefined; hasConsent: boolean }) {
  const updatePreferences = useUpdatePreferencesMutation()
  const grantConsent = useGrantGenderPreferenceConsent()
  const withdrawConsent = useWithdrawGenderPreferenceConsent()

  const [minAge, setMinAge] = useState(preferences?.min_age ?? 18)
  const [maxAge, setMaxAge] = useState(preferences?.max_age ?? 45)
  const [maxDistanceKm, setMaxDistanceKm] = useState(preferences?.max_distance_km ?? 50)
  const [genders, setGenders] = useState<string[]>(preferences?.genders ?? [])

  const toggleGender = (value: string) => {
    setGenders((current) => (current.includes(value) ? current.filter((g) => g !== value) : [...current, value]))
  }

  const save = () => {
    updatePreferences.mutate({
      min_age: minAge,
      max_age: maxAge,
      max_distance_km: maxDistanceKm,
      genders: hasConsent ? genders : undefined,
    })
  }

  return (
    <section>
      <h2 className="font-display text-lg font-semibold">Preferencias</h2>
      <div className="mt-4 grid grid-cols-2 gap-4">
        <label className="text-sm text-white/70">
          Edad mínima
          <input
            type="number"
            min={18}
            max={100}
            value={minAge}
            onChange={(event) => setMinAge(Number(event.target.value))}
            className={textInputClass}
          />
        </label>
        <label className="text-sm text-white/70">
          Edad máxima
          <input
            type="number"
            min={18}
            max={100}
            value={maxAge}
            onChange={(event) => setMaxAge(Number(event.target.value))}
            className={textInputClass}
          />
        </label>
      </div>
      <label className="mt-4 block text-sm text-white/70">
        Distancia máxima ({maxDistanceKm} km)
        <input
          type="range"
          min={1}
          max={500}
          value={maxDistanceKm}
          onChange={(event) => setMaxDistanceKm(Number(event.target.value))}
          className="mt-2 w-full"
        />
      </label>

      <div className="mt-5 rounded-2xl border border-white/10 bg-white/[.03] p-4">
        <p className="text-sm text-white/70">Me interesa conocer a</p>
        <p className="mt-1 text-xs text-white/45">
          Este dato es sensible: solo se guarda con tu consentimiento explícito y puedes retirarlo cuando quieras.
        </p>

        {!hasConsent ? (
          <button
            type="button"
            onClick={() => grantConsent.mutate()}
            disabled={grantConsent.isPending}
            className="mt-3 rounded-full bg-white px-4 py-2 text-sm font-medium text-zinc-950 disabled:opacity-50"
          >
            {grantConsent.isPending ? "Guardando…" : "Dar consentimiento"}
          </button>
        ) : (
          <>
            <div className="mt-3 flex flex-wrap gap-2">
              {GENDER_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => toggleGender(option.value)}
                  className={`rounded-full px-3 py-1.5 text-sm transition-colors ${
                    genders.includes(option.value) ? "bg-white text-zinc-950" : "bg-white/10 text-white/70 hover:bg-white/15"
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
            <button
              type="button"
              onClick={() => withdrawConsent.mutate()}
              disabled={withdrawConsent.isPending}
              className="mt-3 text-xs text-white/40 underline-offset-2 hover:text-white/70 hover:underline"
            >
              Retirar consentimiento
            </button>
          </>
        )}
      </div>

      {updatePreferences.isError && (
        <p className="mt-3 text-sm text-rose-300">{preferencesErrorMessage(updatePreferences.error)}</p>
      )}
      <button
        type="button"
        onClick={save}
        disabled={updatePreferences.isPending}
        className="mt-4 rounded-full border border-white/15 bg-white/5 px-4 py-2 text-sm font-medium text-white hover:bg-white/10 disabled:opacity-50"
      >
        {updatePreferences.isPending ? "Guardando…" : "Guardar preferencias"}
      </button>
    </section>
  )
}

function preferencesErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "CONSENT_REQUIRED") return "Da tu consentimiento antes de guardar esta preferencia."
    return error.message
  }
  return "No se pudieron guardar las preferencias."
}
