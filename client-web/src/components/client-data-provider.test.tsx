import { act, render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ClientDataProvider } from "@/components/client-data-provider"
import { useClientData } from "@/lib/client-data-context"

describe("ClientDataProvider", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it("refreshes client data on the 15 second refresh interval", async () => {
    vi.useFakeTimers()

    let meRequestCount = 0
    let contactsRequestCount = 0
    let conversationRequestCount = 0
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)

      if (url === "/api/client/me") {
        meRequestCount += 1
        return Promise.resolve(jsonResponse(createCurrentUserResponse()))
      }

      if (url === "/api/client/contacts") {
        contactsRequestCount += 1
        return Promise.resolve(jsonResponse(createContactsResponse()))
      }

      if (url === "/api/client/conversations") {
        conversationRequestCount += 1

        return Promise.resolve(
          jsonResponse(
            createConversationsResponse(
              conversationRequestCount === 1
                ? [createConversationResponse("conversation-1")]
                : [
                    createConversationResponse("conversation-1"),
                    createConversationResponse("conversation-2"),
                  ]
            )
          )
        )
      }

      if (url === "/api/client/projects?limit=100") {
        return Promise.resolve(jsonResponse(createProjectsResponse()))
      }

      return Promise.reject(new Error(`unexpected request: ${url}`))
    })

    vi.stubGlobal("fetch", fetchMock)

    render(
      <MemoryRouter>
        <ClientDataProvider>
          <ConversationCount />
        </ClientDataProvider>
      </MemoryRouter>
    )

    await act(async () => undefined)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000)
    })

    expect(screen.getByTestId("conversation-count")).toHaveTextContent("1")
    expect(meRequestCount).toBe(1)
    expect(contactsRequestCount).toBe(1)
    expect(conversationRequestCount).toBe(1)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000)
    })

    expect(screen.getByTestId("conversation-count")).toHaveTextContent("2")
    expect(meRequestCount).toBe(2)
    expect(contactsRequestCount).toBe(2)
    expect(conversationRequestCount).toBe(2)
  })
})

function ConversationCount() {
  const { conversations } = useClientData()

  return <div data-testid="conversation-count">{conversations.length}</div>
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: {
      "Content-Type": "application/json",
    },
    status: 200,
  })
}

function createCurrentUserResponse() {
  return {
    data: {
      user: {
        created_at: "2026-07-09T00:00:00Z",
        email: "me@example.com",
        id: "user-1",
        name: "Me",
      },
    },
    success: true,
  }
}

function createContactsResponse() {
  return {
    data: {
      apps: [],
      groups: [],
      users: [],
    },
    success: true,
  }
}

function createConversationsResponse(conversations: unknown[]) {
  return {
    data: {
      conversations,
    },
    success: true,
  }
}

function createProjectsResponse() {
  return {
    data: {
      next_cursor: null,
      personal_project: {
        avatar: "",
        created_at: "2026-07-09T00:00:00Z",
        current_user_role: "owner",
        description: "",
        group_count: 0,
        id: "personal-project-1",
        is_personal: true,
        member_count: 1,
        name: "个人工作区",
        owner: {
          avatar: "",
          id: "user-1",
          name: "Me",
          nickname: "",
        },
        task_counts: {
          canceled: 0,
          done: 0,
          in_progress: 0,
          todo: 0,
          total: 0,
        },
        updated_at: "2026-07-09T00:00:00Z",
      },
      projects: [],
    },
    success: true,
  }
}

function createConversationResponse(id: string) {
  return {
    created_at: "2026-07-09T00:00:00Z",
    id,
    name: id,
    type: "direct",
  }
}
