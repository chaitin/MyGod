import { SmilePlus } from "lucide-react"

import { ExpressionPickerPopover } from "@/components/expression-picker-popover"
import { messageHoverActionButtonClassName } from "@/components/conversation/message-hover-action-button"

export function MessageReactionPicker({
  align,
  disabled,
  onSelect,
}: {
  align: "start" | "end"
  disabled?: boolean
  onSelect: (text: string) => void
}) {
  return (
    <ExpressionPickerPopover
      align={align}
      onSelect={(item) => onSelect(item.value)}
    >
      <button
        aria-label="添加表情"
        className={messageHoverActionButtonClassName}
        data-slot="message-reaction-add"
        disabled={disabled}
        type="button"
      >
        <SmilePlus className="size-3.5" />
      </button>
    </ExpressionPickerPopover>
  )
}
