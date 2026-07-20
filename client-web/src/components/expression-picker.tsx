import * as React from "react"

import { Button } from "@/components/ui/button"

export type ExpressionItem = {
  label: string
  type: "emoji"
  value: string
}

type ExpressionPickerProps = {
  onSelect: (item: ExpressionItem) => void
}

type StoredExpressionUsage = {
  count: number
  lastUsedAt: number
  value: string
}

const expressionUsageStorageKey = "client-web:expression-picker:usage"
const frequentExpressionLimit = 8
const expressionUsageMaxAgeMs = 30 * 24 * 60 * 60 * 1000

const allExpressionItems: ExpressionItem[] = [
  // 开心与大笑
  expression("😂", "笑哭"),
  expression("🤣", "笑到打滚"),
  expression("😄", "大笑"),
  expression("😅", "流汗笑"),
  expression("😆", "眯眼笑"),
  expression("😀", "笑脸"),
  expression("😃", "开心"),
  expression("😁", "露齿笑"),

  // 友好与喜爱
  expression("😊", "微笑"),
  expression("🙂", "浅笑"),
  expression("😉", "眨眼"),
  expression("🥰", "喜爱"),
  expression("😍", "花痴"),
  expression("😘", "飞吻"),
  expression("🤗", "拥抱"),
  expression("🤩", "星星眼"),

  // 调皮与轻松
  expression("😏", "坏笑"),
  expression("🤭", "偷笑"),
  expression("😜", "眨眼吐舌"),
  expression("🙃", "倒脸"),
  expression("😋", "好吃"),
  expression("🤪", "滑稽"),
  expression("😎", "酷"),
  expression("🤫", "嘘"),

  // 思考与无语
  expression("🤔", "思考"),
  expression("🙄", "翻白眼"),
  expression("🤦", "捂脸"),
  expression("🤷", "摊手"),
  expression("😑", "面无表情"),
  expression("😬", "尴尬"),
  expression("🫠", "融化"),
  expression("🤡", "小丑"),

  // 难过与压力
  expression("😭", "大哭"),
  expression("🥺", "可怜"),
  expression("🥹", "忍住眼泪"),
  expression("😢", "哭"),
  expression("😔", "沮丧"),
  expression("🥲", "含泪微笑"),
  expression("😮‍💨", "叹气"),
  expression("😳", "脸红"),

  // 强烈反应与状态
  expression("😡", "愤怒"),
  expression("😤", "生气"),
  expression("😱", "惊恐"),
  expression("🤯", "爆炸头"),
  expression("🥳", "庆祝"),
  expression("😴", "睡觉"),
  expression("🥱", "打哈欠"),
  expression("🫡", "敬礼"),

  // 手势回应
  expression("👍", "赞"),
  expression("👏", "鼓掌"),
  expression("🙏", "拜托"),
  expression("👌", "好的"),
  expression("💪", "加油"),
  expression("✌️", "胜利"),
  expression("🤝", "握手"),
  expression("👎", "踩"),

  // 聊天动作与状态符号
  expression("👋", "挥手"),
  expression("👀", "关注"),
  expression("❤️", "爱心"),
  expression("🫶", "爱心手势"),
  expression("🔥", "火"),
  expression("🎉", "庆祝礼花"),
  expression("✅", "完成"),
  expression("❌", "错误"),
]

const allExpressionsByValue = new Map(
  allExpressionItems.map((item) => [item.value, item])
)
const defaultFrequentExpressionItems = [
  "😂",
  "😊",
  "😭",
  "👍",
  "❤️",
  "👏",
  "🙏",
  "🎉",
].flatMap((value) => {
  const item = allExpressionsByValue.get(value)
  return item ? [item] : []
})

export function ExpressionPicker({ onSelect }: ExpressionPickerProps) {
  const [usage, setUsage] = React.useState<StoredExpressionUsage[]>(() =>
    readExpressionUsage()
  )
  const frequentExpressionItems = React.useMemo(
    () => getFrequentExpressionItems(usage),
    [usage]
  )

  React.useEffect(() => {
    writeExpressionUsage(usage)
  }, [usage])

  function handleSelect(item: ExpressionItem) {
    const nextUsage = updateExpressionUsage(usage, item.value, Date.now())

    setUsage(nextUsage)
    writeExpressionUsage(nextUsage)
    onSelect(item)
  }

  return (
    <div className="w-80" data-testid="expression-picker">
      <div className="space-y-4">
        <ExpressionSection
          items={frequentExpressionItems}
          label="常用"
          onSelect={handleSelect}
        />
        <ExpressionSection
          items={allExpressionItems}
          label="所有表情"
          onSelect={handleSelect}
        />
      </div>
    </div>
  )
}

