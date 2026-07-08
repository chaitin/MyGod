import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"

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

export function MessageMarkdown({ content }: { content: string }) {
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
        }}
        remarkPlugins={[remarkGfm]}
        skipHtml
        unwrapDisallowed
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}
