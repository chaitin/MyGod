import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { LoginForm } from "@/components/login-form"

describe("LoginForm", () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it("keeps account and password login available", async () => {
    const user = userEvent.setup()
    const onLogin = vi.fn()
    render(<LoginForm onLogin={onLogin} />)

    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "验证码登录",
      "密码登录",
    ])
    expect(screen.getByRole("tab", { name: "验证码登录" })).toHaveAttribute(
      "aria-selected",
      "true"
    )
    await user.click(screen.getByRole("tab", { name: "密码登录" }))
    await user.type(screen.getByLabelText("账号"), "alice@example.com")
    await user.type(screen.getByLabelText("密码"), "secret")
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(onLogin).toHaveBeenCalledWith({
        account: "alice@example.com",
        password: "secret",
      })
    })
  })

  it("requests a code and submits the email-code login form", async () => {
    const user = userEvent.setup()
    const onEmailCodeLogin = vi.fn()
    const onRequestEmailCode = vi.fn()
    render(
      <LoginForm
        onEmailCodeLogin={onEmailCodeLogin}
        onRequestEmailCode={onRequestEmailCode}
      />
    )

    await user.click(screen.getByRole("tab", { name: "验证码登录" }))
    await user.type(screen.getByLabelText("邮箱"), "alice@example.com")
    await user.click(screen.getByRole("button", { name: "获取验证码" }))

    await waitFor(() => {
      expect(onRequestEmailCode).toHaveBeenCalledWith("alice@example.com")
    })
    expect(screen.getByRole("button", { name: "60 秒" })).toBeDisabled()

    await user.type(screen.getByLabelText("验证码"), "12a3456")
    expect(screen.getByLabelText("验证码")).toHaveValue("123456")
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(onEmailCodeLogin).toHaveBeenCalledWith({
        code: "123456",
        email: "alice@example.com",
      })
    })
  })
})
