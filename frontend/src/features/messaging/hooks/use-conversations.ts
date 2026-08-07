import { useQuery } from "@tanstack/react-query"

import * as messagingApi from "@/features/messaging/api/messaging-api"

export const conversationsQueryKey = ["conversations"] as const

export function useConversationsQuery() {
  return useQuery({
    queryKey: conversationsQueryKey,
    queryFn: messagingApi.listConversations,
    // Live events refresh this list, so polling would only add load.
    staleTime: 30_000,
  })
}
