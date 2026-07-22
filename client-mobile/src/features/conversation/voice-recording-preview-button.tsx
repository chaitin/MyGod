import {
  setAudioModeAsync,
  useAudioPlayer,
  useAudioPlayerStatus,
} from "expo-audio"
import { AudioLines, Pause } from "lucide-react-native"
import { useCallback, useEffect, useRef } from "react"
import { Button, Spinner, useToastController } from "tamagui"

import type { AppToastTone } from "@/components/feedback/app-toast"
import { ThemedIcon } from "@/components/icons/themed-icon"
import {
  activateVoicePlayer,
  deactivateVoicePlayer,
} from "@/features/conversation/voice-message-player-state"

export function VoiceRecordingPreviewButton({
  disabled,
  uri,
}: {
  disabled: boolean
  uri: string
}) {
  const toast = useToastController()
  const player = useAudioPlayer(uri, { updateInterval: 100 })
  const status = useAudioPlayerStatus(player)
  const shownErrorRef = useRef("")
  const playerId = player.id

  const pause = useCallback(() => {
    player.pause()
    deactivateVoicePlayer(playerId)
  }, [player, playerId])

  useEffect(() => {
    if (disabled) pause()
  }, [disabled, pause])

  useEffect(() => {
    if (status.didJustFinish) deactivateVoicePlayer(playerId)
  }, [playerId, status.didJustFinish])

  useEffect(() => {
    const playbackError = status.error?.trim() ?? ""
    if (!playbackError) {
      shownErrorRef.current = ""
      return
    }
    if (shownErrorRef.current === playbackError) return

    shownErrorRef.current = playbackError
    toast.show("无法播放语音", {
      customData: { tone: "error" satisfies AppToastTone },
      duration: 4000,
      message: playbackError,
    })
  }, [status.error, toast])

  useEffect(
    () => () => {
      deactivateVoicePlayer(playerId)
    },
    [playerId]
  )

  async function startPlayback() {
    try {
      await setAudioModeAsync({
        allowsRecording: false,
        interruptionMode: "doNotMix",
        playsInSilentMode: true,
        shouldPlayInBackground: false,
        shouldRouteThroughEarpiece: false,
      })
      if (
        status.didJustFinish ||
        (player.duration > 0 && player.currentTime >= player.duration - 0.05)
      ) {
        await player.seekTo(0)
      }
      activateVoicePlayer({ id: playerId, pause: () => player.pause() })
      player.play()
    } catch (error: unknown) {
      pause()
      toast.show("无法播放语音", {
        customData: { tone: "error" satisfies AppToastTone },
        duration: 4000,
        message: error instanceof Error ? error.message : "请稍后重试",
      })
    }
  }

  function handlePress() {
    if (status.playing) {
      pause()
      return
    }
    void startPlayback()
  }

  return (
    <Button
      accessibilityLabel={status.playing ? "暂停录音预览" : "播放录音预览"}
      chromeless
      circular
      disabled={disabled || status.isBuffering}
      icon={
        status.isBuffering ? (
          <Spinner size="small" />
        ) : (
          <ThemedIcon icon={status.playing ? Pause : AudioLines} size={30} />
        )
      }
      onPress={handlePress}
      size="$5"
    />
  )
}
