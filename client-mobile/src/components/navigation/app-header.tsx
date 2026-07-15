import { Menu, type LucideIcon } from "lucide-react-native"
import { useSafeAreaInsets } from "react-native-safe-area-context"
import { Button, H5, Separator, XStack, YStack } from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"

export type HeaderAction = {
  icon: LucideIcon
  label: string
  onPress: () => void
}

export function AppHeader({
  actions = [],
  onMenuPress,
  title,
}: {
  actions?: HeaderAction[]
  onMenuPress: () => void
  title: string
}) {
  const insets = useSafeAreaInsets()

  return (
    <YStack bg="$background" pt={insets.top}>
      <XStack height={60} items="center" px="$3">
        <XStack width={80}>
          <Button
            accessibilityLabel="打开菜单"
            aria-label="打开菜单"
            chromeless
            circular
            icon={<ThemedIcon icon={Menu} size={22} />}
            onPress={onMenuPress}
            size="$4"
          />
        </XStack>

        <H5 flex={1} numberOfLines={1} text="center">
          {title}
        </H5>

        <XStack gap="$1" justify="flex-end" width={80}>
          {actions.slice(0, 2).map((action) => (
            <Button
              accessibilityLabel={action.label}
              aria-label={action.label}
              chromeless
              circular
              icon={<ThemedIcon icon={action.icon} />}
              key={action.label}
              onPress={action.onPress}
              size="$4"
            />
          ))}
        </XStack>
      </XStack>
      <Separator />
    </YStack>
  )
}
