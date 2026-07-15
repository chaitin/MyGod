import { Bot } from "lucide-react-native"
import { Avatar, SizableText, Text, XStack, YStack } from "tamagui"

import { GroupAvatar } from "@/components/avatar/group-avatar"
import { ThemedIcon } from "@/components/icons/themed-icon"
import type { ClientConversation } from "@/data/models"
import { formatUnreadCount } from "@/features/messages/conversation-list-model"
import { resolveServerAssetUrl } from "@/lib/server-asset-url"

export function ConversationAvatar({
  conversation,
  serverUrl,
}: {
  conversation: ClientConversation
  serverUrl: string
}) {
  const avatarUrl = resolveServerAssetUrl(serverUrl, conversation.avatar)

  return (
    <YStack height="$4" width="$4">
      {conversation.type === "group" ? (
        <GroupAvatar
          avatar={conversation.avatar}
          members={conversation.members}
          name={conversation.name}
          serverUrl={serverUrl}
        />
      ) : (
        <Avatar rounded="$2" size="$4">
          {avatarUrl ? <Avatar.Image src={avatarUrl} /> : null}
          <Avatar.Fallback
            bg="$backgroundFocus"
            items="center"
            justify="center"
          >
            {conversation.type === "app" ? (
              <ThemedIcon icon={Bot} size={18} />
            ) : (
              <Text fontWeight="600">
                {getConversationInitial(conversation.name)}
              </Text>
            )}
          </Avatar.Fallback>
        </Avatar>
      )}

      {conversation.unreadCount > 0 ? (
        <XStack
          accessibilityLabel={`${conversation.unreadCount} 条未读消息`}
          bg="$red10"
          height={18}
          items="center"
          justify="center"
          minW={18}
          position="absolute"
          px="$1"
          r={-7}
          rounded="$10"
          t={-7}
          z={1}
        >
          <SizableText color="white" size="$1">
            {formatUnreadCount(conversation.unreadCount)}
          </SizableText>
        </XStack>
      ) : null}
    </YStack>
  )
}

function getConversationInitial(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?"
}
