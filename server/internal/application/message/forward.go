package message

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	fileapp "app/internal/application/file"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxForwardBundleDepth         = 5
	maxForwardSummaryPreviewRunes = 100
	messageTypeForwardBundle      = "forward_bundle"
)

var (
	errForwardContentUnavailable = errors.New("forward content unavailable")
	errForwardMessageLimit       = errors.New("forward message limit exceeded")
	errForwardSourceUnavailable  = errors.New("forward source unavailable")
	forwardMentionTokenPattern   = regexp.MustCompile(`\{\(@(?:(user)/(all)|(user|app)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}))\)\}`)
)

type normalizedForwardCommand struct {
	ClientForwardID       string
	MessageIDs            []string
	Mode                  string
	SourceConversationID  string
	TargetConversationIDs []string
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

type preparedForwardSource struct {
	Body       json.RawMessage
	MessageID  string
	Metrics    ForwardBodyMetrics
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

type forwardMentionTarget struct {
	All        bool
	MemberID   string
	MemberType string
}

type forwardBodyEnvelope struct {
	Content string `json:"content"`
	Type    string `json:"type"`
}

type forwardTemporaryFileBody struct {
	FileID string `json:"file_id"`
}

func (s *Service) Forward(ctx context.Context, cmd ForwardCommand) (ForwardResult, error) {
	normalized, err := normalizeForwardCommand(cmd)
	if err != nil {
		return ForwardResult{}, InvalidRequestError(err.Error(), err)
	}
	sourceMessages, err := s.loadForwardSourceMessages(ctx, cmd.AccountID, normalized.SourceConversationID, normalized.MessageIDs)
	if err != nil {
		return ForwardResult{}, mapForwardSourceError(err)
	}
	preparedSources, err := s.prepareForwardSources(ctx, sourceMessages)
	if err != nil {
		return ForwardResult{}, mapForwardPreparationError(err)
	}
	drafts, err := buildForwardMessageDrafts(normalized, preparedSources)
	if err != nil {
		return ForwardResult{}, InvalidRequestError(err.Error(), err)
	}

	var sender store.User
	if err := s.db.WithContext(ctx).First(&sender, "id = ?", cmd.AccountID).Error; err != nil {
		return ForwardResult{}, internalError(err)
	}
	result := ForwardResult{Results: make([]ForwardTargetResult, 0, len(normalized.TargetConversationIDs))}
	for _, targetConversationID := range normalized.TargetConversationIDs {
		messages, err := s.createForwardedMessagesForTarget(ctx, sender, targetConversationID, drafts)
		if err != nil {
			result.FailedCount++
			result.Results = append(result.Results, newForwardTargetFailure(targetConversationID, err))
			continue
		}
		converted := make([]Message, 0, len(messages))
		for _, value := range messages {
			message, viewErr := newMessageForUser(s.db.WithContext(ctx), value, cmd.AccountID)
			if viewErr != nil {
				message = newMessage(value)
			}
			converted = append(converted, message)
		}
		result.SentCount++
		result.Results = append(result.Results, ForwardTargetResult{
			ConversationID: targetConversationID, Messages: converted, Status: "sent",
		})
	}
	return result, nil
}

func normalizeForwardCommand(cmd ForwardCommand) (normalizedForwardCommand, error) {
	sourceConversationID, err := normalizeRequiredUUID(cmd.SourceConversationID, "会话 ID")
	if err != nil {
		return normalizedForwardCommand{}, err
	}
	parsedForwardID, err := uuid.Parse(strings.TrimSpace(cmd.ClientForwardID))
	if err != nil {
		return normalizedForwardCommand{}, errors.New("客户端转发 ID 格式错误")
	}
	messageIDs, err := normalizeForwardUUIDs(cmd.MessageIDs, "消息 ID", MaxForwardCount)
	if err != nil {
		return normalizedForwardCommand{}, err
	}
	targetConversationIDs, err := normalizeForwardUUIDs(cmd.TargetConversationIDs, "目标会话 ID", MaxForwardTargets)
	if err != nil {
		return normalizedForwardCommand{}, err
	}
	mode := strings.TrimSpace(cmd.Mode)
	if mode != ForwardModeSeparate && mode != ForwardModeMerged {
		return normalizedForwardCommand{}, errors.New("转发模式必须是 separate 或 merged")
	}
	if mode == ForwardModeMerged && len(messageIDs) < 2 {
		return normalizedForwardCommand{}, errors.New("合并转发至少需要两条消息")
	}
	return normalizedForwardCommand{
		ClientForwardID: parsedForwardID.String(), MessageIDs: messageIDs, Mode: mode,
		SourceConversationID: sourceConversationID, TargetConversationIDs: targetConversationIDs,
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

func (s *Service) loadForwardSourceMessages(ctx context.Context, userID, conversationID string, messageIDs []string) ([]store.Message, error) {
	db := s.db.WithContext(ctx)
	member, err := requireReadableConversationMember(db, userID, conversationID)
	if err != nil {
		return nil, err
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	var messages []store.Message
	if store.MessagePartitioningEnabled(db) {
		var registries []store.MessageRegistry
		if err := applyOnlineStoredMessageWindow(db).Where(
			"conversation_id = ? AND id IN ? AND deleted_at IS NULL AND seq >= ?", conversationID, messageIDs, visibleFromSeq,
		).Find(&registries).Error; err != nil {
			return nil, err
		}
		messagesByID, err := store.LoadMessagesByRegistry(ctx, db, registries)
		if err != nil {
			return nil, err
		}
		messages = make([]store.Message, 0, len(registries))
		for _, registry := range registries {
			message, ok := messagesByID[registry.ID]
			if !ok {
				return nil, errForwardSourceUnavailable
			}
			messages = append(messages, message)
		}
	} else if err := applyOnlineStoredMessageWindow(db).Where(
		"conversation_id = ? AND id IN ? AND deleted_at IS NULL AND seq >= ?", conversationID, messageIDs, visibleFromSeq,
	).Find(&messages).Error; err != nil {
		return nil, err
	}
	if len(messages) != len(messageIDs) {
		return nil, errForwardSourceUnavailable
	}
	for _, message := range messages {
		if message.RevokedAt != nil {
			return nil, errForwardSourceUnavailable
		}
	}
	sort.Slice(messages, func(left, right int) bool { return messages[left].Seq < messages[right].Seq })
	return messages, nil
}

func (s *Service) prepareForwardSources(ctx context.Context, messages []store.Message) ([]preparedForwardSource, error) {
	if s.forwardBodies == nil {
		return nil, errors.New("forward body sanitizer is required")
	}
	mentionLabels, err := s.loadForwardMentionLabels(ctx, messages)
	if err != nil {
		return nil, err
	}
	sources := make([]preparedForwardSource, 0, len(messages))
	for _, message := range messages {
		body, summary, metrics, err := s.forwardBodies.SanitizeForwardBody(message.Body, mentionLabels, 0)
		if err != nil {
			return nil, err
		}
		if message.SenderID == nil || message.SenderType == store.MessageSenderTypeSystem {
			return nil, ErrForwardUnsupportedMessage
		}
		senderName, err := s.forwardSenderDisplayName(ctx, message.SenderType, *message.SenderID)
		if err != nil {
			return nil, err
		}
		sources = append(sources, preparedForwardSource{
			Body: body, MessageID: message.ID, Metrics: metrics, SenderName: senderName,
			SenderType: message.SenderType, SentAt: message.CreatedAt, Summary: summary,
		})
	}
	if forwardSourcesLeafCount(sources) > MaxForwardCount {
		return nil, errForwardMessageLimit
	}
	if err := s.validateForwardTemporaryFiles(ctx, sources); err != nil {
		return nil, err
	}
	return sources, nil
}

func (s *Service) forwardSenderDisplayName(ctx context.Context, senderType, senderID string) (string, error) {
	db := s.db.WithContext(ctx)
	switch senderType {
	case store.MessageSenderTypeUser:
		var user store.User
		if err := db.Select("id", "name", "nickname").First(&user, "id = ?", senderID).Error; err != nil {
			return "", err
		}
		if name := strings.TrimSpace(user.Nickname); name != "" {
			return name, nil
		}
		return user.Name, nil
	case store.MessageSenderTypeApp:
		return messageSenderName(db, senderType, senderID)
	default:
		return "", ErrForwardUnsupportedMessage
	}
}

func (s *Service) loadForwardMentionLabels(ctx context.Context, messages []store.Message) (map[string]string, error) {
	targetsByKey := make(map[string]forwardMentionTarget)
	for _, message := range messages {
		collectForwardMentionTargets(message.Body, targetsByKey, 0)
	}
	labels := make(map[string]string, len(targetsByKey))
	for key, target := range targetsByKey {
		if target.All {
			labels[key] = "所有人"
			continue
		}
		name, err := s.forwardSenderDisplayName(ctx, target.MemberType, target.MemberID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if target.MemberType == store.ConversationMemberTypeApp {
				name = "应用"
			} else {
				name = "用户"
			}
		} else if err != nil {
			return nil, err
		}
		labels[key] = name
	}
	return labels, nil
}

func collectForwardMentionTargets(body json.RawMessage, targets map[string]forwardMentionTarget, bundleDepth int) {
	var envelope forwardBodyEnvelope
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
			collectForwardMentionTargets(item.Body, targets, bundleDepth+1)
		}
		return
	}
	for _, match := range forwardMentionTokenPattern.FindAllStringSubmatch(envelope.Content, -1) {
		if len(match) != 5 {
			continue
		}
		target := forwardMentionTarget{}
		key := "all"
		if match[2] == "all" {
			target.All = true
		} else {
			target.MemberType = match[3]
			target.MemberID = strings.ToLower(match[4])
			key = target.MemberType + "/" + target.MemberID
		}
		targets[key] = target
	}
}

func (s *Service) validateForwardTemporaryFiles(ctx context.Context, sources []preparedForwardSource) error {
	fileIDSet := make(map[string]struct{})
	for _, source := range sources {
		collectForwardTemporaryFileIDs(source.Body, fileIDSet)
	}
	if len(fileIDSet) == 0 {
		return nil
	}
	fileIDs := make([]string, 0, len(fileIDSet))
	for fileID := range fileIDSet {
		fileIDs = append(fileIDs, fileID)
	}
	if s.files == nil {
		return errors.New("temporary file validator is required")
	}
	if err := s.files.ValidateTemporaryFiles(ctx, fileIDs); err != nil {
		if code := fileapp.ErrorCodeOf(err); code == fileapp.CodeNotFound || code == fileapp.CodeInvalidRequest {
			return errForwardContentUnavailable
		}
		return err
	}
	return nil
}

func collectForwardTemporaryFileIDs(raw json.RawMessage, fileIDs map[string]struct{}) {
	var envelope forwardBodyEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		return
	}
	switch envelope.Type {
	case "file", "image", "voice":
		var body forwardTemporaryFileBody
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

func buildForwardMessageDrafts(cmd normalizedForwardCommand, sources []preparedForwardSource) ([]forwardMessageDraft, error) {
	if forwardSourcesLeafCount(sources) > MaxForwardCount {
		return nil, fmt.Errorf("本次转发最多包含 %d 条原始消息", MaxForwardCount)
	}
	if cmd.Mode == ForwardModeSeparate {
		drafts := make([]forwardMessageDraft, 0, len(sources))
		for _, source := range sources {
			drafts = append(drafts, forwardMessageDraft{
				Body: source.Body, ClientMessageID: "forward:" + cmd.ClientForwardID + ":" + source.MessageID, Summary: source.Summary,
			})
		}
		return drafts, nil
	}
	items := make([]forwardBundleItem, 0, len(sources))
	metrics := ForwardBodyMetrics{BundleDepth: 1}
	for _, source := range sources {
		items = append(items, forwardBundleItem{
			Body: source.Body, SenderName: source.SenderName, SenderType: source.SenderType,
			SentAt: source.SentAt, Summary: source.Summary,
		})
		metrics.BundleDepth = max(metrics.BundleDepth, source.Metrics.BundleDepth+1)
	}
	if metrics.BundleDepth > maxForwardBundleDepth {
		return nil, fmt.Errorf("聊天记录最多嵌套 %d 层", maxForwardBundleDepth)
	}
	body, err := json.Marshal(forwardBundleMessageBody{ItemCount: len(items), Items: items, Type: messageTypeForwardBundle})
	if err != nil {
		return nil, err
	}
	return []forwardMessageDraft{{
		Body: body, ClientMessageID: "forward:" + cmd.ClientForwardID, Summary: forwardBundleSummary(items),
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
	return strings.TrimSpace(string([]rune(content)[:limit])) + "…"
}

func (s *Service) createForwardedMessagesForTarget(ctx context.Context, user store.User, conversationID string, drafts []forwardMessageDraft) ([]store.Message, error) {
	var messages []store.Message
	var createdMessages []store.Message
	var memberUserIDs []string
	var appEvents []AppEvent
	appEventLockHeld := false

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive || conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
			return errConversationNotSendable
		}
		var member store.ConversationMember
		if err := tx.First(
			&member, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID, store.ConversationMemberTypeUser, user.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}

		clientMessageIDs := make([]string, 0, len(drafts))
		for _, draft := range drafts {
			clientMessageIDs = append(clientMessageIDs, draft.ClientMessageID)
		}
		var existingMessages []store.Message
		if store.MessagePartitioningEnabled(tx) {
			var registries []store.MessageRegistry
			if err := tx.Where(
				"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id IN ?",
				conversationID, store.MessageSenderTypeUser, user.ID, clientMessageIDs,
			).Find(&registries).Error; err != nil {
				return err
			}
			messagesByID, err := store.LoadMessagesByRegistry(ctx, tx, registries)
			if err != nil {
				return err
			}
			for _, registry := range registries {
				if message, ok := messagesByID[registry.ID]; ok {
					existingMessages = append(existingMessages, message)
				}
			}
		} else if err := tx.Where(
			"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id IN ?",
			conversationID, store.MessageSenderTypeUser, user.ID, clientMessageIDs,
		).Find(&existingMessages).Error; err != nil {
			return err
		}
		existingByClientID := make(map[string]store.Message, len(existingMessages))
		for _, message := range existingMessages {
			if message.ClientMessageID != nil {
				existingByClientID[*message.ClientMessageID] = message
			}
		}

		now := time.Now().UTC()
		messages = make([]store.Message, 0, len(drafts))
		for _, draft := range drafts {
			if existing, ok := existingByClientID[draft.ClientMessageID]; ok {
				messages = append(messages, existing)
				continue
			}
			clientMessageID := draft.ClientMessageID
			message := store.Message{
				ID: uuid.NewString(), ConversationID: conversationID, Seq: conversation.LastMessageSeq + 1,
				SenderType: store.MessageSenderTypeUser, SenderID: &user.ID, ClientMessageID: &clientMessageID,
				Body: draft.Body, Summary: draft.Summary, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
			conversation.LastMessageSeq = message.Seq
			messages = append(messages, message)
			createdMessages = append(createdMessages, message)
		}

		if len(createdMessages) > 0 {
			lastMessage := createdMessages[len(createdMessages)-1]
			if err := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{
				"last_message_at": lastMessage.CreatedAt, "last_message_id": lastMessage.ID,
				"last_message_seq": lastMessage.Seq, "last_message_summary": lastMessage.Summary, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			ids, err := loadActiveConversationUserIDs(tx, conversationID)
			if err != nil {
				return err
			}
			memberUserIDs = ids
			if conversation.Kind == store.ConversationKindApp {
				if s.appEventLocker != nil {
					s.appEventLocker.Lock()
					appEventLockHeld = true
				}
				for _, message := range createdMessages {
					events, err := createAppMessageEventOutbox(tx, conversation, user, message)
					if err != nil {
						return err
					}
					appEvents = append(appEvents, events...)
				}
			}
		}
		var maxSeq int64
		for _, message := range messages {
			if message.Seq > maxSeq {
				maxSeq = message.Seq
			}
		}
		if maxSeq > 0 {
			return advanceConversationMemberReadSeq(tx, conversationID, user.ID, maxSeq)
		}
		return nil
	})
	if appEventLockHeld {
		defer s.appEventLocker.Unlock()
	}
	if err != nil {
		return nil, err
	}

	for _, storedMessage := range createdMessages {
		message := newMessage(storedMessage)
		if s.notifications != nil {
			deliveries := make([]Delivery, 0, len(memberUserIDs))
			for _, userID := range memberUserIDs {
				converted, viewErr := newMessageForUser(s.db.WithContext(ctx), storedMessage, userID)
				if viewErr != nil {
					converted = message
				}
				deliveries = append(deliveries, Delivery{Message: converted, UserID: userID})
			}
			s.notifications.PublishMessageCreated(ctx, deliveries)
		}
		if s.afterUserMessageCommit != nil {
			s.afterUserMessageCommit(message)
		}
	}
	if s.appEvents != nil {
		s.appEvents.DeliverAppEvents(ctx, appEvents)
	}
	return messages, nil
}

func newForwardTargetFailure(conversationID string, err error) ForwardTargetResult {
	code := "internal_error"
	message := "转发失败"
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, errConversationAccessDenied):
		code = "forbidden"
		message = "无权向该会话发送消息"
	case errors.Is(err, errConversationNotSendable):
		code = "conversation_not_sendable"
		message = "当前会话不能发送消息"
	}
	return ForwardTargetResult{
		ConversationID: conversationID, Error: &ForwardTargetError{Code: code, Message: message}, Status: "failed",
	}
}

func mapForwardSourceError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return &Error{Code: CodeNotFound, Message: "源会话不存在", Cause: err}
	case errors.Is(err, errConversationAccessDenied):
		return &Error{Code: CodeForbidden, Message: "无权访问源会话", Cause: err}
	case errors.Is(err, errForwardSourceUnavailable):
		return &Error{Code: CodeSourceUnavailable, Message: "部分源消息不存在、不可见或已被撤回", Cause: err}
	default:
		return internalError(err)
	}
}

func mapForwardPreparationError(err error) error {
	switch {
	case errors.Is(err, errForwardMessageLimit):
		return InvalidRequestError(fmt.Sprintf("本次转发最多包含 %d 条原始消息", MaxForwardCount), err)
	case errors.Is(err, ErrForwardUnsupportedMessage):
		return &Error{Code: CodeUnsupportedMessage, Message: "所选消息中包含不能转发的消息", Cause: err}
	case errors.Is(err, errForwardContentUnavailable):
		return &Error{Code: CodeContentUnavailable, Message: "消息附件不存在或已过期", Cause: err}
	default:
		return internalError(err)
	}
}
