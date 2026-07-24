import * as React from "react"
import {
  ImageIcon,
  LoaderCircle,
  Paperclip,
  Send,
  Smile,
  UsersRound,
  X,
} from "lucide-react"
import { toast } from "sonner"
import { getAvatarInitial } from "@/lib/avatar"
import { cn } from "@/lib/utils"
import {
  type ClientConversation,
  type ClientMessage,
} from "@/lib/client-data-api"
import {
  compressImageForMessage,
  imageMessageMaxBytes,
  isAcceptedImageMessageFile,
} from "@/lib/image-message"
import { getFileMessageUploadError } from "@/lib/file-message"
import type { ConversationDraftMention } from "@/lib/conversation-drafts"
import type { VoiceMessageRecording } from "@/lib/voice-message"
import {
  createDraftMentionTemplate,
  createMentionCandidates,
  filterMentionCandidates,
  getClipboardImageFile,
  getMentionTrigger,
  getVisibleMentionIndex,
  insertTextareaText,
  isImeCompositionKeyEvent,
  syncDraftMentions,
  type MentionCandidate,
  type MentionTrigger,
} from "@/lib/conversation-composer"
import { type ExpressionItem } from "@/components/expression-picker"
import { ExpressionPickerPopover } from "@/components/expression-picker-popover"
import { SendVoiceMessageDialog } from "@/components/conversation/send-voice-message-dialog"
import { SmartVoiceInputDialog } from "@/components/conversation/smart-voice-input-dialog"
import { ConversationVoiceMenu } from "@/components/conversation/conversation-voice-menu"
import { MarkdownIcon } from "@/components/icons/markdown-icon"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Button } from "@/components/ui/button"
import { SendFileMessageDialog } from "@/components/send-file-message-dialog"
import { SendImageMessageDialog } from "@/components/send-image-message-dialog"
import { Textarea } from "@/components/ui/textarea"
import { Toggle } from "@/components/ui/toggle"
import type {
  ConversationPanelComposerHandle,
  ConversationPanelMentionTarget,
  ConversationPanelReplyTarget,
} from "@/lib/conversation-panel-types"

export const ConversationPanelComposer = React.forwardRef<
  ConversationPanelComposerHandle,
  {
    conversation: ClientConversation
    draft: string
    draftMentions: ConversationDraftMention[]
    replyTarget: ConversationPanelReplyTarget | null
    onCancelReply: () => void
    onDraftBlur?: () => void
    onDraftChange: (draft: string, mentions: ConversationDraftMention[]) => void
    onSendFile: (file: File) => Promise<ClientMessage | null>
    onSendImage: (image: File) => Promise<ClientMessage | null>
    onSendVoice: (voice: VoiceMessageRecording) => Promise<ClientMessage | null>
    onRichTextModeChange: (richTextMode: boolean) => void
    onSendMessage: (content?: string) => void
    richTextMode: boolean
    sending: boolean
  }
