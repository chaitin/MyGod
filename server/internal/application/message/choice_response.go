package message

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"app/internal/application/conversationaccess"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxChoiceSnapshotIDs = 100

const (
	choiceSnapshotStatusActive  = "active"
	choiceSnapshotStatusDeleted = "deleted"
	choiceSnapshotStatusRevoked = "revoked"
)

var (
	errChoiceMessageUnavailable = errors.New("choice message unavailable")
	errChoiceMessageType        = errors.New("message is not a choice")
	errChoiceOptionInvalid      = errors.New("choice option invalid")
	errChoiceAlreadyResponded   = errors.New("choice already responded")
)

func (s *Service) SubmitChoiceResponse(ctx context.Context, cmd SubmitChoiceResponseCommand) (SubmitChoiceResponseResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return SubmitChoiceResponseResult{}, InvalidRequestError(err.Error(), err)
	}
	messageID, err := normalizeRequiredUUID(cmd.MessageID, "消息 ID")
	if err != nil {
		return SubmitChoiceResponseResult{}, InvalidRequestError(err.Error(), err)
	}
	accountID, err := normalizeRequiredUUID(cmd.AccountID, "用户 ID")
	if err != nil {
		return SubmitChoiceResponseResult{}, InvalidRequestError(err.Error(), err)
	}
	if len(cmd.OptionIDs) == 0 {
		return SubmitChoiceResponseResult{}, InvalidRequestError("至少选择一个选项", nil)
	}

	result := SubmitChoiceResponseResult{ConversationID: conversationID, MessageID: messageID}
	var recipients []string
	var events []AppEvent
	lockHeld := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, err := loadUserConversationAccess(tx, conversationID, accountID, false)
		if err != nil {
			return err
		}
		if err := lockReactionAccessRows(tx, access); err != nil {
			return err
		}
		message, err := lockChoiceMessage(ctx, tx, conversationID, messageID)
		if err != nil {
			return err
		}
		if message.Seq < access.visibleFromSeq() {
			return errConversationAccessDenied
		}
		definition, err := parseChoiceDefinition(message.Body)
		if err != nil {
			return errChoiceMessageType
		}
		if message.RevokedAt != nil {
			return errChoiceMessageUnavailable
		}
		optionIDs, err := normalizeChoiceResponseOptions(definition, cmd.OptionIDs)
		if err != nil {
			return err
		}

		var existing store.MessageChoiceResponse
		existingQuery := tx.Where("message_id = ? AND user_id = ?", messageID, accountID).Limit(1).Find(&existing)
		if existingQuery.Error != nil {
			return existingQuery.Error
		}
		if existingQuery.RowsAffected > 0 {
			existingOptionIDs, err := decodeChoiceResponseOptionIDs(existing.OptionIDs)
			if err != nil {
				return err
			}
			if !equalStringSlices(existingOptionIDs, optionIDs) {
				return errChoiceAlreadyResponded
			}
			result.Response = newChoiceResponse(existing, existingOptionIDs)
			return attachChoiceResultState(tx, message, accountID, &result)
		}

		if err := validateUserDirectAppAccess(tx, access); err != nil {
			return err
		}
		if err := validateUserConversationSendable(tx, access); err != nil {
			return err
		}
		if access.Context.IsTopic() && access.Participant == nil {
			return errConversationAccessDenied
		}

		encodedOptions, err := json.Marshal(optionIDs)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		stored := store.MessageChoiceResponse{
			ID: uuid.NewString(), ConversationID: conversationID, MessageID: messageID,
			UserID: accountID, OptionIDs: encodedOptions, CreatedAt: now,
		}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stored)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.First(&existing, "message_id = ? AND user_id = ?", messageID, accountID).Error; err != nil {
				return err
			}
			existingOptionIDs, err := decodeChoiceResponseOptionIDs(existing.OptionIDs)
			if err != nil {
				return err
			}
			if !equalStringSlices(existingOptionIDs, optionIDs) {
				return errChoiceAlreadyResponded
			}
			result.Response = newChoiceResponse(existing, existingOptionIDs)
			return attachChoiceResultState(tx, message, accountID, &result)
		}

		result.Created = true
		result.Response = newChoiceResponse(stored, optionIDs)
		if err := attachChoiceResultState(tx, message, accountID, &result); err != nil {
			return err
		}
		recipients, err = loadConversationDeliveryUserIDs(tx, access.Context)
		if err != nil {
			return err
		}
		if message.SenderType == store.MessageSenderTypeApp && message.SenderID != nil {
			var sender store.User
			if err := tx.First(&sender, "id = ?", accountID).Error; err != nil {
				return err
			}
			if s.appEventLocker != nil {
				s.appEventLocker.Lock()
				lockHeld = true
			}
			events, err = createChoiceResponseAppEventOutbox(tx, access.Context, message, stored, optionIDs, sender)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if lockHeld {
		defer s.appEventLocker.Unlock()
	}
	if err != nil {
		return SubmitChoiceResponseResult{}, mapChoiceResponseError(err)
	}
	if result.Created && s.notifications != nil {
		s.notifications.PublishMessageChoiceUpdated(ctx, recipients, ChoiceUpdatedEvent{
			ActorOptionIDs: result.Response.OptionIDs, ActorUserID: result.Response.UserID,
			Choice: result.Choice, ConversationID: conversationID, MessageID: messageID,
		})
	}
	if result.Created && s.appEvents != nil {
		s.appEvents.DeliverAppEvents(ctx, events)
	}
	return result, nil
}

