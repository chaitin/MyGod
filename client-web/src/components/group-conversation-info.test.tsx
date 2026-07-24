import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import { GroupConversationInfo } from "@/components/group-conversation-info"
import { Sheet, SheetContent } from "@/components/ui/sheet"
import type { ClientConversation, ClientUser } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"

describe("GroupConversationInfo", () => {
  it("confirms before leaving a group conversation", async () => {
    const user = userEvent.setup()
    const conversation = createGroupConversation()
    const leaveGroupConversation = vi.fn().mockResolvedValue(undefined)

    render(
      <MemoryRouter>
        <ClientDataContext.Provider
          value={createClientDataContextValue({
            conversations: [conversation],
            getConversation: vi.fn((conversationId: string) =>
              conversationId === conversation.id ? conversation : null
            ),
            leaveGroupConversation,
          })}
        >
          <Sheet open>
            <SheetContent showCloseButton={false}>
              <GroupConversationInfo conversationId={conversation.id} />
            </SheetContent>
          </Sheet>
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    await user.click(screen.getByRole("button", { name: "退出群聊" }))

    expect(leaveGroupConversation).not.toHaveBeenCalled()

    const dialog = await screen.findByRole("alertdialog", {
      name: "确认退出群聊",
    })
    expect(
      within(dialog).getByText("退出后将无法继续查看和发送该群聊消息。")
    ).toBeInTheDocument()

    await user.click(within(dialog).getByRole("button", { name: "退出群聊" }))

    await waitFor(() => {
      expect(leaveGroupConversation).toHaveBeenCalledWith(
        "conversation-group-1"
      )
    })
  })

  it("confirms before dissolving a group conversation for the owner", async () => {
    const user = userEvent.setup()
    const conversation = createOwnedGroupConversation()
    const dissolveGroupConversation = vi.fn().mockResolvedValue(undefined)

    render(
      <MemoryRouter>
        <ClientDataContext.Provider
          value={createClientDataContextValue({
            conversations: [conversation],
            dissolveGroupConversation,
            getConversation: vi.fn((conversationId: string) =>
              conversationId === conversation.id ? conversation : null
            ),
          })}
        >
          <Sheet open>
            <SheetContent showCloseButton={false}>
              <GroupConversationInfo conversationId={conversation.id} />
            </SheetContent>
          </Sheet>
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    expect(
      screen.queryByRole("button", { name: "退出群聊" })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "解散群聊" }))

    expect(dissolveGroupConversation).not.toHaveBeenCalled()

    const dialog = await screen.findByRole("alertdialog", {
      name: "确认解散群聊",
    })
    expect(
      within(dialog).getByText(
        "解散后所有成员都无法继续查看和发送该群聊消息。此操作不可恢复。"
      )
    ).toBeInTheDocument()

    await user.click(within(dialog).getByRole("button", { name: "解散群聊" }))

    await waitFor(() => {
      expect(dissolveGroupConversation).toHaveBeenCalledWith(
        "conversation-group-1"
      )
    })
  })

  it("sorts members by owner, admin, app, and regular member", () => {
    const conversation = createGroupConversation()
    conversation.memberCount = 5
    conversation.members = [
      {
        avatar: "",
        email: "alice@example.com",
        id: "user-1",
        name: "Alice",
        nickname: "",
        phone: "",
        role: "member",
        type: "user",
      },
      {
        avatar: "",
        email: "",
        id: "app-1",
        name: "Moli",
        nickname: "",
        phone: "",
        role: "member",
        type: "app",
      },
      {
        avatar: "",
        email: "admin@example.com",
        id: "user-3",
        name: "Admin",
        nickname: "",
        phone: "",
        role: "admin",
        type: "user",
      },
      {
        avatar: "",
        email: "owner@example.com",
        id: "user-2",
        name: "Owner",
        nickname: "",
        phone: "",
        role: "owner",
        type: "user",
      },
      {
        avatar: "",
        email: "bob@example.com",
        id: "user-4",
        name: "Bob",
        nickname: "",
        phone: "",
        role: "member",
        type: "user",
      },
    ]

    render(
      <MemoryRouter>
        <ClientDataContext.Provider
          value={createClientDataContextValue({
            conversations: [conversation],
            getConversation: vi.fn((conversationId: string) =>
              conversationId === conversation.id ? conversation : null
            ),
          })}
        >
          <Sheet open>
            <SheetContent showCloseButton={false}>
              <GroupConversationInfo conversationId={conversation.id} />
            </SheetContent>
          </Sheet>
        </ClientDataContext.Provider>
      </MemoryRouter>
    )

    const memberList = screen.getByText("群成员（5）").nextElementSibling
    expect(memberList).not.toBeNull()
    expect(memberList).toHaveTextContent(/Owner.*Admin.*Moli.*Alice.*Bob/)
  })
})

