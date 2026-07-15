import { Avatar, Card, Paragraph, SizableText, XStack, YStack } from "tamagui"

import { MessageBody } from "@/features/conversation/message-body"
import {
  formatClientMessageBodySummary,
  type MessageMentionLabelResolver,
  type PresentedMessage,
} from "@/features/conversation/conversation-message-presenter"
import { resolveServerAssetUrl } from "@/lib/server-asset-url"

export function MessageBubble({
  fileUrls,
  fileUrlsLoading,
  message,
  resolveMentionLabel,
  serverUrl,
}: {
  fileUrls: ReadonlyMap<string, string>
  fileUrlsLoading: boolean
  message: PresentedMessage
  resolveMentionLabel: MessageMentionLabelResolver
  serverUrl: string
}) {
  if (message.role === "system") {
    return (
      <XStack justify="center" px="$5">
        <Card maxW="85%" p="$2">
          <SizableText color="$color10" size="$2" text="center">
            {formatClientMessageBodySummary(message.body, resolveMentionLabel)}
          </SizableText>
        </Card>
      </XStack>
    )
  }

  const fromMe = message.role === "me"
  const avatar = (
    <MessageAvatar
      avatar={message.avatar}
      name={fromMe ? "我" : message.author}
      serverUrl={serverUrl}
    />
  )

  return (
    <XStack gap="$2" justify={fromMe ? "flex-end" : "flex-start"} px="$4">
      {!fromMe ? avatar : null}
      <YStack
        gap="$1"
        items={fromMe ? "flex-end" : "flex-start"}
        maxW="78%"
      >
        <XStack gap="$2" items="center">
          <SizableText color="$color10" numberOfLines={1} size="$2">
            {message.author}
          </SizableText>
          {message.time ? (
            <SizableText color="$color10" size="$1">
              {message.time}
            </SizableText>
          ) : null}
        </XStack>

        <Card p="$3" theme={fromMe ? "blue" : undefined}>
          {message.replyTo ? (
            <YStack borderColor="$borderColor" borderLeftWidth={2} mb="$2" pl="$2">
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
            fileUrls={fileUrls}
            fileUrlsLoading={fileUrlsLoading}
            resolveMentionLabel={resolveMentionLabel}
          />
        </Card>

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
  serverUrl,
}: {
  avatar: string
  name: string
  serverUrl: string
}) {
  const avatarUrl = resolveServerAssetUrl(serverUrl, avatar)

  return (
    <Avatar rounded="$2" size="$3">
      {avatarUrl ? <Avatar.Image src={avatarUrl} /> : null}
      <Avatar.Fallback bg="$backgroundFocus" items="center" justify="center">
        <SizableText fontWeight="600" size="$2">
          {Array.from(name.trim())[0]?.toUpperCase() ?? "?"}
        </SizableText>
      </Avatar.Fallback>
    </Avatar>
  )
}
