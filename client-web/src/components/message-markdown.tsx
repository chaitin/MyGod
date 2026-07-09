import * as React from "react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"

import {
  parseMentionTemplate,
  type MentionLabelResolver,
} from "@/lib/message-mentions"
import { UserProfilePopover } from "@/components/user-profile-popover"

const allowedMarkdownElements = [
  "a",
  "blockquote",
  "br",
  "code",
  "del",
  "em",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
  "li",
  "mention",
  "ol",
  "p",
  "pre",
  "strong",
  "table",
  "tbody",
  "td",
  "th",
  "thead",
  "tr",
  "ul",
]

type MarkdownAstNode = {
  children?: MarkdownAstNode[]
  data?: {
    hName?: string
    hProperties?: Record<string, string>
  }
  type: string
  value?: string
}

type MarkdownElementNode = {
  properties?: Record<string, unknown>
}

type MarkdownMentionProps = {
  children?: React.ReactNode
  node?: MarkdownElementNode
}

type ReactMarkdownProps = React.ComponentProps<typeof ReactMarkdown>

const fallbackMentionLabelResolver: MentionLabelResolver = () => undefined

export function MessageMarkdown({
  content,
  currentUserId,
  mentionLabelResolver = fallbackMentionLabelResolver,
}: {
  content: string
  currentUserId?: string
  mentionLabelResolver?: MentionLabelResolver
}) {
  const remarkPlugins = React.useMemo<ReactMarkdownProps["remarkPlugins"]>(
    () => [remarkGfm, createRemarkMentionPlugin(mentionLabelResolver)],
    [mentionLabelResolver]
  )

  return (
    <div className="max-w-full space-y-2 break-words">
      <ReactMarkdown
        allowedElements={allowedMarkdownElements}
        components={{
          a: ({ children, href }) =>
            href ? (
              <a
                className="mx-0.5 break-all font-medium text-sky-500 underline-offset-4 hover:text-sky-600 hover:underline"
                href={href}
                rel="noreferrer"
                target="_blank"
              >
                {children}
              </a>
            ) : (
              <span>{children}</span>
            ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-2 border-border bg-foreground/5 py-2 pl-3 text-muted-foreground">
              {children}
            </blockquote>
          ),
          code: ({ children }) => (
            <code className="rounded bg-foreground/5 px-1 py-0.5 font-mono text-[0.92em]">
              {children}
            </code>
          ),
          del: ({ children }) => (
            <del className="text-muted-foreground">{children}</del>
          ),
          h1: ({ children }) => (
            <h1 className="text-lg leading-snug font-semibold">{children}</h1>
          ),
          h2: ({ children }) => (
            <h2 className="text-base leading-snug font-semibold">{children}</h2>
          ),
          h3: ({ children }) => (
            <h3 className="text-sm leading-snug font-semibold">{children}</h3>
          ),
          h4: ({ children }) => (
            <h4 className="text-sm leading-snug text-foreground/80">
              {children}
            </h4>
          ),
          h5: ({ children }) => (
            <h5 className="text-sm leading-snug text-foreground/70">
              {children}
            </h5>
          ),
          h6: ({ children }) => (
            <h6 className="text-sm leading-snug text-foreground/60">
              {children}
            </h6>
          ),
          hr: () => <hr className="h-px border-0 bg-foreground/20" />,
          li: ({ children }) => <li className="pl-1">{children}</li>,
          mention: ({ children, node }: MarkdownMentionProps) => (
            <MarkdownMention currentUserId={currentUserId} node={node}>
              {children}
            </MarkdownMention>
          ),
          ol: ({ children }) => (
            <ol className="list-decimal space-y-1 pl-5">{children}</ol>
          ),
          p: ({ children }) => <p>{children}</p>,
          pre: ({ children }) => (
            <pre className="max-w-full overflow-x-auto rounded bg-foreground/5 p-3 font-mono text-[0.92em] [&_code]:rounded-none [&_code]:bg-transparent [&_code]:p-0">
              {children}
            </pre>
          ),
          table: ({ children }) => (
            <div className="max-w-full overflow-x-auto">
              <table className="w-max min-w-full border-collapse text-xs">
                {children}
              </table>
            </div>
          ),
          td: ({ children }) => (
            <td className="border border-foreground/[0.08] px-2 py-1 align-top">
              {children}
            </td>
          ),
          th: ({ children }) => (
            <th className="border border-foreground/[0.08] bg-foreground/5 px-2 py-1 text-left font-medium">
              {children}
            </th>
          ),
          tr: ({ children }) => <tr>{children}</tr>,
          ul: ({ children }) => (
            <ul className="list-disc space-y-1 pl-5">{children}</ul>
          ),
        } as ReactMarkdownProps["components"]}
        remarkPlugins={remarkPlugins}
        skipHtml
        unwrapDisallowed
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

function MarkdownMention({
  children,
  currentUserId,
  node,
}: MarkdownMentionProps & {
  currentUserId?: string
}) {
  const mentionId = getMarkdownNodeProperty(node, "data-mention-id")
  const mentionType = getMarkdownNodeProperty(node, "data-mention-type")
  const isCurrentUserMention =
    mentionType === "all" ||
    (mentionType === "user" && isSameUserId(mentionId, currentUserId))
  const content = (
    <span
      className={getMentionTextClassName(isCurrentUserMention)}
      data-mention-id={mentionId}
      data-mention-type={mentionType}
    >
      {children}
    </span>
  )

  if (mentionType === "user" && mentionId) {
    return (
      <UserProfilePopover triggerClassName="align-baseline" userId={mentionId}>
        {content}
      </UserProfilePopover>
    )
  }

  return content
}

function getMentionTextClassName(isCurrentUserMention: boolean) {
  return isCurrentUserMention
    ? "mx-0.5 font-medium text-amber-600 hover:text-amber-700"
    : "mx-0.5 font-medium text-sky-500 hover:text-sky-600"
}

function isSameUserId(userId: string | undefined, currentUserId?: string) {
  return (
    typeof userId === "string" &&
    typeof currentUserId === "string" &&
    userId.toLowerCase() === currentUserId.toLowerCase()
  )
}

function createRemarkMentionPlugin(
  mentionLabelResolver: MentionLabelResolver
) {
  return function remarkMentionPlugin() {
    return function transformMentionTokens(tree: MarkdownAstNode) {
      replaceMentionTextNodes(tree, mentionLabelResolver)
    }
  }
}

function replaceMentionTextNodes(
  node: MarkdownAstNode,
  mentionLabelResolver: MentionLabelResolver
) {
  if (!node.children) {
    return
  }

  node.children = node.children.flatMap((child) => {
    if (child.type === "text" && typeof child.value === "string") {
      return createMentionNodes(child.value, mentionLabelResolver)
    }

    if (child.type !== "inlineCode" && child.type !== "code") {
      replaceMentionTextNodes(child, mentionLabelResolver)
    }

    return [child]
  })
}

function createMentionNodes(
  value: string,
  mentionLabelResolver: MentionLabelResolver
) {
  const parts = parseMentionTemplate(value, mentionLabelResolver)

  if (!parts.some((part) => part.type === "mention")) {
    return [{ type: "text", value }]
  }

  return parts.map((part): MarkdownAstNode => {
    if (part.type === "text") {
      return {
        type: "text",
        value: part.text,
      }
    }

    return {
      children: [
        {
          type: "text",
          value: part.label,
        },
      ],
      data: {
        hName: "mention",
        hProperties: {
          "data-mention-id": part.id,
          "data-mention-type": part.targetType,
        },
      },
      type: "mention",
      value: part.label,
    }
  })
}

function getMarkdownNodeProperty(
  node: MarkdownElementNode | undefined,
  name: string
) {
  const value = node?.properties?.[name]
  return typeof value === "string" ? value : undefined
}
