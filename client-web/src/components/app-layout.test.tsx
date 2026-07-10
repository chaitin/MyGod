import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { AppLayout } from "@/components/app-layout"

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
    expect(
      screen.getByRole("menuitem", { name: /第三方账号/ })
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
})
