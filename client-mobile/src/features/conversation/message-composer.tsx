import {
  CirclePlus,
  Keyboard as KeyboardIcon,
  Mic,
  Send,
  Smile,
} from "lucide-react-native"
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react"
import {
  Keyboard,
  Pressable,
  StyleSheet,
  Vibration,
} from "react-native"
import {
  Button,
  SizableText,
  type TamaguiElement,
  useToastController,
  XStack,
  YStack,
} from "tamagui"

import { CompactIconButton } from "@/components/buttons/compact-icon-button"
import type { AppToastTone } from "@/components/feedback/app-toast"
import { AppInput } from "@/components/forms/app-input"
import type {
  PreparedClientMessageUpload,
  PreparedClientVoiceMessage,
} from "@/data/message-upload"
import type { ServerTarget } from "@/data/query"
import {
  ComposerAccessoryPanel,
  type ComposerAccessoryMode,
} from "@/features/conversation/composer-accessory-panel"
import {
  createDraftMentionTemplate,
  findInsertedMentionTrigger,
  getCursorAfterTextChange,
  insertDraftMention,
  syncDraftMentions,
  type DraftMention,
  type MentionSelection,
  type TextSelection,
} from "@/features/conversation/mention-draft"
import type { MentionCandidate } from "@/features/conversation/mention-model"
import { MentionPickerSheet } from "@/features/conversation/mention-picker-sheet"
import {
  pickCameraImageMessage,
  pickFileMessage,
  pickLibraryImageMessage,
} from "@/features/conversation/message-upload-picker"
import { MessageUploadDialog } from "@/features/conversation/message-upload-dialog"
import { MessageVoiceDialog } from "@/features/conversation/message-voice-dialog"
import { useVoiceMessageRecorder } from "@/features/conversation/use-voice-message-recorder"

export type MessageComposerHandle = {
  dismissAccessory: () => void
  insertMention: (target: MentionSelection) => void
}

const COMPOSER_CONTROL_HEIGHT = 38
const COMPOSER_INPUT_GAP = 8
const COMPOSER_INPUT_HORIZONTAL_PADDING = 8
const COMPOSER_LINE_HEIGHT = 22
const COMPOSER_MAX_LINES = 4
const COMPOSER_MAX_CONTROL_HEIGHT =
  COMPOSER_CONTROL_HEIGHT + COMPOSER_LINE_HEIGHT * (COMPOSER_MAX_LINES - 1)
const COMPOSER_PANEL_HEIGHT = 56
const COMPOSER_EXTRA_BOTTOM_PADDING = 4
const COMPOSER_PANEL_VERTICAL_CHROME =
  COMPOSER_PANEL_HEIGHT - COMPOSER_CONTROL_HEIGHT
const COMPOSER_TEXT_VERTICAL_CHROME =
  COMPOSER_CONTROL_HEIGHT - COMPOSER_LINE_HEIGHT

export const MessageComposer = forwardRef<
  MessageComposerHandle,
  {
    disabled: boolean
    mentionCandidates: MentionCandidate[]
    onSend: (content: string) => Promise<boolean>
    onSendUpload: (selection: PreparedClientMessageUpload) => Promise<boolean>
    onSendVoice: (recording: PreparedClientVoiceMessage) => Promise<boolean>
    server: ServerTarget
  }
