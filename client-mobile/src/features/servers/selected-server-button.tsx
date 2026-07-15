import { useRouter } from "expo-router"
import { ChevronRight, Server } from "lucide-react-native"
import { Label, ListItem, YStack } from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"
import { useServers } from "@/features/servers/server-context"

export function SelectedServerButton({ disabled = false }: { disabled?: boolean }) {
  const router = useRouter()
  const { selectedServer } = useServers()

  return (
    <YStack gap="$2">
      <Label>服务器</Label>
      <ListItem
        disabled={disabled}
        icon={<ThemedIcon icon={Server} />}
        iconAfter={<ThemedIcon icon={ChevronRight} />}
        onPress={() => router.push("/server-management")}
        size="$5"
        subTitle={selectedServer.url}
        title={selectedServer.name}
      />
    </YStack>
  )
}
