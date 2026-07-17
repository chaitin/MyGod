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
    const onRequestEmailCode = vi.fn().mockResolvedValue({
      retryAfterSeconds: 5,
    })
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
    expect(screen.getByRole("button", { name: "5 秒" })).toBeDisabled()

    await user.type(screen.getByLabelText("验证码"), "12a345678")
    expect(screen.getByLabelText("验证码")).toHaveValue("12345678")
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(onEmailCodeLogin).toHaveBeenCalledWith({
        code: "12345678",
        email: "alice@example.com",
      })
    })
    expect(
      window.localStorage.getItem("client-web:remembered-email-code-login")
    ).toBe("alice@example.com")
  })

  it("restores the email from the last successful email-code login", () => {
    window.localStorage.setItem(
      "client-web:remembered-email-code-login",
      "alice@example.com"
    )

    render(<LoginForm onEmailCodeLogin={vi.fn()} />)

    expect(screen.getByLabelText("邮箱")).toHaveValue("alice@example.com")
  })

  it("does not remember the email when email-code login fails", async () => {
    const user = userEvent.setup()
    const onEmailCodeLogin = vi.fn().mockRejectedValue(new Error("验证码错误"))
    render(<LoginForm onEmailCodeLogin={onEmailCodeLogin} />)

    await user.type(screen.getByLabelText("邮箱"), "alice@example.com")
    await user.type(screen.getByLabelText("验证码"), "12345678")
    await user.click(screen.getByRole("button", { name: "登录" }))

    await waitFor(() => {
      expect(onEmailCodeLogin).toHaveBeenCalled()
    })
    expect(
      window.localStorage.getItem("client-web:remembered-email-code-login")
    ).toBeNull()
  })

  it("hides email-code login when the server has not enabled it", () => {
    render(<LoginForm emailCodeLoginEnabled={false} onLogin={vi.fn()} />)

    expect(
      screen.queryByRole("tab", { name: "验证码登录" })
    ).not.toBeInTheDocument()
    expect(screen.getByRole("tab", { name: "密码登录" })).toHaveAttribute(
      "aria-selected",
      "true"
    )
  })

  it("hides password login when the server has disabled it", () => {
    render(
      <LoginForm
        emailCodeLoginEnabled
        onEmailCodeLogin={vi.fn()}
        passwordLoginEnabled={false}
      />
    )

    expect(
      screen.queryByRole("tab", { name: "密码登录" })
    ).not.toBeInTheDocument()
    expect(screen.getByRole("tab", { name: "验证码登录" })).toHaveAttribute(
      "aria-selected",
      "true"
    )
  })

  it("hides local login forms when both methods are disabled", () => {
    render(
      <LoginForm emailCodeLoginEnabled={false} passwordLoginEnabled={false}>
        <button type="button">第三方登录</button>
      </LoginForm>
    )

    expect(screen.queryByRole("tab")).not.toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "第三方登录" })
    ).toBeInTheDocument()
  })
})
