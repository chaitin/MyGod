package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	messageapp "app/internal/application/message"
	messagecontentapp "app/internal/application/messagecontent"

	"github.com/google/uuid"
)

const (
	forwardMessageModeMerged      = "merged"
	forwardMessageModeSeparate    = "separate"
	maxForwardBundleDepth         = 5
	maxForwardMessageCount        = 50
	maxForwardTargetCount         = 20
	maxForwardSummaryPreviewRunes = 100
	messageTypeForwardBundle      = "forward_bundle"
)

var (
	errForwardContentUnavailable = errors.New("forward content unavailable")
	errForwardMessageLimit       = errors.New("forward message limit exceeded")
	errForwardSourceUnavailable  = errors.New("forward source unavailable")
	errForwardUnsupportedMessage = errors.New("forward unsupported message")
)

type forwardMessagesRequest struct {
	ClientForwardID       string   `json:"client_forward_id"`
	MessageIDs            []string `json:"message_ids"`
	Mode                  string   `json:"mode"`
	TargetConversationIDs []string `json:"target_conversation_ids"`
}

type normalizedForwardMessagesRequest struct {
	ClientForwardID       string
	MessageIDs            []string
	Mode                  string
	TargetConversationIDs []string
}

type forwardMessagesResponse struct {
	FailedCount int                           `json:"failed_count"`
	Results     []forwardMessagesTargetResult `json:"results"`
	SentCount   int                           `json:"sent_count"`
}

type forwardMessagesTargetResult struct {
	ConversationID string                      `json:"conversation_id"`
	Error          *forwardMessagesTargetError `json:"error,omitempty"`
	Messages       []messageResponse           `json:"messages,omitempty"`
	Status         string                      `json:"status"`
}

type forwardMessagesTargetError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type forwardBundleMessageBody struct {
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

type forwardBodyMetrics struct {
	BundleDepth int
	LeafCount   int
}

type preparedForwardSource struct {
	Body       json.RawMessage
	MessageID  string
	Metrics    forwardBodyMetrics
	SenderName string
	SenderType string
	SentAt     time.Time
	Summary    string
}

type forwardMessageDraft struct {
	Body            json.RawMessage
	ClientMessageID string
	Summary         string
}

func normalizeForwardMessagesRequest(request forwardMessagesRequest) (normalizedForwardMessagesRequest, error) {
	clientForwardID := strings.TrimSpace(request.ClientForwardID)
	parsedForwardID, err := uuid.Parse(clientForwardID)
	if err != nil {
		return normalizedForwardMessagesRequest{}, errors.New("客户端转发 ID 格式错误")
	}

	messageIDs, err := normalizeForwardUUIDs(request.MessageIDs, "消息 ID", maxForwardMessageCount)
	if err != nil {
		return normalizedForwardMessagesRequest{}, err
	}
	targetConversationIDs, err := normalizeForwardUUIDs(request.TargetConversationIDs, "目标会话 ID", maxForwardTargetCount)
	if err != nil {
		return normalizedForwardMessagesRequest{}, err
	}

	mode := strings.TrimSpace(request.Mode)
	if mode != forwardMessageModeSeparate && mode != forwardMessageModeMerged {
		return normalizedForwardMessagesRequest{}, errors.New("转发模式必须是 separate 或 merged")
	}
	if mode == forwardMessageModeMerged && len(messageIDs) < 2 {
		return normalizedForwardMessagesRequest{}, errors.New("合并转发至少需要两条消息")
	}
	return normalizedForwardMessagesRequest{
		ClientForwardID:       parsedForwardID.String(),
		MessageIDs:            messageIDs,
		Mode:                  mode,
		TargetConversationIDs: targetConversationIDs,
	}, nil
}

func normalizeForwardUUIDs(rawIDs []string, fieldName string, limit int) ([]string, error) {
	if len(rawIDs) == 0 {
		return nil, errors.New(fieldName + "不能为空")
	}
	if len(rawIDs) > limit {
		return nil, fmt.Errorf("%s一次最多传 %d 个", fieldName, limit)
	}

	ids := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		parsedID, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return nil, errors.New(fieldName + "格式错误")
		}
		id := parsedID.String()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return ids, nil
}

func collectForwardMentionTargets(body json.RawMessage, targets map[string]messageMentionTarget) {
	collectForwardMentionTargetsAtDepth(body, targets, 0)
}

