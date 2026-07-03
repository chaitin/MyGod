import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { LoginForm } from "@/components/login-form"

function createDeferred() {
  let resolve!: () => void
  const promise = new Promise<void>((promiseResolve) => {
    resolve = promiseResolve
  })

  return { promise, resolve }
}

describe("LoginForm", () => {
  it("keeps the submit text and shows a spinning icon while logging in", async () => {
    const user = userEvent.setup()
    const login = createDeferred()
    const onLogin = vi.fn(() => login.promise)

    render(<LoginForm onLogin={onLogin} />)

    await user.type(screen.getByLabelText("账号"), "alice@example.com")
    await user.type(screen.getByLabelText("密码"), "password")
    await user.click(screen.getByRole("button", { name: "登录" }))

    const submitButton = screen.getByRole("button", { name: "登录" })

    expect(submitButton).toBeDisabled()
    expect(screen.queryByText("登录中...")).not.toBeInTheDocument()
    expect(submitButton.querySelector(".animate-spin")).toBeInTheDocument()

    login.resolve()

    await waitFor(() => expect(submitButton).not.toBeDisabled())
  })
})
