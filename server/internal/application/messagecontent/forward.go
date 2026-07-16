package messagecontent

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	messageapp "app/internal/application/message"
	"app/internal/messageformat"
	"app/internal/store"
)

const (
	maxForwardBundleDepth         = 5
	maxForwardMessageCount        = 50
	maxForwardSummaryPreviewRunes = 100
)

var mentionTokenPattern = regexp.MustCompile(`\{\(@(?:(user)/(all)|(user|app)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}`)

type forwardBundleBody struct {
	ItemCount int                 `json:"item_count"`
	Items     []forwardBundleItem `json:"items"`
	Type      string              `json:"type"`
}

type forwardBundleItem struct {
	Body       json.RawMessage `json:"body"`
	SenderName string          `json:"sender_name"`
	SenderType string          `json:"sender_type"`
	SentAt     time.Time       `json:"sent_at"`
	Summary    string          `json:"summary"`
}

func (s *Service) SanitizeForwardBody(
	raw json.RawMessage,
	mentionLabels map[string]string,
	bundleDepth int,
) (json.RawMessage, string, messageapp.ForwardBodyMetrics, error) {
	body, summary, metrics, err := s.sanitizeForwardBody(raw, mentionLabels, bundleDepth)
	if errors.Is(err, messageapp.ErrForwardUnsupportedMessage) {
		return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
	}
	return body, summary, metrics, err
}

func (s *Service) sanitizeForwardBody(
	raw json.RawMessage,
	mentionLabels map[string]string,
	bundleDepth int,
) (json.RawMessage, string, messageapp.ForwardBodyMetrics, error) {
	var value envelope
	if json.Unmarshal(raw, &value) != nil {
		return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
	}
	leaf := messageapp.ForwardBodyMetrics{LeafCount: 1}
	switch strings.TrimSpace(value.Type) {
	case TypeText:
		var body textBody
		if json.Unmarshal(raw, &body) != nil {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		body.Content = replaceMentions(body.Content, mentionLabels, false)
		encoded, err := json.Marshal(body)
		return encoded, strings.TrimSpace(body.Content), leaf, err
	case TypeMarkdown:
		var body markdownBody
		if json.Unmarshal(raw, &body) != nil {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		body.Content = replaceMentions(body.Content, mentionLabels, true)
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, "", messageapp.ForwardBodyMetrics{}, err
		}
		summary, err := messageformat.MarkdownPlainText(strings.TrimSpace(body.Content))
		return encoded, summary, leaf, err
	case TypeLink:
		summary, err := (linkHandler{}).Summary(raw)
		return cloneRaw(raw), summary, leaf, err
	case TypeCard:
		handler := cardHandler{}
		if handler.Validate(raw) != nil {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		summary, err := handler.Summary(raw)
		return cloneRaw(raw), summary, leaf, err
	case TypeChart:
		handler := chartHandler{}
		if handler.Validate(raw) != nil {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		summary, err := handler.Summary(raw)
		return cloneRaw(raw), summary, leaf, err
	case TypeFile:
		var body fileBody
		if json.Unmarshal(raw, &body) != nil || strings.TrimSpace(body.FileID) == "" || strings.TrimSpace(body.Name) == "" {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		return cloneRaw(raw), "[文件] " + strings.TrimSpace(body.Name), leaf, nil
	case TypeImage:
		var body imageBody
		if json.Unmarshal(raw, &body) != nil || strings.TrimSpace(body.FileID) == "" {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		return cloneRaw(raw), "[图片]", leaf, nil
	case TypeVoice:
		var body voiceBody
		if json.Unmarshal(raw, &body) != nil || strings.TrimSpace(body.FileID) == "" || body.DurationMS <= 0 {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		return cloneRaw(raw), voiceSummary(body.DurationMS, body.Transcript), leaf, nil
	case TypeForwardBundle:
		return s.sanitizeForwardBundle(raw, mentionLabels, bundleDepth)
	default:
		return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
	}
}

func (s *Service) sanitizeForwardBundle(
	raw json.RawMessage,
	mentionLabels map[string]string,
	enclosingDepth int,
) (json.RawMessage, string, messageapp.ForwardBodyMetrics, error) {
	if enclosingDepth >= maxForwardBundleDepth {
		return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
	}
	var bundle forwardBundleBody
	if json.Unmarshal(raw, &bundle) != nil || bundle.Type != TypeForwardBundle || len(bundle.Items) == 0 || len(bundle.Items) > maxForwardMessageCount {
		return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
	}
	items := make([]forwardBundleItem, 0, len(bundle.Items))
	metrics := messageapp.ForwardBodyMetrics{BundleDepth: 1}
	for _, item := range bundle.Items {
		if strings.TrimSpace(item.SenderName) == "" ||
			(item.SenderType != store.MessageSenderTypeUser && item.SenderType != store.MessageSenderTypeApp) || item.SentAt.IsZero() {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
		body, summary, child, err := s.sanitizeForwardBody(item.Body, mentionLabels, enclosingDepth+1)
		if err != nil {
			return nil, "", messageapp.ForwardBodyMetrics{}, err
		}
		item.Body = body
		item.Summary = summary
		items = append(items, item)
		metrics.LeafCount += child.LeafCount
		metrics.BundleDepth = max(metrics.BundleDepth, child.BundleDepth+1)
		if metrics.LeafCount > maxForwardMessageCount {
			return nil, "", messageapp.ForwardBodyMetrics{}, messageapp.ErrForwardUnsupportedMessage
		}
	}
	bundle.ItemCount = len(items)
	bundle.Items = items
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return nil, "", messageapp.ForwardBodyMetrics{}, err
	}
	return encoded, forwardSummary(items), metrics, nil
}

func replaceMentions(content string, labels map[string]string, markdown bool) string {
	return mentionTokenPattern.ReplaceAllStringFunc(content, func(token string) string {
		match := mentionTokenPattern.FindStringSubmatch(token)
		if len(match) != 5 {
			return token
		}
		key := "all"
		if match[2] != "all" {
			key = match[3] + "/" + strings.ToLower(match[4])
		}
		label := labels[key]
		if label == "" {
			switch {
			case match[2] == "all":
				label = "所有人"
			case match[3] == store.ConversationMemberTypeApp:
				label = "应用"
			default:
				label = "用户"
			}
		}
		if markdown {
			label = escapeMarkdown(label)
		}
		return "@" + label
	})
}

func escapeMarkdown(content string) string {
	return strings.NewReplacer(
		"\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]",
		"<", "\\<", ">", "\\>", "`", "\\`",
	).Replace(content)
}

func cloneRaw(raw json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), raw...) }

func voiceSummary(durationMS int, transcript string) string {
	totalSeconds := (durationMS + 999) / 1000
	summary := fmt.Sprintf("[语音] %02d:%02d", totalSeconds/60, totalSeconds%60)
	if transcript = strings.TrimSpace(transcript); transcript != "" {
		return summary + " - " + transcript
	}
	return summary
}

func forwardSummary(items []forwardBundleItem) string {
	if len(items) == 0 {
		return "[聊天记录] 0 条"
	}
	preview := truncateSummary(strings.TrimSpace(items[0].Summary), maxForwardSummaryPreviewRunes)
	if preview == "" {
		preview = "消息"
	}
	return fmt.Sprintf("[聊天记录] %d 条 - %s", len(items), preview)
}

func truncateSummary(content string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(content) <= limit {
		return content
	}
	return strings.TrimSpace(string([]rune(content)[:limit])) + "…"
}

var _ messageapp.ForwardBodySanitizer = (*Service)(nil)
