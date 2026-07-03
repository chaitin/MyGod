import { fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, useLocation } from "react-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"

import App from "./App"

const rememberedCredentialsKey = "client-web:remembered-login"

function LocationProbe() {
  const location = useLocation()

  return (
    <>
      <span data-testid="location">{location.pathname}</span>
      <span data-testid="location-search">{location.search}</span>
    </>
  )
}

function renderApp(path = "/login") {
  return render(
    <ThemeProvider disableTransitionOnChange={false}>
      <MemoryRouter initialEntries={[path]}>
        <App />
        <Toaster position="top-center" />
        <LocationProbe />
      </MemoryRouter>
    </ThemeProvider>
  )
}

function createDirectConversationResponse() {
  return new Response(
    JSON.stringify({
      success: true,
      data: {
        conversation: {
          avatar: "/assets/avatars/builtin/03.webp",
          created_at: "2026-07-03T07:00:00Z",
          id: "conversation-bob",
          last_message_at: "2026-07-03T08:00:00Z",
          last_message_id: "message-1",
          last_message_seq: 12,
          last_message_summary: "好的，我看一下",
          member_count: 2,
          name: "Bob Li",
          type: "direct",
        },
        created: false,
      },
    }),
    {
      headers: {
        "content-type": "application/json",
      },
      status: 200,
    }
  )
}

function createConversationMessage({
  clientMessageId,
  content,
  conversationId,
  createdAt,
  id,
  senderId,
  seq,
}: {
  clientMessageId: string
  content: string
  conversationId: string
  createdAt: string
  id: string
  senderId: string
  seq: number
}) {
  return {
    id,
    conversation_id: conversationId,
    seq,
    sender: {
      type: "user",
      id: senderId,
    },
    body: {
      type: "text",
      content,
    },
    client_message_id: clientMessageId,
    created_at: createdAt,
  }
}

