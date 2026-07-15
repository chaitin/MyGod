import { useQueryClient } from "@tanstack/react-query"
import { useRouter } from "expo-router"
import { useEffect, useState } from "react"
import { SafeAreaView } from "react-native-safe-area-context"
import { Button, H4, Image, Paragraph, Spinner, YStack } from "tamagui"

import { ApiRequestError, isUnauthorizedError } from "@/data/api-client"
import {
  appInfoQueryOptions,
  contactsQueryOptions,
  conversationsQueryOptions,
  currentUserQueryOptions,
  queryKeys,
} from "@/data/query"
import { useAuth } from "@/features/auth/auth-context"
import { useServers } from "@/features/servers/server-context"

const MINIMUM_LOADING_TIME_MS = 1_000

type InitState =
  | { status: "loading" }
  | { message: string; status: "error" }

export function InitScreen() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { signIn, signOut } = useAuth()
  const { isHydrated, selectedServer } = useServers()
  const [attempt, setAttempt] = useState(0)
  const [state, setState] = useState<InitState>({ status: "loading" })

  useEffect(() => {
    if (!isHydrated) {
      return
    }

    let isActive = true
    const minimumLoading = wait(MINIMUM_LOADING_TIME_MS)
    const server = {
      id: selectedServer.id,
      url: selectedServer.url,
    }

    async function initialize() {
      await Promise.resolve()

      if (!isActive) {
        return
      }

      setState({ status: "loading" })
      signOut()

      try {
        queryClient.removeQueries({
          exact: true,
          queryKey: queryKeys.appInfo(server),
        })

        const appInfo = await queryClient.fetchQuery(
          appInfoQueryOptions(server)
        )

        if (!appInfo.authenticated) {
          queryClient.removeQueries({
            exact: true,
            queryKey: queryKeys.contacts(server),
          })
          queryClient.removeQueries({
            exact: true,
            queryKey: queryKeys.conversations(server),
          })
          queryClient.removeQueries({
            exact: true,
            queryKey: queryKeys.currentUser(server),
          })
          await minimumLoading

          if (isActive) {
            router.replace("/login")
          }
          return
        }

        queryClient.removeQueries({
          exact: true,
          queryKey: queryKeys.contacts(server),
        })
        queryClient.removeQueries({
          exact: true,
          queryKey: queryKeys.conversations(server),
        })
        queryClient.removeQueries({
          exact: true,
          queryKey: queryKeys.currentUser(server),
        })

        await Promise.all([
          queryClient.fetchQuery(currentUserQueryOptions(server)),
          queryClient.fetchQuery(contactsQueryOptions(server)),
          queryClient.fetchQuery(conversationsQueryOptions(server)),
          minimumLoading,
        ])

        if (isActive) {
          signIn({ serverUrl: selectedServer.url })
          router.replace("/(app)/(tabs)/messages")
        }
      } catch (error: unknown) {
        await minimumLoading

        if (!isActive) {
          return
        }

        if (isUnauthorizedError(error)) {
          router.replace("/login")
          return
        }

        setState({
          message:
            error instanceof ApiRequestError
              ? error.message
              : "加载工作区失败",
          status: "error",
        })
      }
    }

    void initialize()

    return () => {
      isActive = false
    }
  }, [
    attempt,
    isHydrated,
    queryClient,
    router,
    selectedServer.id,
    selectedServer.url,
    signIn,
    signOut,
  ])

  return (
    <SafeAreaView style={{ flex: 1 }}>
      <YStack
        bg="$background"
        flex={1}
        gap="$5"
        items="center"
        justify="center"
        p="$6"
      >
        <Image
          alt="MagicChat Logo"
          height="$8"
          src={require("../../../assets/images/logo.png")}
          width="$8"
        />

        {state.status === "loading" ? (
          <YStack gap="$3" items="center">
            <Spinner size="large" />
            <H4 text="center">正在加载数据</H4>
            <Paragraph color="$color10" text="center">
              正在连接 {selectedServer.name}
            </Paragraph>
          </YStack>
        ) : (
          <YStack gap="$4" items="center" maxW={360} width="100%">
            <H4 text="center">数据加载失败</H4>
            <Paragraph color="$color10" text="center">
              {state.message}
            </Paragraph>
            <Button
              onPress={() => {
                setState({ status: "loading" })
                setAttempt((current) => current + 1)
              }}
              theme="blue"
              width="100%"
            >
              重试
            </Button>
            <Button
              onPress={() => router.push("/server-management")}
              variant="outlined"
              width="100%"
            >
              服务器管理
            </Button>
          </YStack>
        )}
      </YStack>
    </SafeAreaView>
  )
}

function wait(durationMs: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, durationMs)
  })
}
