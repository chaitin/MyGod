import { RefreshCw, Search } from "lucide-react-native"
import { useRouter } from "expo-router"
import { useMemo, useState } from "react"
import {
  Button,
  Input,
  Paragraph,
  Spinner,
  XStack,
  YStack,
} from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"
import { KeyboardAwareScreen } from "@/components/layout/keyboard-aware-screen"
import { ConversationList } from "@/features/messages/conversation-list"
import { buildConversationListItems } from "@/features/messages/conversation-list-model"
import { useServers } from "@/features/servers/server-context"
import { useClientData } from "@/providers/client-data-provider"

export function MessagesScreen() {
  const router = useRouter()
  const { selectedServer } = useServers()
  const {
    contacts,
    conversations,
    conversationsError,
    isConversationsRefreshing,
    refreshConversations,
  } = useClientData()
  const [keyword, setKeyword] = useState("")
  const items = useMemo(
    () =>
      buildConversationListItems({
        contacts,
        conversations,
        keyword,
      }),
    [contacts, conversations, keyword]
  )

  function handleRefresh() {
    void refreshConversations().catch(() => undefined)
  }

  function handleConversationPress(conversationId: string) {
    router.push({
      params: { conversationId },
      pathname: "/(app)/conversation/[conversationId]",
    })
  }

  return (
    <KeyboardAwareScreen edges={[]} scrollable={false}>
      <YStack gap="$3" p="$4" pb="$3">
        <XStack gap="$2" items="center">
          <ThemedIcon icon={Search} size={18} />
          <Input
            autoCapitalize="none"
            clearButtonMode="while-editing"
            flex={1}
            onChangeText={setKeyword}
            placeholder="搜索会话"
            returnKeyType="search"
            value={keyword}
          />
          <Button
            accessibilityLabel="刷新会话"
            circular
            disabled={isConversationsRefreshing}
            icon={
              isConversationsRefreshing ? (
                <Spinner />
              ) : (
                <ThemedIcon icon={RefreshCw} />
              )
            }
            onPress={handleRefresh}
            size="$4"
          />
        </XStack>

        {conversationsError ? (
          <Paragraph color="$red10" size="$2">
            {conversationsError.message}
          </Paragraph>
        ) : null}
      </YStack>

      <ConversationList
        hasKeyword={keyword.trim().length > 0}
        isRefreshing={isConversationsRefreshing}
        items={items}
        onConversationPress={handleConversationPress}
        onRefresh={handleRefresh}
        serverUrl={selectedServer.url}
      />
    </KeyboardAwareScreen>
  )
}
