import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { MemoryRouter, Route, Routes } from "react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { AppLayout } from "@/components/app-layout"
import { LoginPage } from "@/pages/login-page"
import { defaultAppInfo } from "@/lib/app-info"
import { AppInfoContext } from "@/lib/app-info-context"

const mocks = vi.hoisted(() => ({
  clientData: {
    conversations: [] as Array<{ unreadCount: number }>,
    me: {
      avatar: "",
      createdAt: "2026-07-09T00:00:00Z",
      email: "me@example.com",
      id: "user-1",
      lastOnlineAt: null,
      name: "张三",
      nickname: "三三",
      phone: "",
      status: "active",
    },
    refreshMe: vi.fn(),
  },
  clientLogout: vi.fn(),
  setTheme: vi.fn(),
  updateCurrentClientUser: vi.fn(),
  uploadCurrentClientAvatar: vi.fn(),
}))

beforeEach(() => {
  vi.clearAllMocks()
  mocks.clientData.conversations = []
})

vi.mock("@/lib/client-data-context", () => ({
  useClientData: () => mocks.clientData,
}))

vi.mock("@/components/theme-provider", () => ({
  useTheme: () => ({
    setTheme: mocks.setTheme,
    theme: "system",
  }),
}))

vi.mock("@/lib/client-auth", () => ({
  clientLogout: mocks.clientLogout,
}))

vi.mock("@/lib/client-data-api", () => ({
  updateCurrentClientUser: mocks.updateCurrentClientUser,
  uploadCurrentClientAvatar: mocks.uploadCurrentClientAvatar,
}))

describe("AppLayout", () => {
  it("shows the notification dot only while conversations are unread", () => {
    mocks.clientData.conversations = [{ unreadCount: 0 }, { unreadCount: 2 }]

    const view = render(
      <MemoryRouter initialEntries={["/chat"]}>
        <AppLayout />
      </MemoryRouter>
    )

    const chatLink = screen.getByRole("link", {
      name: "聊天，有未读消息",
    })

    expect(
      chatLink.querySelector('[data-slot="notification-dot"]')
    ).toBeInTheDocument()

    mocks.clientData.conversations = [{ unreadCount: 0 }, { unreadCount: 0 }]
    view.rerender(
      <MemoryRouter initialEntries={["/chat"]}>
        <AppLayout />
      </MemoryRouter>
    )

    const readChatLink = screen.getByRole("link", { name: "聊天" })

    expect(
      readChatLink.querySelector('[data-slot="notification-dot"]')
    ).not.toBeInTheDocument()
  })

  it("splits profile and settings actions in the user avatar menu", async () => {
    const user = userEvent.setup()

    render(
      <MemoryRouter initialEntries={["/chat"]}>
        <AppLayout />
      </MemoryRouter>
    )

    await user.click(screen.getByRole("button", { name: "用户菜单" }))

    expect(
      screen.getByRole("menuitem", { name: /个人资料/ })
    ).toBeInTheDocument()
    expect(screen.getByRole("menuitem", { name: /^设置$/ })).toBeInTheDocument()

    await user.click(screen.getByRole("menuitem", { name: /个人资料/ }))

    const profileDialog = await screen.findByRole("dialog", {
      name: "个人资料",
    })
    expect(within(profileDialog).getByLabelText("昵称")).toBeInTheDocument()
    expect(
      within(profileDialog).queryByText("桌面通知")
    ).not.toBeInTheDocument()

    await user.click(
      within(profileDialog).getByRole("button", { name: "关闭" })
    )
    await user.click(screen.getByRole("button", { name: "用户菜单" }))
    await user.click(screen.getByRole("menuitem", { name: /^设置$/ }))

    const settingsDialog = await screen.findByRole("dialog", { name: "设置" })
    expect(within(settingsDialog).getByText("桌面通知")).toBeInTheDocument()
    expect(
      within(settingsDialog).queryByLabelText("昵称")
    ).not.toBeInTheDocument()
  })

  it("shows download options for all client platforms", async () => {
    const user = userEvent.setup()

    render(
      <MemoryRouter initialEntries={["/chat"]}>
        <AppLayout />
      </MemoryRouter>
    )

    const downloadButton = screen.getByRole("button", { name: "下载客户端" })
    const themeButton = screen.getByRole("button", { name: "配色：跟随系统" })

    expect(downloadButton).toHaveClass(
      "hover:bg-transparent",
      "hover:text-teal-500",
      "data-[state=open]:bg-transparent",
      "data-[state=open]:text-teal-500"
    )
    expect(themeButton).toHaveClass(
      "hover:bg-transparent",
      "hover:text-teal-500",
      "data-[state=open]:bg-transparent",
      "data-[state=open]:text-teal-500"
    )

    await user.click(downloadButton)

    const dialog = await screen.findByRole("dialog", { name: "下载客户端" })
    expect(within(dialog).getByText("Windows")).toBeInTheDocument()
    expect(within(dialog).getByText("macOS")).toBeInTheDocument()
    expect(within(dialog).getByText("Android")).toBeInTheDocument()
    expect(within(dialog).getByText("iOS")).toBeInTheDocument()
    expect(
      within(dialog).getByRole("link", {
        name: "下载 Android 客户端",
      })
    ).toMatchObject({
      href: "https://chat-public-1450770193.cos.ap-guangzhou.myqcloud.com/releases/magic-chat-2026-07-17-1.apk",
      target: "_blank",
    })
    expect(
      within(dialog).getAllByRole("button", { name: "敬请期待" })
    ).toHaveLength(3)
  })

  it("stays on the login page after logout", async () => {
    const user = userEvent.setup()
    mocks.clientLogout.mockResolvedValue(undefined)

    render(<LogoutFlow />)

    await user.click(screen.getByRole("button", { name: "用户菜单" }))
    await user.click(screen.getByRole("menuitem", { name: "退出登录" }))

    const dialog = await screen.findByRole("alertdialog", {
      name: "确认退出登录",
    })
    await user.click(within(dialog).getByRole("button", { name: "退出登录" }))

    expect(
      await screen.findByRole("heading", { name: "即应 智能协作平台" })
    ).toBeInTheDocument()
    expect(screen.queryByTestId("init-page")).not.toBeInTheDocument()
    expect(mocks.clientLogout).toHaveBeenCalledTimes(1)
  })
})

function LogoutFlow() {
  const [authenticated, setAuthenticated] = useState(true)

  return (
    <AppInfoContext.Provider
      value={{
        ...defaultAppInfo,
        authenticated,
        setAuthenticated,
      }}
    >
      <MemoryRouter initialEntries={["/chat"]}>
        <Routes>
          <Route element={<AppLayout />} path="/chat" />
          <Route element={<LoginPage />} path="/login" />
          <Route element={<div data-testid="init-page" />} path="/init" />
        </Routes>
      </MemoryRouter>
    </AppInfoContext.Provider>
  )
}
