import { type LucideIcon } from "lucide-react-native"
import { useTheme } from "tamagui"

export function ThemedIcon({
  icon: Icon,
  size = 20,
}: {
  icon: LucideIcon
  size?: number
}) {
  const theme = useTheme()

  return <Icon color={String(theme.color.val)} size={size} />
}
