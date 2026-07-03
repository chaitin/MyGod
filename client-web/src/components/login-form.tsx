import { useState, type FormEvent } from "react"
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

type LoginCredentials = {
  account: string
  password: string
}

const rememberedLoginStorageKey = "client-web:remembered-login"

type RememberedLoginCredentials = LoginCredentials

export function LoginForm({
  className,
  onLogin,
  ...props
}: React.ComponentProps<"div"> & {
  onLogin?: (credentials: LoginCredentials) => Promise<void> | void
}) {
  const [rememberedCredentials] = useState(readRememberedLoginCredentials)
  const [account, setAccount] = useState(rememberedCredentials?.account ?? "")
  const [pending, setPending] = useState(false)
  const [password, setPassword] = useState(
    rememberedCredentials?.password ?? ""
  )
  const [rememberCredentials, setRememberCredentials] = useState(
    Boolean(rememberedCredentials)
  )
  const [showPassword, setShowPassword] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    setPending(true)

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
      setPending(false)
    }
  }

  return (
    <div className={cn("flex flex-col gap-6", className)} {...props}>
      <Card>
        <CardContent>
          <form onSubmit={handleSubmit}>
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
                      aria-label={showPassword ? "隐藏密码" : "显示密码"}
                      aria-pressed={showPassword}
                      disabled={pending}
                      onClick={() => setShowPassword((visible) => !visible)}
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
                <Label htmlFor="remember-credentials">记住账号密码</Label>
              </div>
              <Field>
                <Button disabled={pending} type="submit">
                  {pending && (
                    <Loader2Icon aria-hidden="true" className="animate-spin" />
                  )}
                  登录
                </Button>
              </Field>
            </FieldGroup>
          </form>
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
