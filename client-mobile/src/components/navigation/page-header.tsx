import { ArrowLeft } from "lucide-react-native"
import { useSafeAreaInsets } from "react-native-safe-area-context"
import { Button, H5, Separator, XStack, YStack } from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"

export function PageHeader({
  actionLabel,
  onActionPress,
  onBackPress,
  title,
}: {
  actionLabel?: string
  onActionPress?: () => void
  onBackPress: () => void
  title: string
}) {
  const insets = useSafeAreaInsets()

  return (
    <YStack bg="$background" pt={insets.top}>
      <XStack height={56} items="center" px="$2">
        <XStack width={72}>
          <Button
            aria-label="返回"
            chromeless
            circular
            icon={<ThemedIcon icon={ArrowLeft} size={22} />}
            onPress={onBackPress}
            size="$4"
          />
        </XStack>

        <H5 flex={1} numberOfLines={1} text="center">
          {title}
        </H5>

        <XStack justify="flex-end" width={72}>
          {actionLabel && onActionPress ? (
            <Button
              aria-label={actionLabel}
              chromeless
              onPress={onActionPress}
              size="$4"
            >
              {actionLabel}
            </Button>
          ) : null}
        </XStack>
      </XStack>
      <Separator />
    </YStack>
  )
}
