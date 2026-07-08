import ReactMarkdown from "react-markdown"

const allowedMarkdownElements = [
  "blockquote",
  "br",
  "code",
  "em",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "li",
  "ol",
  "p",
  "pre",
  "strong",
  "ul",
]

export function MessageMarkdown({ content }: { content: string }) {
  return (
    <div className="max-w-full space-y-2 break-words">
      <ReactMarkdown
        allowedElements={allowedMarkdownElements}
        components={{
          blockquote: ({ children }) => (
            <blockquote className="border-l-2 border-border bg-foreground/5 pl-3 text-muted-foreground">
              {children}
            </blockquote>
          ),
          code: ({ children }) => (
            <code className="rounded bg-foreground/5 px-1 py-0.5 font-mono text-[0.92em]">
              {children}
            </code>
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
          ul: ({ children }) => (
            <ul className="list-disc space-y-1 pl-5">{children}</ul>
          ),
        }}
        skipHtml
        unwrapDisallowed
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}
