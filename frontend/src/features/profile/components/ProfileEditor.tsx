import { motion } from "framer-motion"
import { useState } from "react"

import { useLogout } from "@/features/auth/hooks/use-auth"
import { PhotoGrid } from "@/features/profile/components/PhotoGrid"
import { PreferencesSection } from "@/features/profile/components/PreferencesSection"
import { usePhotosQuery, useProfileQuery, useUpdateProfileMutation } from "@/features/profile/hooks/use-profile"
import type { Profile } from "@/features/profile/types"
import { textInputClass } from "@/shared/components/ui/styles"
import { ApiError } from "@/shared/lib/errors"

const TOTAL_STEPS = 3

export function ProfileEditor() {
  const profileQuery = useProfileQuery()
  const photosQuery = usePhotosQuery()

  if (profileQuery.isPending || photosQuery.isPending) {
    return <p className="text-center text-sm text-white/50">Cargando tu perfil…</p>
  }

  return <ProfileEditorForm profile={profileQuery.data} photoCount={photosQuery.data?.length ?? 0} />
}

// profile/photoCount seed the form's initial state directly (no effect):
// this component only mounts once the query settles, and after a save the
// mutation result matches what the form already holds, so there is nothing
// to resynchronize.
function ProfileEditorForm({ profile, photoCount }: { profile: Profile | null | undefined; photoCount: number }) {
  const updateProfile = useUpdateProfileMutation()
  const logout = useLogout()

  const [bio, setBio] = useState(profile?.bio ?? "")
  const [city, setCity] = useState(profile?.city ?? "")
  const [interests, setInterests] = useState<string[]>(profile?.interests ?? [])
  const [interestInput, setInterestInput] = useState("")

  const onboardingCompleted = profile?.onboarding_completed ?? false
  const stepsDone = [bio.trim().length > 0, photoCount > 0, interests.length > 0].filter(Boolean).length

  const addInterest = () => {
    const value = interestInput.trim()
    if (value && !interests.includes(value) && interests.length < 20) {
      setInterests((current) => [...current, value])
    }
    setInterestInput("")
  }

  const save = (markComplete: boolean) => {
    updateProfile.mutate({ bio, city, interests, onboarding_completed: markComplete })
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4 }}
      className="w-full max-w-xl space-y-8"
    >
      {!onboardingCompleted && (
        <div>
          <div className="mb-2 flex justify-between text-xs text-white/50">
            <span>Completa tu perfil</span>
            <span>
              {stepsDone}/{TOTAL_STEPS}
            </span>
          </div>
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-white/10">
            <motion.div
              className="h-full rounded-full bg-white"
              animate={{ width: `${(stepsDone / TOTAL_STEPS) * 100}%` }}
              transition={{ duration: 0.3 }}
            />
          </div>
        </div>
      )}

      <section>
        <h2 className="font-display text-lg font-semibold">Fotos</h2>
        <div className="mt-4">
          <PhotoGrid />
        </div>
      </section>

      <section>
        <h2 className="font-display text-lg font-semibold">Sobre ti</h2>
        <label className="mt-4 block text-sm text-white/70">
          Bio
          <textarea
            value={bio}
            onChange={(event) => setBio(event.target.value)}
            maxLength={500}
            rows={3}
            className={`${textInputClass} resize-none`}
          />
        </label>
        <label className="mt-4 block text-sm text-white/70">
          Ciudad
          <input value={city} onChange={(event) => setCity(event.target.value)} maxLength={120} className={textInputClass} />
        </label>
        <div className="mt-4">
          <p className="text-sm text-white/70">Intereses</p>
          <div className="mt-2 flex flex-wrap gap-2">
            {interests.map((interest) => (
              <span key={interest} className="flex items-center gap-1 rounded-full bg-white/10 px-3 py-1 text-sm text-white/80">
                {interest}
                <button
                  type="button"
                  onClick={() => setInterests((current) => current.filter((i) => i !== interest))}
                  className="text-white/40 hover:text-white"
                  aria-label={`Quitar interés ${interest}`}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
          <input
            value={interestInput}
            onChange={(event) => setInterestInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault()
                addInterest()
              }
            }}
            placeholder="Escribe un interés y pulsa Enter"
            maxLength={40}
            className={`${textInputClass} mt-2`}
          />
        </div>
      </section>

      <PreferencesSection />

      {updateProfile.isError && <p className="text-sm text-rose-300">{profileErrorMessage(updateProfile.error)}</p>}

      <div className="flex flex-wrap items-center gap-3 border-t border-white/10 pt-6">
        <button
          type="button"
          onClick={() => save(true)}
          disabled={updateProfile.isPending}
          className="rounded-full bg-white px-5 py-2.5 text-sm font-medium text-zinc-950 disabled:opacity-50"
        >
          {updateProfile.isPending ? "Guardando…" : onboardingCompleted ? "Guardar cambios" : "Finalizar perfil"}
        </button>
        {!onboardingCompleted && (
          <button type="button" onClick={() => save(false)} disabled={updateProfile.isPending} className="text-sm text-white/50 hover:text-white/80">
            Guardar y continuar más tarde
          </button>
        )}
        <button type="button" onClick={() => logout.mutate()} className="ml-auto text-sm text-white/40 hover:text-white/70">
          Cerrar sesión
        </button>
      </div>
    </motion.div>
  )
}

function profileErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "ONBOARDING_INCOMPLETE") return "Añade una bio y al menos una foto antes de finalizar tu perfil."
    return error.message
  }
  return "No se pudieron guardar los cambios."
}
