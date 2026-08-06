import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query"

import * as swipeApi from "@/features/swipe/api/swipe-api"

export const discoveryQueryKey = ["discovery"] as const
export const matchesQueryKey = ["matches"] as const

export function useDiscoveryQuery() {
  return useInfiniteQuery({
    queryKey: discoveryQueryKey,
    queryFn: ({ pageParam }) => swipeApi.getDiscovery(pageParam),
    initialPageParam: "" as string,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
  })
}

export function useSwipeMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: swipeApi.createSwipe,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: discoveryQueryKey }),
        queryClient.invalidateQueries({ queryKey: matchesQueryKey }),
      ])
    },
  })
}

export function useBlockMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: swipeApi.blockUser,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: discoveryQueryKey }),
        queryClient.invalidateQueries({ queryKey: matchesQueryKey }),
      ])
    },
  })
}

export function useReportMutation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: swipeApi.reportUser,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: discoveryQueryKey }),
        queryClient.invalidateQueries({ queryKey: matchesQueryKey }),
      ])
    },
  })
}
