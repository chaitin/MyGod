import { Stack } from "expo-router"
import { StatusBar } from "expo-status-bar"

import { AppProviders } from "@/providers/app-providers"

export default function RootLayout() {
  return (
    <AppProviders>
      <StatusBar style="auto" />
      <Stack screenOptions={{ headerShown: false }}>
        <Stack.Screen name="index" />
        <Stack.Screen name="init" />
        <Stack.Screen name="login" />
        <Stack.Screen name="server-management" />
        <Stack.Screen name="(app)" />
      </Stack>
    </AppProviders>
  )
}
