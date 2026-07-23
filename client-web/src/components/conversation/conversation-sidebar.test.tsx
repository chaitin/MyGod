import { fireEvent, render, screen, waitFor } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { ConversationSidebar } from "@/components/conversation/conversation-sidebar"
import { SidebarProvider } from "@/components/ui/sidebar"
import type { ClientConversation, ClientUser } from "@/lib/client-data-api"

describe("ConversationSidebar", () => {
  it("omits the sender in direct chats and shows it in group chats", () => {
    const conversations = [
      createAppConversation({
        id: "mine",
        lastMessageSender: {
          id: "user-1",
          name: "当前用户",
          nickname: "",
          type: "user",
        },
        lastMessageSummary: "我发送的消息",
        name: "我的会话",
      }),
      createAppConversation({
        id: "direct-user",
        lastMessageSender: {
          id: "user-2",
          name: "张三",
          nickname: "小张",
          type: "user",
        },
        lastMessageSummary: "其他人的消息",
        name: "用户私聊",
        type: "direct",
      }),
      createAppConversation({
        id: "app",
        lastMessageSender: {
          id: "app-1",
          name: "发布助手",
          nickname: "",
          type: "app",
        },
        lastMessageSummary: "应用消息",
        name: "应用会话",
      }),
      createAppConversation({
        id: "group-user",
        lastMessageSender: {
          id: "user-2",
          name: "张三",
          nickname: "小张",
          type: "user",
        },
        lastMessageSummary: "群聊消息",
        name: "用户群聊",
        type: "group",
      }),
      createAppConversation({
        id: "group-system",
        lastMessageSender: {
          id: "",
          name: "系统",
          nickname: "",
          type: "system",
        },
        lastMessageSummary: "张三加入群聊",
        name: "系统会话",
        type: "group",
      }),
    ]

    render(
      <SidebarProvider>
        <ConversationSidebar
          activeConversationId=""
          appsById={new Map()}
          contactsById={new Map()}
          conversations={conversations}
          currentUser={createCurrentUser()}
          drafts={{}}
          onCreateGroup={vi.fn()}
          onSelectConversation={vi.fn()}
          onSetConversationPinned={vi.fn()}
        />
      </SidebarProvider>
    )

    expect(screen.getByText("我发送的消息")).toBeInTheDocument()
    expect(screen.getByText("其他人的消息")).toBeInTheDocument()
    expect(screen.getByText("应用消息")).toBeInTheDocument()
    expect(screen.getByText("小张：群聊消息")).toBeInTheDocument()
    expect(screen.getByText("系统：张三加入群聊")).toBeInTheDocument()
  })

  it("keeps mention and draft preview priority", () => {
    const mentioned = createAppConversation({
      id: "mentioned",
      lastMentionedSeq: 2,
      lastMessageSender: {
        id: "user-2",
        name: "张三",
        nickname: "小张",
        type: "user",
      },
      lastMessageSeq: 2,
      lastMessageSummary: "请看一下",
      name: "有提醒",
    })
    const drafted = createAppConversation({
      id: "drafted",
      lastMessageSender: {
        id: "user-2",
        name: "张三",
        nickname: "小张",
        type: "user",
      },
      lastMessageSummary: "旧消息",
      name: "有草稿",
    })
    render(
      <SidebarProvider>
        <ConversationSidebar
          activeConversationId=""
          appsById={new Map()}
          contactsById={new Map()}
          conversations={[mentioned, drafted]}
          currentUser={createCurrentUser()}
          drafts={{
            drafted: {
              mentions: [],
              replyTarget: null,
              text: "尚未发送的内容",
              updatedAt: 1,
            },
            mentioned: {
              mentions: [],
              replyTarget: null,
              text: "不会覆盖 @ 提醒",
              updatedAt: 1,
            },
          }}
          onCreateGroup={vi.fn()}
          onSelectConversation={vi.fn()}
          onSetConversationPinned={vi.fn()}
        />
      </SidebarProvider>
    )

    expect(screen.getByText("[有人 @ 我]")).toBeInTheDocument()
    expect(screen.getByText("请看一下")).toBeInTheDocument()
    expect(screen.getByText("[草稿]")).toBeInTheDocument()
    expect(screen.getByText("尚未发送的内容")).toBeInTheDocument()
    expect(screen.queryByText("不会覆盖 @ 提醒")).not.toBeInTheDocument()
  })

  it("pins an ordinary conversation from its context menu", async () => {
    const onSetConversationPinned = vi.fn().mockResolvedValue(undefined)
    render(
      <SidebarProvider>
        <ConversationSidebar
          activeConversationId="conversation-app-1"
          appsById={new Map()}
          contactsById={new Map()}
          conversations={[createAppConversation()]}
          currentUser={createCurrentUser()}
          drafts={{}}
          onCreateGroup={vi.fn()}
          onSelectConversation={vi.fn()}
          onSetConversationPinned={onSetConversationPinned}
        />
      </SidebarProvider>
    )

    fireEvent.contextMenu(screen.getByText("智能助手").closest("button")!)
    fireEvent.click(await screen.findByText("置顶对话"))

    await waitFor(() =>
      expect(onSetConversationPinned).toHaveBeenCalledWith(
        "conversation-app-1",
        true
      )
    )
  })

  it("does not show a pin action for the built-in assistant", async () => {
    const assistant = createAppConversation()
    assistant.pinned = true
    assistant.members = [
      {
        avatar: "",
        email: "",
        id: "00000000-0000-0000-0000-000000000001",
        name: "茉莉",
        nickname: "",
        phone: "",
        role: "member",
        type: "app",
      },
    ]
    assistant.name = "茉莉"
    render(
      <SidebarProvider>
        <ConversationSidebar
          activeConversationId={assistant.id}
          appsById={new Map()}
          contactsById={new Map()}
          conversations={[assistant]}
          currentUser={createCurrentUser()}
          drafts={{}}
          onCreateGroup={vi.fn()}
          onSelectConversation={vi.fn()}
          onSetConversationPinned={vi.fn()}
        />
      </SidebarProvider>
    )

    fireEvent.contextMenu(screen.getByText("茉莉").closest("button")!)
    expect(await screen.findByText("消息免打扰")).toBeInTheDocument()
    expect(screen.queryByText("置顶对话")).not.toBeInTheDocument()
    expect(screen.queryByText("取消置顶")).not.toBeInTheDocument()
  })

  it("shows muted unread conversations as a dot and toggles mute", async () => {
    const onSetConversationMuted = vi.fn().mockResolvedValue(undefined)
    const conversation = createAppConversation({
      notificationMuted: true,
      unreadCount: 8,
    })
    render(
      <SidebarProvider>
        <ConversationSidebar
          activeConversationId=""
          appsById={new Map()}
          contactsById={new Map()}
          conversations={[conversation]}
          currentUser={createCurrentUser()}
          drafts={{}}
          onCreateGroup={vi.fn()}
          onSelectConversation={vi.fn()}
          onSetConversationMuted={onSetConversationMuted}
          onSetConversationPinned={vi.fn()}
        />
      </SidebarProvider>
    )

    expect(screen.getByLabelText("消息免打扰")).toBeInTheDocument()
    expect(screen.getByLabelText("有未读消息")).toBeInTheDocument()
    expect(screen.queryByLabelText("8 条未读消息")).not.toBeInTheDocument()

    fireEvent.contextMenu(screen.getByText("智能助手").closest("button")!)
    fireEvent.click(await screen.findByText("取消免打扰"))
    await waitFor(() =>
      expect(onSetConversationMuted).toHaveBeenCalledWith(
        "conversation-app-1",
        false
      )
    )
  })

  it("confirms before deleting a conversation", async () => {
    const onDismissConversation = vi.fn().mockResolvedValue(undefined)
    render(
      <SidebarProvider>
        <ConversationSidebar
          activeConversationId=""
          appsById={new Map()}
          contactsById={new Map()}
          conversations={[createAppConversation()]}
          currentUser={createCurrentUser()}
          drafts={{}}
          onCreateGroup={vi.fn()}
          onDismissConversation={onDismissConversation}
          onSelectConversation={vi.fn()}
          onSetConversationPinned={vi.fn()}
        />
      </SidebarProvider>
    )

    fireEvent.contextMenu(screen.getByText("智能助手").closest("button")!)
    fireEvent.click(await screen.findByText("删除对话"))
    expect(await screen.findByText("删除对话？")).toBeInTheDocument()
    fireEvent.click(screen.getByRole("button", { name: "删除" }))

    await waitFor(() =>
      expect(onDismissConversation).toHaveBeenCalledWith("conversation-app-1")
    )
  })
})

function createAppConversation(
  overrides: Partial<ClientConversation> = {}
): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-17T00:00:00Z",
    id: "conversation-app-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSender: null,
    lastMessageSummary: "暂无消息",
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 2,
    members: [],
    name: "智能助手",
    type: "app",
    unreadCount: 0,
    visibility: "private",
    ...overrides,
  }
}

function createCurrentUser(): ClientUser {
  return {
    avatar: "",
    createdAt: "2026-07-17T00:00:00Z",
    email: "me@example.com",
    id: "user-1",
    lastOnlineAt: null,
    name: "当前用户",
    nickname: "",
    phone: "",
    status: "active",
  }
}
