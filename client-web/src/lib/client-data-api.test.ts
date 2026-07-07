import { describe, expect, it, vi } from "vitest"

import {
  ClientDataRequestError,
  createDirectConversation,
  createGroupConversation,
  getCurrentClientUser,
  listClientContacts,
  listClientConversations,
  listConversationMessages,
  sendConversationTextMessage,
  updateCurrentClientUser,
} from "@/lib/client-data-api"

describe("client data API", () => {
  it("loads the current client user with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              avatar: "/assets/avatars/builtin/17.webp",
              created_at: "2026-07-01T12:34:56Z",
              email: "alice@example.com",
              id: "user-1",
              name: "Alice Zhang",
              nickname: "Al",
              phone: "+8613912345678",
              status: "active",
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await expect(getCurrentClientUser(fetcher)).resolves.toEqual({
      avatar: "/assets/avatars/builtin/17.webp",
      createdAt: "2026-07-01T12:34:56Z",
      email: "alice@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "Alice Zhang",
      nickname: "Al",
      phone: "+8613912345678",
      status: "active",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/me", {
      credentials: "include",
      method: "GET",
    })
  })

  it("loads all client contacts with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            contacts: [
              {
                avatar: "/assets/avatars/builtin/03.webp",
                email: "bob@example.com",
                id: "user-2",
                last_online_at: "2026-07-03T01:00:00Z",
                name: "Bob Li",
                nickname: "",
                online: true,
                phone: "+8613912345679",
                type: "user",
              },
            ],
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await expect(listClientContacts(fetcher)).resolves.toEqual([
      {
        avatar: "/assets/avatars/builtin/03.webp",
        email: "bob@example.com",
        id: "user-2",
        lastOnlineAt: "2026-07-03T01:00:00Z",
        name: "Bob Li",
        nickname: "",
        online: true,
        phone: "+8613912345679",
        type: "user",
      },
    ])
    expect(fetcher).toHaveBeenCalledWith("/api/client/contacts/users", {
      credentials: "include",
      method: "GET",
    })
  })

  it("loads client conversations with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversations: [
              {
                avatar: "/assets/avatars/builtin/03.webp",
                created_at: "2026-07-03T07:00:00Z",
                id: "conversation-1",
                last_message_at: "2026-07-03T08:00:00Z",
                last_message_id: "message-1",
                last_message_seq: 12,
                last_message_summary: "好的，我看一下",
                member_count: 2,
                name: "Bob Li",
                type: "direct",
              },
            ],
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await expect(listClientConversations(fetcher)).resolves.toEqual([
      {
        avatar: "/assets/avatars/builtin/03.webp",
        createdAt: "2026-07-03T07:00:00Z",
        id: "conversation-1",
        lastMessageAt: "2026-07-03T08:00:00Z",
        lastMessageId: "message-1",
        lastMessageSeq: 12,
        lastMessageSummary: "好的，我看一下",
        lastReadSeq: 0,
        memberCount: 2,
        name: "Bob Li",
        type: "direct",
        unreadCount: 0,
      },
    ])
    expect(fetcher).toHaveBeenCalledWith("/api/client/conversations", {
      credentials: "include",
      method: "GET",
    })
  })

  it("creates or opens a direct conversation with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation: {
              avatar: "/assets/avatars/builtin/03.webp",
              created_at: "2026-07-03T07:00:00Z",
              id: "conversation-1",
              last_message_at: null,
              last_message_id: null,
              last_message_seq: 0,
              last_message_summary: "",
              member_count: 2,
              name: "Bob Li",
              type: "direct",
            },
            created: true,
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 201,
        }
      )
    )

    await expect(createDirectConversation("user-2", fetcher)).resolves.toEqual({
      avatar: "/assets/avatars/builtin/03.webp",
      createdAt: "2026-07-03T07:00:00Z",
      id: "conversation-1",
      lastMessageAt: null,
      lastMessageId: null,
      lastMessageSeq: 0,
      lastMessageSummary: "",
      lastReadSeq: 0,
      memberCount: 2,
      name: "Bob Li",
      type: "direct",
      unreadCount: 0,
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/conversations/direct", {
      body: JSON.stringify({
        user_id: "user-2",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("creates a group conversation with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation: {
              created_at: "2026-07-03T09:30:00Z",
              created_by_user_id: "user-1",
              id: "conversation-group-1",
              member_count: 2,
              members: [
                {
                  avatar: "/assets/avatars/builtin/17.webp",
                  email: "alice@example.com",
                  id: "user-1",
                  name: "Alice",
                  nickname: "Al",
                  phone: "+8613912345678",
                  role: "owner",
                },
                {
                  avatar: "/assets/avatars/builtin/03.webp",
                  email: "bob@example.com",
                  id: "user-2",
                  name: "Bob Li",
                  nickname: "",
                  phone: "+8613912345679",
                  role: "member",
                },
              ],
              name: "新品讨论组",
              posting_policy: "open",
              status: "active",
              type: "group",
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 201,
        }
      )
    )

    await expect(
      createGroupConversation(
        {
          memberIds: ["user-2"],
          name: "新品讨论组",
        },
        fetcher
      )
    ).resolves.toEqual({
      avatar: "",
      createdAt: "2026-07-03T09:30:00Z",
      id: "conversation-group-1",
      lastMessageAt: null,
      lastMessageId: null,
      lastMessageSeq: 0,
      lastMessageSummary: "",
      lastReadSeq: 0,
      memberCount: 2,
      members: [
        {
          avatar: "/assets/avatars/builtin/17.webp",
          email: "alice@example.com",
          id: "user-1",
          name: "Alice",
          nickname: "Al",
          phone: "+8613912345678",
          role: "owner",
          type: "user",
        },
        {
          avatar: "/assets/avatars/builtin/03.webp",
          email: "bob@example.com",
          id: "user-2",
          name: "Bob Li",
          nickname: "",
          phone: "+8613912345679",
          role: "member",
          type: "user",
        },
      ],
      name: "新品讨论组",
      type: "group",
      unreadCount: 0,
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/conversations/groups", {
      body: JSON.stringify({
        member_ids: ["user-2"],
        name: "新品讨论组",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("loads conversation messages with pagination params", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            messages: [
              {
                id: "message-12",
                conversation_id: "conversation-1",
                seq: 12,
                sender: {
                  type: "user",
                  id: "user-2",
                },
                body: {
                  type: "text",
                  content: "好的，我看一下",
                },
                client_message_id: "client-message-12",
                created_at: "2026-07-03T08:00:00Z",
              },
            ],
            page: {
              limit: 20,
              oldest_seq: 12,
              newest_seq: 12,
              has_more_before: true,
              has_more_after: false,
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await expect(
      listConversationMessages(
        "conversation-1",
        {
          beforeSeq: 13,
          limit: 20,
        },
        fetcher
      )
    ).resolves.toEqual({
      messages: [
        {
          id: "message-12",
          conversationId: "conversation-1",
          seq: 12,
          sender: {
            type: "user",
            id: "user-2",
          },
          body: {
            type: "text",
            content: "好的，我看一下",
          },
          clientMessageId: "client-message-12",
          createdAt: "2026-07-03T08:00:00Z",
        },
      ],
      page: {
        limit: 20,
        oldestSeq: 12,
        newestSeq: 12,
        hasMoreBefore: true,
        hasMoreAfter: false,
      },
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-1/messages?limit=20&before_seq=13",
      {
        credentials: "include",
        method: "GET",
      }
    )
  })

  it("sends a text message with a client message id", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            message: {
              id: "message-13",
              conversation_id: "conversation-1",
              seq: 13,
              sender: {
                type: "user",
                id: "user-1",
              },
              body: {
                type: "text",
                content: "帮我总结今天的消息",
              },
              client_message_id: "client-message-13",
              created_at: "2026-07-03T08:01:00Z",
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 201,
        }
      )
    )

    await expect(
      sendConversationTextMessage(
        "conversation-1",
        {
          clientMessageId: "client-message-13",
          content: "帮我总结今天的消息",
        },
        fetcher
      )
    ).resolves.toEqual({
      id: "message-13",
      conversationId: "conversation-1",
      seq: 13,
      sender: {
        type: "user",
        id: "user-1",
      },
      body: {
        type: "text",
        content: "帮我总结今天的消息",
      },
      clientMessageId: "client-message-13",
      createdAt: "2026-07-03T08:01:00Z",
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-1/messages",
      {
        body: JSON.stringify({
          client_message_id: "client-message-13",
          body: {
            type: "text",
            content: "帮我总结今天的消息",
          },
        }),
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }
    )
  })

  it("updates the current client user profile with partial fields", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              avatar: "/assets/avatars/builtin/03.webp",
              created_at: "2026-07-01T12:34:56Z",
              email: "alice@example.com",
              id: "user-1",
              name: "Alice Zhang",
              nickname: "Al",
              phone: "+8613912345678",
              status: "active",
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await expect(
      updateCurrentClientUser(
        {
          avatar: "/assets/avatars/builtin/03.webp",
        },
        fetcher
      )
    ).resolves.toMatchObject({
      avatar: "/assets/avatars/builtin/03.webp",
      nickname: "Al",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/me", {
      body: JSON.stringify({
        avatar: "/assets/avatars/builtin/03.webp",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    })
  })

  it("throws a typed unauthorized error", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "unauthorized",
            message: "未登录",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 401,
        }
      )
    )

    await expect(getCurrentClientUser(fetcher)).rejects.toMatchObject({
      code: "unauthorized",
      message: "未登录",
      name: "ClientDataRequestError",
      status: 401,
    } satisfies ClientDataRequestError)
  })
})
