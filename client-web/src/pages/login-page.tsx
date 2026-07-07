import { LogInIcon, MoveRight } from "lucide-react"
import { useEffect } from "react"
import { useNavigate } from "react-router"

import { Button } from "@/components/ui/button"
import { LoginForm } from "@/components/login-form"
import { Separator } from "@/components/ui/separator"
import { useAppInfo } from "@/lib/app-info-context"
import { clientLogin } from "@/lib/client-auth"

export function LoginPage() {
  const { appName, authenticated, organizationName, thirdPartyProviders } =
    useAppInfo()
  const navigate = useNavigate()
  const hasThirdPartyProviders = thirdPartyProviders.length > 0

  useEffect(() => {
    if (authenticated) {
      navigate("/init", { replace: true })
    }
  }, [authenticated, navigate])

  async function handleLogin(credentials: {
    account: string
    password: string
  }) {
    await clientLogin(credentials)
    navigate("/init", { replace: true })
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
          <LoginForm
            className="w-full"
            onLogin={handleLogin}
            submitVariant={hasThirdPartyProviders ? "outline" : "default"}
          >
            {hasThirdPartyProviders && (
              <div className="flex flex-col gap-4">
                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                  <Separator className="flex-1" />
                  <span className="shrink-0">其他登录方式</span>
                  <Separator className="flex-1" />
                </div>
                <div className="flex flex-col gap-2">
                  {thirdPartyProviders.map((provider, index) => (
                    <Button
                      asChild
                      key={provider.key}
                      variant={index === 0 ? "default" : "outline"}
                    >
                      <a
                        href={`/api/client/auth/third-party/${encodeURIComponent(
                          provider.key
                        )}/start?redirect=/init`}
                      >
                        <LogInIcon data-icon="inline-start" />
                        使用 {provider.name} 登录
                      </a>
                    </Button>
                  ))}
                </div>
              </div>
            )}
          </LoginForm>
        </div>
      </main>
    </div>
  )
}
