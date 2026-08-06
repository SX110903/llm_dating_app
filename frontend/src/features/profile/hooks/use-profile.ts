import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import * as profileApi from "@/features/profile/api/profile-api"

export function useProfileQuery() {
  return useQuery({ queryKey: ["profile"], queryFn: profileApi.getProfile })
}

export function useUpdateProfileMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: profileApi.updateProfile,
    onSuccess: (profile) => {
      queryClient.setQueryData(["profile"], profile)
    },
  })
}

export function usePreferencesQuery() {
  return useQuery({ queryKey: ["preferences"], queryFn: profileApi.getPreferences })
}

export function useUpdatePreferencesMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: profileApi.updatePreferences,
    onSuccess: (preferences) => {
      queryClient.setQueryData(["preferences"], preferences)
    },
  })
}

export function usePhotosQuery() {
  return useQuery({ queryKey: ["photos"], queryFn: profileApi.listPhotos })
}

export function useUploadPhotoMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ file, onProgress }: { file: File; onProgress?: (percent: number) => void }) =>
      profileApi.uploadPhoto(file, onProgress),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["photos"] })
    },
  })
}

export function useReorderPhotosMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: profileApi.reorderPhotos,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["photos"] })
    },
  })
}

export function useSetPrimaryPhotoMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: profileApi.setPrimaryPhoto,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["photos"] })
    },
  })
}

export function useDeletePhotoMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: profileApi.deletePhoto,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["photos"] })
    },
  })
}
