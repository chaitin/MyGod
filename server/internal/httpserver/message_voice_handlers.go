package httpserver

import (
	"fmt"
	"strings"
)

const (
	maxVoiceMessageDurationMS  = 60_000
	maxVoiceMessageUploadBytes = 1 * 1024 * 1024
	messageTypeVoice           = "voice"
	voiceMessageContentType    = "audio/webm"
	voiceMessageDemoTranscript = "这是一段语音消息的演示转写文字"
)

var webMHeader = []byte{0x1a, 0x45, 0xdf, 0xa3}

type voiceMessageBody struct {
	Type        string `json:"type"`
	FileID      string `json:"file_id"`
	DurationMS  int    `json:"duration_ms"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	Transcript  string `json:"transcript"`
}

func voiceMessageSummary(durationMS int, transcript string) string {
	totalSeconds := (durationMS + 999) / 1000
	summary := fmt.Sprintf("[语音] %02d:%02d", totalSeconds/60, totalSeconds%60)
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return summary
	}
	return summary + " - " + transcript
}
