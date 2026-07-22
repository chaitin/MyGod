import { useRef, useState } from "react"
import { View } from "react-native"
import {
  Avatar,
  Button,
  Paragraph,
  SizableText,
  XStack,
  YStack,
} from "tamagui"

import { CachedAvatarImage } from "@/components/avatar/cached-avatar-image"
import type { EntityReference } from "@/domain/entities/entity-profile"
import type { ServerTarget } from "@/data/query"
import type { ResourceLoadState } from "@/data/resources"
import { MessageBody } from "@/features/conversation/message-body"
import { MessageReactionChips } from "@/features/conversation/message-reactions"
import { TopicReplyPreview } from "@/features/conversation/topic-reply-preview"
import {
  formatClientMessageBodySummary,
  type MessageMentionLabelResolver,
  type PresentedMessage,
} from "@/domain/messages/message-presenter"

export function MessageBubble({
  currentUserId,
  message,
  canAddReaction,
  onAvatarLongPress,
  onAvatarPress,
  onImagePress,
  onMentionPress,
  onOpenTopic,
  onResourceError,
  onResourcePress,
  onSetReaction,
  onVoiceResourcePress,
  resolveMentionLabel,
  resourceStates,
  server,
}: {
  canAddReaction: boolean
  currentUserId: string
  message: PresentedMessage
  onAvatarLongPress?: (sender: EntityReference) => void
  onAvatarPress: (sender: EntityReference) => void
  onImagePress: (fileId: string) => void
  onMentionPress: (target: EntityReference) => void
  onOpenTopic: (conversationId: string) => void
  onResourceError: (fileId: string) => void
  onResourcePress: (fileId: string) => void
  onSetReaction?: (
    messageId: string,
    text: string,
    reacted: boolean
  ) => Promise<void>
  onVoiceResourcePress: (fileId: string) => void
  resolveMentionLabel: MessageMentionLabelResolver
  resourceStates: ReadonlyMap<string, ResourceLoadState>
  server: ServerTarget
}) {
  const didLongPressAvatarRef = useRef(false)
  const [bubblePressed, setBubblePressed] = useState(false)

  if (message.role === "system") {
    return (
      <XStack justify="center" px="$5">
        <XStack bg="$color4" maxW="85%" p="$2" px="$3" rounded="$10">
          <SizableText color="$color10" size="$2" text="center">
            {formatClientMessageBodySummary(message.body, resolveMentionLabel)}
          </SizableText>
        </XStack>
      </XStack>
    )
  }

  const fromMe = message.role === "me"
  const showsBubblePressFeedback =
    message.body.type === "text" ||
    message.body.type === "markdown" ||
    message.body.type === "revoked" ||
    message.body.type === "unsupported"
  const sender = message.sender
  const flushImageBubble =
    message.body.type === "image" && !message.replyTo && !message.topic
  const usesStructuredBubbleWidth =
    Boolean(message.topic) ||
    message.body.type === "voice" ||
    message.body.type === "file" ||
    message.body.type === "chart" ||
    message.body.type === "forward_bundle" ||
    message.body.type === "link" ||
    message.body.type === "card"
  const avatar = sender ? (
    <Button
      aria-label={`查看${fromMe ? "我的" : message.author}资料`}
      chromeless
      height="$3"
      onLongPress={
        onAvatarLongPress
          ? () => {
              didLongPressAvatarRef.current = true
              onAvatarLongPress(sender)
            }
          : undefined
      }
      onPress={() => {
        if (didLongPressAvatarRef.current) {
          didLongPressAvatarRef.current = false
          return
        }
        onAvatarPress(sender)
      }}
      onPressIn={() => {
        didLongPressAvatarRef.current = false
      }}
      p={0}
      width="$3"
    >
      <MessageAvatar
        avatar={message.avatar}
        name={fromMe ? "我" : message.author}
        server={server}
      />
    </Button>
  ) : (
    <MessageAvatar
      avatar={message.avatar}
      name={fromMe ? "我" : message.author}
      server={server}
    />
  )

  return (
    <XStack
      gap="$2"
      items="flex-start"
      justify={fromMe ? "flex-end" : "flex-start"}
      px="$3"
    >
      {!fromMe ? avatar : null}
      <YStack
        gap="$1"
        items={fromMe ? "flex-end" : "flex-start"}
        maxW="82%"
        width={usesStructuredBubbleWidth ? "66%" : undefined}
      >
        <XStack gap="$2" items="center">
          <SizableText color="$color10" numberOfLines={1} size="$2">
            {message.author}
          </SizableText>
        </XStack>

        <View
          onTouchCancel={
            showsBubblePressFeedback
              ? () => setBubblePressed(false)
              : undefined
          }
          onTouchEnd={
            showsBubblePressFeedback
              ? () => setBubblePressed(false)
              : undefined
          }
          onTouchStart={
            showsBubblePressFeedback
              ? () => setBubblePressed(true)
              : undefined
          }
          style={{
            maxWidth: "100%",
            width: usesStructuredBubbleWidth ? "100%" : undefined,
          }}
        >
          <YStack
            bg={
              bubblePressed
                ? fromMe
                  ? "$color5"
                  : "$color2"
                : fromMe
                  ? "$color4"
                  : "$color1"
            }
            rounded="$5"
            borderTopLeftRadius={fromMe ? "$5" : "$1"}
            borderTopRightRadius={fromMe ? "$1" : "$5"}
            borderWidth={0}
            maxW="100%"
            overflow="hidden"
            p={flushImageBubble ? 0 : "$3"}
            width={usesStructuredBubbleWidth ? "100%" : undefined}
          >
            {message.replyTo ? (
              <YStack
                borderColor="$borderColor"
                borderLeftWidth={2}
                mb="$2"
                pl="$2"
              >
                <SizableText fontWeight="600" numberOfLines={1} size="$2">
                  {message.replyTo.author}
                </SizableText>
                <Paragraph color="$color10" numberOfLines={2} size="$2">
                  {message.replyTo.summary}
                </Paragraph>
              </YStack>
            ) : null}
            <MessageBody
              body={message.body}
              currentUserId={currentUserId}
              onImagePress={onImagePress}
              onMentionPress={onMentionPress}
              onResourceError={onResourceError}
              onResourcePress={onResourcePress}
              onVoiceResourcePress={onVoiceResourcePress}
              resolveMentionLabel={resolveMentionLabel}
              resourceStates={resourceStates}
              serverUrl={server.url}
            />
            {message.reactions.length > 0 ? (
              <YStack
                mb={flushImageBubble ? "$2" : undefined}
                mt="$2"
                px={flushImageBubble ? "$2" : undefined}
              >
                <MessageReactionChips
                  align={fromMe ? "end" : "start"}
                  canAdd={
                    canAddReaction && message.body.type !== "revoked"
                  }
                  onSetReaction={
                    onSetReaction && message.body.type !== "revoked"
                      ? (text, reacted) =>
                          onSetReaction(message.id, text, reacted)
                      : undefined
                  }
                  onUserPress={(user) =>
                    onAvatarPress({ id: user.id, type: "user" })
                  }
                  reactions={message.reactions}
                />
              </YStack>
            ) : null}
            {message.topic ? (
              <TopicReplyPreview
                onOpen={() => onOpenTopic(message.topic!.conversationId)}
                server={server}
                topic={message.topic}
              />
            ) : null}
          </YStack>
        </View>

        {message.delegatedByName ? (
          <SizableText color="$color10" size="$1">
            由 {message.delegatedByName} 代发
          </SizableText>
        ) : null}
      </YStack>
      {fromMe ? avatar : null}
    </XStack>
  )
}

function MessageAvatar({
  avatar,
  name,
  server,
}: {
  avatar: string
  name: string
  server: ServerTarget
}) {
  return (
    <Avatar rounded="$2" size="$3" theme={name === "我" ? "teal" : undefined}>
      <CachedAvatarImage avatar={avatar} server={server} />
      <Avatar.Fallback bg="$backgroundFocus" items="center" justify="center">
        <SizableText fontWeight="600" size="$2">
          {Array.from(name.trim())[0]?.toUpperCase() ?? "?"}
        </SizableText>
      </Avatar.Fallback>
    </Avatar>
  )
}
