import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import * as matchesApi from "@/features/matches/api/matches-api"
import { discoveryQueryKey, matchesQueryKey } from "@/features/swipe/hooks/use-swipe"

export function useMatchesQuery() {
  return useQuery({ queryKey: matchesQueryKey, queryFn: matchesApi.listMatches })
}

export function useUnmatchMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: matchesApi.unmatch,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: matchesQueryKey }),
        queryClient.invalidateQueries({ queryKey: discoveryQueryKey }),
      ])
    },
  })
}
