import {
  AudioLines,
  BarChart3,
  ChevronDown,
  ChevronUp,
  Download,
  ExternalLink,
  FileText,
  ImageIcon,
  Link as LinkIcon,
  MessagesSquare,
  Play,
} from "lucide-react-native"
import { useState } from "react"
import { Alert, Linking } from "react-native"
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
import {
  formatClientMessageBodySummary,
  formatFileSize,
  formatMarkdownAsPlainText,
  formatMentionTemplateText,
  formatVoiceDuration,
  type MessageMentionLabelResolver,
} from "@/features/conversation/conversation-message-presenter"

export function MessageBody({
  body,
  fileUrls,
  fileUrlsLoading,
  resolveMentionLabel,
}: {
  body: ClientMessageBody
  fileUrls: ReadonlyMap<string, string>
  fileUrlsLoading: boolean
  resolveMentionLabel: MessageMentionLabelResolver
}) {
  if (body.type === "text") {
    return (
      <Paragraph selectable>
        {formatMentionTemplateText(body.content, resolveMentionLabel)}
      </Paragraph>
    )
  }

  if (body.type === "markdown") {
    return (
      <Paragraph selectable>
        {formatMentionTemplateText(
          formatMarkdownAsPlainText(body.content),
          resolveMentionLabel
        )}
      </Paragraph>
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
      <YStack gap="$2" minW={220}>
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
    const url = fileUrls.get(body.fileId)
    return (
      <XStack gap="$3" items="center" minW={220}>
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
          disabled={!url}
          icon={
            fileUrlsLoading && !url ? (
              <Spinner />
            ) : (
              <ThemedIcon icon={Download} size={18} />
            )
          }
          onPress={url ? () => void openExternalUrl(url) : undefined}
          size="$3"
        />
      </XStack>
    )
  }

  if (body.type === "image") {
    const url = fileUrls.get(body.fileId)
    if (!url) {
      return (
        <XStack gap="$2" items="center" minW={160} p="$2">
          {fileUrlsLoading ? <Spinner /> : <ThemedIcon icon={ImageIcon} />}
          <SizableText color="$color10">图片暂时无法加载</SizableText>
        </XStack>
      )
    }

    const size = getImageDisplaySize(body.width, body.height)
    return (
      <Image
        accessibilityLabel="查看图片"
        height={size.height}
        objectFit="cover"
        onPress={() => void openExternalUrl(url)}
        rounded="$3"
        src={url}
        width={size.width}
      />
    )
  }

  if (body.type === "voice") {
    const url = fileUrls.get(body.fileId)
    return (
      <YStack gap="$2" minW={220}>
        <XStack gap="$3" items="center">
          <ThemedIcon icon={AudioLines} size={22} />
          <SizableText flex={1}>
            语音 {formatVoiceDuration(body.durationMS)}
          </SizableText>
          <Button
            accessibilityLabel="播放语音"
            chromeless
            circular
            disabled={!url}
            icon={
              fileUrlsLoading && !url ? (
                <Spinner />
              ) : (
                <ThemedIcon icon={Play} size={18} />
              )
            }
            onPress={url ? () => void openExternalUrl(url) : undefined}
            size="$3"
          />
        </XStack>
        {body.transcript ? (
          <Paragraph color="$color10" size="$2">
            {body.transcript}
          </Paragraph>
        ) : null}
      </YStack>
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
    return <Paragraph color="$color10">该消息已被撤回</Paragraph>
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
    <Card gap="$2" maxW={280} onPress={onPress} p="$3">
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
    <YStack gap="$2" minW={240}>
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
