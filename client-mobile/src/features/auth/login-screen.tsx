import { Redirect, useRouter } from "expo-router"
import { useRef, useState } from "react"
import {
  AlertDialog,
  Button,
  Card,
  H3,
  Image,
  Input,
  Label,
  Paragraph,
  Spinner,
  type TamaguiElement,
  XStack,
  YStack,
} from "tamagui"

import { appConfig } from "@/config/app-config"
import { KeyboardAwareScreen } from "@/components/layout/keyboard-aware-screen"
import { ApiRequestError } from "@/data/api-client"
import { useCachedAppInfo, useLoginMutation } from "@/data/hooks"
import { useAuth } from "@/features/auth/auth-context"
import { SelectedServerButton } from "@/features/servers/selected-server-button"
import { useServers } from "@/features/servers/server-context"

const ACCOUNT_INPUT_ID = "login-account"
const PASSWORD_INPUT_ID = "login-password"

export function LoginScreen() {
  const router = useRouter()
  const { isAuthenticated } = useAuth()
  const { isHydrated, selectedServer } = useServers()
  const appInfoQuery = useCachedAppInfo(selectedServer)
  const loginMutation = useLoginMutation(selectedServer)
  const accountInputRef = useRef<TamaguiElement>(null)
  const passwordInputRef = useRef<TamaguiElement>(null)
  const [account, setAccount] = useState("")
  const [password, setPassword] = useState("")
  const [loginError, setLoginError] = useState<string | null>(null)
  const canSignIn = account.trim().length > 0 && password.length > 0
  const appName = appInfoQuery.data?.appName ?? appConfig.name
  const organizationName =
    appInfoQuery.data?.organizationName ?? appConfig.organizationName

  if (isAuthenticated) {
    return <Redirect href="/(app)/(tabs)/messages" />
  }

  if (!isHydrated || !appInfoQuery.data) {
    return <Redirect href="/init" />
  }

  async function handleSignIn() {
    if (!canSignIn || loginMutation.isPending) {
      return
    }

    setLoginError(null)

    try {
      await loginMutation.mutateAsync({ account, password })
      router.replace("/init")
    } catch (error: unknown) {
      setLoginError(
        error instanceof ApiRequestError ? error.message : "登录失败"
      )
    }
  }

  return (
    <>
      <KeyboardAwareScreen
        items="center"
        justify="center"
        px="$5"
        py="$8"
      >
        <YStack gap="$6" maxW={440} width="100%">
          <YStack gap="$3" items="center">
            <Image
              alt={`${appName} Logo`}
              height="$8"
              src={require("../../../assets/images/logo.png")}
              width="$8"
            />
            <H3 text="center">{appName} 智能协作平台</H3>
            <Paragraph color="$color10" text="center">
              登录到{organizationName}的工作空间
            </Paragraph>
          </YStack>

          <YStack gap="$3">
            <SelectedServerButton disabled={loginMutation.isPending} />

            <Card size="$5">
              <YStack gap="$4" p="$4">
                <YStack gap="$2">
                  <Label htmlFor={ACCOUNT_INPUT_ID}>账号</Label>
                  <Input
                    autoCapitalize="none"
                    autoComplete="email"
                    disabled={loginMutation.isPending}
                    id={ACCOUNT_INPUT_ID}
                    keyboardType="email-address"
                    onChangeText={setAccount}
                    onSubmitEditing={() => passwordInputRef.current?.focus()}
                    placeholder="请输入邮箱或账号"
                    ref={accountInputRef}
                    returnKeyType="next"
                    value={account}
                  />
                </YStack>

                <YStack gap="$2">
                  <Label htmlFor={PASSWORD_INPUT_ID}>密码</Label>
                  <Input
                    autoCapitalize="none"
                    autoComplete="password"
                    disabled={loginMutation.isPending}
                    id={PASSWORD_INPUT_ID}
                    onChangeText={setPassword}
                    onSubmitEditing={() => void handleSignIn()}
                    placeholder="请输入密码"
                    ref={passwordInputRef}
                    returnKeyType="done"
                    secureTextEntry
                    value={password}
                  />
                </YStack>

                <Button
                  disabled={!canSignIn || loginMutation.isPending}
                  icon={loginMutation.isPending ? <Spinner /> : undefined}
                  onPress={() => void handleSignIn()}
                  size="$5"
                  theme="blue"
                >
                  {loginMutation.isPending ? "登录中…" : "登录"}
                </Button>
              </YStack>
            </Card>
          </YStack>
        </YStack>
      </KeyboardAwareScreen>

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) {
            setLoginError(null)
          }
        }}
        open={loginError !== null}
      >
        <AlertDialog.Portal>
          <AlertDialog.Overlay bg="$shadow6" opacity={0.5} />
          <AlertDialog.Content bordered elevate gap="$4" maxW={440} width="90%">
            <AlertDialog.Title>登录失败</AlertDialog.Title>
            <AlertDialog.Description>{loginError}</AlertDialog.Description>
            <XStack justify="flex-end">
              <AlertDialog.Action asChild>
                <Button theme="blue">知道了</Button>
              </AlertDialog.Action>
            </XStack>
          </AlertDialog.Content>
        </AlertDialog.Portal>
      </AlertDialog>
    </>
  )
}