function createClientDataContextValue(
  overrides: Partial<ClientDataContextValue>
): ClientDataContextValue {
  const me: ClientUser = {
    avatar: "",
    createdAt: "2026-07-09T00:00:00Z",
    email: "alice@example.com",
    id: "user-1",
    lastOnlineAt: null,
    name: "Alice",
    nickname: "",
    phone: "",
    status: "active",
  }
  const conversation = createGroupConversation()

  return {
    contactApps: [],
    contactGroups: [],
    contacts: [],
    contactsError: null,
    contactsLoading: false,
    contactsRefreshing: false,
    conversations: [conversation],
    me,
    meError: null,
    meLoading: false,
    meRefreshing: false,
    personalProject: createPersonalProject(me),
    projects: [],
    projectsError: null,
    projectsLoading: false,
    projectsLoadingMore: false,
    projectsNextCursor: null,
    projectsRefreshing: false,
    addGroupConversationMembers: vi.fn(),
    compactConversationMessages: vi.fn(),
    registerConversationMessageView: vi.fn(() => vi.fn()),
    createGroupConversation: vi.fn(),
    createProject: vi.fn(),
    dismissConversation: vi.fn(),
    ensureConversationMessages: vi.fn(),
    dissolveGroupConversation: vi.fn(),
    getConversation: vi.fn((conversationId: string) =>
      conversationId === conversation.id ? conversation : null
    ),
    getConversationMessageState: vi.fn(),
    handleIncomingConversationMessage: vi.fn(),
    handleIncomingConversationMessageUpdate: vi.fn(),
    handleIncomingMessageChoiceUpdate: vi.fn(),
    handleIncomingMessageReactionsUpdate: vi.fn(),
    joinGroupConversation: vi.fn(),
    leaveGroupConversation: vi.fn(),
    loadBeforeConversationMessages: vi.fn(),
    loadMoreProjects: vi.fn(),
    markConversationRead: vi.fn(),
    setConversationPinned: vi.fn(),
    setConversationMuted: vi.fn(),
    mergeIncomingConversationMessage: vi.fn(),
    openAppConversation: vi.fn(),
    openDirectConversation: vi.fn(),
    refreshContacts: vi.fn(),
    refreshConversations: vi.fn(),
    restoreConversation: vi.fn(),
    refreshMe: vi.fn(),
    refreshProjects: vi.fn(),
    removeConversation: vi.fn(),
    removeGroupConversationMember: vi.fn(),
    respondToChoice: vi.fn(),
    revokeConversationMessage: vi.fn(),
    setMessageReaction: vi.fn(),
    sendConversationFile: vi.fn(),
    sendConversationImage: vi.fn(),
    sendConversationVoice: vi.fn(),
    sendConversationLink: vi.fn(),
    sendConversationMarkdown: vi.fn(),
    sendConversationCard: vi.fn(),
    sendConversationText: vi.fn(),
    setGroupConversationPrivate: vi.fn(),
    setGroupConversationPublic: vi.fn(),
    syncLoadedConversationMessages: vi.fn(),
    updateConversationLastChoiceSeq: vi.fn(),
    updateConversationLastMentionedSeq: vi.fn(),
    updateConversationLastMessage: vi.fn(),
    updateConversationPinned: vi.fn(),
    updateConversationMuted: vi.fn(),
    updateGroupConversationAvatar: vi.fn(),
    updateGroupConversationName: vi.fn(),
    ...overrides,
  }
}

function createPersonalProject(me: ClientUser) {
  return {
    avatar: "",
    createdAt: "2026-07-10T00:00:00Z",
    currentUserRole: "owner" as const,
    description: "",
    groupCount: 0,
    id: "personal-project-1",
    isPersonal: true,
    memberCount: 1,
    name: "个人工作区",
    owner: {
      avatar: me.avatar,
      id: me.id,
      name: me.name,
      nickname: me.nickname,
    },
    taskCounts: {
      canceled: 0,
      done: 0,
      inProgress: 0,
      todo: 0,
      total: 0,
    },
    updatedAt: "2026-07-10T00:00:00Z",
  }
}

function createGroupConversation(): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-09T00:00:00Z",
    id: "conversation-group-1",
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSender: null,
    lastMessageSummary: "",
    lastChoiceSeq: 0,
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 2,
    members: [
      {
        avatar: "",
        email: "owner@example.com",
        id: "user-2",
        name: "Owner",
        nickname: "",
        phone: "",
        role: "owner",
        type: "user",
      },
      {
        avatar: "",
        email: "alice@example.com",
        id: "user-1",
        name: "Alice",
        nickname: "",
        phone: "",
        role: "member",
        type: "user",
      },
    ],
    name: "产品讨论组",
    type: "group",
    unreadCount: 0,
    visibility: "private",
  }
}

function createOwnedGroupConversation(): ClientConversation {
  return {
    ...createGroupConversation(),
    members: [
      {
        avatar: "",
        email: "alice@example.com",
        id: "user-1",
        name: "Alice",
        nickname: "",
        phone: "",
        role: "owner",
        type: "user",
      },
      {
        avatar: "",
        email: "bob@example.com",
        id: "user-2",
        name: "Bob",
        nickname: "",
        phone: "",
        role: "member",
        type: "user",
      },
    ],
  }
}
