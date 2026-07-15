import { Send } from "lucide-react-native"
import { useState } from "react"
import { Button, Input, Separator, Spinner, XStack, YStack } from "tamagui"

import { ThemedIcon } from "@/components/icons/themed-icon"

export function MessageComposer({
  disabled,
  onSend,
}: {
  disabled: boolean
  onSend: (content: string) => Promise<boolean>
}) {
  const [content, setContent] = useState("")
  const canSend = content.trim().length > 0 && !disabled

  async function handleSend() {
    const message = content.trim()
    if (!message || disabled) return
    if (await onSend(message)) setContent("")
  }

  return (
    <YStack bg="$background">
      <Separator />
      <XStack gap="$2" items="center" p="$3">
        <Input
          autoCapitalize="sentences"
          disabled={disabled}
          flex={1}
          onChangeText={setContent}
          onSubmitEditing={() => void handleSend()}
          placeholder="输入消息"
          returnKeyType="send"
          value={content}
        />
        <Button
          accessibilityLabel="发送消息"
          circular
          disabled={!canSend}
          icon={
            disabled ? <Spinner /> : <ThemedIcon icon={Send} size={18} />
          }
          onPress={() => void handleSend()}
          size="$4"
          theme="blue"
        />
      </XStack>
    </YStack>
  )
}
