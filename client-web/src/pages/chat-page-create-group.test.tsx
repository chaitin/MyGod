import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ChatPage } from "@/pages/chat-page"
import type { ClientConversation, ClientUser } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"
import {
  readLastConversationId,
  writeLastConversationId,
} from "@/lib/last-conversation"

describe("ChatPage create group dialog", () => {
  it("creates groups with and without selected apps", async () => {
    for (const appIds of [[], ["app-1"]]) {
      const user = userEvent.setup()
      const createGroupConversation = vi
        .fn()
        .mockResolvedValue(createGroupConversationResponse())
      const view = renderChatPage({ createGroupConversation })

      await openCreateGroupDialog(user)
      expect(screen.getByLabelText("群聊名称")).toHaveValue("新建群聊")

      if (appIds.length > 0) {
        await user.click(screen.getByRole("tab", { name: "应用" }))
        await user.click(screen.getByRole("checkbox", { name: "茉莉" }))
      }

      await user.click(screen.getByRole("button", { name: "创建" }))
      expect(createGroupConversation).toHaveBeenCalledWith(
        "新建群聊",
        [],
        appIds
      )

      view.unmount()
    }
  })
})

describe("ChatPage last conversation", () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it("records the active conversation for the current user", async () => {
    const conversation = createConversation("conversation-1", "产品群")
    renderChatPage(
      createConversationOverrides([conversation]),
      "/chat/conversation-1"
    )

    await waitFor(() =>
      expect(readLastConversationId("user-1")).toBe("conversation-1")
    )
  })

  it("restores the last valid conversation when entering /chat", async () => {
    const conversation = createConversation("conversation-1", "产品群")
    const overrides = createConversationOverrides([conversation])
    writeLastConversationId("user-1", conversation.id)

    renderChatPage(overrides)

    await waitFor(() =>
      expect(screen.getByTestId("chat-location")).toHaveTextContent(
        "/chat/conversation-1"
      )
    )
    expect(overrides.ensureConversationMessages).toHaveBeenCalledWith(
      "conversation-1"
    )
  })

  it("clears a stored conversation that is no longer available", async () => {
    writeLastConversationId("user-1", "missing-conversation")

    renderChatPage()

    await waitFor(() => expect(readLastConversationId("user-1")).toBe(""))
    expect(screen.getByTestId("chat-location")).toHaveTextContent("/chat")
  })

  it("keeps an explicit conversation route and records it as the latest", async () => {
    const previousConversation = createConversation(
      "conversation-1",
      "之前的群聊"
    )
    const explicitConversation = createConversation(
      "conversation-2",
      "显式打开的群聊"
    )
    writeLastConversationId("user-1", previousConversation.id)

    renderChatPage(
      createConversationOverrides([previousConversation, explicitConversation]),
      "/chat/conversation-2"
    )

    expect(screen.getByTestId("chat-location")).toHaveTextContent(
      "/chat/conversation-2"
    )
    await waitFor(() =>
      expect(readLastConversationId("user-1")).toBe("conversation-2")
    )
  })
})

async function openCreateGroupDialog(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "新建 Agent" }))
  await user.click(screen.getByRole("menuitem", { name: "发起群聊" }))
}

function renderChatPage(
  overrides: Partial<ClientDataContextValue> = {},
  initialEntry = "/chat"
) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ClientDataContext.Provider value={createClientDataValue(overrides)}>
        <Routes>
          <Route
            path="/chat/:conversationId?"
            element={
              <>
                <ChatPage />
                <LocationProbe />
              </>
            }
          />
        </Routes>
      </ClientDataContext.Provider>
    </MemoryRouter>
  )
}

function LocationProbe() {
  return <output data-testid="chat-location">{useLocation().pathname}</output>
}

function createConversationOverrides(
  conversations: ClientConversation[]
): Partial<ClientDataContextValue> {
  return {
    conversations,
    ensureConversationMessages: vi.fn(),
    getConversation: vi.fn(
      (conversationId: string) =>
        conversations.find(
          (conversation) => conversation.id === conversationId
        ) ?? null
    ),
  }
}

function createClientDataValue(
  overrides: Partial<ClientDataContextValue>
): ClientDataContextValue {
  const me: ClientUser = {
    avatar: "",
    createdAt: "2026-07-10T00:00:00Z",
    email: "alice@example.com",
    id: "user-1",
    lastOnlineAt: null,
    name: "Alice",
    nickname: "",
    phone: "",
    status: "active",
  }

  return {
    contactApps: [
      {
        avatar: "/assets/apps/assistant.webp",
        description: "AI 助手",
        id: "app-1",
        name: "茉莉",
        online: true,
        type: "app",
      },
    ],
    contactGroups: [],
    contacts: [
      {
        avatar: "",
        email: "bob@example.com",
        id: "user-2",
        lastOnlineAt: null,
        name: "Bob",
        nickname: "",
        online: false,
        phone: "",
        type: "user",
      },
    ],
    contactsError: null,
    contactsLoading: false,
    contactsRefreshing: false,
    conversations: [],
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
    createGroupConversation: vi.fn(),
    createProject: vi.fn(),
    dissolveGroupConversation: vi.fn(),
    ensureConversationMessages: vi.fn(),
    getConversation: vi.fn(() => null),
    getConversationMessageState: vi.fn(),
    handleIncomingConversationMessage: vi.fn(),
    handleIncomingConversationMessageUpdate: vi.fn(),
    joinGroupConversation: vi.fn(),
    leaveGroupConversation: vi.fn(),
    loadBeforeConversationMessages: vi.fn(),
    loadMoreProjects: vi.fn(),
    markConversationRead: vi.fn(),
    mergeIncomingConversationMessage: vi.fn(),
    openAppConversation: vi.fn(),
    openDirectConversation: vi.fn(),
    refreshContacts: vi.fn(),
    refreshConversations: vi.fn(),
    refreshMe: vi.fn(),
    refreshProjects: vi.fn(),
    removeConversation: vi.fn(),
    removeGroupConversationMember: vi.fn(),
    revokeConversationMessage: vi.fn(),
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
    updateConversationLastMentionedSeq: vi.fn(),
    updateConversationLastMessage: vi.fn(),
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

function createGroupConversationResponse(): ClientConversation {
  return createConversation("conversation-group-1", "新建群聊")
}

function createConversation(id: string, name: string): ClientConversation {
  return {
    avatar: "",
    createdAt: "2026-07-10T00:00:00Z",
    id,
    lastMessageAt: null,
    lastMessageId: null,
    lastMessageSeq: 0,
    lastMessageSummary: "",
    lastMentionedSeq: 0,
    lastReadSeq: 0,
    memberCount: 1,
    members: [],
    name,
    type: "group",
    unreadCount: 0,
    visibility: "private",
  }
}
