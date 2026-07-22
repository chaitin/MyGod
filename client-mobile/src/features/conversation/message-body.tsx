import {
  BarChart3,
  ChevronDown,
  ChevronUp,
  Download,
  ExternalLink,
  FileText,
  ImageIcon,
  Link as LinkIcon,
  MessagesSquare,
} from "lucide-react-native"
import { useRef, useState } from "react"
import { Alert, Linking, Pressable } from "react-native"
import {
  Button,
  Card,
  Image,
  Paragraph,
  Separator,
  SizableText,
  Spinner,
  XStack,
  YStack,
} from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"
import type { ClientMessageBody } from "@/data/models"
import type { ResourceLoadState } from "@/data/resources"
import type { EntityReference } from "@/domain/entities/entity-profile"
import {
  formatClientMessageBodySummary,
  formatFileSize,
  type MessageMentionLabelResolver,
} from "@/domain/messages/message-presenter"
import { MarkdownMessage } from "@/features/conversation/markdown-message"
import { MessageMentionText } from "@/features/conversation/message-mention-text"
import { VoiceMessagePlayer } from "@/features/conversation/voice-message-player"

export function MessageBody({
  body,
  currentUserId,
  onImagePress,
  onMentionPress,
  onResourceError,
  onResourcePress,
  onVoiceResourcePress,
  resolveMentionLabel,
  resourceStates,
  serverUrl,
}: {
  body: ClientMessageBody
  currentUserId: string
  onImagePress: (fileId: string) => void
  onMentionPress: (target: EntityReference) => void
  onResourceError: (fileId: string) => void
  onResourcePress: (fileId: string) => void
  onVoiceResourcePress: (fileId: string) => void
  resolveMentionLabel: MessageMentionLabelResolver
  resourceStates: ReadonlyMap<string, ResourceLoadState>
  serverUrl: string
}) {
  const retriedImageIds = useRef(new Set<string>())

  if (body.type === "text") {
    return (
      <Paragraph selectable>
        <MessageMentionText
          content={body.content}
          currentUserId={currentUserId}
          onMentionPress={onMentionPress}
          resolveMentionLabel={resolveMentionLabel}
        />
      </Paragraph>
    )
  }

  if (body.type === "markdown") {
    return (
      <MarkdownMessage
        content={body.content}
        currentUserId={currentUserId}
        onMentionPress={onMentionPress}
        resolveMentionLabel={resolveMentionLabel}
        serverUrl={serverUrl}
      />
    )
  }

  if (body.type === "link") {
    return (
      <MessageLinkCard
        description={body.url}
        icon={LinkIcon}
        onPress={() => void openExternalUrl(body.url)}
        title={body.title || "链接"}
      />
    )
  }

  if (body.type === "card") {
    return (
      <MessageLinkCard
        description={body.description}
        icon={ExternalLink}
        onPress={body.url.trim() ? () => void openExternalUrl(body.url) : undefined}
        title={body.title}
      />
    )
  }

  if (body.type === "chart") {
    return (
      <YStack gap="$2" width="100%">
        <XStack gap="$2" items="center">
          <ThemedIcon icon={BarChart3} size={18} />
          <SizableText fontWeight="600">{body.title}</SizableText>
        </XStack>
        {body.description ? (
          <Paragraph color="$color10" size="$2">
            {body.description}
          </Paragraph>
        ) : null}
        <Separator />
        <Paragraph color="$color10" size="$2">
          {formatChartPreview(body.chartType, body.data)}
        </Paragraph>
      </YStack>
    )
  }

  if (body.type === "file") {
    const state = resourceStates.get(body.fileId)
    const isLoading = state?.status === "loading"
    return (
      <XStack gap="$3" items="center" width="100%">
        <ThemedIcon icon={FileText} size={24} />
        <YStack flex={1}>
          <SizableText fontWeight="600" numberOfLines={1}>
            {body.name}
          </SizableText>
          <SizableText color="$color10" size="$2">
            {formatFileSize(body.sizeBytes)}
          </SizableText>
        </YStack>
        <Button
          accessibilityLabel={`打开文件 ${body.name}`}
          chromeless
          circular
          disabled={isLoading}
          icon={
            isLoading ? (
              <Spinner />
            ) : (
              <ThemedIcon icon={Download} size={18} />
            )
          }
          onPress={() => onResourcePress(body.fileId)}
          size="$3"
        />
      </XStack>
    )
  }

  if (body.type === "image") {
    const state = resourceStates.get(body.fileId)
    const resource = state?.resource
    if (!resource) {
      return (
        <XStack
          gap="$2"
          items="center"
          minW={160}
          onPress={() => onImagePress(body.fileId)}
          p="$2"
        >
          {state?.status === "loading" ? (
            <Spinner />
          ) : (
            <ThemedIcon icon={ImageIcon} />
          )}
          <SizableText color="$color10">
            {state?.status === "error" ? "图片加载失败，点击重试" : "正在加载图片"}
          </SizableText>
        </XStack>
      )
    }

    const size = getImageDisplaySize(body.width, body.height)
    return (
      <Pressable
        accessibilityLabel="查看图片"
        onPress={() => onImagePress(body.fileId)}
        style={{
          borderRadius: 7,
          height: size.height,
          overflow: "hidden",
          width: size.width,
        }}
      >
        <Image
          height={size.height}
          objectFit="cover"
          onError={() => {
            if (retriedImageIds.current.has(body.fileId)) return
            retriedImageIds.current.add(body.fileId)
            onResourceError(body.fileId)
          }}
          pointerEvents="none"
          src={resource.uri}
          width={size.width}
        />
      </Pressable>
    )
  }

  if (body.type === "voice") {
    const state = resourceStates.get(body.fileId)
    return (
      <VoiceMessagePlayer
        durationMS={body.durationMS}
        fileId={body.fileId}
        onResourceError={onResourceError}
        onResourceRequest={onVoiceResourcePress}
        state={state}
        transcript={body.transcript}
      />
    )
  }

  if (body.type === "forward_bundle") {
    return (
      <ForwardBundleBody
        body={body}
        resolveMentionLabel={resolveMentionLabel}
      />
    )
  }

  if (body.type === "revoked") {
    return <Paragraph color="$gray11">该消息已被撤回</Paragraph>
  }

  if (body.type === "unsupported") {
    return <Paragraph color="$color10">暂不支持查看该消息</Paragraph>
  }

  return (
    <Paragraph text="center">
      {formatClientMessageBodySummary(body, resolveMentionLabel)}
    </Paragraph>
  )
}

