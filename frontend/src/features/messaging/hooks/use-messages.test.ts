import { QueryClient } from "@tanstack/react-query"
import { describe, expect, it } from "vitest"

import { insertMessageInCache, messagesQueryKey } from "@/features/messaging/hooks/use-messages"
import type { HistoryPage, Message } from "@/features/messaging/types"

const MATCH_ID = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"

function message(id: string, createdAt: string): Message {
  return {
    id,
    match_id: MATCH_ID,
    sender_id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e",
    type: "text",
    content: "hola",
    read_at: null,
    created_at: createdAt,
  }
}

function seed(client: QueryClient, pages: HistoryPage[]) {
  client.setQueryData(messagesQueryKey(MATCH_ID), { pages, pageParams: pages.map(() => undefined) })
}

function cached(client: QueryClient): HistoryPage[] {
  return client.getQueryData<{ pages: HistoryPage[] }>(messagesQueryKey(MATCH_ID))?.pages ?? []
}

describe("insertMessageInCache", () => {
  it("prepends a new message to the newest page", () => {
    const client = new QueryClient()
    seed(client, [{ messages: [message("aaa", "2026-08-07T12:00:00Z")] }])

    insertMessageInCache(client, message("bbb", "2026-08-07T12:00:05Z"))

    expect(cached(client)[0].messages.map((item) => item.id)).toEqual(["bbb", "aaa"])
  })

  it("ignores a message already present, so the send response and the socket echo cannot duplicate it", () => {
    const client = new QueryClient()
    seed(client, [{ messages: [message("aaa", "2026-08-07T12:00:00Z")] }])

    insertMessageInCache(client, message("aaa", "2026-08-07T12:00:00Z"))
    insertMessageInCache(client, message("aaa", "2026-08-07T12:00:00Z"))

    expect(cached(client)[0].messages).toHaveLength(1)
  })

  it("leaves an unloaded conversation untouched instead of inventing a page", () => {
    const client = new QueryClient()

    insertMessageInCache(client, message("aaa", "2026-08-07T12:00:00Z"))

    expect(client.getQueryData(messagesQueryKey(MATCH_ID))).toBeUndefined()
  })

  it("does not disturb older pages", () => {
    const client = new QueryClient()
    seed(client, [
      { messages: [message("bbb", "2026-08-07T12:00:05Z")], next_cursor: "cursor" },
      { messages: [message("aaa", "2026-08-07T12:00:00Z")] },
    ])

    insertMessageInCache(client, message("ccc", "2026-08-07T12:00:09Z"))

    const pages = cached(client)
    expect(pages[0].messages.map((item) => item.id)).toEqual(["ccc", "bbb"])
    expect(pages[1].messages.map((item) => item.id)).toEqual(["aaa"])
  })
})
