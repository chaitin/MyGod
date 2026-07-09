import type {
  ClientConversation,
  ClientConversationMember,
  ClientUser,
  ContactApp,
  ContactUser,
} from "@/lib/client-data-api"
import type { MentionLabelResolver } from "@/lib/message-mentions"

export function createConversationMentionLabelResolver({
  appsById,
  contactsById,
  conversation,
  currentUser,
}: {
  appsById?: ReadonlyMap<string, ContactApp>
  contactsById?: ReadonlyMap<string, ContactUser>
  conversation?: ClientConversation | null
  currentUser?: Pick<ClientUser, "id" | "name" | "nickname"> | null
}): MentionLabelResolver {
  return (target) => {
    if (target.type === "all") {
      return "所有人"
    }

    if (target.type === "app") {
      const member = conversation?.members?.find(
        (currentMember) =>
          currentMember.type === "app" && currentMember.id === target.id
      )
      if (member) {
        return getConversationMemberMentionLabel(member)
      }

      return appsById?.get(target.id)?.name
    }

    if (currentUser?.id === target.id) {
      return formatMentionUserName(currentUser)
    }

    const member = conversation?.members?.find(
      (currentMember) =>
        currentMember.type === "user" && currentMember.id === target.id
    )
    if (member) {
      return getConversationMemberMentionLabel(member)
    }

    const contact = contactsById?.get(target.id)
    if (contact) {
      return formatMentionUserName(contact)
    }

    return undefined
  }
}

export function getConversationMemberMentionLabel(
  member: ClientConversationMember
) {
  if (member.type === "app") {
    return member.name.trim()
  }

  return formatMentionUserName(member)
}

function formatMentionUserName(user: { name: string; nickname: string }) {
  const name = user.name.trim()
  const nickname = user.nickname.trim()

  return nickname || name
}
