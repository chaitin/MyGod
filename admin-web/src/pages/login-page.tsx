import type { FormEvent } from "react"
import { useId, useState } from "react"
import { EyeIcon, EyeOffIcon } from "lucide-react"
import { Navigate, useLocation, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { useProductInfo } from "@/components/product-info-provider"
import { Card, CardContent, CardFooter } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { AdminLoginRequestError, adminLogin, setAuthSession } from "@/lib/auth"
import { defaultConsolePage } from "@/lib/console-pages"

type LoginPageProps = {
  authenticated: boolean
  onLogin: () => void
}

type LoginLocationState = {
  from?: {
    pathname?: string
    search?: string
  }
}

export default function LoginPage({ authenticated, onLogin }: LoginPageProps) {
  const { appName } = useProductInfo()
  const accountId = useId()
  const passwordId = useId()
  const location = useLocation()
  const navigate = useNavigate()
  const [account, setAccount] = useState("")
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)
  const [password, setPassword] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const redirectTo = getRedirectPath(location.state)

  if (authenticated) {
    return <Navigate replace to={redirectTo} />
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    setError("")
    setPending(true)

    try {
      await adminLogin({
        account,
        password,
      })

      setAuthSession()
      onLogin()
      navigate(redirectTo, { replace: true })
    } catch (loginError) {
      const message = getLoginErrorMessage(loginError)

      setError(message)
      toast.error(message)
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="flex min-h-svh items-center justify-center bg-background p-6">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <h1 className="text-left text-2xl font-medium">
          {appName}管理控制面板
        </h1>
        <form onSubmit={handleSubmit}>
          <Card>
            <CardContent>
              <FieldGroup className="gap-4">
                <Field data-invalid={Boolean(error)}>
                  <FieldLabel htmlFor={accountId}>账号</FieldLabel>
                  <Input
                    aria-invalid={Boolean(error)}
                    autoComplete="username"
                    disabled={pending}
                    id={accountId}
                    onChange={(event) => {
                      setAccount(event.target.value)
                      setError("")
                    }}
                    placeholder="请输入账号"
                    value={account}
                  />
                </Field>
                <Field data-invalid={Boolean(error)}>
                  <FieldLabel htmlFor={passwordId}>密码</FieldLabel>
                  <InputGroup>
                    <InputGroupInput
                      aria-invalid={Boolean(error)}
                      autoComplete="current-password"
                      disabled={pending}
                      id={passwordId}
                      onChange={(event) => {
                        setPassword(event.target.value)
                        setError("")
                      }}
                      placeholder="请输入密码"
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
              </FieldGroup>
            </CardContent>
            <CardFooter>
              <Button className="w-full" disabled={pending} type="submit">
                {pending ? "登录中..." : "登录"}
              </Button>
            </CardFooter>
          </Card>
        </form>
      </div>
    </main>
  )
}

function getLoginErrorMessage(error: unknown) {
  if (error instanceof AdminLoginRequestError) {
    return error.message
  }

  return "登录失败，请稍后重试"
}

function getRedirectPath(state: unknown) {
  const from = (state as LoginLocationState | null)?.from

  if (!from?.pathname || from.pathname === "/login") {
    return defaultConsolePage
  }

  return `${from.pathname}${from.search ?? ""}`
}