function ExpressionSection({
  items,
  label,
  onSelect,
}: {
  items: ExpressionItem[]
  label: string
  onSelect: (item: ExpressionItem) => void
}) {
  return (
    <section aria-label={label}>
      <h3 className="mb-2 text-xs font-medium text-muted-foreground">
        {label}
      </h3>
      <div
        className="grid gap-1"
        style={{ gridTemplateColumns: "repeat(8, minmax(0, 1fr))" }}
      >
        {items.map((item) => (
          <Button
            key={`${label}-${item.value}-${item.label}`}
            aria-label={item.label}
            className="text-lg"
            size="icon-sm"
            title={item.label}
            type="button"
            variant="ghost"
            onClick={() => onSelect(item)}
          >
            <span aria-hidden="true">{item.value}</span>
          </Button>
        ))}
      </div>
    </section>
  )
}

function getFrequentExpressionItems(usage: StoredExpressionUsage[]) {
  const usedItems = usage
    .slice()
    .sort((left, right) => {
      if (right.count !== left.count) {
        return right.count - left.count
      }

      return right.lastUsedAt - left.lastUsedAt
    })
    .flatMap((item) => {
      const expressionItem = allExpressionsByValue.get(item.value)

      return expressionItem ? [expressionItem] : []
    })

  const usedValues = new Set(usedItems.map((item) => item.value))
  const fallbackItems = Array.from(
    new Map(
      [...defaultFrequentExpressionItems, ...allExpressionItems].map((item) => [
        item.value,
        item,
      ])
    ).values()
  ).filter((item) => !usedValues.has(item.value))

  return [...usedItems, ...fallbackItems].slice(0, frequentExpressionLimit)
}

function readExpressionUsage(): StoredExpressionUsage[] {
  if (typeof window === "undefined") {
    return []
  }

  try {
    const rawUsage = window.localStorage.getItem(expressionUsageStorageKey)
    if (!rawUsage) {
      return []
    }

    return normalizeExpressionUsage(JSON.parse(rawUsage), Date.now())
  } catch {
    return []
  }
}

function writeExpressionUsage(usage: StoredExpressionUsage[]) {
  if (typeof window === "undefined") {
    return
  }

  try {
    window.localStorage.setItem(
      expressionUsageStorageKey,
      JSON.stringify(usage)
    )
  } catch {
    // Ignore storage errors. Selecting an expression should still work.
  }
}

function normalizeExpressionUsage(
  value: unknown,
  now: number
): StoredExpressionUsage[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.flatMap((item) => {
    if (!isStoredExpressionUsage(item)) {
      return []
    }
    if (!allExpressionsByValue.has(item.value)) {
      return []
    }
    if (now - item.lastUsedAt > expressionUsageMaxAgeMs) {
      return []
    }

    return [item]
  })
}

function updateExpressionUsage(
  usage: StoredExpressionUsage[],
  value: string,
  now: number
) {
  const nextUsageByValue = new Map(
    normalizeExpressionUsage(usage, now).map((item) => [item.value, item])
  )
  const previousUsage = nextUsageByValue.get(value)

  nextUsageByValue.set(value, {
    count: (previousUsage?.count ?? 0) + 1,
    lastUsedAt: now,
    value,
  })

  return Array.from(nextUsageByValue.values())
}

function isStoredExpressionUsage(
  value: unknown
): value is StoredExpressionUsage {
  if (!value || typeof value !== "object") {
    return false
  }

  const usage = value as Partial<StoredExpressionUsage>

  return (
    typeof usage.count === "number" &&
    usage.count > 0 &&
    typeof usage.lastUsedAt === "number" &&
    usage.lastUsedAt > 0 &&
    typeof usage.value === "string"
  )
}

function expression(value: string, label: string): ExpressionItem {
  return {
    label,
    type: "emoji",
    value,
  }
}
