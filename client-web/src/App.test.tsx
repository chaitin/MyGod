import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter, useLocation } from "react-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"

import App from "./App"

const rememberedCredentialsKey = "client-web:remembered-login"

function LocationProbe() {
  const location = useLocation()

  return <span data-testid="location">{location.pathname}</span>
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

function createClientFetchMock({
  currentUserAvatar = "/assets/avatars/builtin/17.webp",
  currentUserNickname = "Al",
  loginStatus = 200,
  logoutStatus = 200,
}: {
  currentUserAvatar?: string
  currentUserNickname?: string
  loginStatus?: 200 | 401
  logoutStatus?: 200 | 500
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
                name: "Alice",
                nickname: "Al",
                phone: "+8613912345678",
                type: "user",
              },
              {
                avatar: "/assets/avatars/builtin/03.webp",
                email: "bob@example.com",
                id: "user-2",
                name: "Bob Li",
                nickname: "",
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

describe("App", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", createClientFetchMock())
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

  it("登录后跳转到 /chat 并可以和内置 AI 助手发送消息", async () => {
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
    const assistantConversationItem = screen.getByRole("button", {
      name: /AI 助手/,
    })
    expect(assistantConversationItem).toBeInTheDocument()
    expect(assistantConversationItem).toHaveAttribute("data-slot", "item")
    expect(assistantConversationItem).toHaveAttribute("data-size", "sm")
    expect(screen.getByRole("heading", { name: "AI 助手" })).toBeInTheDocument()

    await user.type(
      screen.getByPlaceholderText("输入消息，Enter 发送"),
      "帮我总结今天的消息"
    )
    await user.click(screen.getByRole("button", { name: "发送消息" }))

    expect(screen.getByText("帮我总结今天的消息")).toBeInTheDocument()
    expect(screen.getByText(/收到，我会先作为你的内置助手/)).toBeInTheDocument()
  }, 10_000)

  it("当前用户没有头像时侧栏头像占位使用 muted 背景", async () => {
    vi.stubGlobal(
      "fetch",
      createClientFetchMock({
        currentUserAvatar: "",
      })
    )

    renderApp("/chat")

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

  it("点击侧栏头像菜单可以退出登录", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const user = userEvent.setup()

    renderApp("/chat")

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
  }, 10_000)

  it("点击侧栏头像菜单可以打开设置对话框", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const user = userEvent.setup()

    renderApp("/chat")

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
      "justify-center"
    )
    expect(screen.getByTestId("contact-detail-shell")).not.toHaveClass(
      "items-center"
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
    const aliceConversationButton = within(aliceContactItem).getByRole(
      "button",
      { name: "与 Al 对话" }
    )
    expect(aliceConversationButton).toBeInTheDocument()
    expect(aliceConversationButton).toHaveAttribute("data-size", "icon-xs")
    expect(aliceConversationButton).toHaveAttribute("data-variant", "ghost")
    expect(aliceConversationButton).toHaveClass("opacity-0")
    expect(within(aliceContactItem).queryByText("对话")).not.toBeInTheDocument()
    expect(within(bobContactItem).getByText("Bob Li")).toBeInTheDocument()
    expect(
      within(bobContactItem).queryByText("bob@example.com")
    ).not.toBeInTheDocument()
    const bobConversationButton = within(bobContactItem).getByRole("button", {
      name: "与 Bob Li 对话",
    })
    expect(bobConversationButton).toHaveClass("opacity-0")
    expect(bobConversationButton).toHaveClass(
      "group-hover/contact-item:opacity-100"
    )

    await user.click(bobContactItem)
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
    expect(bobConversationButton).toHaveClass("opacity-100")
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

  it("最左侧导航底部可以切换并记住配色", async () => {
    const user = userEvent.setup()

    const { unmount } = renderApp("/chat")

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
    renderApp("/chat")

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