func (s *Service) ListChoiceSnapshots(ctx context.Context, cmd ListChoiceSnapshotsCommand) (ListChoiceSnapshotsResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return ListChoiceSnapshotsResult{}, InvalidRequestError(err.Error(), err)
	}
	accountID, err := normalizeRequiredUUID(cmd.AccountID, "用户 ID")
	if err != nil {
		return ListChoiceSnapshotsResult{}, InvalidRequestError(err.Error(), err)
	}
	if len(cmd.MessageIDs) == 0 || len(cmd.MessageIDs) > maxChoiceSnapshotIDs {
		return ListChoiceSnapshotsResult{}, InvalidRequestError("一次必须查询 1 到 100 条选择消息", nil)
	}
	messageIDs := make([]string, 0, len(cmd.MessageIDs))
	seen := make(map[string]struct{}, len(cmd.MessageIDs))
	for _, rawID := range cmd.MessageIDs {
		messageID, normalizeErr := normalizeRequiredUUID(rawID, "消息 ID")
		if normalizeErr != nil {
			return ListChoiceSnapshotsResult{}, InvalidRequestError(normalizeErr.Error(), normalizeErr)
		}
		if _, ok := seen[messageID]; ok {
			continue
		}
		seen[messageID] = struct{}{}
		messageIDs = append(messageIDs, messageID)
	}

	db := s.db.WithContext(ctx)
	access, err := loadUserConversationAccess(db, conversationID, accountID, false)
	if err != nil {
		return ListChoiceSnapshotsResult{}, mapChoiceResponseError(err)
	}
	messages := make([]Message, 0, len(messageIDs))
	activeIndexes := make([]int, 0, len(messageIDs))
	result := ListChoiceSnapshotsResult{ConversationID: conversationID, Snapshots: make([]ChoiceSnapshot, len(messageIDs))}
	for index, messageID := range messageIDs {
		result.Snapshots[index] = ChoiceSnapshot{MessageID: messageID, Status: choiceSnapshotStatusDeleted}
		stored, found, loadErr := loadChoiceMessageForSnapshot(ctx, db, conversationID, messageID)
		if loadErr != nil {
			return ListChoiceSnapshotsResult{}, mapChoiceResponseError(loadErr)
		}
		if !found || stored.DeletedAt != nil || stored.Seq < access.visibleFromSeq() {
			continue
		}
		if stored.RevokedAt != nil {
			result.Snapshots[index].Status = choiceSnapshotStatusRevoked
			continue
		}
		if _, parseErr := parseChoiceDefinition(stored.Body); parseErr != nil {
			return ListChoiceSnapshotsResult{}, mapChoiceResponseError(errChoiceMessageType)
		}
		result.Snapshots[index].Status = choiceSnapshotStatusActive
		activeIndexes = append(activeIndexes, index)
		messages = append(messages, newMessage(stored))
	}
	if err := attachMessageChoices(db, messages, accountID); err != nil {
		return ListChoiceSnapshotsResult{}, internalError(err)
	}
	for index, message := range messages {
		if message.Choice == nil {
			return ListChoiceSnapshotsResult{}, internalError(errChoiceMessageType)
		}
		result.Snapshots[activeIndexes[index]].Choice = message.Choice
	}
	return result, nil
}

