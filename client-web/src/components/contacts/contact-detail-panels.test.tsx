import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { MemoryRouter } from "react-router"
import { describe, expect, it, vi } from "vitest"

import {
  AppDetailPanel,
  ContactDetailPanel,
} from "@/components/contacts/contact-detail-panels"
import type { ContactApp, ContactUser } from "@/lib/client-data-api"
import {
  ClientDataContext,
  type ClientDataContextValue,
} from "@/lib/client-data-context"

const contact: ContactUser = {
  avatar: "",
  email: "me@example.com",
  id: "current-user",
  lastOnlineAt: null,
  name: "当前用户",
  nickname: "",
  online: true,
  phone: "",
  type: "user",
}

describe("ContactDetailPanel", () => {
  it("does not render a message action for the current user", () => {
    render(
      <ContactDetailPanel
        canStartConversation={false}
        contact={contact}
        onStartConversation={vi.fn()}
        startingConversation={false}
      />
    )

    expect(
      screen.queryByRole("button", { name: "发消息" })
    ).not.toBeInTheDocument()
  })

  it("renders a message action for another contact", () => {
    render(
      <ContactDetailPanel
        canStartConversation
        contact={{ ...contact, id: "other-user" }}
        onStartConversation={vi.fn()}
        startingConversation={false}
      />
    )

    expect(screen.getByRole("button", { name: "发消息" })).toBeInTheDocument()
  })
})

describe("AppDetailPanel", () => {
  const app: ContactApp = {
    avatar: "",
    creatorUserId: "current-user",
    description: "分析消息",
    id: "app-1",
    name: "分析助手",
    online: false,
    type: "app",
  }

  it("renders the owned application actions and opens its developer profile", async () => {
    const user = userEvent.setup()
    const onDelete = vi.fn().mockResolvedValue(undefined)
    renderWithClientData(
      <AppDetailPanel
        app={app}
        developer={contact}
        onDelete={onDelete}
        onEditProfile={vi.fn()}
        onStartConversation={vi.fn()}
        onViewAccessInfo={vi.fn()}
        startingConversation={false}
      />
    )

    const accessInfoButton = screen.getByRole("button", {
      name: "开发指南",
    })
    const editProfileButton = screen.getByRole("button", { name: "修改资料" })
    const deleteButton = screen.getByRole("button", { name: "删除应用" })
    expect(accessInfoButton).toBeInTheDocument()
    expect(editProfileButton).toBeInTheDocument()
    expect(deleteButton).toBeInTheDocument()
    expect(
      editProfileButton.compareDocumentPosition(accessInfoButton) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      accessInfoButton.compareDocumentPosition(deleteButton) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByText("开发者")).toBeInTheDocument()
    expect(screen.getByText("当前用户")).toBeInTheDocument()
    const developerLink = screen.getByRole("button", { name: "当前用户资料" })

    await user.click(developerLink)
    expect(await screen.findByText("用户资料")).toBeInTheDocument()
    expect(screen.getByText("me@example.com")).toBeInTheDocument()
  })

  it("does not render the access information action without permission", () => {
    render(
      <AppDetailPanel
        app={{ ...app, creatorUserId: "other-user" }}
        onStartConversation={vi.fn()}
        startingConversation={false}
      />
    )

    expect(
      screen.queryByRole("button", { name: "开发指南" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "修改资料" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "删除应用" })
    ).not.toBeInTheDocument()
    expect(screen.queryByText("开发者")).not.toBeInTheDocument()
  })

  it("requires the exact application name before deletion", async () => {
    const user = userEvent.setup()
    const onDelete = vi.fn().mockResolvedValue(undefined)
    render(
      <AppDetailPanel
        app={app}
        onDelete={onDelete}
        onEditProfile={vi.fn()}
        onStartConversation={vi.fn()}
        onViewAccessInfo={vi.fn()}
        startingConversation={false}
      />
    )

    await user.click(screen.getByRole("button", { name: "删除应用" }))
    const confirmation = screen.getByRole("alertdialog", {
      name: "确认删除应用",
    })
    const confirmButton = within(confirmation).getByRole("button", {
      name: "删除应用",
    })
    const confirmationInput =
      within(confirmation).getByLabelText("请输入应用名称“分析助手”以确认删除")
    expect(confirmButton).toBeDisabled()

    await user.type(confirmationInput, "错误名称")
    expect(confirmButton).toBeDisabled()
    await user.clear(confirmationInput)
    await user.type(confirmationInput, "分析助手")
    expect(confirmButton).toBeEnabled()
    await user.click(confirmButton)

    await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1))
  })
})

function renderWithClientData(node: ReactNode) {
  return render(
    <MemoryRouter>
      <ClientDataContext.Provider
        value={
          {
            contacts: [contact],
            me: {
              avatar: contact.avatar,
              createdAt: "2026-07-17T00:00:00Z",
              email: contact.email,
              id: contact.id,
              lastOnlineAt: null,
              name: contact.name,
              nickname: contact.nickname,
              phone: contact.phone,
              status: "active",
            },
            openDirectConversation: vi.fn(),
          } as unknown as ClientDataContextValue
        }
      >
        {node}
      </ClientDataContext.Provider>
    </MemoryRouter>
  )
}
