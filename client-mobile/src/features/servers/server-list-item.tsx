import { Check } from "lucide-react-native"
import ReanimatedSwipeable, {
  type SwipeableMethods,
} from "react-native-gesture-handler/ReanimatedSwipeable"
import { Button, ListItem } from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"
import type { ServerConfig } from "@/features/servers/server-model"

export function ServerListItem({
  isSelected,
  onDelete,
  onSelect,
  server,
}: {
  isSelected: boolean
  onDelete: () => void
  onSelect: () => void
  server: ServerConfig
}) {
  const content = (
    <ListItem
      bg="$background"
      iconAfter={isSelected ? <ThemedIcon icon={Check} /> : undefined}
      onPress={onSelect}
      size="$5"
      subTitle={server.url}
      title={server.name}
    />
  )

  if (server.isBuiltIn) {
    return content
  }

  return (
    <ReanimatedSwipeable
      friction={2}
      renderRightActions={(
        _progress,
        _translation,
        swipeableMethods: SwipeableMethods
      ) => (
        <Button
          height="100%"
          onPress={() => {
            swipeableMethods.close()
            onDelete()
          }}
          rounded="$0"
          theme="red"
          width={88}
        >
          删除
        </Button>
      )}
      rightThreshold={40}
    >
      {content}
    </ReanimatedSwipeable>
  )
}