func lockChoiceMessage(ctx context.Context, db *gorm.DB, conversationID, messageID string) (store.Message, error) {
	return loadChoiceMessage(ctx, db, conversationID, messageID, clause.Locking{Strength: "UPDATE"})
}

func loadChoiceMessageForSnapshot(ctx context.Context, db *gorm.DB, conversationID, messageID string) (store.Message, bool, error) {
	if store.MessagePartitioningEnabled(db) {
		var registry store.MessageRegistry
		query := applyOnlineStoredMessageWindow(db).Where("id = ? AND conversation_id = ?", messageID, conversationID).Limit(1).Find(&registry)
		if query.Error != nil {
			return store.Message{}, false, query.Error
		}
		if query.RowsAffected == 0 {
			return store.Message{}, false, nil
		}
		scope, err := store.ScopeMessagePartition(ctx, db, int(registry.PartitionYear))
		if err != nil {
			return store.Message{}, false, err
		}
		var message store.Message
		result := scope.Where("id = ? AND conversation_id = ?", messageID, conversationID).Limit(1).Find(&message)
		if result.Error != nil {
			return store.Message{}, false, result.Error
		}
		return message, result.RowsAffected > 0, nil
	}
	var message store.Message
	result := applyOnlineStoredMessageWindow(db).Where("id = ? AND conversation_id = ?", messageID, conversationID).Limit(1).Find(&message)
	if result.Error != nil {
		return store.Message{}, false, result.Error
	}
	return message, result.RowsAffected > 0, nil
}

func loadChoiceMessage(ctx context.Context, db *gorm.DB, conversationID, messageID string, locking clause.Locking) (store.Message, error) {
	if store.MessagePartitioningEnabled(db) {
		var registry store.MessageRegistry
		query := applyOnlineStoredMessageWindow(db)
		if locking.Strength != "" {
			query = query.Clauses(locking)
		}
		if err := query.First(&registry, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
			return store.Message{}, err
		}
		scope, err := store.ScopeMessagePartition(ctx, db, int(registry.PartitionYear))
		if err != nil {
			return store.Message{}, err
		}
		if locking.Strength != "" {
			scope = scope.Clauses(locking)
		}
		var message store.Message
		if err := scope.Take(&message, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
			return store.Message{}, err
		}
		return message, nil
	}
	query := applyOnlineStoredMessageWindow(db)
	if locking.Strength != "" {
		query = query.Clauses(locking)
	}
	var message store.Message
	if err := query.First(&message, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
		return store.Message{}, err
	}
	return message, nil
}

func normalizeChoiceResponseOptions(definition choiceDefinition, rawOptionIDs []string) ([]string, error) {
	if len(rawOptionIDs) == 0 || len(rawOptionIDs) > len(definition.Options) {
		return nil, errChoiceOptionInvalid
	}
	requested := make(map[string]struct{}, len(rawOptionIDs))
	for _, rawOptionID := range rawOptionIDs {
		optionID := strings.TrimSpace(rawOptionID)
		if optionID == "" {
			return nil, errChoiceOptionInvalid
		}
		if _, ok := requested[optionID]; ok {
			return nil, errChoiceOptionInvalid
		}
		requested[optionID] = struct{}{}
	}
	if definition.Selection == "single" && len(requested) != 1 {
		return nil, errChoiceOptionInvalid
	}
	result := make([]string, 0, len(requested))
	for _, option := range definition.Options {
		if _, ok := requested[option.ID]; ok {
			result = append(result, option.ID)
			delete(requested, option.ID)
		}
	}
	if len(requested) > 0 {
		return nil, errChoiceOptionInvalid
	}
	return result, nil
}

