import {
  RefreshControl,
  SectionList,
  StyleSheet,
  type SectionListRenderItemInfo,
} from "react-native"
import { ListItem, Paragraph, Separator, XStack, YStack } from "tamagui"

import { ContactDirectoryAvatar } from "@/features/contacts/contact-directory-avatar"
import {
  getContactDisplayName,
  type DirectoryItem,
  type DirectorySection,
} from "@/features/contacts/contact-directory-model"

export function ContactDirectoryList({
  emptyLabel,
  isRefreshing,
  onRefresh,
  sections,
  serverUrl,
}: {
  emptyLabel: string
  isRefreshing: boolean
  onRefresh: () => void
  sections: DirectorySection[]
  serverUrl: string
}) {
  return (
    <SectionList<DirectoryItem, DirectorySection>
      contentContainerStyle={
        sections.length === 0
          ? [styles.content, styles.emptyContent]
          : styles.content
      }
      ItemSeparatorComponent={() => <Separator />}
      keyboardDismissMode="on-drag"
      keyboardShouldPersistTaps="handled"
      keyExtractor={(item) => item.key}
      ListEmptyComponent={
        <YStack flex={1} items="center" justify="center" p="$8">
          <Paragraph color="$color10" text="center">
            没有匹配的{emptyLabel}
          </Paragraph>
        </YStack>
      }
      refreshControl={
        <RefreshControl onRefresh={onRefresh} refreshing={isRefreshing} />
      }
      renderItem={(itemInfo) => (
        <DirectoryListItem itemInfo={itemInfo} serverUrl={serverUrl} />
      )}
      renderSectionHeader={({ section }) =>
        section.title ? <DirectorySectionHeader section={section} /> : null
      }
      sections={sections}
      showsVerticalScrollIndicator={false}
      stickySectionHeadersEnabled={false}
      style={styles.list}
    />
  )
}

function DirectorySectionHeader({ section }: { section: DirectorySection }) {
  return (
    <XStack
      bg="$background"
      items="center"
      justify="space-between"
      pb="$2"
      pt="$4"
      px="$4"
    >
      <Paragraph fontWeight="600">{section.title}</Paragraph>
      <Paragraph bg="$backgroundPress" px="$2" rounded="$10" size="$1">
        {section.count}
      </Paragraph>
    </XStack>
  )
}

function DirectoryListItem({
  itemInfo,
  serverUrl,
}: {
  itemInfo: SectionListRenderItemInfo<DirectoryItem, DirectorySection>
  serverUrl: string
}) {
  const { item } = itemInfo

  if (item.type === "user") {
    const displayName = getContactDisplayName(item.value)

    return (
      <ListItem
        icon={
          <ContactDirectoryAvatar
            avatar={item.value.avatar}
            name={displayName}
            online={item.value.online}
            serverUrl={serverUrl}
            type="user"
          />
        }
        size="$5"
        subTitle={item.value.email}
        title={displayName}
      />
    )
  }

  if (item.type === "app") {
    return (
      <ListItem
        icon={
          <ContactDirectoryAvatar
            avatar={item.value.avatar}
            name={item.value.name}
            online={item.value.online}
            serverUrl={serverUrl}
            type="app"
          />
        }
        size="$5"
        subTitle={item.value.description || "智能应用"}
        title={item.value.name}
      />
    )
  }

  return (
    <ListItem
      icon={
        <ContactDirectoryAvatar
          avatar={item.value.avatar}
          members={item.value.avatarMembers}
          name={item.value.name}
          serverUrl={serverUrl}
          type="group"
        />
      }
      size="$5"
      subTitle={`${item.value.memberCount} 人 · ${
        item.value.joined ? "已加入" : "公开群组"
      }`}
      title={item.value.name}
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
