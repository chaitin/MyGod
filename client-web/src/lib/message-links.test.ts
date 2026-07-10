import { describe, expect, it } from "vitest"

import { linkifyMessageText } from "@/lib/message-links"

describe("linkifyMessageText", () => {
  it("finds HTTP links with ports, paths, queries, and hashes", () => {
    const url =
      "https://api.example.com:8080/v1/users?id=123&active=true#result-1"

    expect(linkifyMessageText(`查看 ${url} 的结果`)).toEqual([
      { type: "text", value: "查看 " },
      { href: url, type: "link", value: url },
      { type: "text", value: " 的结果" },
    ])
  })

  it("supports multiple links and treats percent signs as ordinary URL characters", () => {
    expect(
      linkifyMessageText(
        "http://api:80/a_%zz?value=50%#part 和 https://example.com"
      )
    ).toEqual([
      {
        href: "http://api:80/a_%zz?value=50%#part",
        type: "link",
        value: "http://api:80/a_%zz?value=50%#part",
      },
      { type: "text", value: " 和 " },
      {
        href: "https://example.com",
        type: "link",
        value: "https://example.com",
      },
    ])
  })

  it("keeps surrounding sentence punctuation out of the link", () => {
    expect(
      linkifyMessageText("请看（https://example.com/a_(b)），谢谢。")
    ).toEqual([
      { type: "text", value: "请看（" },
      {
        href: "https://example.com/a_(b)",
        type: "link",
        value: "https://example.com/a_(b)",
      },
      { type: "text", value: "），谢谢。" },
    ])
  })

  it("stops a link before adjacent Chinese text", () => {
    const url =
      "http://localhost:20070/chat?conversation_id=b6ef4519-34f6-4ac4-8037-dd5caed173e3"

    expect(linkifyMessageText(`啊啊${url}阿斯顿`)).toEqual([
      { type: "text", value: "啊啊" },
      { href: url, type: "link", value: url },
      { type: "text", value: "阿斯顿" },
    ])
  })

  it("does not link unsupported URL candidates", () => {
    for (const value of [
      "www.example.com",
      "ftp://example.com/file",
      "https://user:pass@example.com",
      "https://example.com:0/path",
      "https://example.com:65536/path",
    ]) {
      expect(linkifyMessageText(value)).toEqual([{ type: "text", value }])
    }
  })
})
