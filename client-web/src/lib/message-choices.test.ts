import { describe, expect, it, vi } from "vitest"

import {
  formatClientMessageBodySummary,
  listConversationMessageChoiceSnapshots,
  normalizeConversationMemberChoiceReceivedEventPayload,
  normalizeMessage,
  normalizeMessageChoiceUpdatedEventPayload,
  submitConversationMessageChoiceResponse,
  type ClientMessage,
} from "@/lib/client-data-api"
import {
  applyMessageChoiceSnapshot,
  applyMessageChoiceState,
} from "@/lib/client-data-state"

describe("message choices", () => {
  it("normalizes choice messages and their summary", () => {
    const message = normalizeMessage({
      body: {
        type: "choice",
        content_type: "markdown",
        content: "## 请选择\n\n- 一个项目",
        selection: "single",
        options: [
          { id: "project-a", label: "项目 A" },
          { id: "project-b", label: "项目 B" },
        ],
      },
      choice: {
        my_option_ids: ["project-a"],
        options: [
          { id: "project-a", response_count: 2 },
          { id: "project-b", response_count: 1 },
        ],
        response_count: 3,
      },
      client_message_id: "client-1",
      conversation_id: "conversation-1",
      created_at: "2026-07-23T10:00:00Z",
      id: "message-1",
      reaction_version: 0,
      reactions: [],
      sender: { id: "app-1", type: "app" },
      seq: 1,
    })

    expect(message.body).toEqual({
      type: "choice",
      contentType: "markdown",
      content: "## 请选择\n\n- 一个项目",
      selection: "single",
      options: [
        { id: "project-a", label: "项目 A" },
        { id: "project-b", label: "项目 B" },
      ],
    })
    expect(message.choice).toEqual({
      myOptionIds: ["project-a"],
      options: [
        { id: "project-a", responseCount: 2 },
        { id: "project-b", responseCount: 1 },
      ],
      responseCount: 3,
    })
    expect(formatClientMessageBodySummary(message.body)).toBe(
      "[选择] 请选择\n一个项目"
    )
  })

  it("treats a legacy null choice answer as an empty selection", () => {
    const message = normalizeMessage({
      body: {
        type: "choice",
        content_type: "text",
        content: "请选择",
        selection: "single",
        options: [
          { id: "yes", label: "是" },
          { id: "no", label: "否" },
        ],
      },
      choice: {
        my_option_ids: null,
        options: [
          { id: "yes", response_count: 0 },
          { id: "no", response_count: 0 },
        ],
        response_count: 0,
      },
      client_message_id: "client-1",
      conversation_id: "conversation-1",
      created_at: "2026-07-23T10:00:00Z",
      id: "message-1",
      reaction_version: 0,
      reactions: [],
      sender: { id: "app-1", type: "app" },
      seq: 1,
    })

    expect(message.choice?.myOptionIds).toEqual([])
  })

  it("submits an answer and loads ordered reconnect snapshots", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse(
          {
            success: true,
            data: {
              conversation_id: "conversation-1",
              message_id: "message-1",
              created: true,
              response: {
                id: "response-1",
                user_id: "user-1",
                option_ids: ["a", "c"],
                created_at: "2026-07-23T10:01:00Z",
              },
              choice: {
                my_option_ids: ["a", "c"],
                options: [
                  { id: "a", response_count: 1 },
                  { id: "b", response_count: 0 },
                  { id: "c", response_count: 1 },
                ],
                response_count: 1,
              },
            },
          },
          201
        )
      )
      .mockResolvedValueOnce(
        jsonResponse({
          success: true,
          data: {
            conversation_id: "conversation-1",
            snapshots: [
              {
                message_id: "message-2",
                status: "active",
                choice: {
                  my_option_ids: [],
                  options: [
                    { id: "yes", response_count: 2 },
                    { id: "no", response_count: 0 },
                  ],
                  response_count: 2,
                },
              },
              {
                message_id: "message-1",
                status: "active",
                choice: {
                  my_option_ids: ["a", "c"],
                  options: [
                    { id: "a", response_count: 1 },
                    { id: "b", response_count: 0 },
                    { id: "c", response_count: 1 },
                  ],
                  response_count: 1,
                },
              },
            ],
          },
        })
      )

    await expect(
      submitConversationMessageChoiceResponse(
        "conversation-1",
        "message-1",
        ["a", "c"],
        fetcher
      )
    ).resolves.toMatchObject({
      conversationId: "conversation-1",
      messageId: "message-1",
      created: true,
      choice: { myOptionIds: ["a", "c"], responseCount: 1 },
    })
    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "/api/client/conversations/conversation-1/messages/message-1/choice-response",
      expect.objectContaining({
        body: JSON.stringify({ option_ids: ["a", "c"] }),
        method: "PUT",
      })
    )

    await expect(
      listConversationMessageChoiceSnapshots(
        "conversation-1",
        ["message-2", "message-1"],
        fetcher
      )
    ).resolves.toMatchObject([
      {
        messageId: "message-2",
        choice: { responseCount: 2 },
        status: "active",
      },
      {
        messageId: "message-1",
        choice: { myOptionIds: ["a", "c"], responseCount: 1 },
        status: "active",
      },
    ])
  })

  it("normalizes revoked and deleted choice tombstones without failing the batch", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      jsonResponse({
        success: true,
        data: {
          conversation_id: "conversation-1",
          snapshots: [
            { message_id: "message-1", status: "revoked" },
            { message_id: "message-2", status: "deleted" },
          ],
        },
      })
    )

    await expect(
      listConversationMessageChoiceSnapshots(
        "conversation-1",
        ["message-1", "message-2"],
        fetcher
      )
    ).resolves.toEqual([
      {
        choice: null,
        conversationId: "conversation-1",
        messageId: "message-1",
        status: "revoked",
      },
      {
        choice: null,
        conversationId: "conversation-1",
        messageId: "message-2",
        status: "deleted",
      },
    ])
  })

  it("normalizes realtime events and never regresses a newer local answer", () => {
    expect(
      normalizeConversationMemberChoiceReceivedEventPayload({
        conversation_id: "conversation-1",
        last_choice_seq: 9,
      })
    ).toEqual({ conversationId: "conversation-1", lastChoiceSeq: 9 })
    expect(
      normalizeMessageChoiceUpdatedEventPayload({
        actor_option_ids: ["b"],
        actor_user_id: "user-2",
        conversation_id: "conversation-1",
        message_id: "message-1",
        choice: {
          my_option_ids: [],
          options: [
            { id: "a", response_count: 1 },
            { id: "b", response_count: 2 },
          ],
          response_count: 3,
        },
      })
    ).toMatchObject({
      actorOptionIds: ["b"],
      actorUserId: "user-2",
      choice: { responseCount: 3 },
    })

    const message = createChoiceMessage()
    expect(
      applyMessageChoiceState(message, {
        myOptionIds: [],
        options: [
          { id: "a", responseCount: 1 },
          { id: "b", responseCount: 0 },
        ],
        responseCount: 1,
      })
    ).toBe(message)

    const withoutMyAnswer = {
      ...message,
      choice: { ...message.choice!, myOptionIds: [] },
    }
    expect(
      applyMessageChoiceState(withoutMyAnswer, {
        myOptionIds: ["a"],
        options: [
          { id: "a", responseCount: 1 },
          { id: "b", responseCount: 0 },
        ],
        responseCount: 1,
      }).choice
    ).toEqual(message.choice)
  })

  it("turns unavailable reconnect snapshots into tombstones", () => {
    const message = createChoiceMessage()
    expect(
      applyMessageChoiceSnapshot(message, {
        choice: null,
        conversationId: message.conversationId,
        messageId: message.id,
        status: "revoked",
      })
    ).toMatchObject({ body: { type: "revoked" }, choice: undefined })
    expect(
      applyMessageChoiceSnapshot(message, {
        choice: null,
        conversationId: message.conversationId,
        messageId: message.id,
        status: "deleted",
      })
    ).toBeNull()
  })
})

function createChoiceMessage(): ClientMessage {
  return {
    body: {
      type: "choice",
      contentType: "text",
      content: "请选择",
      selection: "single",
      options: [
        { id: "a", label: "A" },
        { id: "b", label: "B" },
      ],
    },
    choice: {
      myOptionIds: ["a"],
      options: [
        { id: "a", responseCount: 2 },
        { id: "b", responseCount: 1 },
      ],
      responseCount: 3,
    },
    clientMessageId: "client-1",
    conversationId: "conversation-1",
    createdAt: "2026-07-23T10:00:00Z",
    id: "message-1",
    reactionVersion: 0,
    reactions: [],
    sender: { id: "app-1", type: "app" },
    seq: 1,
  }
}

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    headers: { "content-type": "application/json" },
    status,
  })
}
