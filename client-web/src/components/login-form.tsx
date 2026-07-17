import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react"
import { EyeIcon, EyeOffIcon, Loader2Icon } from "lucide-react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

type LoginCredentials = {
  account: string
  password: string
}

type EmailCodeLoginCredentials = {
  code: string
  email: string
}

type EmailCodeRequestResult = {
  retryAfterSeconds: number
}

type LoginMode = "password" | "email-code"

const rememberedLoginStorageKey = "client-web:remembered-login"
const rememberedEmailCodeLoginStorageKey =
  "client-web:remembered-email-code-login"

type RememberedLoginCredentials = LoginCredentials

export function LoginForm({
  children,
  className,
  emailCodeLoginEnabled = true,
  onEmailCodeLogin,
  onLogin,
  onRequestEmailCode,
  passwordLoginEnabled = true,
  submitVariant = "default",
  ...props
}: React.ComponentProps<"div"> & {
  children?: ReactNode
  emailCodeLoginEnabled?: boolean
  onEmailCodeLogin?: (
    credentials: EmailCodeLoginCredentials
  ) => Promise<void> | void
  onLogin?: (credentials: LoginCredentials) => Promise<void> | void
  onRequestEmailCode?: (
    email: string
  ) => Promise<EmailCodeRequestResult> | EmailCodeRequestResult
  passwordLoginEnabled?: boolean
  submitVariant?: "default" | "outline"
}) {
  const [rememberedCredentials] = useState(readRememberedLoginCredentials)
  const [account, setAccount] = useState(rememberedCredentials?.account ?? "")
  const [email, setEmail] = useState(readRememberedEmailCodeLoginEmail)
  const [emailCode, setEmailCode] = useState("")
  const [emailCodeLoginPending, setEmailCodeLoginPending] = useState(false)
  const [loginMode, setLoginMode] = useState<LoginMode>("email-code")
  const [password, setPassword] = useState(
    rememberedCredentials?.password ?? ""
  )
  const [passwordLoginPending, setPasswordLoginPending] = useState(false)
  const [rememberCredentials, setRememberCredentials] = useState(
    Boolean(rememberedCredentials)
  )
  const [requestCodePending, setRequestCodePending] = useState(false)
  const [retryCodeAfter, setRetryCodeAfter] = useState(0)
  const [showPassword, setShowPassword] = useState(false)
  const emailInputRef = useRef<HTMLInputElement>(null)
  const pending =
    passwordLoginPending || emailCodeLoginPending || requestCodePending
  const activeLoginMode = emailCodeLoginEnabled
    ? passwordLoginEnabled
      ? loginMode
      : "email-code"
    : "password"
  const localLoginEnabled = emailCodeLoginEnabled || passwordLoginEnabled

  useEffect(() => {
    if (retryCodeAfter <= 0) {
      return
    }
    const timer = window.setTimeout(() => {
      setRetryCodeAfter((seconds) => Math.max(0, seconds - 1))
    }, 1000)
    return () => window.clearTimeout(timer)
  }, [retryCodeAfter])

  async function handlePasswordSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    setPasswordLoginPending(true)

    try {
      await onLogin?.({
        account,
        password,
      })
      updateRememberedLoginCredentials(rememberCredentials, {
        account,
        password,
      })
    } catch (loginError) {
      toast.error(getLoginErrorMessage(loginError))
    } finally {
      setPasswordLoginPending(false)
    }
  }

  async function handleRequestEmailCode() {
    if (!emailInputRef.current?.reportValidity()) {
      return
    }

    setRequestCodePending(true)
    try {
      if (!onRequestEmailCode) {
        throw new Error("邮箱验证码登录服务暂未接入")
      }
      const result = await onRequestEmailCode(email.trim())
      setRetryCodeAfter(Math.max(1, Math.ceil(result.retryAfterSeconds)))
      toast.success("验证码已发送")
    } catch (requestError) {
      toast.error(getEmailCodeRequestErrorMessage(requestError))
    } finally {
      setRequestCodePending(false)
    }
  }

  async function handleEmailCodeSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    setEmailCodeLoginPending(true)
    try {
      if (!onEmailCodeLogin) {
        throw new Error("邮箱验证码登录服务暂未接入")
      }
      const normalizedEmail = email.trim()
      await onEmailCodeLogin({ code: emailCode, email: normalizedEmail })
      updateRememberedEmailCodeLoginEmail(normalizedEmail)
    } catch (loginError) {
      toast.error(getLoginErrorMessage(loginError))
    } finally {
      setEmailCodeLoginPending(false)
    }
  }

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <CardContent className="flex flex-col gap-5">
          {localLoginEnabled && (
            <Tabs
              onValueChange={(mode) => {
                if (mode === "password" || mode === "email-code") {
                  setLoginMode(mode)
                }
              }}
              value={activeLoginMode}
            >
              <TabsList
                className={cn(
                  "grid w-full",
                  emailCodeLoginEnabled && passwordLoginEnabled
                    ? "grid-cols-2"
                    : "grid-cols-1"
                )}
              >
                {emailCodeLoginEnabled && (
                  <TabsTrigger disabled={pending} value="email-code">
                    验证码登录
                  </TabsTrigger>
                )}
                {passwordLoginEnabled && (
                  <TabsTrigger disabled={pending} value="password">
                    密码登录
                  </TabsTrigger>
                )}
              </TabsList>

              {emailCodeLoginEnabled && (
                <TabsContent className="pt-2" value="email-code">
                  <form onSubmit={handleEmailCodeSubmit}>
                    <FieldGroup className="gap-4">
                      <Field>
                        <FieldLabel htmlFor="email-code-email">邮箱</FieldLabel>
                        <Input
                          autoComplete="email"
                          disabled={pending}
                          id="email-code-email"
                          name="email"
                          onChange={(event) => setEmail(event.target.value)}
                          placeholder="请输入邮箱"
                          ref={emailInputRef}
                          required
                          type="email"
                          value={email}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="email-code">验证码</FieldLabel>
                        <InputGroup>
                          <InputGroupInput
                            autoComplete="one-time-code"
                            disabled={pending}
                            id="email-code"
                            inputMode="numeric"
                            maxLength={8}
                            name="code"
                            onChange={(event) =>
                              setEmailCode(
                                event.target.value
                                  .replace(/\D/g, "")
                                  .slice(0, 8)
                              )
                            }
                            pattern="[0-9]{8}"
                            placeholder="请输入 8 位验证码"
                            required
                            value={emailCode}
                          />
                          <InputGroupAddon align="inline-end">
                            <InputGroupButton
                              className="min-w-20"
                              disabled={pending || retryCodeAfter > 0}
                              onClick={handleRequestEmailCode}
                            >
                              {requestCodePending && (
                                <Loader2Icon
                                  aria-hidden="true"
                                  className="animate-spin"
                                />
                              )}
                              {requestCodePending
                                ? "发送中"
                                : retryCodeAfter > 0
                                  ? `${retryCodeAfter} 秒`
                                  : "获取验证码"}
                            </InputGroupButton>
                          </InputGroupAddon>
                        </InputGroup>
                      </Field>
                      <Field>
                        <Button
                          disabled={pending}
                          type="submit"
                          variant={submitVariant}
                        >
                          {emailCodeLoginPending && (
                            <Loader2Icon
                              aria-hidden="true"
                              className="animate-spin"
                            />
                          )}
                          登录
                        </Button>
                      </Field>
                    </FieldGroup>
                  </form>
                </TabsContent>
              )}

              {passwordLoginEnabled && (
                <TabsContent className="pt-2" value="password">
                  <form onSubmit={handlePasswordSubmit}>
                    <FieldGroup className="gap-4">
                      <Field>
                        <FieldLabel htmlFor="account">账号</FieldLabel>
                        <Input
                          autoComplete="username"
                          disabled={pending}
                          id="account"
                          name="account"
                          onChange={(event) => {
                            setAccount(event.target.value)
                          }}
                          placeholder="输入账号"
                          required
                          type="text"
                          value={account}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="password">密码</FieldLabel>
                        <InputGroup>
                          <InputGroupInput
                            autoComplete="current-password"
                            disabled={pending}
                            id="password"
                            name="password"
                            onChange={(event) => {
                              setPassword(event.target.value)
                            }}
                            placeholder="请输入密码"
                            required
                            type={showPassword ? "text" : "password"}
                            value={password}
                          />
                          <InputGroupAddon align="inline-end">
                            <InputGroupButton
                              aria-label={
                                showPassword ? "隐藏密码" : "显示密码"
                              }
                              aria-pressed={showPassword}
                              disabled={pending}
                              onClick={() =>
                                setShowPassword((visible) => !visible)
                              }
                              size="icon-xs"
                            >
                              {showPassword ? <EyeOffIcon /> : <EyeIcon />}
                            </InputGroupButton>
                          </InputGroupAddon>
                        </InputGroup>
                      </Field>
                      <div className="flex w-fit items-center gap-2 text-sm text-muted-foreground select-none">
                        <Checkbox
                          checked={rememberCredentials}
                          disabled={pending}
                          id="remember-credentials"
                          onCheckedChange={(checked) =>
                            setRememberCredentials(checked === true)
                          }
                        />
                        <Label htmlFor="remember-credentials">
                          记住账号密码
                        </Label>
                      </div>
                      <Field>
                        <Button
                          disabled={pending}
                          type="submit"
                          variant={submitVariant}
                        >
                          {passwordLoginPending && (
                            <Loader2Icon
                              aria-hidden="true"
                              className="animate-spin"
                            />
                          )}
                          登录
                        </Button>
                      </Field>
                    </FieldGroup>
                  </form>
                </TabsContent>
              )}
            </Tabs>
          )}
          {children}
        </CardContent>
      </Card>
    </div>
  )
}

function getLoginErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }

  return "登录失败，请稍后重试"
}

function getEmailCodeRequestErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message
  }

  return "验证码发送失败，请稍后重试"
}

function readRememberedLoginCredentials(): RememberedLoginCredentials | null {
  try {
    const value = window.localStorage.getItem(rememberedLoginStorageKey)

    if (!value) {
      return null
    }

    const parsed = JSON.parse(value) as Partial<RememberedLoginCredentials>

    if (typeof parsed.account !== "string") {
      return null
    }

    if (typeof parsed.password !== "string") {
      return null
    }

    return {
      account: parsed.account,
      password: parsed.password,
    }
  } catch {
    return null
  }
}

function updateRememberedLoginCredentials(
  remember: boolean,
  credentials: RememberedLoginCredentials
) {
  try {
    if (!remember) {
      window.localStorage.removeItem(rememberedLoginStorageKey)
      return
    }

    window.localStorage.setItem(
      rememberedLoginStorageKey,
      JSON.stringify(credentials)
    )
  } catch {
    // Login should not fail just because the browser rejected local storage.
  }
}

function readRememberedEmailCodeLoginEmail() {
  try {
    return window.localStorage.getItem(rememberedEmailCodeLoginStorageKey) ?? ""
  } catch {
    return ""
  }
}

function updateRememberedEmailCodeLoginEmail(email: string) {
  try {
    window.localStorage.setItem(rememberedEmailCodeLoginStorageKey, email)
  } catch {
    // Login should not fail just because the browser rejected local storage.
  }
}