>(function MessageComposer(
  {
    disabled,
    mentionCandidates,
    onSend,
    onSendUpload,
    onSendVoice,
    server,
  },
  ref
) {
  const toast = useToastController()
  const voiceRecorder = useVoiceMessageRecorder()
  const inputRef = useRef<TamaguiElement>(null)
  const contentRef = useRef("")
  const mentionsRef = useRef<DraftMention[]>([])
  const mentionTriggerRef = useRef<TextSelection | null>(null)
  const selectionRef = useRef<TextSelection>({ end: 0, start: 0 })
  const selectedUploadRef = useRef<PreparedClientMessageUpload | null>(null)
  const shouldFocusAfterPickerCloseRef = useRef(false)
  const uploadInFlightRef = useRef(false)
  const voiceUploadInFlightRef = useRef(false)
  const voiceRecordingRef = useRef<PreparedClientVoiceMessage | null>(null)
  const inputVoiceGestureRef = useRef(false)
  const mountedRef = useRef(true)
  const [content, setContent] = useState("")
  const [inputHeight, setInputHeight] = useState(COMPOSER_CONTROL_HEIGHT)
  const [accessoryMode, setAccessoryMode] =
    useState<ComposerAccessoryMode>(null)
  const [preparingUpload, setPreparingUpload] = useState(false)
  const [selectedUpload, setSelectedUpload] =
    useState<PreparedClientMessageUpload | null>(null)
  const [mentionPickerOpen, setMentionPickerOpen] = useState(false)
  const [voiceMode, setVoiceMode] = useState(false)
  const [pendingSelection, setPendingSelection] =
    useState<TextSelection>()
  const canSend = content.trim().length > 0 && !disabled
  const voiceInteractionActive =
    voiceRecorder.status === "requesting" ||
    voiceRecorder.status === "recording" ||
    voiceRecorder.status === "processing"
  const interactionDisabled =
    disabled || preparingUpload || voiceInteractionActive
  const voicePrompt = getVoicePrompt(
    voiceRecorder.status,
    voiceRecorder.elapsedMS
  )
  const visibleControlHeight = voiceMode
    ? COMPOSER_CONTROL_HEIGHT
    : inputHeight
  const composerPanelHeight =
    visibleControlHeight + COMPOSER_PANEL_VERTICAL_CHROME
  const inputScrollEnabled = inputHeight >= COMPOSER_MAX_CONTROL_HEIGHT

  useEffect(() => {
    voiceRecordingRef.current = voiceRecorder.recording
  }, [voiceRecorder.recording])

  useEffect(() => {
    if (voiceRecorder.status === "recording") Vibration.vibrate(35)
  }, [voiceRecorder.status])

  useEffect(() => {
    if (!voiceRecorder.error) return

    toast.show("无法录音", {
      customData: { tone: "error" satisfies AppToastTone },
      duration: 4000,
      message: voiceRecorder.error,
    })
    voiceRecorder.clearError()
  }, [toast, voiceRecorder])

  useEffect(() => {
    if (!pendingSelection) return

    const frame = requestAnimationFrame(() => setPendingSelection(undefined))
    return () => cancelAnimationFrame(frame)
  }, [pendingSelection])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      if (!uploadInFlightRef.current) {
        selectedUploadRef.current?.cleanup?.()
        selectedUploadRef.current = null
      }
      if (!voiceUploadInFlightRef.current) {
        voiceRecordingRef.current?.cleanup()
        voiceRecordingRef.current = null
      }
    }
  }, [])

  useImperativeHandle(ref, () => ({
    dismissAccessory() {
      setAccessoryMode(null)
    },
    insertMention(target) {
      if (!disabled) insertMentionTarget(target)
    },
  }))

  function updateDraft(value: string, mentions: DraftMention[]) {
    contentRef.current = value
    mentionsRef.current = mentions
    setContent(value)
  }

  function recordSelection(nextSelection: TextSelection) {
    selectionRef.current = nextSelection
  }

  function requestSelection(nextSelection: TextSelection) {
    recordSelection(nextSelection)
    setPendingSelection(nextSelection)
  }

  function replaceSelectedUpload(
    selection: PreparedClientMessageUpload | null
  ) {
    if (selectedUploadRef.current !== selection) {
      selectedUploadRef.current?.cleanup?.()
    }
    selectedUploadRef.current = selection
    if (mountedRef.current) setSelectedUpload(selection)
  }

  function handleContentChange(value: string) {
    const previousValue = contentRef.current
    const nextMentions = syncDraftMentions(
      mentionsRef.current,
      previousValue,
      value
    )
    const cursor = getCursorAfterTextChange(previousValue, value)
    const nextSelection = { end: cursor, start: cursor }
    const mentionTrigger = findInsertedMentionTrigger(previousValue, value)

    updateDraft(value, nextMentions)
    recordSelection(nextSelection)

    if (mentionTrigger && mentionCandidates.length > 0) {
      mentionTriggerRef.current = mentionTrigger
      setAccessoryMode(null)
      Keyboard.dismiss()
      setMentionPickerOpen(true)
    }
  }

  function handleSelectionChange(
    event: { nativeEvent: { selection: TextSelection } }
  ) {
    const nextSelection = event.nativeEvent.selection
    recordSelection(nextSelection)
  }

  function handleInputContentSizeChange(
    event: { nativeEvent: { contentSize: { height: number; width: number } } }
  ) {
    const measuredHeight = Math.ceil(event.nativeEvent.contentSize.height)
    const nextHeight = Math.max(
      COMPOSER_CONTROL_HEIGHT,
      Math.min(
        COMPOSER_MAX_CONTROL_HEIGHT,
        measuredHeight + COMPOSER_TEXT_VERTICAL_CHROME
      )
    )
    setInputHeight((currentHeight) =>
      currentHeight === nextHeight ? currentHeight : nextHeight
    )
  }

  function insertMentionTarget(
    target: MentionSelection,
    explicitSelection?: TextSelection
  ) {
    const result = insertDraftMention({
      mentions: mentionsRef.current,
      selection: explicitSelection ?? selectionRef.current,
      target,
      value: contentRef.current,
    })
    const nextSelection = { end: result.cursor, start: result.cursor }

    updateDraft(result.value, result.mentions)
    setVoiceMode(false)
    requestSelection(nextSelection)
    mentionTriggerRef.current = null
    setAccessoryMode(null)

    if (mentionPickerOpen) {
      shouldFocusAfterPickerCloseRef.current = true
      setMentionPickerOpen(false)
    } else {
      focusInputAfterRender()
    }
  }

  function handleMentionSelect(candidate: MentionCandidate) {
    insertMentionTarget(
      candidate,
      mentionTriggerRef.current ?? selectionRef.current
    )
  }

  function handleMentionPickerOpenChange(open: boolean) {
    setMentionPickerOpen(open)
    if (open) {
      setAccessoryMode(null)
      shouldFocusAfterPickerCloseRef.current = false
      return
    }

    mentionTriggerRef.current = null
    shouldFocusAfterPickerCloseRef.current = true
  }

  function handleMentionPickerAnimationComplete(open: boolean) {
    if (open || !shouldFocusAfterPickerCloseRef.current) return

    shouldFocusAfterPickerCloseRef.current = false
    if (!disabled) inputRef.current?.focus()
  }

  function focusInputAfterRender() {
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  async function handleSend() {
    const message = createDraftMentionTemplate(
      contentRef.current,
      mentionsRef.current
    ).trim()
    if (!message || disabled) return
    if (await onSend(message)) {
      mentionTriggerRef.current = null
      setMentionPickerOpen(false)
      updateDraft("", [])
      setInputHeight(COMPOSER_CONTROL_HEIGHT)
      requestSelection({ end: 0, start: 0 })
    }
  }

  function handleAccessoryToggle(mode: Exclude<ComposerAccessoryMode, null>) {
    if (interactionDisabled) return

    Keyboard.dismiss()
    setVoiceMode(false)
    setMentionPickerOpen(false)
    setAccessoryMode((current) => (current === mode ? null : mode))
  }

  function handleEmojiPress(emoji: string) {
    if (interactionDisabled) return

    setVoiceMode(false)
    const currentValue = contentRef.current
    const selection = clampSelection(selectionRef.current, currentValue.length)
    const nextValue =
      currentValue.slice(0, selection.start) +
      emoji +
      currentValue.slice(selection.end)
    const nextMentions = syncDraftMentions(
      mentionsRef.current,
      currentValue,
      nextValue
    )
    const cursor = selection.start + emoji.length

    updateDraft(nextValue, nextMentions)
    requestSelection({ end: cursor, start: cursor })
  }

  async function handleUploadPick(
    picker: () => Promise<PreparedClientMessageUpload | null>
  ) {
    if (interactionDisabled) return

    setAccessoryMode(null)
    setPreparingUpload(true)

    try {
      const selection = await picker()
      if (selection) {
        if (mountedRef.current) replaceSelectedUpload(selection)
        else selection.cleanup?.()
      }
    } catch (error: unknown) {
      toast.show("无法选择文件", {
        customData: { tone: "error" satisfies AppToastTone },
        duration: 4000,
        message: error instanceof Error ? error.message : "请稍后重试",
      })
    } finally {
      if (mountedRef.current) setPreparingUpload(false)
    }
  }

  async function handleUploadConfirm() {
    if (!selectedUpload || disabled) return

    const selection = selectedUpload
    uploadInFlightRef.current = true
    try {
      if (await onSendUpload(selection)) replaceSelectedUpload(null)
    } finally {
      uploadInFlightRef.current = false
      if (!mountedRef.current && selectedUploadRef.current === selection) {
        replaceSelectedUpload(null)
      }
    }
  }

  function handleVoiceModeToggle() {
    if (interactionDisabled) return

    const nextVoiceMode = !voiceMode
    setAccessoryMode(null)
    setMentionPickerOpen(false)
    setVoiceMode(nextVoiceMode)
    if (nextVoiceMode) Keyboard.dismiss()
    else focusInputAfterRender()
  }

  function handleVoicePressIn() {
    if (interactionDisabled || !voiceMode) return
    void voiceRecorder.startRecording()
  }

  function handleVoicePressOut() {
    if (!voiceMode) return
    void voiceRecorder.stopRecording()
  }

  function handleInputPress() {
    if (inputVoiceGestureRef.current) {
      inputVoiceGestureRef.current = false
      return
    }
    if (interactionDisabled) return

    setAccessoryMode(null)
    setMentionPickerOpen(false)
    focusInputAfterRender()
  }

  function handleInputLongPress() {
    if (interactionDisabled || contentRef.current.trim()) return

    inputVoiceGestureRef.current = true
    setAccessoryMode(null)
    setMentionPickerOpen(false)
    inputRef.current?.blur()
    Keyboard.dismiss()
    void voiceRecorder.startRecording()
  }

  function handleInputPressOut() {
    if (!inputVoiceGestureRef.current) return
    void voiceRecorder.stopRecording()
  }

  async function handleVoiceConfirm() {
    const recording = voiceRecorder.recording
    if (!recording || disabled) return

    voiceUploadInFlightRef.current = true
    try {
      if (await onSendVoice(recording)) voiceRecorder.resetRecording()
    } finally {
      voiceUploadInFlightRef.current = false
      if (!mountedRef.current && voiceRecordingRef.current === recording) {
        recording.cleanup()
        voiceRecordingRef.current = null
      }
    }
  }

  return (
    <>
      <YStack bg="$background">
        <XStack
          height={composerPanelHeight}
          items="center"
          pb={COMPOSER_EXTRA_BOTTOM_PADDING}
          px="$2"
        >
          <CompactIconButton
            accessibilityLabel={voiceMode ? "切换到键盘输入" : "切换到语音输入"}
            disabled={interactionDisabled}
            icon={voiceMode ? KeyboardIcon : Mic}
            iconSize={26}
            onPress={handleVoiceModeToggle}
            strokeWidth={1.5}
          />
          <YStack
            bg={voiceInteractionActive ? "$color5" : "$color1"}
            flex={1}
            height={visibleControlHeight}
            mx={COMPOSER_INPUT_GAP}
            rounded="$4"
          >
            {voiceMode ? (
              <VoiceRecordButton
                disabled={disabled || preparingUpload}
                onPressIn={handleVoicePressIn}
                onPressOut={handleVoicePressOut}
                prompt={voicePrompt}
                recording={voiceInteractionActive}
              />
            ) : (
              <>
                {voiceInteractionActive ? (
                  <XStack
                    height={inputHeight}
                    items="center"
                    justify="center"
                    px={COMPOSER_INPUT_HORIZONTAL_PADDING}
                    width="100%"
                  >
                    <SizableText size="$4" text="center">
                      {voicePrompt}
                    </SizableText>
                  </XStack>
                ) : (
                  <AppInput
                    autoCapitalize="sentences"
                    bg="transparent"
                    borderWidth={0}
                    color="$gray12"
                    disabled={disabled}
                    fontFamily="$body"
                    fontSize="$4"
                    focusStyle={{ borderWidth: 0, outlineWidth: 0 }}
                    height={inputHeight}
                    minH={0}
                    multiline
                    onChangeText={handleContentChange}
                    onContentSizeChange={handleInputContentSizeChange}
                    onFocus={() => setAccessoryMode(null)}
                    onSelectionChange={handleSelectionChange}
                    placeholder="发消息 或 按住说话"
                    placeholderTextColor="$gray9"
                    px={COMPOSER_INPUT_HORIZONTAL_PADDING}
                    py={0}
                    ref={inputRef}
                    returnKeyType="default"
                    scrollEnabled={inputScrollEnabled}
                    selection={pendingSelection}
                    style={styles.composerInput}
                    submitBehavior="newline"
                    textAlignVertical="center"
                    unstyled
                    value={content}
                    width="100%"
                  />
                )}
                {content.trim().length === 0 ? (
                  <Pressable
                    accessibilityHint="短按输入文字，长按录制语音"
                    accessibilityLabel="发消息 或 按住说话"
                    delayLongPress={400}
                    disabled={disabled || preparingUpload}
                    onLongPress={handleInputLongPress}
                    onPress={handleInputPress}
                    onPressIn={() => {
                      inputVoiceGestureRef.current = false
                    }}
                    onPressOut={handleInputPressOut}
                    style={styles.inputGestureTarget}
                  />
                ) : null}
              </>
            )}
          </YStack>
          <XStack gap="$1" items="center">
            <CompactIconButton
              accessibilityLabel="选择表情"
              disabled={interactionDisabled}
              icon={Smile}
              iconSize={26}
              onPress={() => handleAccessoryToggle("emoji")}
              strokeWidth={1.5}
            />
            {!voiceMode && content.trim().length > 0 ? (
              <CompactIconButton
                accessibilityLabel="发送消息"
                disabled={!canSend}
                icon={Send}
                iconSize={26}
                loading={disabled}
                onPress={() => void handleSend()}
                strokeWidth={1.5}
              />
            ) : (
              <CompactIconButton
                accessibilityLabel="添加图片或附件"
                disabled={interactionDisabled}
                icon={CirclePlus}
                iconSize={26}
                loading={preparingUpload}
                onPress={() => handleAccessoryToggle("attachments")}
                strokeWidth={1.5}
              />
            )}
          </XStack>
        </XStack>
        <ComposerAccessoryPanel
          disabled={interactionDisabled}
          mode={accessoryMode}
          onCameraPress={() => void handleUploadPick(pickCameraImageMessage)}
          onEmojiPress={handleEmojiPress}
          onFilePress={() => void handleUploadPick(pickFileMessage)}
          onLibraryPress={() =>
            void handleUploadPick(pickLibraryImageMessage)
          }
        />
      </YStack>

      <MentionPickerSheet
        candidates={mentionCandidates}
        onAnimationComplete={handleMentionPickerAnimationComplete}
        onOpenChange={handleMentionPickerOpenChange}
        onSelect={handleMentionSelect}
        open={mentionPickerOpen}
        server={server}
      />
      <MessageUploadDialog
        onCancel={() => replaceSelectedUpload(null)}
        onConfirm={() => void handleUploadConfirm()}
        selection={selectedUpload}
        sending={disabled}
      />
      <MessageVoiceDialog
        onCancel={voiceRecorder.resetRecording}
        onConfirm={() => void handleVoiceConfirm()}
        recording={voiceRecorder.recording}
        sending={disabled}
      />
    </>
  )
})

