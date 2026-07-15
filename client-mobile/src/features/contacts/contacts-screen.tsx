import { RefreshCw, Search } from "lucide-react-native"
import { useMemo, useState } from "react"
import {
  Button,
  Input,
  Paragraph,
  SizableText,
  Spinner,
  Tabs,
  XStack,
  YStack,
} from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"
import { KeyboardAwareScreen } from "@/components/layout/keyboard-aware-screen"
import { appConfig } from "@/config/app-config"
import { useCachedAppInfo } from "@/data/hooks"
import {
  buildDirectorySections,
  type DirectoryTab,
} from "@/features/contacts/contact-directory-model"
import { ContactDirectoryList } from "@/features/contacts/contact-directory-list"
import { useServers } from "@/features/servers/server-context"
import { useClientData } from "@/providers/client-data-provider"

const DIRECTORY_TABS: { label: string; value: DirectoryTab }[] = [
  { label: "联系人", value: "user" },
  { label: "应用", value: "app" },
  { label: "群组", value: "group" },
]

export function ContactsScreen() {
  const { selectedServer } = useServers()
  const appInfoQuery = useCachedAppInfo(selectedServer)
  const {
    contacts,
    contactsError,
    isContactsRefreshing,
    refreshContacts,
  } = useClientData()
  const [activeTab, setActiveTab] = useState<DirectoryTab>("user")
  const [keywords, setKeywords] = useState<Record<DirectoryTab, string>>({
    app: "",
    group: "",
    user: "",
  })
  const activeKeyword = keywords[activeTab]
  const organizationName =
    appInfoQuery.data?.organizationName ?? appConfig.organizationName
  const sections = useMemo(
    () =>
      buildDirectorySections({
        activeTab,
        contacts,
        keyword: activeKeyword,
        organizationName,
      }),
    [activeKeyword, activeTab, contacts, organizationName]
  )

  function handleTabChange(value: string) {
    if (value === "user" || value === "app" || value === "group") {
      setActiveTab(value)
    }
  }

  function handleKeywordChange(value: string) {
    setKeywords((current) => ({
      ...current,
      [activeTab]: value,
    }))
  }

  function handleRefresh() {
    void refreshContacts().catch(() => undefined)
  }

  return (
    <KeyboardAwareScreen edges={[]} scrollable={false}>
      <YStack gap="$3" p="$4" pb="$3">
        <Tabs onValueChange={handleTabChange} value={activeTab}>
          <Tabs.List width="100%">
            {DIRECTORY_TABS.map((tab) => (
              <Tabs.Tab flex={1} key={tab.value} value={tab.value}>
                <SizableText>{tab.label}</SizableText>
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </Tabs>

        <XStack gap="$2" items="center">
          <ThemedIcon icon={Search} size={18} />
          <Input
            autoCapitalize="none"
            clearButtonMode="while-editing"
            flex={1}
            onChangeText={handleKeywordChange}
            placeholder={`搜索${getDirectoryTabLabel(activeTab)}`}
            returnKeyType="search"
            value={activeKeyword}
          />
          <Button
            accessibilityLabel="刷新通讯录"
            circular
            disabled={isContactsRefreshing}
            icon={
              isContactsRefreshing ? (
                <Spinner />
              ) : (
                <ThemedIcon icon={RefreshCw} />
              )
            }
            onPress={handleRefresh}
            size="$4"
          />
        </XStack>

        {contactsError ? (
          <Paragraph color="$red10" size="$2">
            {contactsError.message}
          </Paragraph>
        ) : null}
      </YStack>

      <ContactDirectoryList
        emptyLabel={getDirectoryTabLabel(activeTab)}
        isRefreshing={isContactsRefreshing}
        onRefresh={handleRefresh}
        sections={sections}
        serverUrl={selectedServer.url}
      />
    </KeyboardAwareScreen>
  )
}

function getDirectoryTabLabel(tab: DirectoryTab) {
  if (tab === "app") {
    return "应用"
  }

  if (tab === "group") {
    return "群组"
  }

  return "联系人"
}
