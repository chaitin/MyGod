import { MoveRight } from "lucide-react"
import { useNavigate } from "react-router"

import { LoginForm } from "@/components/login-form"
import { useAppInfo } from "@/lib/app-info-context"
import { clientLogin } from "@/lib/client-auth"

export function LoginPage() {
  const { appName, organizationName } = useAppInfo()
  const navigate = useNavigate()

  async function handleLogin(credentials: {
    account: string
    password: string
  }) {
    await clientLogin(credentials)
    navigate("/chat", { replace: true })
  }

  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <main className="flex flex-1 items-center justify-center px-4 py-10">
        <div className="flex w-full max-w-sm flex-col gap-6">
          <div className="flex flex-col gap-2">
            <h1 className="text-left text-2xl font-medium">
              {appName} 智能协作平台
            </h1>
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <MoveRight className="size-4" />
              <span>登录到{organizationName}的工作空间</span>
            </p>
          </div>
          <LoginForm className="w-full" onLogin={handleLogin} />
        </div>
      </main>
    </div>
  )
}
