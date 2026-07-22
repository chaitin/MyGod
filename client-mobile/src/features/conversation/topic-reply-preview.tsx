import { MessagesSquare } from "lucide-react-native"
import { Pressable } from "react-native"
import { Avatar, Separator, SizableText, XStack, YStack } from "tamagui"

import { CachedAvatarImage } from "@/components/avatar/cached-avatar-image"
import { ThemedIcon } from "@/components/icons/themed-icon"
import type { ServerTarget } from "@/data/query"
import type { PresentedMessage } from "@/domain/messages/message-presenter"

export function TopicReplyPreview({
  onOpen,
  server,
  topic,
}: {
  onOpen: () => void
  server: ServerTarget
  topic: NonNullable<PresentedMessage["topic"]>
}) {
  const latestReplyTime = topic.recentReplies.at(-1)?.time ?? ""

  return (
    <Pressable
      accessibilityLabel="查看话题"
      accessibilityRole="button"
      onPress={onOpen}
    >
      {({ pressed }) => (
        <YStack mt="$3" opacity={pressed ? 0.72 : 1} width="100%">
          <Separator borderColor="$borderColor" mb="$2" />

          {topic.recentReplies.length > 0 ? (
            <>
              <YStack gap="$2">
                {topic.recentReplies.map((reply) => (
                  <XStack gap="$2" items="center" key={reply.id} minW={0}>
                    <TopicReplyAvatar
                      avatar={reply.avatar}
                      name={reply.author}
                      server={server}
                    />
                    <SizableText flex={1} minW={0} numberOfLines={1} size="$2">
                      <SizableText fontWeight="600" size="$2">
                        {reply.author}
                      </SizableText>
                      <SizableText color="$color10" size="$2">
                        ：{reply.summary}
                      </SizableText>
                    </SizableText>
                  </XStack>
                ))}
              </YStack>
              <Separator borderColor="$borderColor" my="$3" />
            </>
          ) : null}

          <XStack gap="$3" items="center" justify="space-between">
            <XStack gap="$2" items="center">
              <ThemedIcon icon={MessagesSquare} size={17} />
              <SizableText color="$color10" fontWeight="600" size="$3">
                查看话题
              </SizableText>
            </XStack>
            {latestReplyTime ? (
              <SizableText color="$color10" size="$2">
                {latestReplyTime}
              </SizableText>
            ) : null}
          </XStack>
        </YStack>
      )}
    </Pressable>
  )
}

function TopicReplyAvatar({
  avatar,
  name,
  server,
}: {
  avatar: string
  name: string
  server: ServerTarget
}) {
  return (
    <Avatar rounded="$2" size={20}>
      <CachedAvatarImage avatar={avatar} server={server} />
      <Avatar.Fallback bg="$backgroundFocus" items="center" justify="center">
        <SizableText fontSize={9} fontWeight="600" lineHeight={11}>
          {Array.from(name.trim())[0]?.toUpperCase() ?? "?"}
        </SizableText>
      </Avatar.Fallback>
    </Avatar>
  )
}
