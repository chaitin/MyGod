import {
  Dialog,
  SizableText,
  Spinner,
  VisuallyHidden,
  XStack,
  YStack,
} from "tamagui"

import { AppButton } from "@/components/forms/app-button"
import type { PreparedClientVoiceMessage } from "@/data/message-upload"
import { formatVoiceDuration } from "@/domain/messages/message-presenter"
import { VoiceRecordingPreviewButton } from "@/features/conversation/voice-recording-preview-button"

export function MessageVoiceDialog({
  onCancel,
  onConfirm,
  recording,
  sending,
}: {
  onCancel: () => void
  onConfirm: () => void
  recording: PreparedClientVoiceMessage | null
  sending: boolean
}) {
  if (!recording) return null

  return (
    <Dialog
      modal
      onOpenChange={(open) => {
        if (!open && !sending) onCancel()
      }}
      open
    >
      <Dialog.Portal>
        <Dialog.Overlay bg="$shadow6" opacity={0.5} />
        <Dialog.Content bordered elevate gap="$4" maxW={440} width="90%">
          <Dialog.Title fontSize="$4" lineHeight="$5">
            发送语音
          </Dialog.Title>
          <VisuallyHidden>
            <Dialog.Description>
              选择取消或将录制完成的语音发送到当前会话
            </Dialog.Description>
          </VisuallyHidden>

          <YStack
            bg="$backgroundPress"
            gap="$3"
            items="center"
            justify="center"
            minH={120}
            rounded="$4"
          >
            <VoiceRecordingPreviewButton
              disabled={sending}
              uri={recording.upload.uri}
            />
            <SizableText color="$color10" size="$3">
              语音 {formatVoiceDuration(recording.durationMS)}
            </SizableText>
          </YStack>

          <XStack gap="$3" width="100%">
            <AppButton
              accessibilityLabel="取消发送语音"
              disabled={sending}
              grow={1}
              onPress={onCancel}
              theme="gray"
            >
              取消
            </AppButton>
            <AppButton
              accessibilityLabel="发送语音"
              disabled={sending}
              grow={1}
              icon={sending ? <Spinner size="small" /> : undefined}
              onPress={onConfirm}
              theme="accent"
            >
              {sending ? "发送中…" : "发送"}
            </AppButton>
          </XStack>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog>
  )
}
