import { FlatList, RefreshControl, StyleSheet } from "react-native"
import {
  ListItem,
  Paragraph,
  Separator,
  SizableText,
  XStack,
  YStack,
} from "tamagui"

import { ConversationAvatar } from "@/features/messages/conversation-avatar"
import type { ConversationListItemModel } from "@/features/messages/conversation-list-model"

export function ConversationList({
  hasKeyword,
  isRefreshing,
  items,
  onConversationPress,
  onRefresh,
  serverUrl,
}: {
  hasKeyword: boolean
  isRefreshing: boolean
  items: ConversationListItemModel[]
  onConversationPress: (conversationId: string) => void
  onRefresh: () => void
  serverUrl: string
}) {
  return (
    <FlatList
      contentContainerStyle={
        items.length === 0
          ? [styles.content, styles.emptyContent]
          : styles.content
      }
      data={items}
      ItemSeparatorComponent={() => <Separator />}
      keyboardDismissMode="on-drag"
      keyboardShouldPersistTaps="handled"
      keyExtractor={(item) => item.conversation.id}
      ListEmptyComponent={
        <YStack flex={1} items="center" justify="center" p="$8">
          <Paragraph color="$color10" text="center">
            {hasKeyword ? "没有匹配的会话" : "暂无会话"}
          </Paragraph>
        </YStack>
      }
      refreshControl={
        <RefreshControl onRefresh={onRefresh} refreshing={isRefreshing} />
      }
      renderItem={({ item }) => (
        <ConversationListItem
          item={item}
          onPress={() => onConversationPress(item.conversation.id)}
          serverUrl={serverUrl}
        />
      )}
      showsVerticalScrollIndicator={false}
      style={styles.list}
    />
  )
}

function ConversationListItem({
  item,
  onPress,
  serverUrl,
}: {
  item: ConversationListItemModel
  onPress: () => void
  serverUrl: string
}) {
  const { conversation } = item

  return (
    <ListItem
      icon={
        <ConversationAvatar conversation={conversation} serverUrl={serverUrl} />
      }
      onPress={onPress}
      size="$5"
      subTitle={
        <XStack gap="$1" items="center" maxW="100%">
          {item.hasUnreadMention ? (
            <SizableText color="$red10" fontWeight="600" size="$2">
              [有人 @ 我]
            </SizableText>
          ) : null}
          <SizableText
            color="$color10"
            flex={1}
            numberOfLines={1}
            size="$2"
          >
            {item.description}
          </SizableText>
        </XStack>
      }
      title={
        <XStack gap="$2" items="center" maxW="100%">
          <SizableText flex={1} fontWeight="500" numberOfLines={1}>
            {conversation.name}
          </SizableText>
          {item.lastMessageTime ? (
            <SizableText color="$color10" size="$2">
              {item.lastMessageTime}
            </SizableText>
          ) : null}
        </XStack>
      }
    />
  )
}

const styles = StyleSheet.create({
  content: {
    flexGrow: 1,
    paddingBottom: 16,
  },
  emptyContent: {
    justifyContent: "center",
  },
  list: {
    flex: 1,
  },
})