func attachChoiceResultState(db *gorm.DB, message store.Message, userID string, result *SubmitChoiceResponseResult) error {
	values := []Message{newMessage(message)}
	if err := attachMessageChoices(db, values, userID); err != nil {
		return err
	}
	if values[0].Choice == nil {
		return errChoiceMessageType
	}
	result.Choice = *values[0].Choice
	return nil
}

func decodeChoiceResponseOptionIDs(raw json.RawMessage) ([]string, error) {
	var optionIDs []string
	if err := json.Unmarshal(raw, &optionIDs); err != nil {
		return nil, err
	}
	return optionIDs, nil
}

func newChoiceResponse(value store.MessageChoiceResponse, optionIDs []string) ChoiceResponse {
	return ChoiceResponse{
		CreatedAt: value.CreatedAt, ID: value.ID, OptionIDs: append([]string(nil), optionIDs...), UserID: value.UserID,
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func mapChoiceResponseError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFoundError("选择消息不存在", err)
	case errors.Is(err, errConversationAccessDenied):
		return forbidden("无权回复该选择", err)
	case errors.Is(err, errConversationNotSendable):
		return forbidden("当前会话不能回复选择", err)
	case errors.Is(err, errAppDirectAccessDenied):
		return forbidden("你当前无权直接使用此应用", err)
	case errors.Is(err, errChoiceMessageType):
		return conflict("该消息不是选择消息", err)
	case errors.Is(err, errChoiceMessageUnavailable):
		return conflict("该选择已被撤回，无法提交", err)
	case errors.Is(err, errChoiceAlreadyResponded):
		return conflict("你已经提交过答案", err)
	case errors.Is(err, errChoiceOptionInvalid):
		return InvalidRequestError("提交的选择项不存在或不符合选择模式", err)
	default:
		return internalError(err)
	}
}

func createChoiceResponseAppEventOutbox(
	db *gorm.DB,
	access conversationaccess.Context,
	message store.Message,
	response store.MessageChoiceResponse,
	optionIDs []string,
	sender store.User,
) ([]AppEvent, error) {
	if message.SenderType != store.MessageSenderTypeApp || message.SenderID == nil {
		return nil, nil
	}
	appIDs, err := lockAndFilterActiveConversationApps(db, access.MembershipConversationID, []string{*message.SenderID})
	if err != nil || len(appIDs) == 0 {
		return nil, err
	}
	conversationPayload := appMessageConversationPayload{ID: access.Conversation.ID, Name: access.Conversation.Name, Type: access.Conversation.Kind}
	if access.IsTopic() && access.ParentConversation != nil && access.Topic != nil {
		conversationPayload.Parent = &appMessageConversationReference{
			ID: access.ParentConversation.ID, Name: access.ParentConversation.Name, Type: access.ParentConversation.Kind,
		}
		conversationPayload.Source = &appMessageTopicSourcePayload{ID: access.Topic.SourceMessageID, Seq: access.Topic.SourceMessageSeq}
	}
	payload := struct {
		ChoiceMessage appMessagePayload             `json:"choice_message"`
		Conversation  appMessageConversationPayload `json:"conversation"`
		Response      struct {
			CreatedAt time.Time `json:"created_at"`
			ID        string    `json:"id"`
			OptionIDs []string  `json:"option_ids"`
		} `json:"response"`
		Sender appMessageSenderPayload `json:"sender"`
	}{
		ChoiceMessage: appMessagePayload{Body: message.Body, CreatedAt: message.CreatedAt, ID: message.ID, Seq: message.Seq, Summary: message.Summary},
		Conversation:  conversationPayload,
		Sender: appMessageSenderPayload{
			Email: sender.Email, ID: sender.ID, Name: sender.Name, Nickname: sender.Nickname, Type: store.MessageSenderTypeUser,
		},
	}
	payload.Response.CreatedAt = response.CreatedAt
	payload.Response.ID = response.ID
	payload.Response.OptionIDs = append([]string(nil), optionIDs...)
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	stored := store.AppEventOutbox{AppID: appIDs[0], Event: "choice.response_created", Payload: raw, CreatedAt: time.Now().UTC()}
	if err := db.Create(&stored).Error; err != nil {
		return nil, err
	}
	return []AppEvent{{AppID: stored.AppID, Cursor: stored.ID, Event: stored.Event, Payload: stored.Payload}}, nil
}
