import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import { ConversationPanel } from "@/components/conversation-panel"
import type {
  ClientConversation,
  ClientConversationMember,
} from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"

describe("ConversationPanel header profile", () => {
  it("opens the direct conversation user profile and previews its avatar", async () => {
    const user = userEvent.setup()
    const otherMember = createMember({
      avatar: "/assets/users/li-si.webp",
      email: "lisi@example.com",
      id: "user-2",
      name: "李四",
      phone: "13800138000",
    })
    const conversation = createConversation({
      avatar: otherMember.avatar,
      members: [createMember(), otherMember],
      name: otherMember.name,
      type: "direct",
    })

    renderConversationHeader(conversation, {
      contacts: [
        {
          ...otherMember,
          lastOnlineAt: null,
          online: true,
          type: "user",
        },
      ],
    })

    await user.click(screen.getByRole("button", { name: "李四资料" }))

    expect(await screen.findByText("用户资料")).toBeInTheDocument()
    expect(screen.getByText("lisi@example.com")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "预览李四头像" }))

    expect(
      await screen.findByRole("dialog", { name: "李四头像预览" })
    ).toBeInTheDocument()
  })

  it("opens the application profile from an app conversation header", async () => {
    const user = userEvent.setup()
    const developer = createMember({
      email: "developer@example.com",
      id: "user-2",
      name: "应用开发者",
    })
    const appMember = createMember({
      avatar: "/assets/apps/assistant.webp",
      email: "",
      id: "app-1",
      name: "智能助手",
      type: "app",
    })
    const conversation = createConversation({
      avatar: appMember.avatar,
      id: "conversation-app-1",
      members: [createMember(), appMember],
      name: appMember.name,
      type: "app",
    })

    renderConversationHeader(conversation, {
      contactApps: [
        {
          avatar: appMember.avatar,
          creatorUserId: developer.id,
          description: "企业智能助手",
          id: appMember.id,
          name: appMember.name,
          online: true,
          type: "app",
        },
      ],
      contacts: [
        {
          ...developer,
          lastOnlineAt: null,
          online: true,
          type: "user",
        },
      ],
    })

    const appProfileTrigger = screen.getByRole("button", {
      name: "智能助手资料",
    })

    await user.click(appProfileTrigger)

    expect(await screen.findByText("企业智能助手")).toBeInTheDocument()
    const profile = screen.getByRole("dialog")
    expect(within(profile).getByText("类型")).toBeInTheDocument()
    expect(within(profile).getByText("应用")).toBeInTheDocument()
    expect(within(profile).getByText("开发者")).toBeInTheDocument()
    const developerLink = within(profile).getByRole("button", {
      name: "应用开发者资料",
    })

    await user.click(developerLink)
    expect(await screen.findByText("用户资料")).toBeInTheDocument()
    expect(screen.getByText("developer@example.com")).toBeInTheDocument()
  })

  it("opens the group profile and previews its composite avatar", async () => {
    const user = userEvent.setup()
    const conversation = createConversation({
      memberCount: 3,
      members: [
        createMember(),
        createMember({ id: "user-2", name: "李四" }),
        createMember({ id: "user-3", name: "王五" }),
      ],
      name: "项目群",
      type: "group",
    })

    renderConversationHeader(conversation)

    await user.click(screen.getByRole("button", { name: "项目群资料" }))

    expect(await screen.findByText("3 人群聊")).toBeInTheDocument()
    expect(screen.getByText("群聊资料")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "预览项目群头像" }))

    expect(
      await screen.findByRole("dialog", { name: "项目群头像预览" })
    ).toBeInTheDocument()
  })
})

function renderConversationHeader(
  conversation: ClientConversation,
  clientDataOverrides: Partial<ClientDataContextValue> = {}
) {
  render(
    <MemoryRouter>
      <ClientDataContext.Provider
        value={createClientDataValue(clientDataOverrides)}
      >
        <ConversationPanel
          conversation={conversation}
          currentUserId="user-1"
          draft=""
          historyError={null}
          historyLoading={false}
          historyLoadingBefore={false}
          messages={[]}
          onCancelReply={vi.fn()}
          onDraftChange={vi.fn()}
          onLoadBeforeMessages={vi.fn()}
          onReplyToMessage={vi.fn()}
          onRevokeMessage={vi.fn()}
          onRichTextModeChange={vi.fn()}
          onSendFile={async () => null}
          onSendImage={async () => null}
          onSendVoice={async () => null}
          onSendMessage={vi.fn()}
          replyTarget={null}
          richTextMode={false}
          sending={false}
        />
      </ClientDataContext.Provider>
    </MemoryRouter>
  )
}

function createConversation(
  overrides: Partial<ClientConversation> = {}
): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-10T00:00:00Z",
    id: "conversation-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSummary: "",
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 2,
    members: [],
    name: "测试会话",
    type: "direct",
    unreadCount: 0,
    visibility: "private",
    ...overrides,
    lastMessageSender: overrides.lastMessageSender ?? null,
  }
}

function createMember(
  overrides: Partial<ClientConversationMember> = {}
): ClientConversationMember {
  return {
    avatar: "",
    email: "me@example.com",
    id: "user-1",
    name: "张三",
    nickname: "",
    phone: "",
    role: "member",
    type: "user",
    ...overrides,
  }
}

function createClientDataValue(
  overrides: Partial<ClientDataContextValue> = {}
): ClientDataContextValue {
  return {
    contactApps: [],
    contactGroups: [],
    contacts: [],
    contactsError: null,
    contactsLoading: false,
    contactsRefreshing: false,
    conversations: [],
    me: {
      avatar: "",
      createdAt: "2026-07-09T00:00:00Z",
      email: "me@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "张三",
      nickname: "",
      phone: "",
      status: "active",
    },
    meError: null,
    meLoading: false,
    meRefreshing: false,
    openAppConversation: vi.fn(),
    openDirectConversation: vi.fn(),
    ...overrides,
  } as ClientDataContextValue
}
