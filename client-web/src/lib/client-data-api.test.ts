import { describe, expect, it, vi } from "vitest"

import {
  ClientDataRequestError,
  createDirectConversation,
  createGroupConversation,
  getCurrentClientUser,
  joinGroupConversation,
  listClientContacts,
  listClientConversations,
  listConversationMessages,
  normalizeMessageUpdatedEventPayload,
  openAppConversation,
  revokeConversationMessage,
  sendConversationFileMessage,
  sendConversationImageMessage,
  sendConversationLinkMessage,
  sendConversationMarkdownMessage,
  sendConversationTextMessage,
  setGroupConversationPrivate,
  setGroupConversationPublic,
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

  it("loads unified client contacts with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            apps: [
              {
                avatar: "/assets/apps/assistant.webp",
                description: "专属 AI 助理",
                id: "app-1",
                name: "AI 女菩萨",
                online: false,
                type: "app",
              },
            ],
            groups: [
              {
                avatar: "",
                id: "group-1",
                joined: false,
                member_count: 8,
                name: "公开群",
                type: "group",
                visibility: "public",
              },
            ],
            users: [
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

    await expect(listClientContacts(fetcher)).resolves.toEqual({
      apps: [
        {
          avatar: "/assets/apps/assistant.webp",
          description: "专属 AI 助理",
          id: "app-1",
          name: "AI 女菩萨",
          online: false,
          type: "app",
        },
      ],
      groups: [
        {
          avatar: "",
          id: "group-1",
          joined: false,
          memberCount: 8,
          name: "公开群",
          type: "group",
          visibility: "public",
        },
      ],
      users: [
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
      ],
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/contacts", {
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
        visibility: "private",
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
      visibility: "private",
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

  it("creates or opens an app conversation with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation: {
              avatar: "/assets/apps/assistant.webp",
              created_at: "2026-07-03T07:00:00Z",
              id: "conversation-app-1",
              last_message_at: null,
              last_message_id: null,
              last_message_seq: 0,
              last_message_summary: "",
              member_count: 2,
              name: "AI 女菩萨",
              type: "app",
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

    await expect(openAppConversation("app-1", fetcher)).resolves.toEqual({
      avatar: "/assets/apps/assistant.webp",
      createdAt: "2026-07-03T07:00:00Z",
      id: "conversation-app-1",
      lastMessageAt: null,
      lastMessageId: null,
      lastMessageSeq: 0,
      lastMessageSummary: "",
      lastReadSeq: 0,
      memberCount: 2,
      name: "AI 女菩萨",
      type: "app",
      unreadCount: 0,
      visibility: "private",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/conversations/apps", {
      body: JSON.stringify({
        app_id: "app-1",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
  })

  it("creates a group conversation with credentials", async () => {
    const fetcher = vi.fn().mockImplementation(
      () =>
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
      visibility: "private",
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

  it("joins a public group conversation with credentials", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            conversation: {
              avatar: "",
              created_at: "2026-07-03T09:30:00Z",
              id: "conversation-group-1",
              member_count: 3,
              name: "公开群",
              type: "group",
              visibility: "public",
            },
            message: {
              body: {
                actor: {
                  display_name: "Alice",
                  id: "user-1",
                },
                event: "group_member_joined",
                type: "system_event",
              },
              client_message_id: "",
              conversation_id: "conversation-group-1",
              created_at: "2026-07-03T09:31:00Z",
              id: "message-join-1",
              sender: {
                type: "system",
              },
              seq: 1,
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
      joinGroupConversation("conversation-group-1", fetcher)
    ).resolves.toEqual({
      conversation: {
        avatar: "",
        createdAt: "2026-07-03T09:30:00Z",
        id: "conversation-group-1",
        lastMessageAt: null,
        lastMessageId: null,
        lastMessageSeq: 0,
        lastMessageSummary: "",
        lastReadSeq: 0,
        memberCount: 3,
        name: "公开群",
        type: "group",
        unreadCount: 0,
        visibility: "public",
      },
      message: {
        body: {
          actor: {
            displayName: "Alice",
            id: "user-1",
          },
          event: "group_member_joined",
          type: "system_event",
        },
        clientMessageId: "",
        conversationId: "conversation-group-1",
        createdAt: "2026-07-03T09:31:00Z",
        id: "message-join-1",
        sender: {
          id: "",
          type: "system",
        },
        seq: 1,
      },
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/groups/conversation-group-1/join",
      {
        credentials: "include",
        method: "POST",
      }
    )
  })

  it("sets group conversation visibility with credentials", async () => {
    const fetcher = vi.fn().mockImplementation(
      () =>
        new Response(
          JSON.stringify({
            success: true,
            data: {
              conversation: {
                avatar: "",
                created_at: "2026-07-03T09:30:00Z",
                id: "conversation-group-1",
                member_count: 2,
                name: "新品讨论组",
                type: "group",
                visibility: "public",
              },
              message: null,
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
      setGroupConversationPublic("conversation-group-1", fetcher)
    ).resolves.toMatchObject({
      conversation: {
        id: "conversation-group-1",
        type: "group",
        visibility: "public",
      },
      message: null,
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/groups/conversation-group-1/public",
      {
        credentials: "include",
        method: "POST",
      }
    )

    fetcher.mockClear()
    await setGroupConversationPrivate("conversation-group-1", fetcher)
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/groups/conversation-group-1/private",
      {
        credentials: "include",
        method: "POST",
      }
    )
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
                delegated_by: {
                  type: "app",
                  id: "app-assistant",
                  name: "女菩萨",
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
          delegatedBy: {
            type: "app",
            id: "app-assistant",
            name: "女菩萨",
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

  it("sends reply references for all conversation message create APIs", async () => {
    const fetcher = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            success: true,
            data: {
              message: {
                id: "message-reply",
                conversation_id: "conversation-1",
                seq: 13,
                sender: {
                  type: "user",
                  id: "user-1",
                },
                body: {
                  type: "text",
                  content: "回复内容",
                },
                client_message_id: "client-message-reply",
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
    )

    await sendConversationTextMessage(
      "conversation-1",
      {
        clientMessageId: "client-text",
        content: "文本回复",
        replyToMessageId: "message-quoted",
      },
      fetcher
    )
    await sendConversationMarkdownMessage(
      "conversation-1",
      {
        clientMessageId: "client-markdown",
        content: "**富文本回复**",
        replyToMessageId: "message-quoted",
      },
      fetcher
    )
    await sendConversationLinkMessage(
      "conversation-1",
      {
        clientMessageId: "client-link",
        url: "https://example.com",
        replyToMessageId: "message-quoted",
      },
      fetcher
    )
    await sendConversationFileMessage(
      "conversation-1",
      {
        clientMessageId: "client-file",
        file: new File(["file"], "report.txt", { type: "text/plain" }),
        replyToMessageId: "message-quoted",
      },
      fetcher
    )
    await sendConversationImageMessage(
      "conversation-1",
      {
        clientMessageId: "client-image",
        image: new File(["image"], "photo.webp", { type: "image/webp" }),
        replyToMessageId: "message-quoted",
      },
      fetcher
    )

    const textBody = JSON.parse(String(fetcher.mock.calls[0][1]?.body))
    expect(textBody.reply_to_message_id).toBe("message-quoted")
    const markdownBody = JSON.parse(String(fetcher.mock.calls[1][1]?.body))
    expect(markdownBody.reply_to_message_id).toBe("message-quoted")
    const linkBody = JSON.parse(String(fetcher.mock.calls[2][1]?.body))
    expect(linkBody.reply_to_message_id).toBe("message-quoted")

    const fileBody = fetcher.mock.calls[3][1]?.body
    expect(fileBody).toBeInstanceOf(FormData)
    expect((fileBody as FormData).get("reply_to_message_id")).toBe(
      "message-quoted"
    )
    const imageBody = fetcher.mock.calls[4][1]?.body
    expect(imageBody).toBeInstanceOf(FormData)
    expect((imageBody as FormData).get("reply_to_message_id")).toBe(
      "message-quoted"
    )
  })

  it("normalizes reply reference details on messages", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            messages: [
              {
                id: "message-reply",
                conversation_id: "conversation-1",
                seq: 13,
                sender: {
                  type: "user",
                  id: "user-1",
                },
                body: {
                  type: "text",
                  content: "我回复一下",
                },
                client_message_id: "client-message-reply",
                created_at: "2026-07-03T08:01:00Z",
                reply_to_message_id: "message-quoted",
                reply_to: {
                  id: "message-quoted",
                  seq: 12,
                  sender: {
                    type: "user",
                    id: "user-2",
                    name: "Bob",
                  },
                  summary: "需要被引用的消息",
                },
              },
            ],
            page: {
              has_more_after: false,
              has_more_before: false,
              limit: 20,
              newest_seq: 13,
              oldest_seq: 13,
            },
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
        }
      )
    )

    await expect(
      listConversationMessages("conversation-1", {}, fetcher)
    ).resolves.toMatchObject({
      messages: [
        {
          id: "message-reply",
          replyToMessageId: "message-quoted",
          replyTo: {
            id: "message-quoted",
            seq: 12,
            sender: {
              id: "user-2",
              name: "Bob",
              type: "user",
            },
            summary: "需要被引用的消息",
          },
        },
      ],
    })
  })

  it("normalizes revoked messages without exposing the original body", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            messages: [
              {
                id: "message-revoked",
                conversation_id: "conversation-1",
                seq: 12,
                sender: {
                  type: "user",
                  id: "user-2",
                },
                client_message_id: "client-message-revoked",
                created_at: "2026-07-03T08:00:00Z",
                revoked_at: "2026-07-03T08:02:00Z",
                revoked_by_user_id: "user-2",
              },
            ],
            page: {
              limit: 20,
              oldest_seq: 12,
              newest_seq: 12,
              has_more_before: false,
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
      listConversationMessages("conversation-1", {}, fetcher)
    ).resolves.toEqual({
      messages: [
        {
          id: "message-revoked",
          conversationId: "conversation-1",
          seq: 12,
          sender: {
            type: "user",
            id: "user-2",
          },
          body: {
            type: "revoked",
          },
          clientMessageId: "client-message-revoked",
          createdAt: "2026-07-03T08:00:00Z",
          revokedAt: "2026-07-03T08:02:00Z",
          revokedByUserId: "user-2",
        },
      ],
      page: {
        hasMoreAfter: false,
        hasMoreBefore: false,
        limit: 20,
        newestSeq: 12,
        oldestSeq: 12,
      },
    })
  })

  it("revokes a conversation message and normalizes both returned messages", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            message: {
              id: "message-12",
              conversation_id: "conversation-1",
              seq: 12,
              sender: {
                type: "user",
                id: "user-1",
              },
              client_message_id: "client-message-12",
              created_at: "2026-07-03T08:00:00Z",
              revoked_at: "2026-07-03T08:02:00Z",
              revoked_by_user_id: "user-1",
            },
            system_message: {
              id: "message-13",
              conversation_id: "conversation-1",
              seq: 13,
              sender: {
                type: "system",
              },
              body: {
                actor: {
                  display_name: "Al",
                  id: "user-1",
                },
                event: "message_revoked",
                type: "system_event",
              },
              client_message_id: "",
              created_at: "2026-07-03T08:02:00Z",
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
      revokeConversationMessage("conversation-1", "message-12", fetcher)
    ).resolves.toEqual({
      message: {
        id: "message-12",
        conversationId: "conversation-1",
        seq: 12,
        sender: {
          type: "user",
          id: "user-1",
        },
        body: {
          type: "revoked",
        },
        clientMessageId: "client-message-12",
        createdAt: "2026-07-03T08:00:00Z",
        revokedAt: "2026-07-03T08:02:00Z",
        revokedByUserId: "user-1",
      },
      systemMessage: {
        id: "message-13",
        conversationId: "conversation-1",
        seq: 13,
        sender: {
          type: "system",
          id: "",
        },
        body: {
          actor: {
            displayName: "Al",
            id: "user-1",
          },
          event: "message_revoked",
          type: "system_event",
        },
        clientMessageId: "",
        createdAt: "2026-07-03T08:02:00Z",
      },
    })
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-1/messages/message-12/revoke",
      {
        credentials: "include",
        method: "POST",
      }
    )
  })

  it("normalizes message.updated realtime payloads", () => {
    expect(
      normalizeMessageUpdatedEventPayload({
        message: {
          id: "message-12",
          conversation_id: "conversation-1",
          seq: 12,
          sender: {
            type: "user",
            id: "user-2",
          },
          client_message_id: "client-message-12",
          created_at: "2026-07-03T08:00:00Z",
          revoked_at: "2026-07-03T08:02:00Z",
          revoked_by_user_id: "user-2",
        },
      })
    ).toEqual({
      id: "message-12",
      conversationId: "conversation-1",
      seq: 12,
      sender: {
        type: "user",
        id: "user-2",
      },
      body: {
        type: "revoked",
      },
      clientMessageId: "client-message-12",
      createdAt: "2026-07-03T08:00:00Z",
      revokedAt: "2026-07-03T08:02:00Z",
      revokedByUserId: "user-2",
    })
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
