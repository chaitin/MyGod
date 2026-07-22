import {
  setAudioModeAsync,
  useAudioPlayer,
  useAudioPlayerStatus,
} from "expo-audio"
import { AudioLines, Pause, Play } from "lucide-react-native"
import { useCallback, useEffect, useRef } from "react"
import {
  Button,
  Paragraph,
  SizableText,
  Spinner,
  useToastController,
  XStack,
  YStack,
} from "tamagui"

import type { AppToastTone } from "@/components/feedback/app-toast"
import { ThemedIcon } from "@/components/icons/themed-icon"
import type { ResourceLoadState } from "@/data/resources"
import { formatVoiceDuration } from "@/domain/messages/message-presenter"
import {
  activateVoicePlayer,
  deactivateVoicePlayer,
} from "@/features/conversation/voice-message-player-state"

export function VoiceMessagePlayer({
  durationMS,
  fileId,
  onResourceError,
  onResourceRequest,
  state,
  transcript,
}: {
  durationMS: number
  fileId: string
  onResourceError: (fileId: string) => void
  onResourceRequest: (fileId: string) => void
  state: ResourceLoadState | undefined
  transcript: string
}) {
  const toast = useToastController()
  const resourceUri = state?.resource?.uri ?? ""
  const player = useAudioPlayer(resourceUri || null, { updateInterval: 100 })
  const playerStatus = useAudioPlayerStatus(player)
  const playerId = player.id
  const playWhenReadyRef = useRef(false)
  const retriedResourceRef = useRef(false)
  const shownPlaybackErrorRef = useRef("")
  const isLoading =
    state?.status === "loading" ||
    (resourceUri.length > 0 && playerStatus.isBuffering)

  const startPlayback = useCallback(async () => {
    try {
      await setAudioModeAsync({
        allowsRecording: false,
        interruptionMode: "doNotMix",
        playsInSilentMode: true,
        shouldPlayInBackground: false,
        shouldRouteThroughEarpiece: false,
      })
      if (
        playerStatus.didJustFinish ||
        (player.duration > 0 && player.currentTime >= player.duration - 0.05)
      ) {
        await player.seekTo(0)
      }
      activateVoicePlayer({ id: playerId, pause: () => player.pause() })
      player.play()
    } catch (error: unknown) {
      deactivateVoicePlayer(player.id)
      toast.show("无法播放语音", {
        customData: { tone: "error" satisfies AppToastTone },
        duration: 4000,
        message: error instanceof Error ? error.message : "请稍后重试",
      })
    }
  }, [player, playerId, playerStatus.didJustFinish, toast])

  useEffect(() => {
    if (!resourceUri || !playWhenReadyRef.current) return
    playWhenReadyRef.current = false
    void startPlayback()
  }, [resourceUri, startPlayback])

  useEffect(() => {
    if (playerStatus.didJustFinish) deactivateVoicePlayer(playerId)
  }, [playerId, playerStatus.didJustFinish])

  useEffect(() => {
    const playbackError = playerStatus.error?.trim() ?? ""
    if (!playbackError) {
      shownPlaybackErrorRef.current = ""
      return
    }
    if (!retriedResourceRef.current) {
      retriedResourceRef.current = true
      playWhenReadyRef.current = true
      onResourceError(fileId)
      return
    }
    if (shownPlaybackErrorRef.current === playbackError) return

    shownPlaybackErrorRef.current = playbackError
    toast.show("无法播放语音", {
      customData: { tone: "error" satisfies AppToastTone },
      duration: 4000,
      message: playbackError,
    })
  }, [fileId, onResourceError, playerStatus.error, toast])

  useEffect(
    () => () => {
      deactivateVoicePlayer(playerId)
    },
    [playerId]
  )

  function handlePress() {
    if (playerStatus.playing) {
      player.pause()
      deactivateVoicePlayer(playerId)
      return
    }

    if (!resourceUri) {
      playWhenReadyRef.current = true
      onResourceRequest(fileId)
      return
    }

    void startPlayback()
  }

  return (
    <YStack gap="$2" width="100%">
      <XStack gap="$3" items="center">
        <ThemedIcon icon={AudioLines} size={22} />
        <SizableText flex={1}>
          语音 {formatVoiceDuration(durationMS)}
        </SizableText>
        <Button
          accessibilityLabel={playerStatus.playing ? "暂停语音" : "播放语音"}
          chromeless
          circular
          disabled={isLoading}
          icon={
            isLoading ? (
              <Spinner size="small" />
            ) : (
              <ThemedIcon
                icon={playerStatus.playing ? Pause : Play}
                size={18}
              />
            )
          }
          onPress={handlePress}
          size="$3"
        />
      </XStack>
      {transcript ? (
        <Paragraph color="$color10" size="$2">
          {transcript}
        </Paragraph>
      ) : null}
    </YStack>
  )
}
