import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { UserSettingsDialog } from "@/components/user-settings-dialog"

const mocks = vi.hoisted(() => ({
  permission: "default" as "default" | "denied" | "granted" | "unsupported",
  playMessageNotificationSound: vi.fn(),
  requestBrowserNotificationPermission: vi.fn(),
}))

vi.mock("@/lib/browser-notifications", () => ({
  getBrowserNotificationPermission: () => mocks.permission,
  requestBrowserNotificationPermission:
    mocks.requestBrowserNotificationPermission,
}))

vi.mock("@/lib/message-notification-sound", () => ({
  playMessageNotificationSound: mocks.playMessageNotificationSound,
}))

describe("UserSettingsDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.permission = "default"
    mocks.requestBrowserNotificationPermission.mockResolvedValue("granted")
  })

  it("unlocks the message sound before requesting desktop notification permission", async () => {
    const user = userEvent.setup()
    render(<UserSettingsDialog onOpenChange={() => undefined} open />)

    await user.click(screen.getByRole("button", { name: "开启桌面通知" }))

    expect(mocks.playMessageNotificationSound).toHaveBeenCalledOnce()
    expect(mocks.requestBrowserNotificationPermission).toHaveBeenCalledOnce()
    expect(
      mocks.playMessageNotificationSound.mock.invocationCallOrder[0]
    ).toBeLessThan(
      mocks.requestBrowserNotificationPermission.mock.invocationCallOrder[0]
    )
    expect(screen.getByText("桌面通知已开启")).toBeInTheDocument()
  })
})
