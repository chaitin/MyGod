import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { AppCredentialsDialog } from "@/components/contacts/app-credentials-dialog"
import type { ClientAppCredentials } from "@/lib/client-api/apps"

const mocks = vi.hoisted(() => ({
  regenerateClientAppSecret: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock("@/lib/client-api/apps", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/client-api/apps")>()),
  regenerateClientAppSecret: mocks.regenerateClientAppSecret,
}))

vi.mock("sonner", () => ({
  toast: {
    error: mocks.toastError,
    success: mocks.toastSuccess,
  },
}))

describe("AppCredentialsDialog", () => {
  it("confirms and regenerates the connection secret", async () => {
    const user = userEvent.setup()
    const onCredentialsChange = vi.fn()
    const nextCredentials = createCredentials("new-secret")
    mocks.regenerateClientAppSecret.mockResolvedValueOnce(nextCredentials)

    render(
      <AppCredentialsDialog
        credentials={createCredentials("current-secret")}
        onCredentialsChange={onCredentialsChange}
        onOpenChange={vi.fn()}
        open
      />
    )

    expect(screen.getByRole("dialog", { name: "开发指南" })).toBeInTheDocument()
    expect(screen.getByLabelText("连接密钥")).toHaveValue("current-secret")
    expect(screen.getByLabelText("应用 ID")).toHaveValue("app-1")
    expect(screen.getByRole("link", { name: "开发文档" })).toHaveAttribute(
      "href",
      "https://github.com/chaitin/MagicChat/blob/main/APPLICATION_DEVELOPMENT.md"
    )
    expect(screen.getByRole("link", { name: "开发文档" })).toHaveAttribute(
      "target",
      "_blank"
    )

    await user.click(screen.getByRole("button", { name: "重置连接密钥" }))
    const confirmation = screen.getByRole("alertdialog", {
      name: "重置连接密钥",
    })
    await user.click(screen.getByRole("button", { name: "确认重置" }))

    await waitFor(() =>
      expect(onCredentialsChange).toHaveBeenCalledWith(nextCredentials)
    )
    expect(mocks.regenerateClientAppSecret).toHaveBeenCalledWith("app-1")
    expect(mocks.toastSuccess).toHaveBeenCalledWith("连接密钥已重置")
    expect(confirmation).not.toBeInTheDocument()
  })
})

function createCredentials(connectionSecret: string): ClientAppCredentials {
  return {
    app: {
      avatar: "",
      connectionStatus: "offline",
      createdAt: "2026-07-17T07:00:00Z",
      description: "分析消息",
      enabled: true,
      id: "app-1",
      name: "分析助手",
      updatedAt: "2026-07-17T07:00:00Z",
      userIds: [],
      visibility: "creator",
    },
    connectionSecret,
  }
}
