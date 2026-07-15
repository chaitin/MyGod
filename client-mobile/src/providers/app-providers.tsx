import { focusManager, QueryClientProvider } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { AppState, useColorScheme } from "react-native"
import { GestureHandlerRootView } from "react-native-gesture-handler"
import { SafeAreaProvider } from "react-native-safe-area-context"
import { TamaguiProvider } from "tamagui"

import { tamaguiConfig } from "../../tamagui.config"
import { createClientQueryClient } from "@/data/query"
import { AuthProvider } from "@/features/auth/auth-context"
import { ServerProvider } from "@/features/servers/server-context"
import { ClientDataProvider } from "@/providers/client-data-provider"
import { RealtimeProvider } from "@/providers/realtime-provider"

export function AppProviders({ children }: React.PropsWithChildren) {
  const colorScheme = useColorScheme()
  const theme = colorScheme === "dark" ? "dark" : "light"
  const [queryClient] = useState(createClientQueryClient)

  useEffect(() => {
    const subscription = AppState.addEventListener("change", (status) => {
      focusManager.setFocused(status === "active")
    })

    return () => subscription.remove()
  }, [])

  return (
    <QueryClientProvider client={queryClient}>
      <GestureHandlerRootView style={{ flex: 1 }}>
        <SafeAreaProvider>
          <TamaguiProvider config={tamaguiConfig} defaultTheme={theme}>
            <ServerProvider>
              <AuthProvider>
                <ClientDataProvider>
                  <RealtimeProvider>{children}</RealtimeProvider>
                </ClientDataProvider>
              </AuthProvider>
            </ServerProvider>
          </TamaguiProvider>
        </SafeAreaProvider>
      </GestureHandlerRootView>
    </QueryClientProvider>
  )
}