function createConversationMessagesResponse({
  hasMoreBefore = false,
  messages,
}: {
  conversationId?: string
  hasMoreBefore?: boolean
  messages: ReturnType<typeof createConversationMessage>[]
}) {
  const seqs = messages.map((message) => message.seq)

  return new Response(
    JSON.stringify({
      success: true,
      data: {
        messages,
        page: {
          limit: 20,
          oldest_seq: seqs.length > 0 ? Math.min(...seqs) : 0,
          newest_seq: seqs.length > 0 ? Math.max(...seqs) : 0,
          has_more_before: hasMoreBefore,
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
}

function createDefaultConversationMessagesResponse(conversationId: string) {
  if (conversationId === "conversation-bob") {
    return createConversationMessagesResponse({
      conversationId,
      messages: [
        createConversationMessage({
          clientMessageId: "client-message-12",
          content: "好的，我看一下",
          conversationId,
          createdAt: "2026-07-03T08:00:00Z",
          id: "message-12",
          senderId: "user-2",
          seq: 12,
        }),
      ],
    })
  }

  if (conversationId === "conversation-team") {
    return createConversationMessagesResponse({
      conversationId,
      messages: [
        createConversationMessage({
          clientMessageId: "client-message-3",
          content: "今天下午同步",
          conversationId,
          createdAt: "2026-07-03T07:30:00Z",
          id: "message-3",
          senderId: "user-2",
          seq: 3,
        }),
      ],
    })
  }

  return createConversationMessagesResponse({
    conversationId,
    messages: [],
  })
}

function createSendMessageResponse({
  clientMessageId = "client-message-13",
  content = "帮我总结今天的消息",
  conversationId = "conversation-bob",
}: {
  clientMessageId?: string
  content?: string
  conversationId?: string
} = {}) {
  return new Response(
    JSON.stringify({
      success: true,
      data: {
        message: createConversationMessage({
          clientMessageId,
          content,
          conversationId,
          createdAt: "2026-07-03T08:01:00Z",
          id: "message-13",
          senderId: "user-1",
          seq: 13,
        }),
      },
    }),
    {
      headers: {
        "content-type": "application/json",
      },
      status: 201,
    }
  )
}

type ConversationMessagesHandler = (
  conversationId: string,
  url: URL,
  init?: RequestInit
) => Promise<Response> | Response

function createClientFetchMock({
  conversationMessagesHandler,
  currentUserAvatar = "/assets/avatars/builtin/17.webp",
  currentUserNickname = "Al",
  directConversationResponse,
  logoutResponse,
  loginStatus = 200,
  logoutStatus = 200,
  sendMessageResponse,
}: {
  conversationMessagesHandler?: ConversationMessagesHandler
  currentUserAvatar?: string
  currentUserNickname?: string
  directConversationResponse?: Promise<Response>
  logoutResponse?: Promise<Response>
  loginStatus?: 200 | 401
  logoutStatus?: 200 | 500
  sendMessageResponse?: Promise<Response>
} = {}) {
  let currentAvatar = currentUserAvatar
  let currentNickname = currentUserNickname

  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input)

    if (path === "/api/client/info") {
      return new Response(
        JSON.stringify({
          success: true,
          data: {
            app_name: "星环协作",
            organization_name: "长亭科技",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    }

    if (path === "/api/client/auth/login") {
      if (loginStatus === 401) {
        return new Response(
          JSON.stringify({
            success: false,
            error: {
              code: "invalid_credentials",
              message: "邮箱或密码错误",
            },
          }),
          {
            headers: {
              "content-type": "application/json",
            },
            status: 401,
          }
        )
      }

      return new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              created_at: "2026-07-01T00:00:00Z",
              email: "alice@example.com",
              id: "user-1",
              name: "Alice",
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
    }

    if (path === "/api/client/auth/logout") {
      if (logoutResponse) {
        return logoutResponse
      }

      if (logoutStatus === 500) {
        return new Response(
          JSON.stringify({
            success: false,
            error: {
              code: "internal_error",
              message: "退出登录失败",
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

      return new Response(
        JSON.stringify({
          success: true,
          data: {},
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    }

    if (path === "/api/client/me") {
      if (init?.method === "PATCH") {
        const body = JSON.parse(String(init.body ?? "{}")) as {
          avatar?: string
          nickname?: string
        }

        if (body.avatar) {
          currentAvatar = body.avatar
        }
        if (body.nickname !== undefined) {
          currentNickname = body.nickname
        }

        return new Response(
          JSON.stringify({
            success: true,
            data: {
              user: {
                avatar: currentAvatar,
                created_at: "2026-07-01T00:00:00Z",
                email: "alice@example.com",
                id: "user-1",
                name: "Alice",
                nickname: currentNickname,
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
      }

      return new Response(
        JSON.stringify({
          success: true,
          data: {
            user: {
              avatar: currentAvatar,
              created_at: "2026-07-01T00:00:00Z",
              email: "alice@example.com",
              id: "user-1",
              name: "Alice",
              nickname: currentNickname,
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
    }

    if (path === "/api/client/contacts/users") {
      return new Response(
        JSON.stringify({
          success: true,
          data: {
            contacts: [
              {
                avatar: "/assets/avatars/builtin/17.webp",
                email: "alice@example.com",
                id: "user-1",
                last_online_at: null,
                name: "Alice",
                nickname: "Al",
                online: true,
                phone: "+8613912345678",
                type: "user",
              },
              {
                avatar: "/assets/avatars/builtin/03.webp",
                email: "bob@example.com",
                id: "user-2",
                last_online_at: "2026-07-03T01:00:00Z",
                name: "Bob Li",
                nickname: "",
                online: false,
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
    }

    if (path === "/api/client/conversations") {
      return new Response(
        JSON.stringify({
          success: true,
          data: {
            conversations: [
              {
                avatar: "/assets/avatars/builtin/03.webp",
                created_at: "2026-07-03T07:00:00Z",
                id: "conversation-bob",
                last_message_at: "2026-07-03T08:00:00Z",
                last_message_id: "message-1",
                last_message_seq: 12,
                last_message_summary: "好的，我看一下",
                member_count: 2,
                name: "Bob Li",
                type: "direct",
              },
              {
                avatar: "",
                created_at: "2026-07-03T06:00:00Z",
                id: "conversation-team",
                last_message_at: "2026-07-02T07:30:00Z",
                last_message_id: "message-2",
                last_message_seq: 3,
                last_message_summary: "今天下午同步",
                member_count: 3,
                name: "产品讨论组",
                type: "group",
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
    }

    const messagesMatch = path.match(
      /^\/api\/client\/conversations\/([^/]+)\/messages(?:\?.*)?$/
    )
    if (messagesMatch) {
      const conversationId = decodeURIComponent(messagesMatch[1])

      if (init?.method === "POST") {
        if (sendMessageResponse) {
          return sendMessageResponse
        }

        const body = JSON.parse(String(init.body ?? "{}")) as {
          body?: {
            content?: string
          }
          client_message_id?: string
        }

        return createSendMessageResponse({
          clientMessageId: body.client_message_id,
          content: body.body?.content,
          conversationId,
        })
      }

      const url = new URL(path, "http://localhost")

      if (conversationMessagesHandler) {
        return conversationMessagesHandler(conversationId, url, init)
      }

      return createDefaultConversationMessagesResponse(conversationId)
    }

    if (
      path === "/api/client/conversations/direct" &&
      init?.method === "POST"
    ) {
      if (directConversationResponse) {
        return directConversationResponse
      }

      const body = JSON.parse(String(init.body ?? "{}")) as {
        user_id?: string
      }

      if (body.user_id === "user-2") {
        return new Response(
          JSON.stringify({
            success: true,
            data: {
              conversation: {
                avatar: "/assets/avatars/builtin/03.webp",
                created_at: "2026-07-03T07:00:00Z",
                id: "conversation-bob",
                last_message_at: "2026-07-03T08:00:00Z",
                last_message_id: "message-1",
                last_message_seq: 12,
                last_message_summary: "好的，我看一下",
                member_count: 2,
                name: "Bob Li",
                type: "direct",
              },
              created: false,
            },
          }),
          {
            headers: {
              "content-type": "application/json",
            },
            status: 200,
          }
        )
      }

      return new Response(
        JSON.stringify({
          success: false,
          error: {
            code: "invalid_request",
            message: "不能和自己创建私聊",
          },
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 400,
        }
      )
    }

    return new Response(
      JSON.stringify({
        success: false,
        error: {
          code: "not_found",
          message: "未找到接口",
        },
      }),
      {
        headers: {
          "content-type": "application/json",
        },
        status: 404,
      }
    )
  })
}

class LoadedImage {
  complete = true
  crossOrigin: string | null = null
  naturalWidth = 1
  referrerPolicy = ""
  src = ""

  addEventListener() {}
  removeEventListener() {}
}

class AppWebSocketMock {
  static CLOSED = 3
  static CLOSING = 2
  static CONNECTING = 0
  static OPEN = 1
  static instances: AppWebSocketMock[] = []

  closeCount = 0
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onopen: ((event: Event) => void) | null = null
  readyState: number = AppWebSocketMock.CONNECTING
  sent: string[] = []
  url: string

  constructor(url: string) {
    this.url = url
    AppWebSocketMock.instances.push(this)
  }

  close() {
    if (this.readyState === AppWebSocketMock.CLOSED) {
      return
    }
    this.closeCount += 1
    this.readyState = AppWebSocketMock.CLOSED
    this.onclose?.(new CloseEvent("close", { code: 1000 }))
  }

  failClose(code = 1006) {
    if (this.readyState === AppWebSocketMock.CLOSED) {
      return
    }
    this.readyState = AppWebSocketMock.CLOSED
    this.onclose?.(new CloseEvent("close", { code }))
  }

  open() {
    this.readyState = AppWebSocketMock.OPEN
    this.onopen?.(new Event("open"))
  }

  receive(payload: unknown) {
    this.onmessage?.(
      new MessageEvent("message", {
        data: JSON.stringify(payload),
      })
    )
  }

  send(data: string) {
    this.sent.push(data)
  }
}

async function openLatestAppWebSocket(
  afterCount = 0,
  options: { ready?: boolean } = {}
) {
  await waitFor(
    () => expect(AppWebSocketMock.instances.length).toBeGreaterThan(afterCount),
    { timeout: 4_000 }
  )
  const socket =
    AppWebSocketMock.instances[AppWebSocketMock.instances.length - 1]
  socket.open()
  if (options.ready ?? true) {
    socket.receive({
      v: 1,
      kind: "event",
      event: "system.ready",
      payload: {},
    })
  }

  return socket
}

describe("App", () => {
  beforeEach(() => {
    AppWebSocketMock.instances = []
    vi.stubGlobal("fetch", createClientFetchMock())
    vi.stubGlobal("WebSocket", AppWebSocketMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    window.localStorage.clear()
    document.documentElement.classList.remove("dark", "light")
  })

  it("在 /login 渲染登录页", async () => {
    renderApp("/login")

    const loginTitle = await screen.findByRole("heading", {
      level: 1,
      name: "星环协作 智能协作平台",
    })
    const loginSubtitle = screen.getByText("登录到长亭科技的工作空间")
    const loginCard = screen
      .getByPlaceholderText("输入账号")
      .closest("[data-slot='card']")

    expect(loginSubtitle).toBeInTheDocument()
    expect(loginTitle.nextElementSibling).toContainElement(loginSubtitle)
    expect(loginTitle.nextElementSibling).toHaveClass("text-muted-foreground")
    expect(
      loginTitle.nextElementSibling?.querySelector(".lucide-move-right")
    ).toBeInTheDocument()
    expect(loginCard).toBeInTheDocument()
    expect(loginCard?.querySelector("[data-slot='card-title']")).toBeNull()
    expect(loginCard).not.toHaveTextContent("登录到长亭科技的工作空间")
    expect(
      screen.queryByText("使用管理员分配的企业账号登录。")
    ).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText("输入账号")).toBeInTheDocument()
    expect(
      screen.queryByPlaceholderText("请输入企业账号")
    ).not.toBeInTheDocument()
    expect(screen.getByTestId("location")).toHaveTextContent("/login")
    await waitFor(() => expect(document.title).toBe("登录 - 星环协作"))
  })

  it("登录后进入聊天页时不默认选中会话，点击后再显示聊天区", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const user = userEvent.setup()

    renderApp("/login")

    await screen.findByText("星环协作 智能协作平台")
    await user.type(screen.getByLabelText("账号"), "alice@example.com")
    await user.type(screen.getByLabelText("密码"), "password")
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    )
    await openLatestAppWebSocket()
    expect(fetch).toHaveBeenCalledWith("/api/client/auth/login", {
      body: JSON.stringify({
        email: "alice@example.com",
        password: "password",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "POST",
    })
    expect(
      await screen.findByRole(
        "navigation",
        { name: "主导航" },
        { timeout: 4_000 }
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole("navigation", { name: "主导航" }).parentElement
    ).toHaveClass("bg-sidebar")
    const sidebarUserAvatar = await screen.findByRole("img", { name: "Al" })
    const userMenuTrigger = screen.getByRole("button", { name: "用户菜单" })
    expect(userMenuTrigger).toHaveAttribute("data-variant", "ghost")
    expect(userMenuTrigger).toHaveAttribute("data-size", "icon-sm")
    expect(sidebarUserAvatar).toHaveAttribute(
      "src",
      "/assets/avatars/builtin/17.webp"
    )
    expect(sidebarUserAvatar.parentElement).toHaveClass("bg-muted")
    expect(sidebarUserAvatar.parentElement).toHaveClass(
      "group-hover/avatar-trigger:bg-background",
      "group-hover/avatar-trigger:after:border-ring",
      "group-data-[state=open]/avatar-trigger:bg-background",
      "group-data-[state=open]/avatar-trigger:after:border-ring"
    )
    expect(sidebarUserAvatar.parentElement).not.toHaveClass(
      "group-hover/avatar-trigger:bg-white",
      "group-hover/avatar-trigger:after:border-primary"
    )
    expect(screen.getByRole("link", { name: "聊天" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(screen.getByRole("link", { name: "通讯录" })).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "任务" })).toBeInTheDocument()
    expect(
      await screen.findByRole("heading", { name: "消息" })
    ).toBeInTheDocument()
    const createAgentButton = screen.getByRole("button", { name: "新建 Agent" })

    expect(createAgentButton).toHaveAttribute("data-size", "icon-sm")
    expect(createAgentButton).toHaveAttribute("data-variant", "ghost")
    expect(createAgentButton).toHaveTextContent("")
    expect(createAgentButton.querySelector(".lucide-plus")).toBeInTheDocument()
    await user.click(createAgentButton)
    const createGroupChatItem = await screen.findByRole("menuitem", {
      name: "发起群聊",
    })

    expect(createGroupChatItem).toBeInTheDocument()
    await user.click(createGroupChatItem)
    const bobConversationItem = screen.getByRole("button", {
      name: /Bob Li/,
    })
    expect(bobConversationItem).toBeInTheDocument()
    expect(bobConversationItem).toHaveAttribute("data-slot", "item")
    expect(bobConversationItem).toHaveAttribute("data-size", "sm")
    expect(bobConversationItem).not.toHaveClass("bg-primary/10")
    const bobConversationTime = within(bobConversationItem).getByText("16:00")
    expect(bobConversationTime).toBeInTheDocument()
    expect(bobConversationTime).toHaveClass("pr-2")
    expect(
      within(bobConversationItem).getByText("好的，我看一下")
    ).toBeInTheDocument()
    const teamConversationItem = screen.getByRole("button", {
        name: /产品讨论组/,
      })
    expect(teamConversationItem).toBeInTheDocument()
    expect(
      within(teamConversationItem).getByText("07-02")
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("heading", { name: "Bob Li" })
    ).not.toBeInTheDocument()
    expect(screen.queryByText("选择会话")).not.toBeInTheDocument()
    expect(screen.queryByText("从左侧选择一个会话")).not.toBeInTheDocument()
    expect(screen.getByTestId("chat-detail-shell")).toHaveClass("bg-muted")
    expect(screen.getByTestId("chat-detail-shell")).not.toHaveClass(
      "bg-background"
    )
    const chatEmptyState = screen.getByTestId("chat-empty-state")
    expect(chatEmptyState).toHaveTextContent("选择一个会话开始聊天")
    expect(chatEmptyState).toHaveClass(
      "flex-1",
      "items-center",
      "justify-center"
    )
    expect(
      screen.queryByPlaceholderText("输入消息，Enter 发送")
    ).not.toBeInTheDocument()

    await user.click(bobConversationItem)

    expect(screen.getByTestId("chat-detail-shell")).toHaveClass("bg-background")
    expect(screen.getByTestId("chat-detail-shell")).not.toHaveClass("bg-muted")
    expect(screen.queryByTestId("chat-empty-state")).not.toBeInTheDocument()
    expect(screen.getByRole("heading", { name: "Bob Li" })).toBeInTheDocument()
    expect(screen.getByTestId("conversation-panel-header")).toContainElement(
      screen.getByRole("heading", { name: "Bob Li" })
    )
    expect(
      screen.queryByTestId("conversation-history-empty")
    ).not.toBeInTheDocument()
    const initialHistory = await screen.findByTestId(
      "conversation-panel-history"
    )
    expect(initialHistory).toHaveAttribute("data-slot", "scroll-area")
    expect(initialHistory).toHaveClass("bg-muted/30")
    expect(
      within(initialHistory).getByText("好的，我看一下")
    ).toBeInTheDocument()
    const composer = screen.getByTestId("conversation-panel-composer")
    const composerContent = screen.getByTestId(
      "conversation-panel-composer-content"
    )
    const editorRow = screen.getByTestId("conversation-panel-editor-row")
    const toolbarRow = screen.getByTestId("conversation-panel-toolbar-row")

    expect(composer).toContainElement(composerContent)
    expect(composerContent).toHaveClass("w-full")
    expect(composerContent).not.toHaveClass("max-w-4xl", "mx-auto")
    expect(composer).toContainElement(editorRow)
    expect(composer).toContainElement(toolbarRow)
    expect(editorRow).toContainElement(screen.getByPlaceholderText("输入消息"))
    expect(toolbarRow).toContainElement(
      screen.getByRole("button", { name: "选择表情" })
    )
    expect(toolbarRow).toContainElement(
      screen.getByRole("button", { name: "上传文件" })
    )
    expect(toolbarRow).toContainElement(
      screen.getByRole("button", { name: "插入图片" })
    )
    expect(toolbarRow).toContainElement(
      screen.getByRole("button", { name: "发送消息" })
    )
    expect(screen.getByRole("button", { name: "发送消息" })).toHaveTextContent(
      "发送"
    )
    expect(screen.getByRole("button", { name: "选择表情" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "上传文件" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "插入图片" })).toBeDisabled()
    expect(screen.getByPlaceholderText("输入消息")).toBeEnabled()

    await user.type(
      screen.getByPlaceholderText("输入消息"),
      "帮我总结今天的消息"
    )
    await user.click(screen.getByRole("button", { name: "发送消息" }))

    expect(
      screen.queryByTestId("conversation-history-empty")
    ).not.toBeInTheDocument()
    const history = await screen.findByTestId("conversation-panel-history")
    expect(history).toHaveAttribute("data-slot", "scroll-area")
    expect(history).toHaveClass("bg-muted/30")
    expect(
      within(history).getByText("帮我总结今天的消息")
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/收到，我会先作为你的内置助手/)
    ).not.toBeInTheDocument()
  }, 10_000)

  it("当前用户没有头像时侧栏头像占位使用 muted 背景", async () => {
    vi.stubGlobal(
      "fetch",
      createClientFetchMock({
        currentUserAvatar: "",
      })
    )

    renderApp("/chat")

    await openLatestAppWebSocket()
    await screen.findByRole(
      "navigation",
      { name: "主导航" },
      { timeout: 4_000 }
    )
    const fallback = screen.getByText("A")

    expect(fallback).toHaveAttribute("data-slot", "avatar-fallback")
    expect(fallback).toHaveClass("bg-muted", "text-muted-foreground")
    expect(fallback).not.toHaveClass("bg-primary", "text-primary-foreground")
  }, 10_000)

  it("确认退出时按钮保持文案并显示旋转图标", async () => {
    vi.stubGlobal("Image", LoadedImage)
    let resolveLogout!: (response: Response) => void
    const logoutResponse = new Promise<Response>((resolve) => {
      resolveLogout = resolve
    })
    vi.stubGlobal("fetch", createClientFetchMock({ logoutResponse }))
    const user = userEvent.setup()

    renderApp("/chat")

    await openLatestAppWebSocket()
    await user.click(
      await screen.findByRole(
        "button",
        { name: "用户菜单" },
        { timeout: 4_000 }
      )
    )
    await user.click(await screen.findByRole("menuitem", { name: "退出登录" }))

    const confirmDialog = await screen.findByRole("alertdialog", {
      name: "确认退出登录",
    })
    await user.click(
      within(confirmDialog).getByRole("button", { name: "退出登录" })
    )

    const logoutButton = within(confirmDialog).getByRole("button", {
      name: "退出登录",
    })

    expect(logoutButton).toBeDisabled()
    expect(
      within(confirmDialog).queryByText("退出中...")
    ).not.toBeInTheDocument()
    expect(logoutButton.querySelector(".animate-spin")).toBeInTheDocument()

    resolveLogout(
      new Response(
        JSON.stringify({
          success: true,
          data: {},
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        }
      )
    )

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/login")
    )
  }, 10_000)

  it("点击侧栏头像菜单可以退出登录", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const user = userEvent.setup()

    renderApp("/chat")

    const realtimeSocket = await openLatestAppWebSocket()
    const userMenuButton = await screen.findByRole(
      "button",
      { name: "用户菜单" },
      { timeout: 4_000 }
    )
    expect(userMenuButton).toHaveClass(
      "bg-muted",
      "hover:bg-background",
      "data-[state=open]:bg-background"
    )
    expect(userMenuButton).not.toHaveClass("hover:bg-white")

    await user.click(userMenuButton)
    await user.click(await screen.findByRole("menuitem", { name: "退出登录" }))

    const confirmDialog = await screen.findByRole("alertdialog", {
      name: "确认退出登录",
    })
    expect(
      within(confirmDialog).getByText("当前会话将结束，你可以稍后重新登录。")
    ).toBeInTheDocument()
    expect(
      within(confirmDialog).queryByText(
        "退出后需要重新登录才能继续使用当前工作空间。"
      )
    ).not.toBeInTheDocument()
    expect(fetch).not.toHaveBeenCalledWith("/api/client/auth/logout", {
      credentials: "include",
      method: "POST",
    })

    await user.click(
      within(confirmDialog).getByRole("button", { name: "取消" })
    )
    expect(
      screen.queryByRole("alertdialog", { name: "确认退出登录" })
    ).not.toBeInTheDocument()
    expect(fetch).not.toHaveBeenCalledWith("/api/client/auth/logout", {
      credentials: "include",
      method: "POST",
    })
    expect(screen.getByTestId("location")).toHaveTextContent("/chat")

    await user.click(userMenuButton)
    await user.click(await screen.findByRole("menuitem", { name: "退出登录" }))
    await user.click(
      within(
        await screen.findByRole("alertdialog", {
          name: "确认退出登录",
        })
      ).getByRole("button", { name: "退出登录" })
    )

    expect(fetch).toHaveBeenCalledWith("/api/client/auth/logout", {
      credentials: "include",
      method: "POST",
    })
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/login")
    )
    expect(realtimeSocket.closeCount).toBe(1)
  }, 10_000)

  it("点击侧栏头像菜单可以打开设置对话框", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const user = userEvent.setup()

    renderApp("/chat")

    await openLatestAppWebSocket()
    const userMenuButton = await screen.findByRole(
      "button",
      { name: "用户菜单" },
      { timeout: 4_000 }
    )

    await user.click(userMenuButton)
    const userMenu = await screen.findByRole("menu")
    const userSummary = within(userMenu).getByRole("group", {
      name: "用户信息",
    })

    expect(userSummary).toHaveClass(
      "grid",
      "grid-cols-[3rem_minmax(0,1fr)]",
      "items-center",
      "gap-x-3",
      "px-2",
      "py-3"
    )
    expect(userSummary).not.toHaveClass("m-1", "bg-muted/60")
    const menuAvatar = within(userSummary).getByRole("img", { name: "Al" })
    expect(menuAvatar).toHaveAttribute("src", "/assets/avatars/builtin/17.webp")
    expect(menuAvatar.parentElement).toHaveClass("row-span-2", "size-12")

    expect(
      within(userSummary).getByRole("group", { name: "姓名信息" })
    ).toHaveTextContent("Al")
    expect(
      within(userSummary).getByRole("group", { name: "联系方式" })
    ).toHaveTextContent("alice@example.com")
    expect(within(userSummary).queryByText("Alice")).not.toBeInTheDocument()
    expect(
      within(userSummary).queryByText("+8613912345678")
    ).not.toBeInTheDocument()
    expect(within(userSummary).queryByText("昵称")).not.toBeInTheDocument()
    expect(within(userSummary).queryByText("手机号")).not.toBeInTheDocument()
    expect(userSummary.nextElementSibling).toHaveAttribute(
      "data-slot",
      "dropdown-menu-item"
    )
    expect(userSummary.nextElementSibling).toHaveTextContent("设置")

    await user.click(await screen.findByRole("menuitem", { name: "设置" }))

    let dialog = await screen.findByRole("dialog", { name: "设置" })
    const getMeRequestCount = () =>
      (fetch as ReturnType<typeof vi.fn>).mock.calls.filter(
        ([input, init]) =>
          String(input) === "/api/client/me" &&
          (!init || (init as RequestInit).method === "GET")
      ).length

    expect(within(dialog).queryByText("Alice")).not.toBeInTheDocument()
    expect(
      within(dialog).queryByText("alice@example.com")
    ).not.toBeInTheDocument()
    expect(within(dialog).getByLabelText("姓名")).toHaveValue("Alice")
    expect(within(dialog).getByLabelText("邮箱")).toHaveValue(
      "alice@example.com"
    )
    expect(within(dialog).getByLabelText("昵称")).toHaveValue("Al")
    expect(
      within(dialog).getByRole("button", { name: "更换头像" })
    ).toBeInTheDocument()
    expect(
      within(dialog)
        .getByRole("button", { name: "更换头像" })
        .querySelector(".lucide-camera")
    ).toBeInTheDocument()
    expect(
      within(dialog).queryAllByRole("button", { name: /选择头像/ })
    ).toHaveLength(0)

    await user.clear(within(dialog).getByLabelText("昵称"))
    await user.type(within(dialog).getByLabelText("昵称"), "Alice A")
    await user.click(within(dialog).getByRole("button", { name: "提交" }))

    expect(fetch).toHaveBeenCalledWith("/api/client/me", {
      body: JSON.stringify({
        nickname: "Alice A",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    })
    await waitFor(() => {
      expect(getMeRequestCount()).toBeGreaterThanOrEqual(2)
    })
    expect(await screen.findByText("昵称已保存")).toBeInTheDocument()
    dialog = await screen.findByRole("dialog", { name: "设置" })
    expect(within(dialog).getByLabelText("昵称")).toHaveValue("Alice A")

    await user.click(within(dialog).getByRole("button", { name: "更换头像" }))

    const avatarDialog = await screen.findByRole("dialog", {
      name: "选择头像",
    })
    expect(
      within(avatarDialog).getAllByRole("button", { name: /选择头像/ })
    ).toHaveLength(64)
    expect(
      within(avatarDialog).getByRole("button", { name: "选择头像 17" })
    ).toHaveAttribute("aria-pressed", "true")

    await user.click(
      within(avatarDialog).getByRole("button", { name: "选择头像 03" })
    )

    expect(within(dialog).getByLabelText("昵称")).toHaveValue("Alice A")
    expect(screen.getByRole("dialog", { name: "选择头像" })).toBeInTheDocument()
    expect(
      within(avatarDialog).getByRole("button", { name: "选择头像 03" })
    ).toHaveAttribute("aria-pressed", "true")
    expect(
      within(dialog).getByRole("img", { hidden: true, name: "Alice A" })
    ).toHaveAttribute("src", "/assets/avatars/builtin/17.webp")

    await user.click(within(avatarDialog).getByRole("button", { name: "保存" }))

    expect(fetch).toHaveBeenCalledWith("/api/client/me", {
      body: JSON.stringify({
        avatar: "/assets/avatars/builtin/03.webp",
      }),
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    })
    await waitFor(() => {
      expect(getMeRequestCount()).toBeGreaterThanOrEqual(3)
    })
    expect(await screen.findByText("头像已保存")).toBeInTheDocument()
    expect(
      screen.queryByRole("dialog", { name: "选择头像" })
    ).not.toBeInTheDocument()
    dialog = await screen.findByRole("dialog", { name: "设置" })
    expect(
      within(dialog).getByRole("img", { name: "Alice A" })
    ).toHaveAttribute("src", "/assets/avatars/builtin/03.webp")
  }, 10_000)

  it("侧栏头像菜单没有昵称时用姓名作为显示名", async () => {
    vi.stubGlobal("Image", LoadedImage)
    vi.stubGlobal(
      "fetch",
      createClientFetchMock({
        currentUserNickname: "",
      })
    )
    const user = userEvent.setup()

    renderApp("/chat")

    await openLatestAppWebSocket()
    const userMenuButton = await screen.findByRole(
      "button",
      { name: "用户菜单" },
      { timeout: 4_000 }
    )

    await user.click(userMenuButton)
    const userSummary = within(await screen.findByRole("menu")).getByRole(
      "group",
      {
        name: "用户信息",
      }
    )

    const menuAvatar = within(userSummary).getByRole("img", { name: "Alice" })
    expect(menuAvatar).toHaveAttribute("src", "/assets/avatars/builtin/17.webp")
    expect(
      within(userSummary).getByRole("group", { name: "姓名信息" })
    ).toHaveTextContent("Alice")
    expect(
      within(userSummary).getByRole("group", { name: "姓名信息" })
    ).not.toHaveTextContent("Alice | Alice")
    expect(
      within(userSummary).getByRole("group", { name: "联系方式" })
    ).toHaveTextContent("alice@example.com")
    expect(
      within(userSummary).queryByText("+8613912345678")
    ).not.toBeInTheDocument()
    expect(within(userSummary).queryByText("未设置")).not.toBeInTheDocument()
  }, 10_000)

  it("登录失败时用顶部居中的 toast 展示后端错误", async () => {
    vi.stubGlobal("fetch", createClientFetchMock({ loginStatus: 401 }))
    const user = userEvent.setup()

    renderApp("/login")

    await screen.findByText("星环协作 智能协作平台")
    await user.type(screen.getByLabelText("账号"), "alice@example.com")
    await user.type(screen.getByLabelText("密码"), "wrong")
    await user.click(screen.getByRole("button", { name: "登录" }))

    expect(screen.getByTestId("location")).toHaveTextContent("/login")
    expect(await screen.findByText("邮箱或密码错误")).toBeInTheDocument()
  })

  it("登录成功后记住账号密码并在下次打开登录页时回填", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const user = userEvent.setup()

    const { unmount } = renderApp("/login")

    await screen.findByText("星环协作 智能协作平台")
    await user.type(screen.getByLabelText("账号"), "alice@example.com")
    await user.type(screen.getByLabelText("密码"), "password")
    expect(
      screen.getByRole("checkbox", { name: "记住账号密码" })
    ).toHaveAttribute("data-slot", "checkbox")
    expect(screen.getByText("记住账号密码")).toHaveAttribute(
      "data-slot",
      "label"
    )
    await user.click(screen.getByRole("checkbox", { name: "记住账号密码" }))
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    )
    expect(window.localStorage.getItem(rememberedCredentialsKey)).toBe(
      JSON.stringify({
        account: "alice@example.com",
        password: "password",
      })
    )

    unmount()
    renderApp("/login")

    await screen.findByText("星环协作 智能协作平台")
    expect(screen.getByLabelText("账号")).toHaveValue("alice@example.com")
    expect(screen.getByLabelText("密码")).toHaveValue("password")
    expect(screen.getByRole("checkbox", { name: "记住账号密码" })).toBeChecked()
  }, 10_000)

  it("聊天、通讯录和任务是独立路由页面", async () => {
    const user = userEvent.setup()

    renderApp("/chat")

    await openLatestAppWebSocket()
    expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    expect(
      await screen.findByRole("heading", { name: "消息" }, { timeout: 4_000 })
    ).toBeInTheDocument()
    await waitFor(() => expect(document.title).toBe("聊天 - 星环协作"))
    expect(screen.getByRole("link", { name: "聊天" })).toHaveClass(
      "bg-primary",
      "text-primary-foreground"
    )
    for (const label of ["聊天", "通讯录", "任务"]) {
      const navLink = screen.getByRole("link", { name: label })

      expect(navLink).toHaveClass("rounded-full")
      expect(navLink).not.toHaveClass("rounded-md")
    }
    expect(
      screen.getByRole("link", { name: "聊天" }).querySelector("svg")
    ).toHaveClass("lucide-message-circle-more")
    expect(
      screen.getByRole("link", { name: "通讯录" }).querySelector("svg")
    ).toHaveClass("lucide-circle-user-round")
    expect(
      screen.getByRole("link", { name: "任务" }).querySelector("svg")
    ).toHaveClass("lucide-circle-check-big")
    expect(
      screen.getByRole("link", { name: "聊天" }).querySelector("svg")
    ).toHaveAttribute("stroke-width", "2.5")
    expect(
      screen.getByRole("link", { name: "通讯录" }).querySelector("svg")
    ).toHaveAttribute("stroke-width", "2")
    expect(screen.getByRole("link", { name: "通讯录" })).toHaveClass(
      "text-muted-foreground"
    )

    await user.click(screen.getByRole("link", { name: "通讯录" }))
    expect(screen.getByTestId("location")).toHaveTextContent("/contacts")
    expect(document.title).toBe("联系人 - 星环协作")
    expect(
      screen.getByRole("heading", { level: 1, name: "联系人" })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole("heading", { level: 2, name: "联系人" })
    ).not.toBeInTheDocument()
    expect(screen.getByText("选择一个联系人查看详情")).toBeInTheDocument()
    const refreshButton = screen.getByRole("button", { name: "刷新" })
    expect(refreshButton).toHaveAttribute("aria-label", "刷新")
    expect(refreshButton).toHaveAttribute("title", "刷新")
    expect(refreshButton).toHaveAttribute("data-size", "icon-sm")
    expect(refreshButton).toHaveAttribute("data-variant", "ghost")
    expect(refreshButton).toHaveTextContent("")
    expect(screen.getByRole("listbox", { name: "联系人列表" })).toHaveClass(
      "has-data-[size=sm]:gap-1"
    )
    const aliceContactItem = screen.getByRole("option", { name: "Al" })
    const bobContactItem = screen.getByRole("option", { name: "Bob Li" })
    const emptyDetailState = screen.getByTestId("contact-empty-state")

    expect(screen.getByTestId("contact-detail-shell")).toHaveClass(
      "items-start",
      "justify-center",
      "bg-muted"
    )
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass(
      "items-center",
      "bg-background"
    )
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass("pt-14")
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass("pt-21")
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass("pt-20")
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass("py-6")
    expect(screen.queryByTestId("contact-empty-card")).not.toBeInTheDocument()
    expect(emptyDetailState).not.toHaveAttribute("data-slot", "card")
    expect(emptyDetailState).toHaveClass(
      "flex-1",
      "items-center",
      "justify-center",
      "self-stretch",
      "text-muted-foreground"
    )
    expect(emptyDetailState).not.toHaveClass("min-h-96", "max-w-sm")
    expect(screen.queryByTestId("contact-detail-panel")).not.toBeInTheDocument()
    expect(aliceContactItem).toHaveAttribute("aria-selected", "false")
    expect(aliceContactItem).toHaveAttribute("data-slot", "item")
    expect(aliceContactItem).toHaveAttribute("data-size", "sm")
    expect(aliceContactItem).toHaveClass("px-2")
    expect(aliceContactItem).not.toHaveClass("px-3")
    expect(aliceContactItem).toHaveClass("py-1.5")
    expect(aliceContactItem).not.toHaveClass("py-2.5")
    expect(within(aliceContactItem).getByText("Al")).toBeInTheDocument()
    expect(
      within(aliceContactItem).queryByText("Al - Alice")
    ).not.toBeInTheDocument()
    expect(
      within(aliceContactItem).getByTestId("contact-avatar")
    ).toHaveAttribute("data-size", "sm")
    expect(within(aliceContactItem).getByTestId("contact-avatar")).toHaveClass(
      "bg-muted",
      "rounded-sm"
    )
    expect(within(aliceContactItem).getByTestId("contact-avatar")).toHaveClass(
      "after:rounded-sm"
    )
    expect(
      within(aliceContactItem).getByTestId("contact-avatar")
    ).not.toHaveClass("rounded-md")
    expect(
      within(within(aliceContactItem).getByTestId("contact-avatar")).getByText(
        "A"
      )
    ).toHaveClass("rounded-sm")
    expect(
      within(aliceContactItem).getByTestId("contact-avatar")
    ).not.toHaveClass("size-7")
    expect(
      within(aliceContactItem).queryByText("alice@example.com")
    ).not.toBeInTheDocument()
    expect(within(aliceContactItem).getByLabelText("在线")).toHaveClass(
      "bg-emerald-500"
    )
    const aliceConversationButton = within(aliceContactItem).getByRole(
      "button",
      { name: "与 Al 对话" }
    )
    expect(aliceConversationButton).toBeInTheDocument()
    expect(aliceConversationButton).toHaveAttribute("data-size", "icon-xs")
    expect(aliceConversationButton).toHaveAttribute("data-variant", "ghost")
    expect(aliceConversationButton).toBeDisabled()
    expect(aliceConversationButton).not.toHaveClass("opacity-0")
    expect(aliceConversationButton).not.toHaveClass(
      "group-hover/contact-item:opacity-100"
    )
    const aliceActions = aliceConversationButton.closest(
      '[data-slot="item-actions"]'
    )
    expect(aliceActions).toHaveClass("opacity-0")
    expect(aliceActions).toHaveClass("group-hover/contact-item:opacity-100")
    expect(within(aliceContactItem).queryByText("对话")).not.toBeInTheDocument()
    expect(within(bobContactItem).getByText("Bob Li")).toBeInTheDocument()
    expect(
      within(bobContactItem).queryByText("bob@example.com")
    ).not.toBeInTheDocument()
    expect(within(bobContactItem).getByLabelText("离线")).toHaveClass(
      "bg-muted-foreground/30"
    )
    const bobConversationButton = within(bobContactItem).getByRole("button", {
      name: "与 Bob Li 对话",
    })
    expect(bobConversationButton).not.toHaveClass("opacity-0")
    const bobActions = bobConversationButton.closest(
      '[data-slot="item-actions"]'
    )
    expect(bobActions).toHaveClass("opacity-0")
    expect(bobActions).toHaveClass("group-hover/contact-item:opacity-100")

    await user.click(bobContactItem)
    expect(screen.getByTestId("contact-detail-shell")).toHaveClass(
      "bg-background"
    )
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass(
      "bg-muted"
    )
    expect(screen.queryByTestId("contact-empty-state")).not.toBeInTheDocument()
    expect(screen.queryByTestId("contact-detail-card")).not.toBeInTheDocument()
    const contactDetailPanel = screen.getByTestId("contact-detail-panel")
    expect(contactDetailPanel).not.toHaveAttribute("data-slot", "card")
    expect(contactDetailPanel).toHaveClass("mt-30", "max-w-sm")
    expect(contactDetailPanel).not.toHaveClass(
      "mt-14",
      "max-w-md",
      "min-h-96",
      "shadow-xs",
      "ring-1"
    )
    expect(
      within(contactDetailPanel).queryByRole("heading", { name: "Bob Li" })
    ).not.toBeInTheDocument()
    expect(
      within(contactDetailPanel).queryByText("成员")
    ).not.toBeInTheDocument()
    expect(within(contactDetailPanel).getByText("姓名")).toBeInTheDocument()
    expect(within(contactDetailPanel).getByText("昵称")).toBeInTheDocument()
    expect(
      within(contactDetailPanel).queryByText("状态")
    ).not.toBeInTheDocument()
    expect(
      within(contactDetailPanel).queryByText("最近在线 2026-07-03 01:00")
    ).not.toBeInTheDocument()
    expect(
      within(contactDetailPanel).queryByText("在线")
    ).not.toBeInTheDocument()
    expect(
      within(contactDetailPanel).queryByText("离线")
    ).not.toBeInTheDocument()
    expect(
      within(contactDetailPanel).getByText("姓名").parentElement?.parentElement
    ).toHaveClass("gap-1")
    expect(
      within(contactDetailPanel).getByText("姓名").parentElement
    ).toHaveClass("py-2")
    expect(
      within(contactDetailPanel).getByText("姓名").parentElement
    ).not.toHaveClass("py-3")
    expect(
      contactDetailPanel.querySelectorAll(".lucide-user-round")
    ).toHaveLength(1)
    expect(
      contactDetailPanel.querySelector(".lucide-user-pen")
    ).toBeInTheDocument()
    expect(
      contactDetailPanel.querySelector(".lucide-at-sign")
    ).not.toBeInTheDocument()
    expect(within(contactDetailPanel).getByText("未设置")).toHaveClass(
      "text-muted-foreground"
    )
    expect(
      within(contactDetailPanel).getByTestId("contact-detail-avatar")
    ).toHaveClass("bg-muted", "rounded-sm")
    expect(
      within(contactDetailPanel).getByTestId("contact-detail-avatar")
    ).toHaveClass("after:rounded-sm")
    expect(
      within(contactDetailPanel).getByTestId("contact-detail-avatar")
    ).not.toHaveClass("rounded-lg")
    expect(
      within(
        within(contactDetailPanel).getByTestId("contact-detail-avatar")
      ).getByText("B")
    ).toHaveClass("rounded-sm")
    expect(
      within(contactDetailPanel).queryByText("会话")
    ).not.toBeInTheDocument()
    expect(
      within(contactDetailPanel).queryByText("可发起一对一对话")
    ).not.toBeInTheDocument()
    const sendMessageButton = within(contactDetailPanel).getByRole("button", {
      name: "发消息",
    })
    expect(sendMessageButton).toHaveAttribute("type", "button")
    expect(sendMessageButton).toHaveClass("w-full")
    expect(sendMessageButton).not.toHaveClass("mt-auto")
    expect(aliceContactItem).toHaveAttribute("aria-selected", "false")
    expect(bobContactItem).toHaveAttribute("aria-selected", "true")
    expect(bobConversationButton).not.toHaveClass("opacity-100")
    expect(bobActions).toHaveClass("opacity-100")
    expect(
      screen.queryByRole("heading", { level: 2, name: "Bob Li" })
    ).not.toBeInTheDocument()
    expect(screen.getAllByText("bob@example.com").length).toBeGreaterThan(0)
    expect(screen.getByRole("link", { name: "通讯录" })).toHaveClass(
      "bg-primary",
      "text-primary-foreground"
    )
    expect(
      screen.getByRole("link", { name: "通讯录" }).querySelector("svg")
    ).toHaveAttribute("stroke-width", "2.5")
    expect(
      screen.getByRole("link", { name: "聊天" }).querySelector("svg")
    ).toHaveAttribute("stroke-width", "2")
    expect(screen.getByRole("link", { name: "聊天" })).toHaveClass(
      "text-muted-foreground"
    )
    expect(screen.getByRole("link", { name: "聊天" })).not.toHaveClass(
      "bg-primary"
    )

    await user.click(screen.getByRole("link", { name: "任务" }))
    expect(screen.getByTestId("location")).toHaveTextContent("/tasks")
    expect(document.title).toBe("任务 - 星环协作")
    expect(screen.getByText("待完善")).toBeInTheDocument()
    expect(screen.getByText("待完善").closest("main")).toHaveClass("bg-muted")
    expect(screen.getByText("待完善").closest("main")).not.toHaveClass(
      "bg-background"
    )
    expect(
      screen.queryByRole("heading", { name: "任务" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "新建" })
    ).not.toBeInTheDocument()
    expect(screen.queryByPlaceholderText("搜索任务")).not.toBeInTheDocument()
    expect(screen.queryByText("确认登录与路由原型")).not.toBeInTheDocument()
    expect(screen.queryByText("梳理通讯录用户模型")).not.toBeInTheDocument()
    expect(screen.queryByText("定义内置助手的默认能力")).not.toBeInTheDocument()
    expect(screen.getByRole("link", { name: "任务" })).toHaveClass(
      "bg-primary",
      "text-primary-foreground"
    )

    await user.click(screen.getByRole("link", { name: "聊天" }))
    expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    expect(document.title).toBe("聊天 - 星环协作")
    expect(screen.getByRole("heading", { name: "消息" })).toBeInTheDocument()
  }, 10_000)

  it("联系人详情里的发消息会创建私聊并跳到对应聊天", async () => {
    const user = userEvent.setup()
    const fetcher = fetch as unknown as ReturnType<typeof vi.fn>

    renderApp("/contacts")

    await openLatestAppWebSocket()
    await screen.findByRole("heading", { name: "联系人" }, { timeout: 4_000 })
    const bobContactItem = screen.getByRole("option", { name: "Bob Li" })

    await user.click(bobContactItem)
    await user.click(
      within(screen.getByTestId("contact-detail-panel")).getByRole("button", {
        name: "发消息",
      })
    )

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    )
    expect(screen.getByTestId("location-search")).toHaveTextContent(
      "?conversation_id=conversation-bob"
    )
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
    expect(
      await screen.findByRole("heading", { name: "Bob Li" })
    ).toBeInTheDocument()
  }, 10_000)

  it("联系人详情里的发消息等待接口时显示 loading 图标", async () => {
    const user = userEvent.setup()
    let resolveDirectConversation!: (response: Response) => void
    const directConversationResponse = new Promise<Response>((resolve) => {
      resolveDirectConversation = resolve
    })
    vi.stubGlobal(
      "fetch",
      createClientFetchMock({ directConversationResponse })
    )

    renderApp("/contacts")

    await openLatestAppWebSocket()
    await screen.findByRole("heading", { name: "联系人" }, { timeout: 4_000 })
    await user.click(screen.getByRole("option", { name: "Bob Li" }))
    const sendMessageButton = within(
      screen.getByTestId("contact-detail-panel")
    ).getByRole("button", { name: "发消息" })

    await user.click(sendMessageButton)

    expect(sendMessageButton).toBeDisabled()
    expect(sendMessageButton.querySelector(".animate-spin")).toBeInTheDocument()

    resolveDirectConversation(createDirectConversationResponse())
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    )
  }, 10_000)

  it("联系人列表里的消息按钮会直接进入私聊", async () => {
    const user = userEvent.setup()

    renderApp("/contacts")

    await openLatestAppWebSocket()
    await screen.findByRole("heading", { name: "联系人" }, { timeout: 4_000 })
    const bobContactItem = screen.getByRole("option", { name: "Bob Li" })

    await user.click(
      within(bobContactItem).getByRole("button", { name: "与 Bob Li 对话" })
    )

    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    )
    expect(screen.getByTestId("location-search")).toHaveTextContent(
      "?conversation_id=conversation-bob"
    )
  }, 10_000)

  it("联系人列表里的消息按钮等待接口时显示 loading 图标", async () => {
    const user = userEvent.setup()
    let resolveDirectConversation!: (response: Response) => void
    const directConversationResponse = new Promise<Response>((resolve) => {
      resolveDirectConversation = resolve
    })
    vi.stubGlobal(
      "fetch",
      createClientFetchMock({ directConversationResponse })
    )

    renderApp("/contacts")

    await openLatestAppWebSocket()
    await screen.findByRole("heading", { name: "联系人" }, { timeout: 4_000 })
    const bobConversationButton = within(
      screen.getByRole("option", { name: "Bob Li" })
    ).getByRole("button", { name: "与 Bob Li 对话" })

    await user.click(bobConversationButton)

    expect(bobConversationButton).toBeDisabled()
    expect(
      bobConversationButton.querySelector(".animate-spin")
    ).toBeInTheDocument()
    expect(
      bobConversationButton.querySelector(".lucide-message-circle")
    ).not.toBeInTheDocument()

    resolveDirectConversation(createDirectConversationResponse())
    await waitFor(() =>
      expect(screen.getByTestId("location")).toHaveTextContent("/chat")
    )
  }, 10_000)

  it("联系人里的自己不能发起私聊", async () => {
    const user = userEvent.setup()
    const fetcher = fetch as unknown as ReturnType<typeof vi.fn>

    renderApp("/contacts")

    await openLatestAppWebSocket()
    await screen.findByRole("heading", { name: "联系人" }, { timeout: 4_000 })
    const selfContactItem = screen.getByRole("option", { name: "Al" })

    expect(
      within(selfContactItem).getByRole("button", { name: "与 Al 对话" })
    ).toBeDisabled()

    await user.click(selfContactItem)
    const detailMessageButton = within(
      screen.getByTestId("contact-detail-panel")
    ).getByRole("button", { name: "发消息" })

    expect(detailMessageButton).toBeDisabled()
    await user.click(detailMessageButton)

    expect(fetcher).not.toHaveBeenCalledWith(
      "/api/client/conversations/direct",
      expect.anything()
    )
  }, 10_000)

  it("聊天页会根据 conversation_id 参数选中会话", async () => {
    renderApp("/chat?conversation_id=conversation-team")

    await openLatestAppWebSocket()

    expect(
      await screen.findByRole(
        "heading",
        { name: "产品讨论组" },
        { timeout: 4_000 }
      )
    ).toBeInTheDocument()
    expect(
      within(
        await screen.findByTestId("conversation-panel-history")
      ).getAllByText("今天下午同步").length
    ).toBeGreaterThan(0)
  }, 10_000)

  it("打开会话时先显示消息加载状态，接口返回空列表后再显示空态", async () => {
    let resolveMessages!: (response: Response) => void
    const pendingMessages = new Promise<Response>((resolve) => {
      resolveMessages = resolve
    })
    const fetcher = createClientFetchMock({
      conversationMessagesHandler: (conversationId) => {
        if (conversationId === "conversation-bob") {
          return pendingMessages
        }

        return createDefaultConversationMessagesResponse(conversationId)
      },
    })
    vi.stubGlobal("fetch", fetcher)

    renderApp("/chat?conversation_id=conversation-bob")

    await openLatestAppWebSocket()
    expect(
      await screen.findByRole(
        "heading",
        { name: "Bob Li" },
        { timeout: 4_000 }
      )
    ).toBeInTheDocument()
    const loading = await screen.findByTestId("conversation-history-loading")

    expect(loading).toHaveTextContent("正在加载消息")
    expect(
      screen.queryByTestId("conversation-history-empty")
    ).not.toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-bob/messages?limit=20",
      {
        credentials: "include",
        method: "GET",
      }
    )

    resolveMessages(
      createConversationMessagesResponse({
        conversationId: "conversation-bob",
        messages: [],
      })
    )

    const historyEmpty = await screen.findByTestId("conversation-history-empty")

    expect(historyEmpty).toHaveTextContent("暂无消息")
    expect(historyEmpty).toHaveTextContent("发送第一条消息开始对话")
    expect(
      screen.queryByTestId("conversation-history-loading")
    ).not.toBeInTheDocument()
  }, 10_000)

  it("发送消息时调用接口并用返回消息更新聊天记录", async () => {
    const user = userEvent.setup()
    let resolveSendMessage!: (response: Response) => void
    const sendMessageResponse = new Promise<Response>((resolve) => {
      resolveSendMessage = resolve
    })
    const fetcher = createClientFetchMock({ sendMessageResponse })
    vi.stubGlobal("fetch", fetcher)

    renderApp("/chat?conversation_id=conversation-bob")

    await openLatestAppWebSocket()
    await screen.findByText("好的，我看一下", undefined, { timeout: 4_000 })
    const editor = screen.getByPlaceholderText("输入消息") as HTMLTextAreaElement
    const sendButton = screen.getByRole("button", { name: "发送消息" })

    await user.type(editor, "帮我总结今天的消息")
    await user.click(sendButton)

    expect(sendButton).toBeDisabled()
    expect(sendButton.querySelector(".animate-spin")).toBeInTheDocument()
    expect(
      within(screen.getByTestId("conversation-panel-history")).queryByText(
        "帮我总结今天的消息"
      )
    ).not.toBeInTheDocument()

    const sendCall = fetcher.mock.calls.find(([input, init]) => {
      return (
        String(input) ===
          "/api/client/conversations/conversation-bob/messages" &&
        init?.method === "POST"
      )
    })
    expect(sendCall).toBeDefined()

    const requestBody = JSON.parse(String(sendCall?.[1]?.body ?? "{}")) as {
      body?: {
        content?: string
        type?: string
      }
      client_message_id?: string
    }

    expect(requestBody.client_message_id).toEqual(expect.any(String))
    expect(requestBody.body).toEqual({
      type: "text",
      content: "帮我总结今天的消息",
    })

    resolveSendMessage(
      createSendMessageResponse({
        clientMessageId: requestBody.client_message_id,
        content: "帮我总结今天的消息",
      })
    )

    expect(
      await screen.findByText("帮我总结今天的消息")
    ).toBeInTheDocument()
    expect(editor).toHaveValue("")
    await waitFor(() => expect(sendButton).not.toBeDisabled())
  }, 10_000)

  it("聊天历史上滚到顶部时继续拉取更早消息", async () => {
    const fetcher = createClientFetchMock({
      conversationMessagesHandler: (conversationId, url) => {
        if (conversationId !== "conversation-bob") {
          return createDefaultConversationMessagesResponse(conversationId)
        }

        if (url.searchParams.get("before_seq") === "21") {
          return createConversationMessagesResponse({
            conversationId,
            messages: [
              createConversationMessage({
                clientMessageId: "client-message-1",
                content: "更早的消息",
                conversationId,
                createdAt: "2026-07-03T07:00:00Z",
                id: "message-1",
                senderId: "user-2",
                seq: 1,
              }),
            ],
          })
        }

        return createConversationMessagesResponse({
          conversationId,
          hasMoreBefore: true,
          messages: [
            createConversationMessage({
              clientMessageId: "client-message-21",
              content: "最新消息",
              conversationId,
              createdAt: "2026-07-03T08:00:00Z",
              id: "message-21",
              senderId: "user-2",
              seq: 21,
            }),
          ],
        })
      },
    })
    vi.stubGlobal("fetch", fetcher)

    renderApp("/chat?conversation_id=conversation-bob")

    await openLatestAppWebSocket()
    const history = await screen.findByTestId("conversation-panel-history")
    expect(within(history).getByText("最新消息")).toBeInTheDocument()

    const viewport = history.querySelector('[data-slot="scroll-area-viewport"]')
    expect(viewport).toBeInstanceOf(HTMLElement)
    Object.defineProperty(viewport, "scrollTop", {
      configurable: true,
      value: 0,
      writable: true,
    })

    fireEvent.scroll(viewport as HTMLElement)

    await waitFor(() =>
      expect(within(history).getByText("更早的消息")).toBeInTheDocument()
    )
    expect(fetcher).toHaveBeenCalledWith(
      "/api/client/conversations/conversation-bob/messages?limit=20&before_seq=21",
      {
        credentials: "include",
        method: "GET",
      }
    )
  }, 10_000)

  it("登录后的页面加载完成后建立实时连接", async () => {
    renderApp("/chat")

    expect(AppWebSocketMock.instances).toHaveLength(0)
    await waitFor(() => expect(AppWebSocketMock.instances).toHaveLength(1), {
      timeout: 4_000,
    })
    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    expect(
      screen.queryByRole("heading", { name: "消息" })
    ).not.toBeInTheDocument()
    expect(AppWebSocketMock.instances[0].url).toMatch(/\/api\/client\/ws$/)

    AppWebSocketMock.instances[0].open()
    expect(screen.getByText("正在为你加载数据")).toBeInTheDocument()
    expect(
      screen.queryByRole("heading", { name: "消息" })
    ).not.toBeInTheDocument()

    AppWebSocketMock.instances[0].receive({
      v: 1,
      kind: "event",
      event: "system.ready",
      payload: {},
    })
    expect(
      await screen.findByRole("heading", { name: "消息" }, { timeout: 4_000 })
    ).toBeInTheDocument()
  })

  it("最左侧导航底部可以切换并记住配色", async () => {
    const user = userEvent.setup()

    const { unmount } = renderApp("/chat")

    await openLatestAppWebSocket()
    const themeButton = await screen.findByRole(
      "button",
      {
        name: "配色：跟随系统",
      },
      { timeout: 4_000 }
    )
    expect(themeButton).toBeInTheDocument()
    expect(themeButton).toHaveAttribute("aria-label", "配色：跟随系统")
    expect(themeButton.querySelector(".lucide-sun-moon")).toBeInTheDocument()
    expect(window.localStorage.getItem("theme")).toBeNull()

    await user.click(themeButton)
    await user.click(screen.getByRole("menuitemradio", { name: "黑暗模式" }))

    expect(window.localStorage.getItem("theme")).toBe("dark")
    await waitFor(() => expect(document.documentElement).toHaveClass("dark"))

    unmount()
    const previousSocketCount = AppWebSocketMock.instances.length
    renderApp("/chat")
    await openLatestAppWebSocket(previousSocketCount)

    expect(
      await screen.findByRole(
        "button",
        { name: "配色：黑暗模式" },
        { timeout: 4_000 }
      )
    ).toBeInTheDocument()
    expect(document.documentElement).toHaveClass("dark")
  }, 12_000)
})
