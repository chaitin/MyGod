import { describe, expect, it, vi } from "vitest"

import {
  listConversationMessageReactionSnapshots,
  listConversationMessageReactionUsers,
  normalizeMessageReactions,
  normalizeMessageReactionsUpdatedEventPayload,
  setConversationMessageReaction,
  type ClientMessage,
  type MessageReactionsUpdatedEvent,
} from "@/lib/client-data-api"
import {
  applyMessageReactionSnapshot,
  applyMessageReactionsUpdate,
} from "@/lib/client-data-state"

describe("message reactions", () => {
  it("treats a legacy null reaction list as empty", () => {
    expect(normalizeMessageReactions(null)).toEqual([])
  })

  it("sends a reaction update and normalizes the exact response snapshot", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation_id: "conversation-1",
            message_id: "message-1",
            reaction_version: 3,
            reactions: [
              {
                count: 2,
                reacted_by_me: true,
                text: "自定义文本",
                users: [
                  { id: "user-1", name: "Alice" },
                  { id: "user-2", name: "Bob" },
                ],
              },
            ],
          },
        }),
        { headers: { "content-type": "application/json" }, status: 200 }
      )
    )

    await expect(
      setConversationMessageReaction(
        "conversation-1",
        "message-1",
        { reacted: true, text: "自定义文本" },
        fetcher
      )
    ).resolves.toEqual({
      conversationId: "conversation-1",
      messageId: "message-1",
      reactionVersion: 3,
      reactions: [
        {
          count: 2,
          reactedByMe: true,
          text: "自定义文本",
          users: [
            { id: "user-1", name: "Alice" },
            { id: "user-2", name: "Bob" },
          ],
        },
      ],
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-1/messages/message-1/reactions",
      expect.objectContaining({
        body: JSON.stringify({ reacted: true, text: "自定义文本" }),
        method: "PUT",
      })
    )
  })

  it("normalizes realtime snapshots", () => {
    expect(
      normalizeMessageReactionsUpdatedEventPayload({
        actor_reacted: true,
        actor_text: "👍",
        actor_user_id: "user-1",
        conversation_id: "conversation-1",
        message_id: "message-1",
        reaction_version: 4,
        reactions: [
          {
            count: 3,
            text: "👍",
            users: [
              { id: "user-1", name: "Alice" },
              { id: "user-2", name: "Bob" },
              { id: "user-3", name: "Carol" },
            ],
          },
        ],
      })
    ).toEqual({
      actorReacted: true,
      actorText: "👍",
      actorUserId: "user-1",
      conversationId: "conversation-1",
      messageId: "message-1",
      reactionVersion: 4,
      reactions: [
        {
          count: 3,
          text: "👍",
          users: [
            { id: "user-1", name: "Alice" },
            { id: "user-2", name: "Bob" },
            { id: "user-3", name: "Carol" },
          ],
        },
      ],
    })
  })

  it("loads ordered per-user reaction snapshots", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation_id: "conversation-1",
            snapshots: [
              {
                message_id: "message-2",
                reaction_version: 5,
                reactions: [
                  {
                    count: 3,
                    reacted_by_me: false,
                    text: "👍",
                    users: [
                      { id: "user-1", name: "Alice" },
                      { id: "user-2", name: "Bob" },
                      { id: "user-3", name: "Carol" },
                    ],
                  },
                ],
              },
              {
                message_id: "message-1",
                reaction_version: 2,
                reactions: [],
              },
            ],
          },
        }),
        { headers: { "content-type": "application/json" }, status: 200 }
      )
    )

    await expect(
      listConversationMessageReactionSnapshots(
        "conversation-1",
        ["message-2", "message-1"],
        fetcher
      )
    ).resolves.toEqual([
      {
        conversationId: "conversation-1",
        messageId: "message-2",
        reactionVersion: 5,
        reactions: [
          {
            count: 3,
            reactedByMe: false,
            text: "👍",
            users: [
              { id: "user-1", name: "Alice" },
              { id: "user-2", name: "Bob" },
              { id: "user-3", name: "Carol" },
            ],
          },
        ],
      },
      {
        conversationId: "conversation-1",
        messageId: "message-1",
        reactionVersion: 2,
        reactions: [],
      },
    ])
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-1/messages/reactions/query",
      expect.objectContaining({
        body: JSON.stringify({ message_ids: ["message-2", "message-1"] }),
        method: "POST",
      })
    )
  })

  it("loads the complete reaction user list on demand", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation_id: "conversation-1",
            message_id: "message-1",
            text: "🏷",
            users: [
              { id: "user-1", name: "Alice" },
              { id: "user-2", name: "Bob" },
            ],
          },
        }),
        { headers: { "content-type": "application/json" }, status: 200 }
      )
    )

    await expect(
      listConversationMessageReactionUsers(
        "conversation-1",
        "message-1",
        "🏷",
        fetcher
      )
    ).resolves.toEqual([
      { id: "user-1", name: "Alice" },
      { id: "user-2", name: "Bob" },
    ])
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-1/messages/message-1/reactions/users?text=%F0%9F%8F%B7",
      expect.objectContaining({ method: "GET" })
    )
  })

  it("applies only newer versions and preserves per-user state", () => {
    const message = createMessage()
    const otherUserEvent: MessageReactionsUpdatedEvent = {
      actorReacted: true,
      actorText: "😂",
      actorUserId: "user-2",
      conversationId: "conversation-1",
      messageId: "message-1",
      reactionVersion: 3,
      reactions: [
        {
          count: 3,
          text: "👍",
          users: [
            { id: "user-1", name: "Me" },
            { id: "user-2", name: "Alice" },
            { id: "user-3", name: "Bob" },
          ],
        },
        {
          count: 1,
          text: "😂",
          users: [{ id: "user-2", name: "Alice" }],
        },
      ],
    }
    const updated = applyMessageReactionsUpdate(
      message,
      otherUserEvent,
      "user-1"
    )
    expect(updated.reactions).toEqual([
      {
        count: 3,
        reactedByMe: true,
        text: "👍",
        users: [
          { id: "user-1", name: "Me" },
          { id: "user-2", name: "Alice" },
          { id: "user-3", name: "Bob" },
        ],
      },
      {
        count: 1,
        reactedByMe: false,
        text: "😂",
        users: [{ id: "user-2", name: "Alice" }],
      },
    ])

    const stale = applyMessageReactionsUpdate(
      updated,
      { ...otherUserEvent, reactionVersion: 3, reactions: [] },
      "user-1"
    )
    expect(stale).toBe(updated)

    const versionGap = applyMessageReactionsUpdate(
      updated,
      { ...otherUserEvent, reactionVersion: 5, reactions: [] },
      "user-1"
    )
    expect(versionGap).toBe(updated)

    const ownRemoval = applyMessageReactionsUpdate(
      updated,
      {
        ...otherUserEvent,
        actorReacted: false,
        actorText: "👍",
        actorUserId: "user-1",
        reactionVersion: 4,
        reactions: [
          {
            count: 2,
            text: "👍",
            users: [
              { id: "user-2", name: "Alice" },
              { id: "user-3", name: "Bob" },
            ],
          },
        ],
      },
      "user-1"
    )
    expect(ownRemoval.reactions).toEqual([
      {
        count: 2,
        reactedByMe: false,
        text: "👍",
        users: [
          { id: "user-2", name: "Alice" },
          { id: "user-3", name: "Bob" },
        ],
      },
    ])

    const revoked = {
      ...ownRemoval,
      body: { type: "revoked" } as const,
      reactions: [],
    }
    expect(
      applyMessageReactionsUpdate(
        revoked,
        { ...otherUserEvent, reactionVersion: 5 },
        "user-1"
      )
    ).toBe(revoked)
    expect(
      applyMessageReactionSnapshot(revoked, {
        conversationId: revoked.conversationId,
        messageId: revoked.id,
        reactionVersion: 6,
        reactions: [
          {
            count: 1,
            reactedByMe: true,
            text: "🎉",
            users: [{ id: "user-1", name: "Me" }],
          },
        ],
      })
    ).toBe(revoked)
  })
})

function createMessage(): ClientMessage {
  return {
    body: { content: "hello", type: "text" },
    clientMessageId: "client-message-1",
    conversationId: "conversation-1",
    createdAt: "2026-07-21T00:00:00Z",
    id: "message-1",
    reactionVersion: 2,
    reactions: [
      {
        count: 2,
        reactedByMe: true,
        text: "👍",
        users: [
          { id: "user-1", name: "Me" },
          { id: "user-2", name: "Alice" },
        ],
      },
    ],
    sender: { id: "user-2", type: "user" },
    seq: 1,
  }
}
