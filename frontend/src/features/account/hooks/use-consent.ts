import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import * as consentApi from "@/features/account/api/consent-api"

export function useGenderPreferenceConsentQuery() {
  return useQuery({ queryKey: ["gender-preference-consent"], queryFn: consentApi.getGenderPreferenceConsent })
}

export function useGrantGenderPreferenceConsent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: consentApi.grantGenderPreferenceConsent,
    onSuccess: (consent) => {
      queryClient.setQueryData(["gender-preference-consent"], consent)
    },
  })
}

export function useWithdrawGenderPreferenceConsent() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: consentApi.withdrawGenderPreferenceConsent,
    onSuccess: () => {
      queryClient.setQueryData(["gender-preference-consent"], null)
      queryClient.invalidateQueries({ queryKey: ["preferences"] })
    },
  })
}
