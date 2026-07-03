import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ProfileSettingsDialog } from "@/components/profile-settings-dialog"
import type { ClientUser } from "@/lib/client-data-api"

const user: ClientUser = {
  avatar: "/assets/avatars/builtin/17.webp",
  createdAt: "2026-07-01T00:00:00Z",
  email: "alice@example.com",
  id: "user-1",
  lastOnlineAt: null,
  name: "Alice",
  nickname: "Al",
  phone: "+8613912345678",
  status: "active",
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

function createDeferred<T = void>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })

  return { promise, resolve }
}

describe("ProfileSettingsDialog", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("shows readonly profile fields and saves nickname changes inline", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const userEventSetup = userEvent.setup()
    const nicknameSave = createDeferred()
    const onNicknameSave = vi.fn(() => nicknameSave.promise)
    const onOpenChange = vi.fn()

    render(
      <ProfileSettingsDialog
        open
        onNicknameSave={onNicknameSave}
        onOpenChange={onOpenChange}
        user={user}
      />
    )

    const dialog = screen.getByRole("dialog", { name: "设置" })
    expect(dialog).toHaveAttribute("data-slot", "dialog-content")

    expect(within(dialog).getByLabelText("姓名")).toHaveValue("Alice")
    expect(within(dialog).getByLabelText("姓名")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("姓名")).toHaveAttribute("readonly")
    expect(within(dialog).getByLabelText("姓名")).toHaveClass(
      "bg-muted/50",
      "border-muted-foreground/20"
    )
    expect(within(dialog).getByLabelText("邮箱")).toHaveValue(
      "alice@example.com"
    )
    expect(within(dialog).getByLabelText("邮箱")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("邮箱")).toHaveAttribute("readonly")
    expect(within(dialog).getByLabelText("邮箱")).toHaveClass(
      "bg-muted/50",
      "border-muted-foreground/20"
    )
    expect(within(dialog).getByLabelText("手机号")).toHaveValue(
      "+8613912345678"
    )
    expect(within(dialog).getByLabelText("手机号")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("手机号")).toHaveAttribute("readonly")
    expect(within(dialog).getByLabelText("手机号")).toHaveAttribute(
      "placeholder",
      "未设置"
    )
    expect(within(dialog).getByLabelText("手机号")).toHaveClass(
      "bg-muted/50",
      "border-muted-foreground/20"
    )
    expect(within(dialog).getByLabelText("昵称")).toHaveValue("Al")
    expect(within(dialog).getByLabelText("昵称")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("昵称")).toHaveAttribute(
      "placeholder",
      "输入昵称"
    )
    expect(within(dialog).getByLabelText("昵称")).toHaveClass("flex-1")
    expect(
      within(dialog).queryByRole("button", { name: "提交" })
    ).not.toBeInTheDocument()
    expect(
      within(dialog).getByRole("button", { name: "关闭" })
    ).toHaveAttribute("data-variant", "default")

    await userEventSetup.clear(within(dialog).getByLabelText("昵称"))
    await userEventSetup.type(within(dialog).getByLabelText("昵称"), "Alice A")

    const submitNicknameButton = within(dialog).getByRole("button", {
      name: "提交",
    })

    expect(submitNicknameButton).toHaveTextContent("提交")
    expect(submitNicknameButton).toHaveClass("h-9")
    expect(submitNicknameButton).not.toHaveClass("h-8")
    await userEventSetup.click(submitNicknameButton)

    expect(onNicknameSave).toHaveBeenCalledWith("Alice A")
    expect(
      within(dialog).getByRole("button", { name: "提交" })
    ).toBeDisabled()
    expect(within(dialog).queryByText("提交中...")).not.toBeInTheDocument()
    expect(
      within(dialog)
        .getByRole("button", { name: "提交" })
        .querySelector(".animate-spin")
    ).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalled()
    nicknameSave.resolve()
    await waitFor(() =>
      expect(
        within(dialog).queryByRole("button", { name: "提交" })
      ).not.toBeInTheDocument()
    )

    await userEventSetup.click(
      within(dialog).getByRole("button", { name: "关闭" })
    )

    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("supports local avatar edits", async () => {
    vi.stubGlobal("Image", LoadedImage)
    const userEventSetup = userEvent.setup()
    const avatarSave = createDeferred()
    const onAvatarSave = vi.fn(() => avatarSave.promise)
    const onOpenChange = vi.fn()

    render(
      <ProfileSettingsDialog
        open
        onAvatarSave={onAvatarSave}
        onOpenChange={onOpenChange}
        user={user}
      />
    )

    const dialog = screen.getByRole("dialog", { name: "设置" })
    expect(dialog).toHaveAttribute("data-slot", "dialog-content")
    expect(within(dialog).getByRole("img", { name: "Al" })).toHaveAttribute(
      "src",
      "/assets/avatars/builtin/17.webp"
    )
    expect(within(dialog).queryByText("Alice")).not.toBeInTheDocument()
    expect(
      within(dialog).queryByText("alice@example.com")
    ).not.toBeInTheDocument()
    expect(within(dialog).getByLabelText("姓名")).toHaveValue("Alice")
    expect(within(dialog).getByLabelText("姓名")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("姓名")).toHaveAttribute("readonly")
    expect(within(dialog).getByLabelText("姓名")).toHaveClass(
      "bg-muted/50",
      "border-muted-foreground/20"
    )
    expect(within(dialog).getByLabelText("邮箱")).toHaveValue(
      "alice@example.com"
    )
    expect(within(dialog).getByLabelText("邮箱")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("邮箱")).toHaveAttribute("readonly")
    expect(within(dialog).getByLabelText("邮箱")).toHaveClass(
      "bg-muted/50",
      "border-muted-foreground/20"
    )
    expect(within(dialog).getByLabelText("手机号")).toHaveValue(
      "+8613912345678"
    )
    expect(within(dialog).getByLabelText("手机号")).not.toBeDisabled()
    expect(within(dialog).getByLabelText("手机号")).toHaveAttribute("readonly")
    expect(within(dialog).getByLabelText("手机号")).toHaveAttribute(
      "placeholder",
      "未设置"
    )
    expect(within(dialog).getByLabelText("手机号")).toHaveClass(
      "bg-muted/50",
      "border-muted-foreground/20"
    )
    expect(within(dialog).queryByText("状态")).not.toBeInTheDocument()
    expect(within(dialog).queryByText("正常")).not.toBeInTheDocument()
    expect(within(dialog).queryByText("创建时间")).not.toBeInTheDocument()
    expect(
      within(dialog).queryByText("2026-07-01T00:00:00Z")
    ).not.toBeInTheDocument()

    const nicknameInput = within(dialog).getByLabelText("昵称")
    const changeAvatarButton = within(dialog).getByRole("button", {
      name: "更换头像",
    })
    const identityRow = screen.getByTestId("profile-settings-identity-row")

    expect(changeAvatarButton).toHaveAttribute("data-slot", "button")
    expect(nicknameInput).toHaveValue("Al")
    expect(nicknameInput).toHaveAttribute("placeholder", "输入昵称")
    expect(identityRow).toHaveClass("items-start", "gap-4")
    expect(identityRow).not.toHaveClass("justify-center")
    expect(identityRow).toContainElement(changeAvatarButton)
    expect(identityRow).toContainElement(nicknameInput)
    expect(changeAvatarButton).toHaveClass("group/avatar-change", "relative")
    expect(
      changeAvatarButton.querySelector("[data-slot='avatar']")
    ).toHaveClass("size-17")
    expect(
      changeAvatarButton.querySelector("[data-slot='avatar']")
    ).not.toHaveClass("size-20")
    const avatarHoverOverlay = changeAvatarButton.querySelector(
      "[data-slot='avatar-hover-overlay']"
    )
    expect(avatarHoverOverlay).toBeInTheDocument()
    expect(avatarHoverOverlay).toHaveClass(
      "bg-foreground/40",
      "opacity-0",
      "group-hover/avatar-change:opacity-100",
      "group-focus-visible/avatar-change:opacity-100"
    )
    expect(
      changeAvatarButton.querySelector(".lucide-camera")
    ).toBeInTheDocument()
    expect(
      within(dialog).queryAllByRole("button", { name: /选择头像/ })
    ).toHaveLength(0)

    await userEventSetup.clear(nicknameInput)
    await userEventSetup.type(nicknameInput, "Alice A")
    await userEventSetup.click(changeAvatarButton)

    const avatarDialog = await screen.findByRole("dialog", {
      name: "选择头像",
    })
    expect(avatarDialog).toHaveAttribute("data-slot", "dialog-content")
    expect(
      within(avatarDialog).getAllByRole("button", { name: /选择头像/ })
    ).toHaveLength(64)
    expect(
      within(avatarDialog).getByRole("button", { name: "选择头像 17" })
    ).toHaveAttribute("aria-pressed", "true")
    expect(
      within(avatarDialog).getByRole("button", { name: "选择头像 17" })
    ).toHaveAttribute("data-slot", "button")

    await userEventSetup.click(
      within(avatarDialog).getByRole("button", { name: "选择头像 03" })
    )

    expect(nicknameInput).toHaveValue("Alice A")
    expect(screen.getByRole("dialog", { name: "选择头像" })).toBeInTheDocument()
    expect(
      within(avatarDialog).getByRole("button", { name: "选择头像 03" })
    ).toHaveAttribute("aria-pressed", "true")
    expect(
      within(dialog).getByRole("img", { hidden: true, name: "Alice A" })
    ).toHaveAttribute("src", "/assets/avatars/builtin/17.webp")

    await userEventSetup.click(
      within(avatarDialog).getByRole("button", { name: "保存" })
    )

    const avatarSaveButton = within(avatarDialog).getByRole("button", {
      name: "保存",
    })

    expect(onAvatarSave).toHaveBeenCalledWith("/assets/avatars/builtin/03.webp")
    expect(avatarSaveButton).toBeDisabled()
    expect(within(avatarDialog).queryByText("保存中...")).not.toBeInTheDocument()
    expect(avatarSaveButton.querySelector(".animate-spin")).toBeInTheDocument()
    avatarSave.resolve()

    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "选择头像" })
      ).not.toBeInTheDocument()
    )
    expect(
      within(dialog).getByRole("img", { name: "Alice A" })
    ).toHaveAttribute("src", "/assets/avatars/builtin/03.webp")
  })
})
