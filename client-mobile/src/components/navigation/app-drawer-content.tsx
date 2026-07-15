import { usePathname, useRouter, type Href } from "expo-router"
import { Check, ChevronRight, LogOut } from "lucide-react-native"
import { SafeAreaView } from "react-native-safe-area-context"
import {
  Avatar,
  Button,
  H4,
  ListItem,
  Paragraph,
  Separator,
  Text,
  useTheme,
  XStack,
  YGroup,
  YStack,
} from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"
import { appConfig } from "@/config/app-config"
import { useAuth } from "@/features/auth/auth-context"
import { appSections } from "@/navigation/app-sections"

export function AppDrawerContent({ closeDrawer }: { closeDrawer: () => void }) {
  const pathname = usePathname()
  const router = useRouter()
  const theme = useTheme()
  const { session, signOut } = useAuth()

  function navigateTo(href: Href) {
    closeDrawer()
    router.replace(href)
  }

  function handleLogout() {
    closeDrawer()
    signOut()
    router.replace("/login")
  }

  return (
    <SafeAreaView
      edges={["top", "bottom"]}
      style={{
        backgroundColor: String(theme.background.val),
        flex: 1,
      }}
    >
      <YStack bg="$background" flex={1}>
        <YStack gap="$4" p="$4">
          <XStack gap="$3" items="center">
            <Avatar circular size="$5" theme="blue">
              <Avatar.Fallback>
                <Text fontWeight="700">MC</Text>
              </Avatar.Fallback>
            </Avatar>
            <YStack flex={1}>
              <H4>{appConfig.name}</H4>
              <Paragraph color="$color10" size="$2">
                AI 工作空间
              </Paragraph>
            </YStack>
          </XStack>
          <Separator />
        </YStack>

        <YStack flex={1} gap="$3" px="$4">
          <Paragraph color="$color10" size="$2">
            工作台
          </Paragraph>
          <YGroup
            borderColor="$borderColor"
            borderWidth={1}
            rounded="$4"
            size="$4"
          >
            {appSections.map((item) => {
              const active = pathname.endsWith(`/${item.routeName}`)

              return (
                <YGroup.Item key={item.routeName}>
                  <ListItem
                    icon={<ThemedIcon icon={item.icon} />}
                    iconAfter={
                      <ThemedIcon icon={active ? Check : ChevronRight} />
                    }
                    onPress={() => navigateTo(item.href)}
                    theme={active ? "blue" : undefined}
                    title={item.label}
                  />
                </YGroup.Item>
              )
            })}
          </YGroup>
        </YStack>

        <YStack gap="$3" p="$4">
          <Separator />
          <YGroup
            borderColor="$borderColor"
            borderWidth={1}
            rounded="$4"
            size="$4"
          >
            <YGroup.Item>
              <ListItem
                icon={
                  <Avatar circular size="$3" theme="blue">
                    <Avatar.Fallback>
                      <Text>演</Text>
                    </Avatar.Fallback>
                  </Avatar>
                }
                subTitle={session?.serverUrl ?? "未选择服务器"}
                title="演示账号"
              />
            </YGroup.Item>
          </YGroup>
          <Button
            icon={<ThemedIcon icon={LogOut} />}
            onPress={handleLogout}
            theme="red"
            variant="outlined"
          >
            退出登录
          </Button>
        </YStack>
      </YStack>
    </SafeAreaView>
  )
}