>(function ConversationPanelComposer(
  {
    conversation,
    draft,
    draftMentions,
    replyTarget,
    onCancelReply,
    onDraftBlur,
    onDraftChange,
    onSendFile,
    onSendImage,
    onSendVoice,
    onRichTextModeChange,
    onSendMessage,
    richTextMode,
    sending,
  },
  ref
) {
  const fileInputRef = React.useRef<HTMLInputElement | null>(null)
  const imageInputRef = React.useRef<HTMLInputElement | null>(null)
  const mentionOptionRefs = React.useRef<Array<HTMLButtonElement | null>>([])
  const textareaRef = React.useRef<HTMLTextAreaElement | null>(null)
  const previousSendingRef = React.useRef(sending)
  const shouldFocusAfterSendingRef = React.useRef(false)
  const [expressionPickerOpen, setExpressionPickerOpen] = React.useState(false)
  const [fileDialogOpen, setFileDialogOpen] = React.useState(false)
  const [imageDialogOpen, setImageDialogOpen] = React.useState(false)
  const [imagePreparing, setImagePreparing] = React.useState(false)
  const [sendVoiceDialogOpen, setSendVoiceDialogOpen] = React.useState(false)
  const [smartVoiceDialogOpen, setSmartVoiceDialogOpen] = React.useState(false)
  const [mentionTrigger, setMentionTrigger] =
    React.useState<MentionTrigger | null>(null)
  const [selectedMentionIndex, setSelectedMentionIndex] = React.useState(0)
  const [selectedFile, setSelectedFile] = React.useState<File | null>(null)
  const [selectedImage, setSelectedImage] = React.useState<File | null>(null)
  const mentionCandidates = React.useMemo(
    () =>
      conversation.type === "group" ||
      conversation.topic?.parentConversationType === "group"
        ? createMentionCandidates(conversation.members ?? [])
        : [],
    [
      conversation.members,
      conversation.topic?.parentConversationType,
      conversation.type,
    ]
  )
  const filteredMentionCandidates = React.useMemo(
    () =>
      filterMentionCandidates(mentionCandidates, mentionTrigger?.query ?? ""),
    [mentionCandidates, mentionTrigger?.query]
  )

  React.useImperativeHandle(ref, () => ({
    focus() {
      window.requestAnimationFrame(() => {
        textareaRef.current?.focus()
      })
    },
    insertMention(target) {
      insertMentionTarget(target)
    },
    openDroppedFile(file) {
      if (sending || imagePreparing) {
        return
      }

      if (isAcceptedImageMessageFile(file)) {
        void prepareSelectedImage(file)
        return
      }

      prepareSelectedFile(file)
    },
  }))

  React.useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  React.useEffect(() => {
    if (!mentionTrigger) {
      return
    }

    const visibleSelectedIndex = getVisibleMentionIndex(
      selectedMentionIndex,
      filteredMentionCandidates.length
    )

    mentionOptionRefs.current[visibleSelectedIndex]?.scrollIntoView({
      block: "nearest",
    })
  }, [filteredMentionCandidates.length, mentionTrigger, selectedMentionIndex])

  React.useEffect(() => {
    if (!replyTarget) {
      return
    }

    const frame = window.requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })

    return () => {
      window.cancelAnimationFrame(frame)
    }
  }, [replyTarget])

  React.useEffect(() => {
    const wasSending = previousSendingRef.current
    previousSendingRef.current = sending

    if (sending || !wasSending || !shouldFocusAfterSendingRef.current) {
      return
    }

    shouldFocusAfterSendingRef.current = false
    const textarea = textareaRef.current
    if (!textarea) {
      return
    }

    textarea.focus()
  }, [sending])

  function handleDraftChange(event: React.ChangeEvent<HTMLTextAreaElement>) {
    const nextDraft = event.target.value
    const cursor = event.target.selectionStart
    const nextMentions = syncDraftMentions(draftMentions, draft, nextDraft)

    onDraftChange(nextDraft, nextMentions)
    updateMentionTrigger(nextDraft, cursor)
  }

  function updateMentionTrigger(value: string, cursor: number) {
    if (
      (conversation.type !== "group" &&
        conversation.topic?.parentConversationType !== "group") ||
      mentionCandidates.length === 0
    ) {
      setMentionTrigger(null)
      setSelectedMentionIndex(0)
      return
    }

    setMentionTrigger(getMentionTrigger(value, cursor))
    setSelectedMentionIndex(0)
  }

  function handleSendMessage() {
    if (sending || !draft.trim()) {
      return
    }

    shouldFocusAfterSendingRef.current = true
    onSendMessage(createDraftMentionTemplate(draft, draftMentions))
    setMentionTrigger(null)
    setSelectedMentionIndex(0)
  }

  function handleComposerKeyDown(
    event: React.KeyboardEvent<HTMLTextAreaElement>
  ) {
    if (isImeCompositionKeyEvent(event)) {
      return
    }

    if (mentionTrigger && filteredMentionCandidates.length > 0) {
      if (event.key === "ArrowDown") {
        event.preventDefault()
        setSelectedMentionIndex(
          (currentIndex) =>
            (currentIndex + 1) % filteredMentionCandidates.length
        )
        return
      }
      if (event.key === "ArrowUp") {
        event.preventDefault()
        setSelectedMentionIndex(
          (currentIndex) =>
            (currentIndex - 1 + filteredMentionCandidates.length) %
            filteredMentionCandidates.length
        )
        return
      }
      if (event.key === "Escape") {
        event.preventDefault()
        setMentionTrigger(null)
        return
      }
      if (event.key === "Enter" || event.key === "Tab") {
        event.preventDefault()
        insertMentionCandidate(
          filteredMentionCandidates[
            getVisibleMentionIndex(
              selectedMentionIndex,
              filteredMentionCandidates.length
            )
          ] ?? filteredMentionCandidates[0]
        )
        return
      }
    }

    if (event.key !== "Enter") {
      return
    }

    if (sending) {
      event.preventDefault()
      return
    }

    if (event.shiftKey || event.ctrlKey) {
      event.preventDefault()
      insertTextareaText(event.currentTarget, "\n", handleTextareaValueChange)
      return
    }

    event.preventDefault()
    handleSendMessage()
  }

  function handleTextareaValueChange(value: string, cursor?: number) {
    onDraftChange(value, syncDraftMentions(draftMentions, draft, value))
    updateMentionTrigger(value, cursor ?? value.length)
  }

  function insertMentionCandidate(candidate: MentionCandidate | undefined) {
    if (!candidate) {
      return
    }

    const textarea = textareaRef.current
    const cursor = textarea?.selectionStart ?? draft.length
    const trigger = getMentionTrigger(draft, cursor)

    insertMentionTarget(candidate, {
      end: cursor,
      start: trigger?.start ?? cursor,
    })
  }

  function insertMentionTarget(
    target: ConversationPanelMentionTarget,
    range?: {
      end: number
      start: number
    }
  ) {
    const textarea = textareaRef.current
    const selectionStart =
      range?.start ?? textarea?.selectionStart ?? draft.length
    const selectionEnd = range?.end ?? textarea?.selectionEnd ?? selectionStart

    const mentionText = `@${target.label}`
    const insertedText = `${mentionText} `
    const nextDraft =
      draft.slice(0, selectionStart) + insertedText + draft.slice(selectionEnd)
    const nextMention: ConversationDraftMention = {
      end: selectionStart + mentionText.length,
      id: target.id,
      label: target.label,
      start: selectionStart,
      targetType: target.targetType,
    }

    const nextMentions = [
      ...syncDraftMentions(
        draftMentions.filter(
          (mention) =>
            mention.end <= selectionStart || mention.start >= selectionEnd
        ),
        draft,
        nextDraft
      ),
      nextMention,
    ].sort((mentionA, mentionB) => mentionA.start - mentionB.start)

    onDraftChange(nextDraft, nextMentions)
    setMentionTrigger(null)
    setSelectedMentionIndex(0)

    window.requestAnimationFrame(() => {
      if (!textareaRef.current) {
        return
      }

      const nextCursor = selectionStart + insertedText.length
      textareaRef.current.focus()
      textareaRef.current.setSelectionRange(nextCursor, nextCursor)
    })
  }

  function handleExpressionSelect(item: ExpressionItem) {
    if (sending) {
      return
    }

    const textarea = textareaRef.current

    if (!textarea) {
      handleTextareaValueChange(draft + item.value)
      setExpressionPickerOpen(false)
      return
    }

    insertTextareaText(textarea, item.value, handleTextareaValueChange)
    setExpressionPickerOpen(false)
    window.requestAnimationFrame(() => {
      textarea.focus()
    })
  }

  function handleFileButtonClick() {
    fileInputRef.current?.click()
  }

  function handleImageButtonClick() {
    imageInputRef.current?.click()
  }

  function handleSmartVoiceInputAccept(transcript: string) {
    const textarea = textareaRef.current

    if (textarea) {
      insertTextareaText(textarea, transcript, handleTextareaValueChange)
    } else {
      handleTextareaValueChange(draft + transcript)
    }

    setSmartVoiceDialogOpen(false)
    window.requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })
  }

  function handleFileInputChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0] ?? null

    event.target.value = ""

    if (!file) {
      return
    }

    prepareSelectedFile(file)
  }

  function prepareSelectedFile(file: File) {
    const validationError = getFileMessageUploadError(file)

    if (validationError) {
      setSelectedFile(null)
      setFileDialogOpen(false)
      toast.error(validationError)
      return
    }

    setSelectedFile(file)
    setFileDialogOpen(true)
  }

  function handleImageInputChange(event: React.ChangeEvent<HTMLInputElement>) {
    const image = event.target.files?.[0] ?? null

    event.target.value = ""

    if (!image) {
      return
    }

    void prepareSelectedImage(image)
  }

  function handleComposerPaste(
    event: React.ClipboardEvent<HTMLTextAreaElement>
  ) {
    const image = getClipboardImageFile(event.clipboardData)

    if (!image) {
      return
    }

    event.preventDefault()
    void prepareSelectedImage(image)
  }

  async function prepareSelectedImage(image: File) {
    if (sending || imagePreparing) {
      return
    }

    setImagePreparing(true)
    setSelectedImage(null)
    setImageDialogOpen(false)

    try {
      const compressedImage = await compressImageForMessage(image)

      if (compressedImage.size > imageMessageMaxBytes) {
        toast.error("图片大于 2MB，无法上传")
        return
      }

      setSelectedImage(compressedImage)
      setImageDialogOpen(true)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取图片失败")
    } finally {
      setImagePreparing(false)
    }
  }

  function handleFileDialogOpenChange(open: boolean) {
    if (sending) {
      return
    }

    setFileDialogOpen(open)

    if (!open) {
      setSelectedFile(null)
    }
  }

  async function handleFileSendConfirm() {
    if (!selectedFile || sending) {
      return
    }

    const message = await onSendFile(selectedFile)

    if (message) {
      setFileDialogOpen(false)
      setSelectedFile(null)
    }
  }

  function handleImageDialogOpenChange(open: boolean) {
    if (sending) {
      return
    }

    setImageDialogOpen(open)

    if (!open) {
      setSelectedImage(null)
    }
  }

  async function handleImageSendConfirm() {
    if (!selectedImage || sending) {
      return
    }

    const message = await onSendImage(selectedImage)

    if (message) {
      setImageDialogOpen(false)
      setSelectedImage(null)
    }
  }

  return (
    <footer
      className="shrink-0 border-t p-4"
      data-testid="conversation-panel-composer"
    >
      <input
        ref={fileInputRef}
        className="hidden"
        onChange={handleFileInputChange}
        type="file"
      />
      <input
        ref={imageInputRef}
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={handleImageInputChange}
        type="file"
      />
      <div
        className="flex w-full flex-col gap-2"
        data-testid="conversation-panel-composer-content"
      >
        {replyTarget && (
          <div
            className="flex min-h-11 items-center justify-between gap-3 rounded-md border bg-muted/40 px-3 py-2"
            data-testid="conversation-reply-preview"
          >
            <div className="min-w-0">
              <div className="truncate text-xs font-medium">
                回复 {replyTarget.author}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                {replyTarget.summary}
              </div>
            </div>
            <Button
              aria-label="取消回复"
              disabled={sending}
              onClick={onCancelReply}
              size="icon-sm"
              title="取消回复"
              type="button"
              variant="ghost"
            >
              <X className="size-4" />
            </Button>
          </div>
        )}
        <div className="relative" data-testid="conversation-panel-editor-row">
          <Textarea
            ref={textareaRef}
            value={draft}
            aria-disabled={sending}
            onBlur={onDraftBlur}
            onChange={handleDraftChange}
            onKeyDown={handleComposerKeyDown}
            onSelect={(event) =>
              updateMentionTrigger(
                event.currentTarget.value,
                event.currentTarget.selectionStart
              )
            }
            onPaste={handleComposerPaste}
            placeholder={richTextMode ? "输入 Markdown 消息" : "输入消息"}
            readOnly={sending}
            className="max-h-48 min-h-24 resize-none"
          />
          {mentionTrigger && filteredMentionCandidates.length > 0 && (
            <div className="absolute bottom-full left-0 z-20 mb-2 max-h-72 w-72 overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md">
              {filteredMentionCandidates.map((candidate, index) => (
                <Button
                  key={`${candidate.targetType}-${candidate.id}`}
                  ref={(element) => {
                    mentionOptionRefs.current[index] = element
                  }}
                  className={cn(
                    "h-auto w-full justify-start gap-2 px-2 py-1.5 text-left",
                    index ===
                      getVisibleMentionIndex(
                        selectedMentionIndex,
                        filteredMentionCandidates.length
                      ) && "bg-accent"
                  )}
                  onMouseDown={(event) => {
                    event.preventDefault()
                    insertMentionCandidate(candidate)
                  }}
                  type="button"
                  variant="ghost"
                >
                  <Avatar
                    className={cn(
                      "size-6 rounded-sm after:rounded-sm",
                      candidate.targetType === "all"
                        ? "bg-teal-500"
                        : "bg-muted"
                    )}
                    data-size="sm"
                  >
                    {candidate.targetType === "all" ? (
                      <AvatarFallback className="rounded-sm bg-transparent text-background">
                        <UsersRound className="size-3.5" />
                      </AvatarFallback>
                    ) : candidate.avatar ? (
                      <AvatarImage
                        alt={candidate.label}
                        className="rounded-sm"
                        src={candidate.avatar}
                      />
                    ) : (
                      <AvatarFallback className="rounded-sm text-xs">
                        {getAvatarInitial(candidate.label)}
                      </AvatarFallback>
                    )}
                  </Avatar>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">
                      {candidate.label}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {candidate.description}
                    </span>
                  </span>
                </Button>
              ))}
            </div>
          )}
        </div>
        <div
          className="flex items-center justify-between gap-2"
          data-testid="conversation-panel-toolbar-row"
        >
          <div className="flex items-center gap-1">
            <ExpressionPickerPopover
              align="start"
              onSelect={handleExpressionSelect}
              open={expressionPickerOpen}
              onOpenChange={setExpressionPickerOpen}
            >
              <Button
                aria-label="选择表情"
                disabled={sending}
                size="icon-sm"
                title="选择表情"
                type="button"
                variant="ghost"
              >
                <Smile className="size-4" />
              </Button>
            </ExpressionPickerPopover>
            <Button
              aria-label="上传文件"
              disabled={sending}
              onClick={handleFileButtonClick}
              size="icon-sm"
              title="上传文件"
              type="button"
              variant="ghost"
            >
              <Paperclip className="size-4" />
            </Button>
            <Button
              aria-label="插入图片"
              disabled={sending || imagePreparing}
              onClick={handleImageButtonClick}
              size="icon-sm"
              title="插入图片"
              type="button"
              variant="ghost"
            >
              {imagePreparing ? (
                <LoaderCircle className="size-4 animate-spin" />
              ) : (
                <ImageIcon className="size-4" />
              )}
            </Button>
            <Toggle
              aria-label="支持 markdown"
              className="size-8 p-0"
              disabled={sending}
              onPressedChange={onRichTextModeChange}
              pressed={richTextMode}
              size="sm"
              title="支持 markdown"
              type="button"
            >
              <MarkdownIcon className="size-4" />
            </Toggle>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <ConversationVoiceMenu
              disabled={sending}
              onSendVoiceMessage={() => setSendVoiceDialogOpen(true)}
              onSmartVoiceInput={() => setSmartVoiceDialogOpen(true)}
            />
            <Button
              type="button"
              aria-label="发送消息"
              disabled={sending}
              onClick={handleSendMessage}
            >
              {sending ? (
                <LoaderCircle className="size-4 animate-spin" />
              ) : (
                <Send className="size-4" />
              )}
              <span aria-hidden="true">发送</span>
            </Button>
          </div>
        </div>
      </div>
      <SendFileMessageDialog
        conversationName={conversation.name}
        file={selectedFile}
        onConfirm={() => void handleFileSendConfirm()}
        onOpenChange={handleFileDialogOpenChange}
        open={fileDialogOpen}
        sending={sending}
      />
      <SendImageMessageDialog
        conversationName={conversation.name}
        image={selectedImage}
        onConfirm={() => void handleImageSendConfirm()}
        onOpenChange={handleImageDialogOpenChange}
        open={imageDialogOpen}
        sending={sending}
      />
      <SendVoiceMessageDialog
        conversationName={conversation.name}
        onConfirm={onSendVoice}
        onOpenChange={setSendVoiceDialogOpen}
        open={sendVoiceDialogOpen}
        sending={sending}
      />
      <SmartVoiceInputDialog
        onAccept={handleSmartVoiceInputAccept}
        onOpenChange={setSmartVoiceDialogOpen}
        open={smartVoiceDialogOpen}
      />
    </footer>
  )
})