function getVoicePrompt(
  status: ReturnType<typeof useVoiceMessageRecorder>["status"],
  elapsedMS: number
) {
  if (status === "requesting" || status === "recording") {
    return `正在录音 ${formatRecordingDuration(elapsedMS)}`
  }
  if (status === "processing") return "正在生成语音…"
  return "按住 说话"
}

function formatRecordingDuration(durationMS: number) {
  const totalSeconds = Math.floor(durationMS / 1_000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
}

function VoiceRecordButton({
  disabled,
  onPressIn,
  onPressOut,
  prompt,
  recording,
}: {
  disabled: boolean
  onPressIn: () => void
  onPressOut: () => void
  prompt: string
  recording: boolean
}) {
  return (
    <Pressable
      accessibilityHint="按住开始录音，松开结束录音"
      accessibilityLabel="按住说话"
      accessibilityRole="button"
      disabled={disabled}
      onPressIn={onPressIn}
      onPressOut={onPressOut}
      style={styles.voicePressTarget}
    >
      {({ pressed }) => (
        <Button
          accessible={false}
          bg={recording ? "$color5" : "$color1"}
          borderWidth={0}
          disabled={disabled}
          forceStyle={pressed ? "press" : undefined}
          height={COMPOSER_CONTROL_HEIGHT}
          minH={0}
          pointerEvents="none"
          pressStyle={{ bg: "$color2" }}
          size="$4"
          width="100%"
        >
          {prompt}
        </Button>
      )}
    </Pressable>
  )
}

const styles = StyleSheet.create({
  composerInput: {
    includeFontPadding: false,
    paddingVertical: 0,
  },
  inputGestureTarget: {
    ...StyleSheet.absoluteFill,
  },
  voicePressTarget: {
    flex: 1,
  },
})

function clampSelection(selection: TextSelection, valueLength: number) {
  const start = Math.max(0, Math.min(selection.start, valueLength))
  const end = Math.max(start, Math.min(selection.end, valueLength))
  return { end, start }
}
