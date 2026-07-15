import { Bot } from "lucide-react-native"
import { Avatar, Circle, Text, YStack } from "tamagui"

import {
  GroupAvatar,
  type GroupAvatarMember,
} from "@/components/avatar/group-avatar"
import { ThemedIcon } from "@/components/icons/themed-icon"
import { getContactInitial } from "@/features/contacts/contact-directory-model"
import { resolveServerAssetUrl } from "@/lib/server-asset-url"

export function ContactDirectoryAvatar({
  avatar,
  members,
  name,
  online,
  serverUrl,
  type,
}: {
  avatar: string
  members?: GroupAvatarMember[]
  name: string
  online?: boolean
  serverUrl: string
  type: "user" | "app" | "group"
}) {
  const avatarUrl = resolveServerAssetUrl(serverUrl, avatar)

  return (
    <YStack height="$4" width="$4">
      {type === "group" ? (
        <GroupAvatar
          avatar={avatar}
          members={members}
          name={name}
          serverUrl={serverUrl}
        />
      ) : (
        <Avatar rounded="$2" size="$4">
          {avatarUrl ? <Avatar.Image src={avatarUrl} /> : null}
          <Avatar.Fallback
            bg="$backgroundFocus"
            items="center"
            justify="center"
          >
            {type === "app" ? (
              <ThemedIcon icon={Bot} size={18} />
            ) : (
              <Text fontWeight="600">{getContactInitial(name)}</Text>
            )}
          </Avatar.Fallback>
        </Avatar>
      )}

      {online !== undefined ? (
        <Circle
          bg={online ? "$green9" : "$gray8"}
          borderColor="$background"
          borderWidth={2}
          b={-2}
          position="absolute"
          r={-2}
          size={11}
        />
      ) : null}
    </YStack>
  )
}
