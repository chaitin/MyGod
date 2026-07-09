export type MentionTargetType = "user" | "app" | "all"

export type MentionTarget = {
  id: string
  type: MentionTargetType
}

export type MentionTemplatePart =
  | {
      text: string
      type: "text"
    }
  | {
      id: string
      label: string
      type: "mention"
      targetType: MentionTargetType
    }

export type MentionLabelResolver = (target: MentionTarget) => string | undefined

const mentionTokenPattern =
  /\{\(@(?:(user)\/(all)|(user|app)\/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}/g

export function createMentionToken(target: MentionTarget) {
  if (target.type === "all") {
    return "{(@user/all)}"
  }

  return `{(@${target.type}/${target.id})}`
}

export function parseMentionTemplate(
  content: string,
  resolveLabel: MentionLabelResolver
): MentionTemplatePart[] {
  const parts: MentionTemplatePart[] = []
  let cursor = 0

  for (const match of content.matchAll(mentionTokenPattern)) {
    const index = match.index ?? 0
    if (index > cursor) {
      parts.push({ text: content.slice(cursor, index), type: "text" })
    }

    const targetType = (match[2] === "all" ? "all" : match[3]) as MentionTargetType
    const id = targetType === "all" ? "all" : match[4].toLowerCase()
    parts.push({
      id,
      label: resolveMentionLabel({ id, type: targetType }, resolveLabel),
      targetType,
      type: "mention",
    })
    cursor = index + match[0].length
  }

  if (cursor < content.length) {
    parts.push({ text: content.slice(cursor), type: "text" })
  }

  if (parts.length === 0) {
    return [{ text: content, type: "text" }]
  }

  return parts
}

export function formatMentionTemplateText(
  content: string,
  resolveLabel: MentionLabelResolver
) {
  return parseMentionTemplate(content, resolveLabel)
    .map((part) => (part.type === "mention" ? part.label : part.text))
    .join("")
}

function resolveMentionLabel(
  target: MentionTarget,
  resolveLabel: MentionLabelResolver
) {
  if (target.type === "all") {
    return "@所有人"
  }

  const label = resolveLabel(target)?.trim()
  if (label) {
    return `@${label}`
  }

  return target.type === "app" ? "@应用" : "@用户"
}