function MessageLinkCard({
  description,
  icon,
  onPress,
  title,
}: {
  description: string
  icon: typeof LinkIcon
  onPress?: () => void
  title: string
}) {
  return (
    <Card
      bg="transparent"
      borderWidth={0}
      gap="$2"
      onPress={onPress}
      p={0}
      width="100%"
    >
      <XStack gap="$2" items="center">
        <ThemedIcon icon={icon} size={18} />
        <SizableText flex={1} fontWeight="600" numberOfLines={1}>
          {title}
        </SizableText>
      </XStack>
      {description.trim() ? (
        <>
          <Separator />
          <Paragraph color="$color10" numberOfLines={4} size="$2">
            {description}
          </Paragraph>
        </>
      ) : null}
    </Card>
  )
}

function ForwardBundleBody({
  body,
  resolveMentionLabel,
}: {
  body: Extract<ClientMessageBody, { type: "forward_bundle" }>
  resolveMentionLabel: MessageMentionLabelResolver
}) {
  const [expanded, setExpanded] = useState(false)
  const visibleItems = expanded ? body.items : body.items.slice(0, 3)

  return (
    <YStack gap="$2" width="100%">
      <XStack gap="$2" items="center">
        <ThemedIcon icon={MessagesSquare} size={18} />
        <SizableText fontWeight="600">聊天记录 · {body.itemCount} 条</SizableText>
      </XStack>
      <Separator />
      {visibleItems.map((item, index) => (
        <YStack gap="$1" key={`${item.sentAt}:${index}`}>
          <SizableText fontWeight="600" size="$2">
            {item.senderName}
          </SizableText>
          <Paragraph color="$color10" numberOfLines={2} size="$2">
            {item.summary.trim() ||
              formatClientMessageBodySummary(item.body, resolveMentionLabel)}
          </Paragraph>
        </YStack>
      ))}
      {body.items.length > 3 ? (
        <Button
          chromeless
          iconAfter={
            <ThemedIcon icon={expanded ? ChevronUp : ChevronDown} size={16} />
          }
          onPress={() => setExpanded((current) => !current)}
          size="$2"
        >
          {expanded ? "收起" : `查看全部 ${body.items.length} 条`}
        </Button>
      ) : null}
    </YStack>
  )
}

function formatChartPreview(
  chartType: Extract<ClientMessageBody, { type: "chart" }>["chartType"],
  data: Record<string, unknown>
) {
  const label =
    chartType === "line"
      ? "折线图"
      : chartType === "bar"
        ? "柱状图"
        : chartType === "pie"
          ? "饼图"
          : "雷达图"
  const values =
    chartType === "pie"
      ? Array.isArray(data.items)
        ? data.items
            .slice(0, 5)
            .map((item) => {
              const value = asRecord(item)
              return typeof value?.name === "string" && typeof value.value === "number"
                ? `${value.name} ${value.value}`
                : ""
            })
            .filter(Boolean)
            .join(" · ")
        : ""
      : Array.isArray(data.labels)
        ? data.labels.filter((item): item is string => typeof item === "string").join(" · ")
        : Array.isArray(data.axes)
          ? data.axes
              .map((item) => asRecord(item)?.name)
              .filter((item): item is string => typeof item === "string")
              .join(" · ")
          : ""

  return values ? `${label} · ${values}` : label
}

function getImageDisplaySize(width?: number, height?: number) {
  const displayWidth = 240
  if (!width || !height) return { height: 180, width: displayWidth }
  return {
    height: Math.min(300, Math.max(120, (displayWidth * height) / width)),
    width: displayWidth,
  }
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

async function openExternalUrl(url: string) {
  try {
    await Linking.openURL(url)
  } catch {
    Alert.alert("无法打开", "这个链接暂时无法打开。")
  }
}
