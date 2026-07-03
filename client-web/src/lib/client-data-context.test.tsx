import { act, render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ClientDataProvider } from "@/components/client-data-provider"
import { useClientData } from "@/lib/client-data-context"

function createSuccessResponse(data: unknown) {
  return new Response(
    JSON.stringify({
      success: true,
      data,
    }),
    {
      headers: {
        "content-type": "application/json",
      },
      status: 200,
    }
  )
}

function createMeResponse(name = "Alice Zhang") {
  return createSuccessResponse({
    user: {
      avatar: "/assets/avatars/builtin/17.webp",
      created_at: "2026-07-01T12:34:56Z",
      email: "alice@example.com",
      id: "user-1",
      name,
      nickname: "Al",
      phone: "+8613912345678",
      status: "active",
    },
  })
}

function createContactsResponse(name = "Bob Li") {
  return createSuccessResponse({
    contacts: [
      {
        avatar: "/assets/avatars/builtin/03.webp",
        email: "bob@example.com",
        id: "user-2",
        name,
        nickname: "",
        phone: "+8613912345679",
        type: "user",
      },
    ],
  })
}

function createConversationsResponse(name = "Bob Li") {
  return createSuccessResponse({
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
        name,
        type: "direct",
      },
    ],
  })
}

function createDirectConversationResponse() {
  return createSuccessResponse({
    conversation: {
      avatar: "/assets/avatars/builtin/05.webp",
      created_at: "2026-07-03T09:00:00Z",
      id: "conversation-2",
      last_message_at: null,
      last_message_id: null,
      last_message_seq: 0,
      last_message_summary: "",
      member_count: 2,
      name: "Carol Wang",
      type: "direct",
    },
    created: true,
  })
}

function createClientDataFetchMock() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)

    if (path === "/api/client/me") {
      return createMeResponse()
    }

    if (path === "/api/client/contacts/users") {
      return createContactsResponse()
    }

    if (path === "/api/client/conversations") {
      return createConversationsResponse()
    }

    if (
      path === "/api/client/conversations/direct" &&
      init?.method === "POST"
    ) {
      return createDirectConversationResponse()
    }

    return new Response(null, { status: 404 })
  })
}

function createClientDataErrorFetchMock() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input)

    if (path === "/api/client/me") {
      return createMeResponse()
    }

    if (path === "/api/client/contacts/users") {
      return new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "internal_error",
            message: "通讯录加载失败",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 500,
        }
      )
    }

    if (path === "/api/client/conversations") {
      return createConversationsResponse()
    }

    return new Response(null, { status: 404 })
  })
}

function DataProbe() {
  const {
    conversations,
    contacts,
    contactsRefreshing,
    me,
    meRefreshing,
    openDirectConversation,
    refreshContacts,
    refreshMe,
  } = useClientData()

  return (
    <div>
      <span data-testid="me-name">{me.name}</span>
      <span data-testid="contact-count">{contacts.length}</span>
      <span data-testid="conversation-count">{conversations.length}</span>
      <span data-testid="first-conversation-name">
        {conversations[0]?.name ?? ""}
      </span>
      <span data-testid="me-refreshing">{String(meRefreshing)}</span>
      <span data-testid="contacts-refreshing">
        {String(contactsRefreshing)}
      </span>
      <button type="button" onClick={() => void refreshMe()}>
        refresh me
      </button>
      <button type="button" onClick={() => void refreshContacts()}>
        refresh contacts
      </button>
      <button
        type="button"
        onClick={() => void openDirectConversation("user-3")}
      >
        open direct
      </button>
    </div>
  )
}

function renderProvider() {
  return render(
    <MemoryRouter initialEntries={["/chat"]}>
      <ClientDataProvider>
        <DataProbe />
      </ClientDataProvider>
    </MemoryRouter>
  )
}

async function flushBootstrapPromises() {
  for (let index = 0; index < 10; index += 1) {
    await Promise.resolve()
  }
}

describe("ClientDataProvider", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", createClientDataFetchMock())
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it("keeps the loading page visible for at least two seconds", async () => {
    vi.useFakeTimers()
    renderProvider()

    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    const progressbar = screen.getByRole("progressbar")

    expect(progressbar).toBeInTheDocument()
    expect(progressbar.firstElementChild).toHaveClass(
      "client-loading-progress-indicator"
    )
    expect(screen.queryByTestId("me-name")).not.toBeInTheDocument()

    await act(async () => {
      await flushBootstrapPromises()
    })

    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    expect(screen.queryByTestId("me-name")).not.toBeInTheDocument()

    await act(async () => {
      vi.advanceTimersByTime(1_999)
      await flushBootstrapPromises()
    })

    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    expect(screen.queryByTestId("me-name")).not.toBeInTheDocument()

    await act(async () => {
      vi.advanceTimersByTime(1)
      await flushBootstrapPromises()
    })

    expect(screen.getByTestId("me-name")).toHaveTextContent("Alice Zhang")
    expect(screen.getByTestId("contact-count")).toHaveTextContent("1")
    expect(screen.getByTestId("conversation-count")).toHaveTextContent("1")
    expect(screen.queryByText("正在为你加载数据")).not.toBeInTheDocument()
  })

  it("refreshes me and contacts independently every minute", async () => {
    vi.useFakeTimers()
    const fetcher = fetch as unknown as ReturnType<typeof vi.fn>
    renderProvider()

    await act(async () => {
      await flushBootstrapPromises()
      vi.advanceTimersByTime(2_000)
      await flushBootstrapPromises()
    })
    expect(screen.getByTestId("me-name")).toHaveTextContent("Alice Zhang")
    fetcher.mockClear()

    await act(async () => {
      vi.advanceTimersByTime(60_000)
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(fetcher).toHaveBeenCalledWith("/api/client/me", {
      credentials: "include",
      method: "GET",
    })
    expect(fetcher).toHaveBeenCalledWith("/api/client/contacts/users", {
      credentials: "include",
      method: "GET",
    })
    expect(fetcher).not.toHaveBeenCalledWith("/api/client/conversations", {
      credentials: "include",
      method: "GET",
    })
  })

  it("opens a direct conversation and prepends it to conversations", async () => {
    vi.useFakeTimers()
    const fetcher = fetch as unknown as ReturnType<typeof vi.fn>
    renderProvider()

    await act(async () => {
      await flushBootstrapPromises()
      vi.advanceTimersByTime(2_000)
      await flushBootstrapPromises()
    })

    expect(screen.getByTestId("conversation-count")).toHaveTextContent("1")
    fetcher.mockClear()

    await act(async () => {
      screen.getByRole("button", { name: "open direct" }).click()
      await flushBootstrapPromises()
    })

    expect(fetcher).toHaveBeenCalledWith("/api/client/conversations/direct", {
      body: JSON.stringify({
        user_id: "user-3",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
    expect(screen.getByTestId("conversation-count")).toHaveTextContent("2")
    expect(screen.getByTestId("first-conversation-name")).toHaveTextContent(
      "Carol Wang"
    )
  })

  it("uses the shared button component for bootstrap retry", async () => {
    vi.stubGlobal("fetch", createClientDataErrorFetchMock())
    renderProvider()

    expect(
      await screen.findByText("工作区加载失败", undefined, {
        timeout: 3_000,
      })
    ).toBeInTheDocument()
    const retryButton = screen.getByRole("button", { name: "重试" })

    expect(retryButton).toHaveAttribute("data-slot", "button")
    expect(retryButton).toHaveAttribute("data-variant", "outline")
  })
})
