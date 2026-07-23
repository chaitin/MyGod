import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { AddGroupMembersDialog } from "@/components/add-group-members-dialog"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"
import type { ClientConversation, ClientUser } from "@/lib/client-data-api"

describe("AddGroupMembersDialog", () => {
  it("adds selected apps through the group member dialog", async () => {
    const user = userEvent.setup()
    const conversation = createGroupConversation()
    const addGroupConversationMembers = vi.fn().mockResolvedValue(conversation)

    render(
      <ClientDataContext.Provider
        value={createClientDataContextValue({ addGroupConversationMembers })}
      >
        <AddGroupMembersDialog conversation={conversation} />
      </ClientDataContext.Provider>
    )

    await user.click(screen.getByRole("button", { name: "添加成员" }))
    await user.click(screen.getByRole("tab", { name: "应用" }))
    await user.click(screen.getByRole("checkbox", { name: "茉莉" }))
    await user.click(screen.getByRole("button", { name: "添加" }))

    expect(addGroupConversationMembers).toHaveBeenCalledWith(
      "conversation-group-1",
      [],
      ["app-1"]
    )
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

  return {
    contactApps: [
      {
        avatar: "/assets/apps/assistant.webp",
        creatorUserId: null,
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
    conversations: [createGroupConversation()],
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
    dissolveGroupConversation: vi.fn(),
    ensureConversationMessages: vi.fn(),
    getConversation: vi.fn(),
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
    memberCount: 1,
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
    ],
    name: "产品讨论组",
    type: "group",
    unreadCount: 0,
    visibility: "private",
  }
}
