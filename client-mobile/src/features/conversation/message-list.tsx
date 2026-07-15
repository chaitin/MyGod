import { FlatList, RefreshControl, StyleSheet } from "react-native"
import { Button, Paragraph, Spinner, YStack } from "tamagui"

import { MessageBubble } from "@/features/conversation/message-bubble"
import type {
  MessageMentionLabelResolver,
  PresentedMessage,
} from "@/features/conversation/conversation-message-presenter"

export function MessageList({
  error,
  fileUrls,
  fileUrlsLoading,
  hasOlder,
  isFetchingOlder,
  isLoading,
  isRefreshing,
  messages,
  onLoadOlder,
  onRefresh,
  resolveMentionLabel,
  serverUrl,
}: {
  error: Error | null
  fileUrls: ReadonlyMap<string, string>
  fileUrlsLoading: boolean
  hasOlder: boolean
  isFetchingOlder: boolean
  isLoading: boolean
  isRefreshing: boolean
  messages: PresentedMessage[]
  onLoadOlder: () => void
  onRefresh: () => void
  resolveMentionLabel: MessageMentionLabelResolver
  serverUrl: string
}) {
  if (isLoading) {
    return (
      <YStack flex={1} gap="$2" items="center" justify="center">
        <Spinner />
        <Paragraph color="$color10">正在加载消息</Paragraph>
      </YStack>
    )
  }

  if (error && messages.length === 0) {
    return (
      <YStack flex={1} gap="$3" items="center" justify="center" p="$6">
        <Paragraph color="$color10" text="center">
          {error.message}
        </Paragraph>
        <Button onPress={onRefresh} variant="outlined">
          重试
        </Button>
      </YStack>
    )
  }

  if (messages.length === 0) {
    return (
      <YStack flex={1} gap="$1" items="center" justify="center" p="$6">
        <Paragraph fontWeight="600">暂无消息</Paragraph>
        <Paragraph color="$color10">发送第一条消息开始对话</Paragraph>
      </YStack>
    )
  }

  return (
    <FlatList
      contentContainerStyle={styles.content}
      data={messages}
      inverted
      ItemSeparatorComponent={() => <YStack height="$4" />}
      keyboardDismissMode="on-drag"
      keyboardShouldPersistTaps="handled"
      keyExtractor={(item) => item.id}
      ListFooterComponent={
        hasOlder || isFetchingOlder ? (
          <YStack items="center" pb="$3">
            <Button
              disabled={isFetchingOlder}
              icon={isFetchingOlder ? <Spinner /> : undefined}
              onPress={onLoadOlder}
              size="$3"
              variant="outlined"
            >
              {isFetchingOlder ? "正在加载" : "加载更早消息"}
            </Button>
          </YStack>
        ) : null
      }
      maintainVisibleContentPosition={{ minIndexForVisible: 0 }}
      onEndReached={hasOlder && !isFetchingOlder ? onLoadOlder : undefined}
      onEndReachedThreshold={0.2}
      refreshControl={
        <RefreshControl onRefresh={onRefresh} refreshing={isRefreshing} />
      }
      renderItem={({ item }) => (
        <MessageBubble
          fileUrls={fileUrls}
          fileUrlsLoading={fileUrlsLoading}
          message={item}
          resolveMentionLabel={resolveMentionLabel}
          serverUrl={serverUrl}
        />
      )}
      showsVerticalScrollIndicator={false}
      style={styles.list}
    />
  )
}

const styles = StyleSheet.create({
  content: {
    paddingBottom: 16,
    paddingTop: 16,
  },
  list: {
    flex: 1,
  },
})