func collectForwardMentionTargetsAtDepth(body json.RawMessage, targets map[string]messageMentionTarget, bundleDepth int) {
	var envelope messageBodyEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return
	}
	if envelope.Type == messageTypeForwardBundle {
		if bundleDepth >= maxForwardBundleDepth {
			return
		}
		var bundle forwardBundleMessageBody
		if json.Unmarshal(body, &bundle) != nil {
			return
		}
		for _, item := range bundle.Items {
			collectForwardMentionTargetsAtDepth(item.Body, targets, bundleDepth+1)
		}
		return
	}
	for _, target := range parseMessageMentionTargets(body) {
		key := "all"
		if !target.All {
			key = conversationMemberMentionKey(target.MemberType, target.MemberID)
		}
		targets[key] = target
	}
}

func sanitizeForwardMessageBody(raw json.RawMessage, mentionLabels map[string]string, bundleDepth int) (json.RawMessage, string, forwardBodyMetrics, error) {
	body, summary, metrics, err := messagecontentapp.NewService(messagecontentapp.Dependencies{}).
		SanitizeForwardBody(raw, mentionLabels, bundleDepth)
	if errors.Is(err, messageapp.ErrForwardUnsupportedMessage) {
		err = errForwardUnsupportedMessage
	}
	return body, summary, forwardBodyMetrics{
		BundleDepth: metrics.BundleDepth,
		LeafCount:   metrics.LeafCount,
	}, err
}

func collectForwardTemporaryFileIDs(raw json.RawMessage, fileIDs map[string]struct{}) {
	var envelope messageBodyEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		return
	}
	switch envelope.Type {
	case messageTypeFile:
		var body fileMessageBody
		if json.Unmarshal(raw, &body) == nil && body.FileID != "" {
			fileIDs[body.FileID] = struct{}{}
		}
	case messageTypeImage:
		var body imageMessageBody
		if json.Unmarshal(raw, &body) == nil && body.FileID != "" {
			fileIDs[body.FileID] = struct{}{}
		}
	case messageTypeVoice:
		var body voiceMessageBody
		if json.Unmarshal(raw, &body) == nil && body.FileID != "" {
			fileIDs[body.FileID] = struct{}{}
		}
	case messageTypeForwardBundle:
		var body forwardBundleMessageBody
		if json.Unmarshal(raw, &body) == nil {
			for _, item := range body.Items {
				collectForwardTemporaryFileIDs(item.Body, fileIDs)
			}
		}
	}
}

func buildForwardMessageDrafts(request normalizedForwardMessagesRequest, sources []preparedForwardSource) ([]forwardMessageDraft, error) {
	if forwardSourcesLeafCount(sources) > maxForwardMessageCount {
		return nil, fmt.Errorf("本次转发最多包含 %d 条原始消息", maxForwardMessageCount)
	}
	if request.Mode == forwardMessageModeSeparate {
		drafts := make([]forwardMessageDraft, 0, len(sources))
		for _, source := range sources {
			drafts = append(drafts, forwardMessageDraft{
				Body:            source.Body,
				ClientMessageID: "forward:" + request.ClientForwardID + ":" + source.MessageID,
				Summary:         source.Summary,
			})
		}
		return drafts, nil
	}

	items := make([]forwardBundleItem, 0, len(sources))
	metrics := forwardBodyMetrics{BundleDepth: 1}
	for _, source := range sources {
		items = append(items, forwardBundleItem{
			Body:       source.Body,
			SenderName: source.SenderName,
			SenderType: source.SenderType,
			SentAt:     source.SentAt,
			Summary:    source.Summary,
		})
		metrics.BundleDepth = max(metrics.BundleDepth, source.Metrics.BundleDepth+1)
	}
	if metrics.BundleDepth > maxForwardBundleDepth {
		return nil, fmt.Errorf("聊天记录最多嵌套 %d 层", maxForwardBundleDepth)
	}
	body, err := json.Marshal(forwardBundleMessageBody{
		ItemCount: len(items),
		Items:     items,
		Type:      messageTypeForwardBundle,
	})
	if err != nil {
		return nil, err
	}
	return []forwardMessageDraft{{
		Body:            body,
		ClientMessageID: "forward:" + request.ClientForwardID,
		Summary:         forwardBundleSummary(items),
	}}, nil
}

func forwardSourcesLeafCount(sources []preparedForwardSource) int {
	count := 0
	for _, source := range sources {
		count += source.Metrics.LeafCount
	}
	return count
}

func forwardBundleSummary(items []forwardBundleItem) string {
	if len(items) == 0 {
		return "[聊天记录] 0 条"
	}
	preview := truncateForwardSummary(strings.TrimSpace(items[0].Summary), maxForwardSummaryPreviewRunes)
	if preview == "" {
		preview = "消息"
	}
	return fmt.Sprintf("[聊天记录] %d 条 - %s", len(items), preview)
}

func truncateForwardSummary(content string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(content) <= limit {
		return content
	}
	runes := []rune(content)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
